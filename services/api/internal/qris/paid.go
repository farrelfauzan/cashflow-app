package qris

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// ErrUnknownTransaction means the organization has no attempt with the transaction id AutoGoPay
// reported paid.
var ErrUnknownTransaction = errors.New("qris: no attempt for this transaction")

// SettlePaid records that AutoGoPay reported org's QR transactionID paid at paidAmount (the
// webhook's qris-paid task). It is idempotent: the payment is unique per transaction id, so a
// repeated webhook, or one after the check or reconcile already recorded the payment, records
// nothing new. A paid amount other than the attempt's is recorded as paid and flagged on the
// attempt (amount_mismatch) and in the audit log (qris_amount_mismatch/<attempt id>), for the
// admin (decision 39, point 5). When the check or reconcile already recorded the payment at the
// attempt's amount (their status answer carries none) and the webhook reports another, the
// recorded payment stays and the difference is audited and logged as an error, for an admin to
// correct. An id with no attempt in org is ErrUnknownTransaction.
func (s *Service) SettlePaid(ctx context.Context, org uuid.UUID, transactionID string, paidAmount int64) error {
	return s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetQrisAttemptByTransactionID(ctx, dbgen.GetQrisAttemptByTransactionIDParams{
			OrganizationID: org, TransactionID: transactionID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownTransaction
		}
		if err != nil {
			return fmt.Errorf("qris: reading the attempt of a paid transaction: %w", err)
		}
		a := &Attempt{
			ID: row.ID, TenantID: row.TenantID, TransactionID: row.TransactionID, Amount: row.Amount,
			QRString: row.QrString, Status: row.Status, ExpiresAt: row.ExpiresAt, CheckRequestedAt: row.CheckRequestedAt,
		}
		recorded, inserted, err := s.settle(ctx, tx, org, a, paidAmount)
		if err != nil {
			return err
		}
		switch {
		case inserted && paidAmount != a.Amount:
			s.log.WarnContext(ctx, "QRIS paid at another amount than its QR; flagged for the admin",
				"organization", org, "attempt", a.ID, "amount", a.Amount, "paid", paidAmount)
		case !inserted && recorded != paidAmount:
			s.log.ErrorContext(ctx, "AutoGoPay reports another amount than the payment already recorded; an admin must correct it",
				"organization", org, "attempt", a.ID, "recorded", recorded, "paid", paidAmount)
		default:
			return nil
		}
		if err := s.audit.RecordSystemAction(ctx, tx, org, authz.ActionPaymentRecord,
			authz.Resource{Type: "qris_amount_mismatch", ID: a.ID.String()}); err != nil {
			return fmt.Errorf("qris: auditing the amount mismatch of attempt %s: %w", a.ID, err)
		}
		return nil
	})
}
