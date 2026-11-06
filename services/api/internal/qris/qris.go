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
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qrimage"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

var (
	// ErrNoAttempt means the tenant has no QRIS attempt yet.
	ErrNoAttempt = errors.New("qris: no attempt")
	// ErrQRGone means the tenant's latest QR can no longer be paid: it expired, was paid or was
	// cancelled. The tenant makes a new one.
	ErrQRGone = errors.New("qris: QR no longer valid")
	// ErrInvalidAmount means the amount is below Rp1.
	ErrInvalidAmount = errors.New("qris: amount must be at least Rp1")
	// ErrAmountAboveLimit means the amount is above AutoGoPay's Rp10.000.000; the rest is paid by
	// another QR or bank transfer.
	ErrAmountAboveLimit = errors.New("qris: amount above the QRIS limit")
	// ErrAmountAboveOutstanding means the amount is more than the tenant owes.
	ErrAmountAboveOutstanding = errors.New("qris: amount above what the tenant owes")
	// ErrNothingToPay means the tenant owes nothing.
	ErrNothingToPay = errors.New("qris: nothing to pay")
	// ErrGenerateLimit means the tenant made MaxGeneratesPerDay QRs in GenerateWindow.
	ErrGenerateLimit = errors.New("qris: too many QRs made")
	// ErrBusy means another request is making or checking the tenant's QR right now.
	ErrBusy = errors.New("qris: busy")
	// ErrOverloaded means MaxConcurrentCalls requests are already waiting on AutoGoPay. Nothing
	// reached AutoGoPay, so the page may try again in a moment before it offers bank transfer.
	ErrOverloaded = errors.New("qris: too many requests waiting on AutoGoPay")
	// ErrUnavailable means AutoGoPay failed, or the organization has no AutoGoPay key: QRIS is
	// down for now ("QRIS gangguan") and the page offers bank transfer.
	ErrUnavailable = errors.New("qris: QRIS unavailable")
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

// Attempt is one QRIS attempt.
type Attempt struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	TransactionID    string // AutoGoPay's id for the QR
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

// Gateway is the AutoGoPay QRIS API: autogopay.Client, or autogopaytest.Fake in tests. Every call
// takes the organization's own key.
type Gateway interface {
	Generate(ctx context.Context, key secret.Value, amount int64) (autogopay.QRIS, error)
	Status(ctx context.Context, key secret.Value, transactionID string) (autogopay.Transaction, error)
	Cancel(ctx context.Context, key secret.Value, transactionID string) (autogopay.Transaction, error)
}

// KeyReader decrypts an organization's AutoGoPay key inside a transaction scoped to it.
// autogopay.Service implements it.
type KeyReader interface {
	APIKey(ctx context.Context, tx pgx.Tx, org uuid.UUID) (secret.Value, error)
}

// Limiter counts events under a key in a window. ratelimit.Redis implements it.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// Auditor writes the audit row of a change the system makes with no acting member.
// authz.Service implements it.
type Auditor interface {
	RecordSystemAction(ctx context.Context, tx pgx.Tx, org uuid.UUID, action string, res authz.Resource) error
}

// Deps is what a Service needs. The public reads use Store, Links and Clock; making and checking
// a QR also Gateway, Keys, Limiter and Audit; reconcile (the worker) Store, Clock, Gateway, Keys
// and Audit, with no Links or Limiter.
type Deps struct {
	Store   *store.Store
	Links   LinkOpener
	Clock   clock.Clock
	Gateway Gateway
	Keys    KeyReader
	Limiter Limiter
	Audit   Auditor
	Log     *slog.Logger
	// MaxConcurrentCalls bounds how many MakeQR and Check calls may wait on AutoGoPay at once.
	// Each holds a database connection while it waits, so the API sets it below its pool size:
	// a slow AutoGoPay then never takes every connection, and the payment page with its bank
	// transfer fallback keeps loading. 0 means no bound (tests, the worker).
	MaxConcurrentCalls int
}

// Service makes, reads, checks and settles a tenant's QRIS attempts.
type Service struct {
	store   *store.Store
	links   LinkOpener
	clock   clock.Clock
	gateway Gateway
	keys    KeyReader
	limiter Limiter
	audit   Auditor
	log     *slog.Logger
	// gate holds one token per call waiting on AutoGoPay; nil when unbounded.
	gate chan struct{}
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &Service{
		store: d.Store, links: d.Links, clock: d.Clock, gateway: d.Gateway, keys: d.Keys,
		limiter: d.Limiter, audit: d.Audit, log: log,
	}
	if d.MaxConcurrentCalls > 0 {
		s.gate = make(chan struct{}, d.MaxConcurrentCalls)
	}
	return s
}

// enterGateway takes a place among the calls waiting on AutoGoPay, without waiting for one: false
// means MaxConcurrentCalls are already waiting. A true answer must be paired with leaveGateway.
func (s *Service) enterGateway() bool {
	if s.gate == nil {
		return true
	}
	select {
	case s.gate <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Service) leaveGateway() {
	if s.gate != nil {
		<-s.gate
	}
}

// PublicView is what GET /api/t/{token}/status answers. Amount and ExpiresAt are those of the
// latest attempt and are zero when Status is StatusNone.
type PublicView struct {
	Status    Status
	Amount    int64
	ExpiresAt time.Time
	// CheckedAt is when the tenant said they paid; set only while Status is StatusChecking.
	CheckedAt *time.Time
}

// PublicStatus reads what the page shows about the latest attempt of the tenant behind token.
// Link errors are paylink's (ErrLinkNotFound, ErrLinkExpired).
func (s *Service) PublicStatus(ctx context.Context, token string) (PublicView, error) {
	var out PublicView
	err := s.withLink(ctx, token, func(_ *dbgen.Queries, _, _ uuid.UUID, a *Attempt) error {
		out = view(a, s.clock.Now())
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
		ID: row.ID, TenantID: row.TenantID, TransactionID: row.TransactionID, Amount: row.Amount,
		QRString: row.QrString, Status: row.Status, ExpiresAt: row.ExpiresAt, CheckRequestedAt: row.CheckRequestedAt,
	}, nil
}
