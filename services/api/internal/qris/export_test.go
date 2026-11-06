package qris

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// SettleForTest settles one attempt at paidAmount, as the webhook's task will (S3-6), so tests
// can reach the amount a status answer never carries.
func (s *Service) SettleForTest(ctx context.Context, org, attemptID uuid.UUID, paidAmount int64) error {
	return s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetQrisAttempt(ctx, dbgen.GetQrisAttemptParams{OrganizationID: org, ID: attemptID})
		if err != nil {
			return err
		}
		a := &Attempt{ID: row.ID, TenantID: row.TenantID, TransactionID: row.TransactionID, Amount: row.Amount, Status: row.Status}
		_, _, err = s.settle(ctx, tx, org, a, paidAmount)
		return err
	})
}

// PlanMake exposes planMake's answer as a name, for the table test.
func PlanMake(a *Attempt, amount int64, now time.Time) string {
	return [...]string{"generate", "reuse", "replace"}[planMake(a, amount, now)]
}
