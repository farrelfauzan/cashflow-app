package paylink

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// Preview is what paying an amount through a link would pay, oldest invoice first.
type Preview struct {
	Amount            int64
	OutstandingBefore int64
	OutstandingAfter  int64
	Items             []PreviewItem
}

// PreviewItem is one invoice the amount reaches. Invoice carries what the tenant sees of it
// (no lines are needed here); Balance, Applied and BalanceAfter are the preview's figures.
type PreviewItem struct {
	Invoice      PageInvoice
	Balance      int64
	Applied      int64
	BalanceAfter int64
	PaidInFull   bool
}

// PublicPreview tells the tenant behind a token what paying amount now would pay, without
// recording anything. Link errors are those of PublicPage. An amount below 1 is
// billing.ErrPreviewAmountInvalid; above what the tenant owes now (nothing owed included) it is
// billing.ErrAmountAboveOutstanding. The tenant's invoices are read in one snapshot of the link's
// organization, in FIFO order, and the walk itself is billing.PreviewPayment.
func (s *Service) PublicPreview(ctx context.Context, token string, amount int64) (Preview, error) {
	org, linkID, err := s.Resolve(ctx, token)
	if err != nil {
		return Preview{}, err
	}
	var out Preview
	err = s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		tenantID, _, err := s.OpenLink(ctx, tx, org, linkID)
		if err != nil {
			return err
		}
		out, err = s.readPreview(ctx, tx, org, tenantID, amount)
		return err
	})
	if err != nil {
		return Preview{}, err
	}
	return out, nil
}

func (s *Service) readPreview(ctx context.Context, tx pgx.Tx, org, tenantID uuid.UUID, amount int64) (Preview, error) {
	acct, err := billing.LoadAccountTx(ctx, tx, org, tenantID, tenant.Today(s.clock))
	if err != nil {
		return Preview{}, fmt.Errorf("paylink: loading the tenant's account: %w", err)
	}
	// LoadAccountTx lists invoices oldest period first, then seq: the order allocation pays them.
	open := make([]billing.OpenBalance, 0, len(acct.Invoices))
	byID := make(map[uuid.UUID]billing.AccountInvoice, len(acct.Invoices))
	for _, inv := range acct.Invoices {
		open = append(open, billing.OpenBalance{InvoiceID: inv.ID, Balance: inv.Outstanding})
		byID[inv.ID] = inv
	}
	p, err := billing.PreviewPayment(open, amount)
	if err != nil {
		return Preview{}, err
	}
	out := Preview{
		Amount: p.Amount, OutstandingBefore: p.OutstandingBefore, OutstandingAfter: p.OutstandingAfter,
		Items: make([]PreviewItem, len(p.Items)),
	}
	for i, it := range p.Items {
		out.Items[i] = PreviewItem{
			Invoice: pageInvoice(byID[it.InvoiceID]), Balance: it.Balance, Applied: it.Applied,
			BalanceAfter: it.BalanceAfter, PaidInFull: it.PaidInFull,
		}
	}
	return out, nil
}
