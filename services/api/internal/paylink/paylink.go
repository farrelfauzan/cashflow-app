// Package paylink makes tenant payment links and reads the public payment page behind them
// (phase B, slice 3, decision 39).
//
// A link is /t/<token>, where the token is 32 random bytes from crypto/rand in base64url without
// padding (43 characters). Only the SHA-256 of those bytes is stored, like a staff session, so a
// link cannot be shown twice: a fresh one is minted every time one is sent, and an invoice has
// many. Each is valid until 30 days after its invoice is paid; revoking revokes every link of the
// invoice. A link names one invoice, but the page shows everything the tenant owes, since paying
// through it pays oldest first.
//
// The token is a secret. Nothing in this package logs it, puts it in an error, or stores it.
package paylink

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

var (
	// ErrLinkNotFound means the token is malformed, unknown or revoked. The three are answered
	// alike, so a caller learns nothing about which it was.
	ErrLinkNotFound = errors.New("paylink: link not found")
	// ErrLinkExpired means the link's invoice was paid (or cancelled) LinkLifetimeAfterPaid ago
	// or longer.
	ErrLinkExpired = errors.New("paylink: link expired")
	// ErrInvoiceNotFound means the organization has no such invoice.
	ErrInvoiceNotFound = errors.New("paylink: invoice not found")
	// ErrInvoicePastWindow means a link was asked for an invoice paid LinkLifetimeAfterPaid ago or
	// longer: it would be expired the moment it was made.
	ErrInvoicePastWindow = errors.New("paylink: invoice paid too long ago for a link")
)

// LinkLifetimeAfterPaid is how long a link keeps working after its invoice is fully paid or
// cancelled. Asia/Jakarta has no daylight saving, so it is 30 calendar days to the minute.
const LinkLifetimeAfterPaid = 30 * 24 * time.Hour

// tokenBytes is the entropy of a link token; tokenLength is its base64url length.
const (
	tokenBytes  = 32
	tokenLength = 43
)

// tokenEncoding refuses a non-canonical last character, so each token has exactly one spelling.
var tokenEncoding = base64.RawURLEncoding.Strict()

// Service mints, revokes and resolves payment links and reads the payment page.
type Service struct {
	store     *store.Store
	authz     *authz.Service
	clock     clock.Clock
	webOrigin string
}

// NewService returns a Service. webOrigin is where tenants open the web app (PUBLIC_WEB_ORIGIN),
// the base of every link's URL; c decides "now" for expiry and "today" for the page's figures.
func NewService(st *store.Store, az *authz.Service, c clock.Clock, webOrigin string) *Service {
	return &Service{store: st, authz: az, clock: c, webOrigin: webOrigin}
}

// Link is a freshly minted payment link. URL holds the token: hand it to the tenant and nowhere
// else, and never log it.
type Link struct {
	ID        uuid.UUID
	InvoiceID uuid.UUID
	URL       string
}

// Mint makes a new link to an invoice for a member ("Salin link bayar"). It needs invoice:share
// (audited) on the invoice, in its lease's property and assigned to its collector, so a karyawan
// may share only the invoices of tenants they collect from. Any invoice of the organization may
// get a link, cancelled ones included, unless it was paid or cancelled LinkLifetimeAfterPaid ago
// or longer (ErrInvoicePastWindow). A missing invoice is ErrInvoiceNotFound.
func (s *Service) Mint(ctx context.Context, p authz.Principal, invoiceID uuid.UUID) (Link, error) {
	var out Link
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		if err := s.authorizeShare(ctx, tx, p, invoiceID); err != nil {
			return err
		}
		createdBy := p.UserID
		var err error
		out, err = s.mintInTx(ctx, tx, p.OrganizationID, invoiceID, &createdBy)
		return err
	})
	if err != nil {
		return Link{}, err
	}
	return out, nil
}

// mintInTx makes a new link to an invoice inside tx, scoped to org. It checks no permission: the
// caller has (Mint), or the caller is the platform itself (reminders, createdBy nil).
func (s *Service) mintInTx(ctx context.Context, tx pgx.Tx, org, invoiceID uuid.UUID, createdBy *uuid.UUID) (Link, error) {
	q := dbgen.New(tx)
	inv, err := loadInvoice(ctx, q, org, invoiceID)
	if err != nil {
		return Link{}, err
	}
	st, err := q.GetInvoiceSettlement(ctx, dbgen.GetInvoiceSettlementParams{OrganizationID: org, InvoiceID: invoiceID})
	if err != nil {
		return Link{}, fmt.Errorf("paylink: reading the invoice's balance: %w", err)
	}
	now := s.clock.Now()
	if Expired(now, st.Outstanding, st.SettledAt) {
		return Link{}, ErrInvoicePastWindow
	}
	token, hash, err := newToken()
	if err != nil {
		return Link{}, err
	}
	link := Link{ID: uuid.New(), InvoiceID: invoiceID, URL: s.webOrigin + "/t/" + token}
	var by uuid.NullUUID
	if createdBy != nil {
		by = uuid.NullUUID{UUID: *createdBy, Valid: true}
	}
	err = q.InsertPaymentLink(ctx, dbgen.InsertPaymentLinkParams{
		ID: link.ID, OrganizationID: org, InvoiceID: invoiceID, TenantID: inv.TenantID,
		TokenHash: hash, CreatedAt: now, CreatedByStaffUserID: by,
	})
	if err != nil {
		return Link{}, fmt.Errorf("paylink: storing the link: %w", err)
	}
	return link, nil
}

// RevokeAll revokes every link of an invoice and returns how many were still working. It needs
// invoice:share (audited) as Mint does. Revoking an invoice with no live link revokes nothing
// and is not an error. A missing invoice is ErrInvoiceNotFound.
func (s *Service) RevokeAll(ctx context.Context, p authz.Principal, invoiceID uuid.UUID) (int, error) {
	var n int64
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		if err := s.authorizeShare(ctx, tx, p, invoiceID); err != nil {
			return err
		}
		now := s.clock.Now()
		var err error
		n, err = dbgen.New(tx).RevokePaymentLinks(ctx, dbgen.RevokePaymentLinksParams{
			OrganizationID: p.OrganizationID, InvoiceID: invoiceID, RevokedAt: &now,
		})
		if err != nil {
			return fmt.Errorf("paylink: revoking links: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// authorizeShare checks invoice:share on the invoice in its lease's property, assigned to its
// lease's collector. Authorize audits the allowed check in tx and a denial in its own
// transaction.
func (s *Service) authorizeShare(ctx context.Context, tx pgx.Tx, p authz.Principal, invoiceID uuid.UUID) error {
	inv, err := loadInvoice(ctx, dbgen.New(tx), p.OrganizationID, invoiceID)
	if err != nil {
		return err
	}
	return s.authz.Authorize(ctx, tx, p, authz.Request{Action: authz.ActionInvoiceShare, Resource: authz.Resource{
		Type: "invoice", ID: inv.ID.String(), PropertyID: inv.PropertyID, OwnerID: inv.CollectorStaffUserID.UUID,
	}})
}

func loadInvoice(ctx context.Context, q *dbgen.Queries, org, invoiceID uuid.UUID) (dbgen.GetInvoiceScopeRow, error) {
	inv, err := q.GetInvoiceScope(ctx, dbgen.GetInvoiceScopeParams{OrganizationID: org, ID: invoiceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.GetInvoiceScopeRow{}, ErrInvoiceNotFound
	}
	if err != nil {
		return dbgen.GetInvoiceScopeRow{}, fmt.Errorf("paylink: loading the invoice: %w", err)
	}
	return inv, nil
}

// Resolve finds the organization and link behind a token, with no organization scope, through
// the narrow lookup function (store.LookupPaymentLink). A token that is not exactly 43 base64url
// characters encoding 32 bytes is ErrLinkNotFound before the database is asked, and so is an
// unknown one. A revoked link resolves: whether it still works is checked in the organization's
// scope (PublicPage).
func (s *Service) Resolve(ctx context.Context, token string) (org, linkID uuid.UUID, err error) {
	hash, ok := TokenHash(token)
	if !ok {
		return uuid.Nil, uuid.Nil, ErrLinkNotFound
	}
	org, linkID, err = s.store.LookupPaymentLink(ctx, hash)
	if errors.Is(err, store.ErrPaymentLinkNotFound) {
		return uuid.Nil, uuid.Nil, ErrLinkNotFound
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("paylink: resolving a token: %w", err)
	}
	return org, linkID, nil
}

// Expired reports whether a link to an invoice that still owes outstanding and was last settled
// at settledAt (its newest allocation or correction) no longer works at now. An invoice is open,
// and its links work, while it owes anything; once it owes nothing (paid, or cancelled by a
// correction) they work until LinkLifetimeAfterPaid after it was settled. An invoice reopened by a
// later correction is open again.
func Expired(now time.Time, outstanding int64, settledAt time.Time) bool {
	return outstanding <= 0 && !now.Before(settledAt.Add(LinkLifetimeAfterPaid))
}

// TokenHash checks that token has the shape of a link token and returns the SHA-256 of the 32
// bytes it encodes, which is what the database stores. ok is false for anything else.
func TokenHash(token string) (hash []byte, ok bool) {
	if len(token) != tokenLength {
		return nil, false
	}
	raw, err := tokenEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenBytes {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return sum[:], true
}

// LimitKey is the rate-limit key for requests carrying token: a hash of it, whatever its shape,
// so the token itself never reaches the limiter's store.
func LimitKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("paylink:token:%x", sum)
}

// newToken returns a link token and the hash to store.
func newToken() (token string, hash []byte, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("paylink: reading random bytes for a token: %w", err)
	}
	sum := sha256.Sum256(raw)
	return tokenEncoding.EncodeToString(raw), sum[:], nil
}
