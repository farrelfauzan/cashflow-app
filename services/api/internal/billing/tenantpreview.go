package billing

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// TenantPreview is what a payment a member records for a tenant would pay, oldest invoice first
// (S6-6: "Cocokkan ke tagihan" in Antrean cek shows it before the admin saves).
type TenantPreview struct {
	Amount            int64
	OutstandingBefore int64
	OutstandingAfter  int64
	// CreditAdded is the part of the amount no invoice takes; it becomes the tenant's credit.
	CreditAdded int64
	Items       []TenantPreviewItem
}

// TenantPreviewItem is one invoice the amount reaches, with the preview's figures.
type TenantPreviewItem struct {
	Invoice AccountInvoice
	PreviewItem
}

// PreviewTenantPayment tells a member what recording a payment of amount for a tenant now would
// pay, without recording anything. It needs invoice:read on the tenant through each of its leases,
// current or ended, as the tenant's account does; it is a read, so only a denial is audited. An
// amount above what the tenant owes is allowed, and the rest is CreditAdded, as recording it
// would do. An amount below 1 is ErrPreviewAmountInvalid; a tenant not in the organization is a
// *NotFoundError. The invoices are read in one snapshot, in FIFO order.
func (s *Service) PreviewTenantPayment(ctx context.Context, p authz.Principal, tenantID uuid.UUID, amount int64) (TenantPreview, error) {
	if amount < 1 {
		return TenantPreview{}, ErrPreviewAmountInvalid
	}
	var out TenantPreview
	err := s.store.WithOrgSnapshot(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		leases, err := tenantLeases(ctx, q, p.OrganizationID, tenantID, "")
		if err != nil {
			return err
		}
		req := authz.Request{Action: authz.ActionInvoiceRead, Resource: authz.Resource{Type: "tenant", ID: tenantID.String()}}
		if err := s.authorizeThroughLeases(ctx, tx, p, req, leases); err != nil {
			return err
		}
		acct, err := loadAccount(ctx, q, p.OrganizationID, tenantID, tenant.Today(s.clock))
		if err != nil {
			return err
		}
		// loadAccount lists invoices oldest period first, then seq: the order allocation pays them.
		open := make([]OpenBalance, len(acct.Invoices))
		byID := make(map[uuid.UUID]AccountInvoice, len(acct.Invoices))
		for i, inv := range acct.Invoices {
			open[i] = OpenBalance{InvoiceID: inv.ID, Balance: inv.Outstanding}
			byID[inv.ID] = inv
		}
		pv, err := PreviewRecordedPayment(open, amount)
		if err != nil {
			return err
		}
		out = TenantPreview{
			Amount: pv.Amount, OutstandingBefore: pv.OutstandingBefore, OutstandingAfter: pv.OutstandingAfter,
			CreditAdded: pv.CreditAdded, Items: make([]TenantPreviewItem, len(pv.Items)),
		}
		for i, it := range pv.Items {
			out.Items[i] = TenantPreviewItem{Invoice: byID[it.InvoiceID], PreviewItem: it}
		}
		return nil
	})
	if err != nil {
		return TenantPreview{}, err
	}
	return out, nil
}
