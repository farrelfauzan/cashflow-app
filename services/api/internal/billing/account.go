package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// StaffRef is the member who recorded or created a row.
type StaffRef struct {
	MemberID uuid.UUID
	Name     string
}

// CorrectionRecord is a correction as stored.
type CorrectionRecord struct {
	ID          uuid.UUID
	InvoiceID   uuid.UUID // uuid.Nil when it corrects a payment
	PaymentID   uuid.UUID // uuid.Nil when it corrects an invoice
	AmountDelta int64
	Reason      string
	CreatedBy   StaffRef
	CreatedAt   time.Time
}

// Totals are what a tenant owes over all their invoices, the due date of the oldest one not
// fully paid (nil when nothing is owed), and their credit.
type Totals struct {
	Outstanding         int64
	Overdue             int64 // the part of Outstanding due before today (debt from H+1)
	OldestUnpaidDueDate *time.Time
	Credit              int64
}

// AccountLine is one line of an invoice.
type AccountLine struct {
	ID     uuid.UUID
	Kind   string // rent or charge
	Label  string
	Amount int64
}

// AccountInvoice is one invoice of a tenant with its balance, lines and corrections.
type AccountInvoice struct {
	ID           uuid.UUID
	LeaseID      uuid.UUID
	UnitLabel    string
	PropertyName string
	Period       time.Time
	PeriodEnd    time.Time
	DueDate      time.Time
	IssuedOn     time.Time // the Asia/Jakarta day the invoice was made
	Amount       int64
	Billed       int64
	Paid         int64
	Outstanding  int64
	Status       string // open, partial, paid or cancelled
	Lines        []AccountLine
	Corrections  []CorrectionRecord
}

// PaidAmount is what a payment holds on one invoice now; never zero.
type PaidAmount struct {
	InvoiceID uuid.UUID
	Amount    int64
}

// AccountPayment is one payment of a tenant with what it pays now and its corrections.
type AccountPayment struct {
	ID          uuid.UUID
	Amount      int64
	Net         int64
	Method      string
	ReceivedOn  time.Time // Asia/Jakarta calendar day, as midnight UTC
	Note        string
	RecordedBy  *StaffRef // nil for a gateway payment
	Allocations []PaidAmount
	Corrections []CorrectionRecord
	CreatedAt   time.Time
}

// Account is everything a tenant was billed and paid, across all their leases.
type Account struct {
	TenantID uuid.UUID
	Totals
	Invoices []AccountInvoice // oldest period first, cancelled ones included
	Payments []AccountPayment // oldest received first
}

// Account returns a tenant's money account. It needs invoice:read and payment:read for the
// whole organization, or through every lease the tenant has or had, since the totals span all
// of them; a tenant with no lease needs the organization-wide grant.
func (s *Service) Account(ctx context.Context, p authz.Principal, tenantID uuid.UUID) (Account, error) {
	var out Account
	// One snapshot, so the totals and the invoices agree even while the worker commits one.
	err := s.store.WithOrgSnapshot(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		leases, err := tenantLeases(ctx, q, p.OrganizationID, tenantID, "")
		if err != nil {
			return err
		}
		for _, action := range []string{authz.ActionInvoiceRead, authz.ActionPaymentRead} {
			req := authz.Request{Action: action, Resource: authz.Resource{Type: "tenant", ID: tenantID.String()}}
			if err := s.authorizeThroughLeases(ctx, tx, p, req, leases); err != nil {
				return err
			}
		}
		out, err = loadAccount(ctx, q, p.OrganizationID, tenantID, tenant.Today(s.clock))
		return err
	})
	return out, err
}

// leaseScope is a lease through which a member may act on its tenant's money.
type leaseScope struct {
	PropertyID      uuid.UUID
	CollectorUserID uuid.UUID
}

// tenantLeases checks that the tenant exists in the organization and returns every lease it
// has or had. A missing tenant is a *NotFoundError on field.
func tenantLeases(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, field string) ([]leaseScope, error) {
	if _, err := q.GetTenant(ctx, dbgen.GetTenantParams{OrganizationID: org, ID: tenantID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &NotFoundError{Field: field}
		}
		return nil, fmt.Errorf("loading tenant: %w", err)
	}
	rows, err := q.ListTenantLeaseScopes(ctx, dbgen.ListTenantLeaseScopesParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("loading the tenant's leases: %w", err)
	}
	out := make([]leaseScope, len(rows))
	for i, r := range rows {
		out[i] = leaseScope{PropertyID: r.PropertyID, CollectorUserID: r.CollectorStaffUserID.UUID}
	}
	return out, nil
}

// authorizeThroughLeases allows req through every one of leases: the same request in the
// lease's property, assigned to its collector. It never stops at an organization-wide Allow on
// the bare record, which carries no property, so a Deny that names a property still matches
// (an organization-wide Allow grants every lease anyway). With no lease only the
// organization-wide grant can allow it. Authorize audits each allowed sensitive check in tx and
// records a denial in its own transaction.
func (s *Service) authorizeThroughLeases(ctx context.Context, tx pgx.Tx, p authz.Principal, req authz.Request, leases []leaseScope) error {
	if len(leases) == 0 {
		return s.authz.Authorize(ctx, tx, p, req)
	}
	for _, l := range leases {
		via := req
		via.Resource.PropertyID = l.PropertyID
		via.Resource.OwnerID = l.CollectorUserID
		if err := s.authz.Authorize(ctx, tx, p, via); err != nil {
			return err
		}
	}
	return nil
}

// tenantTotals reads a tenant's balance; today is the current day in Asia/Jakarta
// (tenant.Today), which decides what is overdue.
func tenantTotals(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, today time.Time) (Totals, error) {
	b, err := q.GetTenantBalance(ctx, dbgen.GetTenantBalanceParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return Totals{}, fmt.Errorf("loading the tenant's balance: %w", err)
	}
	overdue, err := q.GetTenantOverdue(ctx, dbgen.GetTenantOverdueParams{OrganizationID: org, TenantID: tenantID, Today: today})
	if err != nil {
		return Totals{}, fmt.Errorf("loading the tenant's overdue amount: %w", err)
	}
	return Totals{Outstanding: b.Outstanding, Overdue: overdue, OldestUnpaidDueDate: b.OldestUnpaidDueDate, Credit: b.Credit}, nil
}

// loadAccount reads a tenant's account with one query per kind of row. It checks no permission.
func loadAccount(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, today time.Time) (Account, error) {
	out := Account{TenantID: tenantID, Invoices: []AccountInvoice{}, Payments: []AccountPayment{}}
	var err error
	if out.Totals, err = tenantTotals(ctx, q, org, tenantID, today); err != nil {
		return Account{}, err
	}
	invRows, err := q.ListAccountInvoices(ctx, dbgen.ListAccountInvoicesParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return Account{}, fmt.Errorf("loading the tenant's invoices: %w", err)
	}
	lineRows, err := q.ListAccountInvoiceLines(ctx, dbgen.ListAccountInvoiceLinesParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return Account{}, fmt.Errorf("loading the tenant's invoice lines: %w", err)
	}
	corrRows, err := q.ListAccountCorrections(ctx, dbgen.ListAccountCorrectionsParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return Account{}, fmt.Errorf("loading the tenant's corrections: %w", err)
	}
	payRows, err := q.ListAccountPayments(ctx, dbgen.ListAccountPaymentsParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return Account{}, fmt.Errorf("loading the tenant's payments: %w", err)
	}
	allocRows, err := q.ListAccountAllocations(ctx, dbgen.ListAccountAllocationsParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return Account{}, fmt.Errorf("loading the tenant's allocations: %w", err)
	}

	lines := map[uuid.UUID][]AccountLine{}
	for _, r := range lineRows {
		lines[r.InvoiceID] = append(lines[r.InvoiceID], AccountLine{ID: r.ID, Kind: r.Kind, Label: r.Label, Amount: r.Amount})
	}
	byInvoice := map[uuid.UUID][]CorrectionRecord{}
	byPayment := map[uuid.UUID][]CorrectionRecord{}
	for _, r := range corrRows {
		c := CorrectionRecord{
			ID: r.ID, InvoiceID: r.InvoiceID.UUID, PaymentID: r.PaymentID.UUID, AmountDelta: r.AmountDelta, Reason: r.Reason,
			CreatedBy: StaffRef{MemberID: r.CreatedByMemberID, Name: r.CreatedByName}, CreatedAt: r.CreatedAt,
		}
		if r.InvoiceID.Valid {
			byInvoice[r.InvoiceID.UUID] = append(byInvoice[r.InvoiceID.UUID], c)
		} else {
			byPayment[r.PaymentID.UUID] = append(byPayment[r.PaymentID.UUID], c)
		}
	}
	allocs := map[uuid.UUID][]PaidAmount{}
	for _, r := range allocRows {
		allocs[r.PaymentID] = append(allocs[r.PaymentID], PaidAmount{InvoiceID: r.InvoiceID, Amount: r.Amount})
	}

	for _, r := range invRows {
		out.Invoices = append(out.Invoices, AccountInvoice{
			ID: r.InvoiceID, LeaseID: r.LeaseID, UnitLabel: r.UnitLabel, PropertyName: r.PropertyName,
			Period: r.Period, PeriodEnd: r.PeriodEnd, DueDate: r.DueDate, IssuedOn: receivedOn(r.CreatedAt),
			Amount: r.Amount, Billed: r.Billed, Paid: r.Paid, Outstanding: r.Outstanding, Status: r.Status,
			Lines: orEmpty(lines[r.InvoiceID]), Corrections: orEmpty(byInvoice[r.InvoiceID]),
		})
	}
	for _, r := range payRows {
		pay := AccountPayment{
			ID: r.ID, Amount: r.Amount, Net: r.Net, Method: r.Method, ReceivedOn: receivedOn(r.ReceivedAt),
			Allocations: orEmpty(allocs[r.ID]), Corrections: orEmpty(byPayment[r.ID]), CreatedAt: r.CreatedAt,
		}
		if r.Note != nil {
			pay.Note = *r.Note
		}
		if r.RecordedByMemberID.Valid {
			ref := StaffRef{MemberID: r.RecordedByMemberID.UUID}
			if r.RecordedByName != nil {
				ref.Name = *r.RecordedByName
			}
			pay.RecordedBy = &ref
		}
		out.Payments = append(out.Payments, pay)
	}
	return out, nil
}

// orEmpty returns s, or an empty slice instead of nil, so lists answer [] rather than null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
