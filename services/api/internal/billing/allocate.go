package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// ErrTenantNotFound is returned by Allocate when the tenant does not exist in the
// transaction's organization.
var ErrTenantNotFound = errors.New("billing: tenant not found")

// Allocate brings one tenant's allocations in line with its payments, invoices and
// corrections (see Plan) and returns the rows it wrote, in order; none when nothing changed.
//
// It runs inside the caller's organization transaction (store.WithOrgTx), the one that
// recorded the payment, invoice or correction, so the change and its allocation commit
// together. It first locks the tenant's row, so allocations of one tenant never interleave:
// a second transaction waits here until the first commits, then sees its rows. It checks no
// permission; the caller authorizes the change that made allocation necessary.
func Allocate(ctx context.Context, tx pgx.Tx, orgID, tenantID uuid.UUID) ([]Row, error) {
	q := dbgen.New(tx)
	if _, err := q.LockTenantForAllocation(ctx, dbgen.LockTenantForAllocationParams{
		OrganizationID: orgID, TenantID: tenantID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("locking tenant %s for allocation: %w", tenantID, err)
	}

	payments, invoices, existing, err := loadAllocationState(ctx, q, orgID, tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := Plan(payments, invoices, existing)
	if err != nil {
		return nil, fmt.Errorf("planning allocation for tenant %s: %w", tenantID, err)
	}
	for _, r := range rows {
		if err := q.CreatePaymentAllocation(ctx, dbgen.CreatePaymentAllocationParams{
			ID: uuid.New(), OrganizationID: orgID, PaymentID: r.PaymentID, InvoiceID: r.InvoiceID,
			TenantID: tenantID, Amount: r.Amount,
		}); err != nil {
			return nil, fmt.Errorf("saving allocation for tenant %s: %w", tenantID, err)
		}
	}
	return rows, nil
}

func loadAllocationState(ctx context.Context, q *dbgen.Queries, orgID, tenantID uuid.UUID) ([]Payment, []Invoice, []Allocation, error) {
	payRows, err := q.ListAllocationPayments(ctx, dbgen.ListAllocationPaymentsParams{OrganizationID: orgID, TenantID: tenantID})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading payments of tenant %s: %w", tenantID, err)
	}
	payments := make([]Payment, len(payRows))
	for i, r := range payRows {
		payments[i] = Payment{ID: r.ID, ReceivedAt: r.ReceivedAt, Seq: r.Seq, Net: r.Net}
	}

	invRows, err := q.ListAllocationInvoices(ctx, dbgen.ListAllocationInvoicesParams{OrganizationID: orgID, TenantID: tenantID})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading invoices of tenant %s: %w", tenantID, err)
	}
	invoices := make([]Invoice, len(invRows))
	for i, r := range invRows {
		invoices[i] = Invoice{ID: r.InvoiceID, Period: r.Period, Seq: r.Seq, Billed: r.Billed}
	}

	allocRows, err := q.ListTenantAllocations(ctx, dbgen.ListTenantAllocationsParams{OrganizationID: orgID, TenantID: tenantID})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading allocations of tenant %s: %w", tenantID, err)
	}
	existing := make([]Allocation, len(allocRows))
	for i, r := range allocRows {
		existing[i] = Allocation{Seq: r.Seq, PaymentID: r.PaymentID, InvoiceID: r.InvoiceID, Amount: r.Amount}
	}
	return payments, invoices, existing, nil
}
