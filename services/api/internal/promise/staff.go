package promise

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// TenantPromisesTx implements billing.PromiseReader for Detail penyewa: the tenant's active
// promise, their broken promises, and what a promise made now may say, read inside tx (scoped to
// org) with the payment page's own reader (LoadPageTx and PageState.Offer), so both show the same
// dates. It checks no permission: billing.Service.Account has authorized the read.
func (c *Closer) TenantPromisesTx(ctx context.Context, tx pgx.Tx, org, tenantID uuid.UUID, outstanding int64, today time.Time) (billing.TenantPromises, error) {
	st, err := LoadPageTx(ctx, tx, org, tenantID, outstanding, today)
	if err != nil {
		return billing.TenantPromises{}, err
	}
	var out billing.TenantPromises
	if w := st.Offer(); w != nil {
		out.Dates = &billing.PromiseDates{Earliest: w.Earliest, Latest: w.Latest}
		out.NextInvoiceDueDate = w.NextDueInRange()
	}
	sums, err := summaries(ctx, dbgen.New(tx), org, []uuid.UUID{tenantID}, nil)
	if err != nil {
		return billing.TenantPromises{}, err
	}
	out.Active, out.Broken = sums[tenantID].Active, sums[tenantID].Broken
	return out, nil
}

// PromiseSummariesTx implements billing.PromiseReader for Penagihan: the active promise and broken
// count of each given tenant, in three reads whatever the number of tenants. leaseIDs, when not
// nil, keeps only the covered invoices of those leases.
func (c *Closer) PromiseSummariesTx(ctx context.Context, tx pgx.Tx, org uuid.UUID, tenantIDs, leaseIDs []uuid.UUID) (map[uuid.UUID]billing.TenantPromiseSummary, error) {
	return summaries(ctx, dbgen.New(tx), org, tenantIDs, leaseIDs)
}

// summaries reads the active promises' newest versions, their covered invoices and the broken
// counts of tenantIDs: at most three queries.
func summaries(ctx context.Context, q *dbgen.Queries, org uuid.UUID, tenantIDs, leaseIDs []uuid.UUID) (map[uuid.UUID]billing.TenantPromiseSummary, error) {
	out := map[uuid.UUID]billing.TenantPromiseSummary{}
	if len(tenantIDs) == 0 {
		return out, nil
	}
	rows, err := q.ListActivePromiseLatest(ctx, dbgen.ListActivePromiseLatestParams{OrganizationID: org, TenantIds: tenantIDs})
	if err != nil {
		return nil, fmt.Errorf("promise: loading the active promises: %w", err)
	}
	if len(rows) > 0 {
		vids := make([]uuid.UUID, len(rows))
		for i, r := range rows {
			vids[i] = r.ID
		}
		crows, err := q.ListPromiseCoveredInvoices(ctx, dbgen.ListPromiseCoveredInvoicesParams{
			OrganizationID: org, VersionIds: vids, LeaseIds: leaseIDs,
		})
		if err != nil {
			return nil, fmt.Errorf("promise: loading covered invoices: %w", err)
		}
		covered := map[uuid.UUID][]uuid.UUID{}
		for _, cr := range crows {
			covered[cr.PromiseVersionID] = append(covered[cr.PromiseVersionID], cr.InvoiceID)
		}
		for _, r := range rows {
			a := &billing.ActivePromise{
				ID: r.PromiseID, Version: int(r.Version), Kind: r.Kind, PromisedDate: r.PromisedDate,
				PromisedAmount: r.PromisedAmount, PayingNow: r.PayingNow, Source: r.Source, CreatedAt: r.CreatedAt,
				CoveredInvoiceIDs: covered[r.ID],
			}
			if a.CoveredInvoiceIDs == nil {
				a.CoveredInvoiceIDs = []uuid.UUID{}
			}
			if r.Note != nil {
				a.Note = *r.Note
			}
			if r.CreatedByMemberID.Valid {
				ref := billing.StaffRef{MemberID: r.CreatedByMemberID.UUID}
				if r.CreatedByName != nil {
					ref.Name = *r.CreatedByName
				}
				a.CreatedBy = &ref
			}
			out[r.TenantID] = billing.TenantPromiseSummary{Active: a}
		}
	}
	brows, err := q.CountBrokenPromisesByTenant(ctx, dbgen.CountBrokenPromisesByTenantParams{OrganizationID: org, TenantIds: tenantIDs})
	if err != nil {
		return nil, fmt.Errorf("promise: counting broken promises: %w", err)
	}
	for _, b := range brows {
		s := out[b.TenantID]
		s.Broken = int(b.Broken)
		out[b.TenantID] = s
	}
	return out, nil
}
