package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// OpenInvoice is one invoice of a tenant that still owes something.
type OpenInvoice struct {
	ID           uuid.UUID
	Kind         string // InvoiceRegular or InvoiceFinal
	LeaseID      uuid.UUID
	UnitLabel    string
	PropertyName string
	Period       time.Time
	PeriodEnd    time.Time
	DueDate      time.Time
	Billed       int64 // what it bills now: its amount plus its corrections
	Outstanding  int64 // billed minus what is allocated to it; always above 0
}

// TenantOpenInvoices returns the tenant's invoices still owed, of any month, in the account's
// order (oldest period first). It needs invoice:read only and is filtered like ListInvoices,
// through the same visibleLeases: a member whose read is conditional sees only the invoices of
// leases they may read, and a tenant they can see none of answers an empty list. A member with
// no invoice:read at all is denied (and audited) before the tenant is looked up, so the answer
// does not tell them whether it exists; an unknown tenant is a *NotFoundError, as in Account.
func (s *Service) TenantOpenInvoices(ctx context.Context, p authz.Principal, tenantID uuid.UUID) ([]OpenInvoice, error) {
	out := []OpenInvoice{}
	org := p.OrganizationID
	// One snapshot, so the visibility and the balances read the same data.
	err := s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		vis, err := s.visibleLeases(ctx, tx, q, p, authz.ActionInvoiceRead, "invoice")
		if err != nil {
			return err
		}
		if _, err := tenantLeases(ctx, q, org, tenantID, ""); err != nil {
			return err
		}
		visibleLease := make(map[uuid.UUID]bool, len(vis.leaseIDs))
		for _, id := range vis.leaseIDs {
			visibleLease[id] = true
		}
		// The account's own reader, so every figure is the same derived balance. Every invoice
		// has a lease (invoice.lease_id is NOT NULL), so vis.orgWide never decides one here.
		rows, err := q.ListAccountInvoices(ctx, dbgen.ListAccountInvoicesParams{OrganizationID: org, TenantID: tenantID})
		if err != nil {
			return fmt.Errorf("loading the tenant's invoices: %w", err)
		}
		for _, r := range rows {
			if r.Outstanding <= 0 || !visibleLease[r.LeaseID] {
				continue
			}
			out = append(out, OpenInvoice{
				ID: r.InvoiceID, Kind: r.Kind, LeaseID: r.LeaseID, UnitLabel: r.UnitLabel, PropertyName: r.PropertyName,
				Period: r.Period, PeriodEnd: r.PeriodEnd, DueDate: r.DueDate, Billed: r.Billed, Outstanding: r.Outstanding,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
