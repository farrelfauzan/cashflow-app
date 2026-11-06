package paylink

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// Page is what the tenant payment page shows, and nothing more: no phone number, payment
// history, staff name, note, correction reason or id of anyone else.
type Page struct {
	// OrganizationName is the organization's display name, so the tenant knows who bills them.
	OrganizationName string
	// TenantName shows whose link this is, so a forwarded link is obvious.
	TenantName string
	// Invoice is the invoice the link was sent for.
	Invoice PageInvoice
	// Outstanding is what the tenant owes over all their invoices; Credit is money paid and not
	// yet used. Paying through the link pays the oldest invoice first.
	Outstanding int64
	Credit      int64
	// Paid is true when the tenant owes nothing.
	Paid bool
	// OpenInvoices are the invoices that still owe something, oldest period first.
	OpenInvoices []PageInvoice
	// BankAccount is where to transfer by hand; nil when the organization has not set one.
	BankAccount *BankAccount
	// QRISAvailable is true when the organization has an AutoGoPay account, so the page offers
	// QRIS. It says nothing about whether AutoGoPay is up right now.
	QRISAvailable bool
	// Promise is the tenant's active payment promise and what a promise made now may say
	// ("Janji pelunasan"), read in the same snapshot.
	Promise promise.PageState
}

// PageInvoice is one invoice as the tenant sees it.
type PageInvoice struct {
	ID           uuid.UUID
	Kind         string // billing.InvoiceRegular or billing.InvoiceFinal
	UnitLabel    string
	PropertyName string
	Period       time.Time
	PeriodEnd    time.Time
	DueDate      time.Time
	// Amount is what the invoice bills now, corrections included; Balance is what it still owes.
	Amount  int64
	Balance int64
	// Lines are as snapshotted when the invoice was made.
	Lines []PageLine
}

// PageLine is one line of an invoice.
type PageLine struct {
	Label  string
	Amount int64
}

// PublicPage reads the payment page behind a token. The token alone grants it, and only for the
// link's own tenant. A malformed, unknown or revoked token is ErrLinkNotFound; a link whose
// invoice was paid or cancelled LinkLifetimeAfterPaid ago or longer is ErrLinkExpired.
// Everything after the token lookup is read in one snapshot of the link's organization, so the
// totals and the invoices agree.
func (s *Service) PublicPage(ctx context.Context, token string) (Page, error) {
	org, linkID, err := s.Resolve(ctx, token)
	if err != nil {
		return Page{}, err
	}
	var out Page
	err = s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		tenantID, invoiceID, err := s.OpenLink(ctx, tx, org, linkID)
		if err != nil {
			return err
		}
		out, err = s.readPage(ctx, tx, dbgen.New(tx), org, tenantID, invoiceID)
		return err
	})
	if err != nil {
		return Page{}, err
	}
	return out, nil
}

// OpenLink checks, inside tx scoped to org, that the link a token resolved to (Resolve) still
// works, and returns its tenant and invoice. A missing or revoked link is ErrLinkNotFound; a link
// whose invoice was paid or cancelled LinkLifetimeAfterPaid ago or longer is ErrLinkExpired. Every
// public endpoint behind a token opens the link this way before reading anything else.
func (s *Service) OpenLink(ctx context.Context, tx pgx.Tx, org, linkID uuid.UUID) (tenantID, invoiceID uuid.UUID, err error) {
	q := dbgen.New(tx)
	link, err := q.GetPaymentLink(ctx, dbgen.GetPaymentLinkParams{OrganizationID: org, ID: linkID})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrLinkNotFound
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("paylink: loading the link: %w", err)
	}
	if link.RevokedAt != nil {
		return uuid.Nil, uuid.Nil, ErrLinkNotFound
	}
	st, err := q.GetInvoiceSettlement(ctx, dbgen.GetInvoiceSettlementParams{OrganizationID: org, InvoiceID: link.InvoiceID})
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("paylink: reading the invoice's balance: %w", err)
	}
	if Expired(s.clock.Now(), st.Outstanding, st.SettledAt) {
		return uuid.Nil, uuid.Nil, ErrLinkExpired
	}
	return link.TenantID, link.InvoiceID, nil
}

// readPage reads what the page shows for a link that works.
func (s *Service) readPage(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, org, tenantID, invoiceID uuid.UUID) (Page, error) {
	o, err := q.GetOrganization(ctx, org)
	if err != nil {
		return Page{}, fmt.Errorf("paylink: loading the organization: %w", err)
	}
	t, err := q.GetTenant(ctx, dbgen.GetTenantParams{OrganizationID: org, ID: tenantID})
	if err != nil {
		return Page{}, fmt.Errorf("paylink: loading the tenant: %w", err)
	}
	today := tenant.Today(s.clock)
	acct, err := billing.LoadAccountTx(ctx, tx, org, tenantID, today)
	if err != nil {
		return Page{}, err
	}
	promises, err := promise.LoadPageTx(ctx, tx, org, tenantID, acct.Outstanding, today)
	if err != nil {
		return Page{}, err
	}
	bank, err := loadBankAccount(ctx, q, org)
	if err != nil {
		return Page{}, err
	}
	qrisAvailable, err := q.HasAutogopayAccount(ctx, org)
	if err != nil {
		return Page{}, fmt.Errorf("paylink: checking for an AutoGoPay account: %w", err)
	}

	out := Page{
		OrganizationName: o.DisplayName,
		TenantName:       t.Name,
		Outstanding:      acct.Outstanding,
		Credit:           acct.Credit,
		Paid:             acct.Outstanding == 0,
		OpenInvoices:     []PageInvoice{},
		BankAccount:      bank,
		QRISAvailable:    qrisAvailable,
		Promise:          promises,
	}
	found := false
	for _, inv := range acct.Invoices {
		if inv.ID == invoiceID {
			out.Invoice, found = pageInvoice(inv), true
		}
		if inv.Outstanding > 0 {
			out.OpenInvoices = append(out.OpenInvoices, pageInvoice(inv))
		}
	}
	if !found {
		// The link's foreign key ties it to an invoice of its own tenant, so this cannot happen.
		return Page{}, errors.New("paylink: the link's invoice is not among its tenant's invoices")
	}
	return out, nil
}

func pageInvoice(inv billing.AccountInvoice) PageInvoice {
	lines := make([]PageLine, len(inv.Lines))
	for i, l := range inv.Lines {
		lines[i] = PageLine{Label: l.Label, Amount: l.Amount}
	}
	return PageInvoice{
		ID: inv.ID, Kind: inv.Kind, UnitLabel: inv.UnitLabel, PropertyName: inv.PropertyName,
		Period: inv.Period, PeriodEnd: inv.PeriodEnd, DueDate: inv.DueDate,
		Amount: inv.Billed, Balance: inv.Outstanding, Lines: lines,
	}
}
