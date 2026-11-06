package qris

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// MaxReconcilePerRun bounds the attempts one organization's reconcile asks AutoGoPay about; the
// rest wait for the next run, oldest first.
const MaxReconcilePerRun = 100

// ReconcileResult counts what one organization's reconcile did.
type ReconcileResult struct {
	Checked int // attempts AutoGoPay answered about
	Paid    int // of those, settled as paid
	Closed  int // of those, closed as expired or cancelled
	Busy    int // skipped because a request held the tenant's lock; the next run takes them
	Failed  int // AutoGoPay could not answer; the next run tries again
}

// ReconcileOrg asks AutoGoPay about org's pending attempts that are past their expiry or that the
// tenant said they paid, and records each answer as the check does: paid is settled through the
// same code as the webhook's task, expired or cancelled closes the attempt, still pending waits.
// Each attempt runs in its own transaction under its tenant's lock, with the organization's own
// key. AutoGoPay failures are counted and logged, not returned; a database failure stops the run.
func (s *Service) ReconcileOrg(ctx context.Context, org uuid.UUID) (ReconcileResult, error) {
	var res ReconcileResult
	var due []dbgen.ListQrisAttemptsToReconcileRow
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		due, err = dbgen.New(tx).ListQrisAttemptsToReconcile(ctx, dbgen.ListQrisAttemptsToReconcileParams{
			OrganizationID: org, Now: s.clock.Now(), MaxRows: MaxReconcilePerRun,
		})
		return err
	})
	if err != nil {
		return res, fmt.Errorf("qris: listing attempts to reconcile: %w", err)
	}
	for _, d := range due {
		if err := s.reconcileOne(ctx, org, d.TenantID, d.ID, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (s *Service) reconcileOne(ctx context.Context, org, tenantID, attemptID uuid.UUID, res *ReconcileResult) error {
	return s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		ok, err := q.TryLockTenantQris(ctx, tenantID)
		if err != nil {
			return fmt.Errorf("qris: locking the tenant's attempts: %w", err)
		}
		if !ok {
			res.Busy++
			return nil
		}
		row, err := q.GetQrisAttempt(ctx, dbgen.GetQrisAttemptParams{OrganizationID: org, ID: attemptID})
		if err != nil {
			return fmt.Errorf("qris: reading attempt %s: %w", attemptID, err)
		}
		if row.Status != AttemptPending {
			return nil // settled or closed since it was listed
		}
		a := &Attempt{
			ID: row.ID, TenantID: row.TenantID, TransactionID: row.TransactionID, Amount: row.Amount,
			QRString: row.QrString, Status: row.Status, ExpiresAt: row.ExpiresAt, CheckRequestedAt: row.CheckRequestedAt,
		}
		key, err := s.apiKey(ctx, tx, org)
		if err != nil {
			res.Failed++
			return nil
		}
		o, err := s.ask(ctx, key, org, a, false, s.clock.Now())
		if err != nil {
			res.Failed++
			return nil
		}
		res.Checked++
		switch o {
		case outcomePaid:
			res.Paid++
		case outcomeExpired, outcomeCancelled:
			res.Closed++
		}
		return s.record(ctx, tx, org, a, o)
	})
}
