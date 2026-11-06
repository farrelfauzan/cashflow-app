package qris

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

// settle records the payment of attempt a, once, and marks the attempt paid. It runs inside tx,
// scoped to org. The payment is keyed by the attempt's transaction id (unique per organization
// and source), so a second settle of the same attempt, from the check, reconcile or the webhook,
// records nothing new; it is allocated FIFO over the tenant's invoices only when it was inserted,
// and then closes the tenant's active promise as kept when it keeps it (promise.CloseIfKept).
// The attempt's paid_amount is always the recorded payment's amount, so a payment recorded at
// another amount than the attempt's flags the attempt (amount_mismatch).
//
// paidAmount is what AutoGoPay says was paid. Its status answer carries no amount, so the check
// and reconcile pass the attempt's own amount; the webhook (S3-6) passes the amount it reports.
//
// It returns the amount of the recorded payment and whether this call inserted it.
func (s *Service) settle(ctx context.Context, tx pgx.Tx, org uuid.UUID, a *Attempt, paidAmount int64) (int64, bool, error) {
	q := dbgen.New(tx)
	ref := a.TransactionID
	now := s.clock.Now()
	// The tenant's allocation lock before the insert, as billing.RecordPayment takes it: a promise
	// version made meanwhile marks the tenant's last payment seq under it.
	if err := billing.LockTenant(ctx, q, org, a.TenantID); err != nil {
		return 0, false, fmt.Errorf("qris: locking the tenant of attempt %s: %w", a.ID, err)
	}
	pay, err := q.InsertPaymentIfNew(ctx, dbgen.InsertPaymentIfNewParams{
		ID: uuid.New(), OrganizationID: org, TenantID: a.TenantID, Amount: paidAmount,
		Method: billing.MethodQRIS, ReceivedAt: now, Source: billing.SourceQRIS, SourceRef: &ref,
	})
	inserted := err == nil
	switch {
	case inserted:
		if _, err := billing.Allocate(ctx, tx, org, a.TenantID); err != nil {
			return 0, false, fmt.Errorf("qris: allocating the payment of attempt %s: %w", a.ID, err)
		}
		if err := s.audit.RecordSystemAction(ctx, tx, org, authz.ActionPaymentRecord,
			authz.Resource{Type: "payment", ID: pay.ID.String()}); err != nil {
			return 0, false, fmt.Errorf("qris: auditing the payment of attempt %s: %w", a.ID, err)
		}
		// A payment that keeps the tenant's active promise closes it as kept at once.
		if _, err := promise.CloseIfKept(ctx, tx, org, a.TenantID, now); err != nil {
			return 0, false, fmt.Errorf("qris: closing the promise kept by attempt %s: %w", a.ID, err)
		}
	case errors.Is(err, pgx.ErrNoRows):
		// An earlier settle recorded it.
		pay, err = q.GetPaymentBySourceRef(ctx, dbgen.GetPaymentBySourceRefParams{
			OrganizationID: org, Source: billing.SourceQRIS, SourceRef: &ref,
		})
		if err != nil {
			return 0, false, fmt.Errorf("qris: loading the recorded payment of attempt %s: %w", a.ID, err)
		}
	default:
		return 0, false, fmt.Errorf("qris: recording the payment of attempt %s: %w", a.ID, err)
	}
	paid := pay.Amount
	if _, err := q.MarkQrisAttemptPaid(ctx, dbgen.MarkQrisAttemptPaidParams{
		OrganizationID: org, ID: a.ID, PaidAmount: &paid, Now: &now,
	}); err != nil {
		return 0, false, fmt.Errorf("qris: marking attempt %s paid: %w", a.ID, err)
	}
	a.Status = AttemptPaid
	return paid, inserted, nil
}

// record applies what AutoGoPay said about attempt a: a payment is settled, an expiry or a cancel
// closes it, an open QR changes nothing.
func (s *Service) record(ctx context.Context, tx pgx.Tx, org uuid.UUID, a *Attempt, o outcome) error {
	switch o {
	case outcomePaid:
		_, _, err := s.settle(ctx, tx, org, a, a.Amount)
		return err
	case outcomeExpired:
		return s.close(ctx, tx, org, a, AttemptExpired)
	case outcomeCancelled:
		return s.close(ctx, tx, org, a, AttemptCancelled)
	default:
		return nil
	}
}

func (s *Service) close(ctx context.Context, tx pgx.Tx, org uuid.UUID, a *Attempt, status string) error {
	now := s.clock.Now()
	if _, err := dbgen.New(tx).CloseQrisAttempt(ctx, dbgen.CloseQrisAttemptParams{
		OrganizationID: org, ID: a.ID, Status: status, ClosedAt: &now,
	}); err != nil {
		return fmt.Errorf("qris: closing attempt %s as %s: %w", a.ID, status, err)
	}
	a.Status = status
	return nil
}

// ask asks AutoGoPay where attempt a stands. With cancelOpen it also cancels the QR when it is
// still open, so the answer is final: a live QR is cancelled at once (AutoGoPay answers paid or
// expired instead when it already is), and one past its expiry is read first and cancelled only
// if AutoGoPay still holds it open. AutoGoPay not knowing a QR past its expiry counts as expired;
// any other failure is ErrUnavailable and changes nothing.
func (s *Service) ask(ctx context.Context, key secret.Value, org uuid.UUID, a *Attempt, cancelOpen bool, now time.Time) (outcome, error) {
	live := now.Before(a.ExpiresAt)
	op := "status"
	var tx autogopay.Transaction
	var err error
	if cancelOpen && live {
		op = "cancel"
		tx, err = s.gateway.Cancel(ctx, key, a.TransactionID)
	} else {
		tx, err = s.gateway.Status(ctx, key, a.TransactionID)
		if err == nil && cancelOpen && tx.Status == autogopay.StatusPending {
			op = "cancel"
			tx, err = s.gateway.Cancel(ctx, key, a.TransactionID)
		}
	}
	if errors.Is(err, autogopay.ErrTransactionNotFound) && !live {
		return outcomeExpired, nil
	}
	if err != nil {
		return outcomeOpen, s.unavailable(ctx, org, a.ID, op, err)
	}
	o, ok := outcomeOf(tx.Status)
	if !ok || (cancelOpen && o == outcomeOpen) {
		return outcomeOpen, s.unavailable(ctx, org, a.ID, op,
			fmt.Errorf("%w: status %q after %s", autogopay.ErrBadResponse, tx.Status, op))
	}
	return o, nil
}

// unavailable logs a failed AutoGoPay call and returns it as ErrUnavailable. AutoGoPay's errors
// never carry the API key.
func (s *Service) unavailable(ctx context.Context, org, attemptID uuid.UUID, op string, err error) error {
	s.log.WarnContext(ctx, "AutoGoPay call failed", "organization", org, "attempt", attemptID, "op", op, "error", err)
	return fmt.Errorf("%w: %s: %w", ErrUnavailable, op, err)
}

// apiKey reads org's AutoGoPay key. An organization without one cannot take QRIS, which the page
// shows like an outage: pay by bank transfer.
func (s *Service) apiKey(ctx context.Context, tx pgx.Tx, org uuid.UUID) (secret.Value, error) {
	key, err := s.keys.APIKey(ctx, tx, org)
	if err != nil {
		return secret.Value{}, s.unavailable(ctx, org, uuid.Nil, "read key", err)
	}
	return key, nil
}

// lockTenant takes the tenant's QRIS lock for the transaction, or answers ErrBusy when another
// request or reconcile holds it.
func lockTenant(ctx context.Context, q *dbgen.Queries, tenantID uuid.UUID) error {
	ok, err := q.TryLockTenantQris(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("qris: locking the tenant's attempts: %w", err)
	}
	if !ok {
		return ErrBusy
	}
	return nil
}

// view is what the page shows about attempt a (nil when there is none) at now.
func view(a *Attempt, now time.Time) PublicView {
	if a == nil {
		return PublicView{Status: StatusNone}
	}
	v := PublicView{Status: PublicStatus(a, now), Amount: a.Amount, ExpiresAt: a.ExpiresAt}
	if v.Status == StatusChecking {
		v.CheckedAt = a.CheckRequestedAt
	}
	return v
}
