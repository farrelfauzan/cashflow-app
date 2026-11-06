package qris

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qrimage"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

const (
	// MaxGeneratesPerDay caps the QRs one tenant can have AutoGoPay make in GenerateWindow,
	// failed generates included: AutoGoPay can block a merchant whose account makes QRs in a
	// loop. Showing a live QR again does not count.
	MaxGeneratesPerDay = 5
	// GenerateWindow is the window of MaxGeneratesPerDay.
	GenerateWindow = 24 * time.Hour

	// opTimeout bounds one make or check, AutoGoPay calls included (at most two, ten seconds
	// each). It stays under the API's request timeout.
	opTimeout = 22 * time.Second
	// cleanupTimeout bounds the one cancel of a QR that could not be recorded.
	cleanupTimeout = 10 * time.Second

	maxTransactionIDLen = 100
	maxOrderIDLen       = 100
)

// MakeQR gives the tenant behind token a QR for amount (whole rupiah, autogopay.MinAmount to
// autogopay.MaxAmount, at most what the tenant owes) and answers what the page shows next.
//
// A live QR for the same amount with more than ReuseMargin left is shown again: AutoGoPay treats
// repeated generates as spam. Otherwise the tenant's pending QR, if any, is cancelled at
// AutoGoPay (or read, when past its expiry) and closed, and a new one is generated. When that
// shows the previous QR was paid meanwhile, the payment is recorded and no new QR is made: the
// answer is StatusPaid and the page reloads what is left to pay.
//
// Generates per tenant are capped (ErrGenerateLimit, counted before the live QR is cancelled, so
// it stays payable) and never retried. Any AutoGoPay failure is ErrUnavailable: the page offers
// bank transfer. A call that finds MaxConcurrentCalls already waiting on AutoGoPay is
// ErrOverloaded: nothing reached AutoGoPay, and the page may try again in a moment. A QR generated but not recorded (the answer was
// unusable, or saving it failed) is cancelled once.
//
// Once a request reaches AutoGoPay, what AutoGoPay answered is recorded even if the tenant's
// request is cancelled, so the work runs on its own deadline; the page finds the result through
// GET /status.
func (s *Service) MakeQR(ctx context.Context, token string, amount int64) (PublicView, error) {
	if amount < autogopay.MinAmount {
		return PublicView{}, ErrInvalidAmount
	}
	if amount > autogopay.MaxAmount {
		return PublicView{}, ErrAmountAboveLimit
	}
	org, linkID, err := s.links.Resolve(ctx, token)
	if err != nil {
		return PublicView{}, err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opTimeout)
	defer cancel()

	var (
		out     PublicView
		key     secret.Value
		made    string // the transaction id of a QR AutoGoPay made, until its attempt is saved
		answer  error  // the error to answer once what was learnt is committed
		entered bool   // this call holds a place among those waiting on AutoGoPay
	)
	defer func() {
		if entered {
			s.leaveGateway()
		}
	}()
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		out, made, answer = PublicView{}, "", nil
		tenantID, _, err := s.links.OpenLink(ctx, tx, org, linkID)
		if err != nil {
			return err
		}
		q := dbgen.New(tx)
		if err := lockTenant(ctx, q, tenantID); err != nil {
			return err
		}
		if err := s.checkAmount(ctx, tx, org, tenantID, amount); err != nil {
			return err
		}
		now := s.clock.Now()
		a, err := latestAttempt(ctx, q, org, tenantID)
		if err != nil {
			return err
		}
		step := planMake(a, amount, now)
		if step == stepReuse {
			out = view(a, now)
			return nil
		}
		// From here this transaction waits on AutoGoPay while it holds a connection.
		if !s.enterGateway() {
			s.log.WarnContext(ctx, "too many QRIS requests waiting on AutoGoPay; answering overloaded", "organization", org)
			return ErrOverloaded
		}
		entered = true
		if key, err = s.apiKey(ctx, tx, org); err != nil {
			return err
		}
		// The cap is counted before the live QR is touched, so a tenant at the cap keeps a QR they
		// can still pay.
		ok, err := s.limiter.Allow(ctx, generateLimitKey(org, tenantID), MaxGeneratesPerDay, GenerateWindow)
		if err != nil {
			return fmt.Errorf("qris: counting generates: %w", err)
		}
		if !ok {
			return ErrGenerateLimit
		}
		if step == stepReplace {
			o, err := s.ask(ctx, key, org, a, true, now)
			if err != nil {
				return err
			}
			if err := s.record(ctx, tx, org, a, o); err != nil {
				return err
			}
			if o == outcomePaid {
				out = view(a, now)
				return nil
			}
		}
		qr, err := s.gateway.Generate(ctx, key, amount)
		made = qr.TransactionID // ErrBadResponse may still name the QR it made
		if err == nil {
			err = validQR(qr, amount, now)
		}
		if err != nil {
			answer = s.unavailable(ctx, org, uuid.Nil, "generate", err)
			return nil // made is cancelled below; the previous QR's close still commits
		}
		created := &Attempt{
			ID: uuid.New(), TenantID: tenantID, TransactionID: qr.TransactionID, Amount: amount,
			QRString: qr.QRString, Status: AttemptPending, ExpiresAt: qr.ExpiresAt,
		}
		if err := q.InsertQrisAttempt(ctx, dbgen.InsertQrisAttemptParams{
			ID: created.ID, OrganizationID: org, TenantID: tenantID, PaymentLinkID: linkID, Amount: amount,
			TransactionID: qr.TransactionID, OrderID: qr.OrderID, QrString: qr.QRString, Status: AttemptPending,
			ExpiresAt: qr.ExpiresAt, CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("qris: saving the new attempt: %w", err)
		}
		out = view(created, now)
		return nil
	})
	if made != "" && (err != nil || answer != nil) {
		s.cancelUnsaved(ctx, key, org, made)
	}
	if err != nil {
		return PublicView{}, err
	}
	if answer != nil {
		return PublicView{}, answer
	}
	return out, nil
}

// checkAmount refuses an amount above what the tenant owes now: the page pays the account, and
// paying more than it owes from a QR is a mistake, not credit.
func (s *Service) checkAmount(ctx context.Context, tx pgx.Tx, org, tenantID uuid.UUID, amount int64) error {
	acct, err := billing.LoadAccountTx(ctx, tx, org, tenantID, tenant.Today(s.clock))
	if err != nil {
		return fmt.Errorf("qris: loading what the tenant owes: %w", err)
	}
	if acct.Outstanding <= 0 {
		return ErrNothingToPay
	}
	if amount > acct.Outstanding {
		return ErrAmountAboveOutstanding
	}
	return nil
}

// validQR checks that a generated QR is what was asked for and fits what qris_attempt stores.
func validQR(qr autogopay.QRIS, amount int64, now time.Time) error {
	switch {
	case qr.Amount != amount:
		return fmt.Errorf("%w: generated Rp%d for Rp%d", autogopay.ErrBadResponse, qr.Amount, amount)
	case qr.TransactionID == "" || utf8.RuneCountInString(qr.TransactionID) > maxTransactionIDLen:
		return fmt.Errorf("%w: transaction id of %d characters", autogopay.ErrBadResponse, utf8.RuneCountInString(qr.TransactionID))
	case utf8.RuneCountInString(qr.OrderID) > maxOrderIDLen:
		return fmt.Errorf("%w: order id of %d characters", autogopay.ErrBadResponse, utf8.RuneCountInString(qr.OrderID))
	case qr.QRString == "" || len(qr.QRString) > qrimage.MaxQRStringBytes:
		return fmt.Errorf("%w: QR string of %d bytes", autogopay.ErrBadResponse, len(qr.QRString))
	case !qr.ExpiresAt.After(now):
		return fmt.Errorf("%w: QR expires at %s, not after now", autogopay.ErrBadResponse, qr.ExpiresAt)
	}
	return nil
}

// cancelUnsaved cancels, once, a QR AutoGoPay made that has no attempt, so it cannot be paid
// with nothing to record it against. A failure is logged: the QR then expires on its own.
func (s *Service) cancelUnsaved(ctx context.Context, key secret.Value, org uuid.UUID, transactionID string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if _, err := s.gateway.Cancel(ctx, key, transactionID); err != nil && !errors.Is(err, autogopay.ErrTransactionNotFound) {
		s.log.ErrorContext(ctx, "cancelling a QR that could not be recorded; it expires on its own",
			"organization", org, "transaction", transactionID, "error", err)
	}
}

func generateLimitKey(org, tenantID uuid.UUID) string {
	return "qris:generate:" + org.String() + ":" + tenantID.String()
}
