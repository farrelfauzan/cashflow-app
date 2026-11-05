// Package qris holds a tenant's QRIS attempts (phase B, slice 3): the QR codes AutoGoPay made for
// a tenant from the payment page, what the page shows about the latest one, and its image.
//
// Attempts belong to the tenant, not to one link or invoice: whichever of the tenant's working
// links the page is opened through, it shows the tenant's latest attempt, and a tenant has at
// most one pending attempt at a time. An attempt's status changes in place; the money is the
// payment row recorded when it is paid.
//
// Every entry point takes the link token, which is a secret: nothing here logs it, stores it or
// puts it in an error.
package qris

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qrimage"
)

var (
	// ErrNoAttempt means the tenant has no QRIS attempt yet.
	ErrNoAttempt = errors.New("qris: no attempt")
	// ErrQRGone means the tenant's latest QR can no longer be paid: it expired, was paid or was
	// cancelled. The tenant makes a new one.
	ErrQRGone = errors.New("qris: QR no longer valid")
)

// Attempt statuses as stored in qris_attempt.status.
const (
	AttemptPending   = "pending"
	AttemptPaid      = "paid"
	AttemptExpired   = "expired"
	AttemptCancelled = "cancelled"
)

// Status is what the payment page shows about the tenant's latest attempt.
type Status string

// The statuses the page shows.
const (
	// StatusNone: the tenant has no attempt yet.
	StatusNone Status = "none"
	// StatusPending: the QR can be paid now.
	StatusPending Status = "pending"
	// StatusChecking: the tenant said they paid ("Saya sudah bayar") and no answer has come yet
	// ("Sedang dicek").
	StatusChecking Status = "checking"
	// StatusPaid: the attempt was paid.
	StatusPaid Status = "paid"
	// StatusExpired: the QR can no longer be paid ("QR kedaluwarsa"); it expired or was
	// cancelled.
	StatusExpired Status = "expired"
)

// Attempt is one QRIS attempt as the page needs it.
type Attempt struct {
	ID               uuid.UUID
	Amount           int64
	QRString         string
	Status           string // AttemptPending, AttemptPaid, AttemptExpired or AttemptCancelled
	ExpiresAt        time.Time
	CheckRequestedAt *time.Time
}

// PublicStatus is what the page shows for the latest attempt a (nil when there is none) at now.
// A pending attempt reaching its expiry shows as expired, unless the tenant asked for a check: a
// payment in the last second still arrives later through the webhook or reconcile, and is
// recorded then. The QR stops at expires_at itself.
func PublicStatus(a *Attempt, now time.Time) Status {
	if a == nil {
		return StatusNone
	}
	switch a.Status {
	case AttemptPaid:
		return StatusPaid
	case AttemptPending:
		if a.CheckRequestedAt != nil {
			return StatusChecking
		}
		if now.Before(a.ExpiresAt) {
			return StatusPending
		}
		return StatusExpired
	default: // expired, cancelled
		return StatusExpired
	}
}

// LinkOpener resolves a payment link token and checks that the link still works. paylink.Service
// is the one implementation.
type LinkOpener interface {
	// Resolve finds the organization and link behind a token, with no organization scope.
	Resolve(ctx context.Context, token string) (org, linkID uuid.UUID, err error)
	// OpenLink checks inside tx that the link works and returns its tenant and invoice.
	OpenLink(ctx context.Context, tx pgx.Tx, org, linkID uuid.UUID) (tenantID, invoiceID uuid.UUID, err error)
}

// Service reads a tenant's QRIS attempts behind a payment link.
type Service struct {
	store *store.Store
	links LinkOpener
	clock clock.Clock
}

// NewService returns a Service. links resolves and opens payment links; c decides "now" for
// expiry.
func NewService(st *store.Store, links LinkOpener, c clock.Clock) *Service {
	return &Service{store: st, links: links, clock: c}
}

// PublicView is what GET /api/t/{token}/status answers. Amount and ExpiresAt are those of the
// latest attempt and are zero when Status is StatusNone.
type PublicView struct {
	Status    Status
	Amount    int64
	ExpiresAt time.Time
}

// PublicStatus reads what the page shows about the latest attempt of the tenant behind token.
// Link errors are paylink's (ErrLinkNotFound, ErrLinkExpired).
func (s *Service) PublicStatus(ctx context.Context, token string) (PublicView, error) {
	var out PublicView
	err := s.withLink(ctx, token, func(_ *dbgen.Queries, _, _ uuid.UUID, a *Attempt) error {
		out.Status = PublicStatus(a, s.clock.Now())
		if a != nil {
			out.Amount, out.ExpiresAt = a.Amount, a.ExpiresAt
		}
		return nil
	})
	if err != nil {
		return PublicView{}, err
	}
	return out, nil
}

// QRImage renders the PNG of the latest attempt of the tenant behind token, with the amount, the
// unit and property of the link's invoice, and the expiry printed under the code. No attempt is
// ErrNoAttempt; an attempt that is not pending, or pending at or past its expiry, is ErrQRGone.
func (s *Service) QRImage(ctx context.Context, token string) ([]byte, error) {
	var in qrimage.Input
	err := s.withLink(ctx, token, func(q *dbgen.Queries, org, invoiceID uuid.UUID, a *Attempt) error {
		if a == nil {
			return ErrNoAttempt
		}
		if a.Status != AttemptPending || !s.clock.Now().Before(a.ExpiresAt) {
			return ErrQRGone
		}
		place, err := q.GetInvoicePlace(ctx, dbgen.GetInvoicePlaceParams{OrganizationID: org, ID: invoiceID})
		if err != nil {
			return fmt.Errorf("qris: loading the invoice's unit: %w", err)
		}
		in = qrimage.Input{
			QRString: a.QRString, Amount: a.Amount, ExpiresAt: a.ExpiresAt,
			UnitLabel: place.UnitLabel, PropertyName: place.PropertyName,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	png, err := qrimage.Render(in)
	if err != nil {
		return nil, fmt.Errorf("qris: rendering the QR image: %w", err)
	}
	return png, nil
}

// withLink resolves token, then in one snapshot of the link's organization opens the link and
// reads its tenant's latest attempt (nil when there is none), and calls fn with them.
func (s *Service) withLink(ctx context.Context, token string, fn func(q *dbgen.Queries, org, invoiceID uuid.UUID, a *Attempt) error) error {
	org, linkID, err := s.links.Resolve(ctx, token)
	if err != nil {
		return err
	}
	return s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		tenantID, invoiceID, err := s.links.OpenLink(ctx, tx, org, linkID)
		if err != nil {
			return err
		}
		q := dbgen.New(tx)
		a, err := latestAttempt(ctx, q, org, tenantID)
		if err != nil {
			return err
		}
		return fn(q, org, invoiceID, a)
	})
}

// latestAttempt reads a tenant's newest attempt, or nil when the tenant has none.
func latestAttempt(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID) (*Attempt, error) {
	row, err := q.GetLatestQrisAttempt(ctx, dbgen.GetLatestQrisAttemptParams{OrganizationID: org, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("qris: loading the latest attempt: %w", err)
	}
	return &Attempt{
		ID: row.ID, Amount: row.Amount, QRString: row.QrString, Status: row.Status,
		ExpiresAt: row.ExpiresAt, CheckRequestedAt: row.CheckRequestedAt,
	}, nil
}
