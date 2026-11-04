package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// dashboardRows is how many tenants in arrears and corrections the dashboard shows.
const dashboardRows = 10

// UnitRef names a unit by its property and label.
type UnitRef struct {
	PropertyName string
	UnitLabel    string
}

// TenantArrears is a tenant who owes money after its due date.
type TenantArrears struct {
	TenantID            uuid.UUID
	TenantName          string
	Units               []UnitRef // of the overdue invoices, by property then unit
	Overdue             int64
	OldestUnpaidDueDate time.Time
	AgeDays             int // 1 on the day after the due date, Asia/Jakarta
}

// DashboardCorrection is one recent correction with what it corrected.
type DashboardCorrection struct {
	CorrectionRecord
	TenantID   uuid.UUID
	TenantName string
	Period     *time.Time // the corrected invoice's period start
	Unit       *UnitRef   // the corrected invoice's unit
	ReceivedOn *time.Time // the corrected payment's Asia/Jakarta day
}

// Dashboard is the Supervisor's home: one month's money and who is in arrears now.
type Dashboard struct {
	Period             time.Time // the first day of the month
	Billed             int64
	Collected          int64
	Outstanding        int64
	InvoiceCount       int
	TenantsOutstanding int
	Overdue            int64 // any month
	TenantsInArrears   int
	Arrears            []TenantArrears       // up to 10, the oldest debt first
	RecentCorrections  []DashboardCorrection // up to 10, newest first
}

// Invoice list filters and sorts (Penagihan).
const (
	InvoiceStatusUnpaid  = "unpaid"
	InvoiceStatusOverdue = "overdue"
)

var (
	invoiceStatuses = map[string][]string{
		"":                   {"open", "partial", "paid", "cancelled"},
		"open":               {"open"},
		"partial":            {"partial"},
		"paid":               {"paid"},
		"cancelled":          {"cancelled"},
		InvoiceStatusUnpaid:  {"open", "partial"},
		InvoiceStatusOverdue: {"open", "partial"},
	}
	invoiceSorts = map[string]bool{"dueDate": true, "-dueDate": true, "tenantName": true, "-outstanding": true}
)

// InvoiceQuery is what Penagihan asks for. Period is YYYY-MM, empty for this month in
// Asia/Jakarta; Status and Sort take the values of the API, empty for every invoice and dueDate.
type InvoiceQuery struct {
	Period     string
	Status     string
	PropertyID uuid.UUID // uuid.Nil for every property
	Search     listing.Search
	Sort       string
	Page       listing.Page
}

// InvoiceListItem is one invoice of Penagihan with its lines.
type InvoiceListItem struct {
	ID             uuid.UUID
	Kind           string // InvoiceRegular or InvoiceFinal
	TenantID       uuid.UUID
	TenantName     string
	LeaseID        uuid.UUID
	PropertyID     uuid.UUID
	PropertyName   string
	UnitLabel      string
	Period         time.Time
	PeriodEnd      time.Time
	DueDate        time.Time
	IssuedOn       time.Time // the Asia/Jakarta day the invoice was made
	Amount         int64
	Billed         int64
	Paid           int64
	Outstanding    int64
	Status         string
	OverdueDays    int // 0 when nothing is overdue
	PastPlannedEnd bool
	Lines          []AccountLine
}

// OldestOverdue is the oldest debt now, with the unit of that invoice.
type OldestOverdue struct {
	TenantID            uuid.UUID
	TenantName          string
	PropertyName        string
	UnitLabel           string
	OldestUnpaidDueDate time.Time
	AgeDays             int
}

// InvoiceSummary is the month's invoices in the chosen property, whatever the status, search
// and page are, plus the oldest debt of any month in that property (nil when none).
type InvoiceSummary struct {
	Billed             int64
	Paid               int64
	Outstanding        int64
	TenantsOutstanding int
	OldestOverdue      *OldestOverdue
}

// InvoiceList is one page of Penagihan with its summary.
type InvoiceList struct {
	listing.Result[InvoiceListItem]
	Summary InvoiceSummary
}

// Dashboard returns the month's money and the arrears now. It needs report:read; a member who
// may read reports only in some properties sees, and every figure counts, only the invoices of
// leases they may read reports on. period is YYYY-MM, empty for this month in Asia/Jakarta.
func (s *Service) Dashboard(ctx context.Context, p authz.Principal, period string) (Dashboard, error) {
	today := tenant.Today(s.clock)
	month, err := parseMonth(period, today)
	if err != nil {
		return Dashboard{}, err
	}
	out := Dashboard{Period: month, Arrears: []TenantArrears{}, RecentCorrections: []DashboardCorrection{}}
	org := p.OrganizationID
	// One snapshot, so the summary, the rows and the totals agree.
	err = s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		vis, err := s.visibleLeases(ctx, tx, q, p, authz.ActionReportRead, "report")
		if err != nil {
			return err
		}
		sum, err := q.SumMonthInvoices(ctx, dbgen.SumMonthInvoicesParams{
			OrganizationID: org, LeaseIds: vis.leaseIDs, MonthStart: month, MonthEnd: month.AddDate(0, 1, 0),
		})
		if err != nil {
			return fmt.Errorf("summing the month's invoices: %w", err)
		}
		out.Billed, out.Collected, out.Outstanding = sum.Billed, sum.Paid, sum.Outstanding
		out.InvoiceCount, out.TenantsOutstanding = int(sum.InvoiceCount), int(sum.TenantsOutstanding)

		od, err := q.SumOverdue(ctx, dbgen.SumOverdueParams{OrganizationID: org, LeaseIds: vis.leaseIDs, Today: today})
		if err != nil {
			return fmt.Errorf("summing arrears: %w", err)
		}
		out.Overdue, out.TenantsInArrears = od.Overdue, int(od.Tenants)

		if out.Arrears, err = loadArrears(ctx, q, org, vis.leaseIDs, today); err != nil {
			return err
		}
		out.RecentCorrections, err = loadRecentCorrections(ctx, q, org, vis)
		return err
	})
	if err != nil {
		return Dashboard{}, err
	}
	return out, nil
}

// ListInvoices returns one page of the month's invoices with their lines, and the month's
// summary. It needs invoice:read; a member who may read invoices only in some properties, or
// only of their own tenants, sees, and every figure counts, only the invoices of those leases.
func (s *Service) ListInvoices(ctx context.Context, p authz.Principal, iq InvoiceQuery) (InvoiceList, error) {
	today := tenant.Today(s.clock)
	month, err := parseMonth(iq.Period, today)
	if err != nil {
		return InvoiceList{}, err
	}
	statuses, ok := invoiceStatuses[iq.Status]
	if !ok {
		return InvoiceList{}, &InputError{Field: "status", Message: "Status tidak dikenal."}
	}
	if iq.Sort == "" {
		iq.Sort = "dueDate"
	}
	if !invoiceSorts[iq.Sort] {
		return InvoiceList{}, &InputError{Field: "sort", Message: "Urutan tidak dikenal."}
	}
	if iq.Page == (listing.Page{}) {
		iq.Page = listing.FirstPage
	}
	org := p.OrganizationID
	property := uuid.NullUUID{UUID: iq.PropertyID, Valid: iq.PropertyID != uuid.Nil}
	out := InvoiceList{Result: listing.Result[InvoiceListItem]{Page: iq.Page, Items: []InvoiceListItem{}}}
	// One snapshot, so the summary, the rows and the totals agree.
	err = s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		vis, err := s.visibleLeases(ctx, tx, q, p, authz.ActionInvoiceRead, "invoice")
		if err != nil {
			return err
		}
		args := dbgen.ListInvoicePageParams{
			Today: today, OrganizationID: org, LeaseIds: vis.leaseIDs, MonthStart: month, MonthEnd: month.AddDate(0, 1, 0),
			PropertyID: property, Q: iq.Search.Text, Statuses: statuses, OverdueOnly: iq.Status == InvoiceStatusOverdue,
			Sort: iq.Sort, PageLimit: iq.Page.Limit(), PageOffset: iq.Page.Offset(),
		}
		if out.Total, err = q.CountInvoicePage(ctx, dbgen.CountInvoicePageParams{
			OrganizationID: args.OrganizationID, LeaseIds: args.LeaseIds, MonthStart: args.MonthStart, MonthEnd: args.MonthEnd,
			PropertyID: args.PropertyID, Q: args.Q, Statuses: args.Statuses, OverdueOnly: args.OverdueOnly, Today: args.Today,
		}); err != nil {
			return fmt.Errorf("counting invoices: %w", err)
		}
		if out.Items, err = loadInvoicePage(ctx, q, args); err != nil {
			return err
		}
		out.Summary, err = loadInvoiceSummary(ctx, q, args)
		return err
	})
	if err != nil {
		return InvoiceList{}, err
	}
	return out, nil
}

// visible is what a member may see of the organization's money: the leases whose invoices they
// may read, and whether the organization-wide grant allows a record with no lease.
type visible struct {
	leaseIDs []uuid.UUID
	orgWide  bool
}

// visibleLeases authorizes action on the whole collection, so a member with no access at all is
// denied and audited, then checks every lease of the organization, active or ended, in its
// property and assigned to its collector. It never stops at an organization-wide Allow: the
// bare request carries no property, so only the check through each lease meets a Deny that names
// a property.
func (s *Service) visibleLeases(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, p authz.Principal, action, resourceType string) (visible, error) {
	bare := authz.Request{Action: action, Resource: authz.Resource{Type: resourceType}}
	all, err := s.authz.AuthorizeList(ctx, tx, p, bare)
	if err != nil {
		return visible{}, err
	}
	scopes, err := q.ListLeaseScopes(ctx, p.OrganizationID)
	if err != nil {
		return visible{}, fmt.Errorf("listing lease scopes: %w", err)
	}
	// A record with no lease belongs to no property, so the bare check decides it, as in the
	// tenant list. all can be false under an Allow on everything with a Deny on one property.
	out := visible{leaseIDs: []uuid.UUID{}, orgWide: all || authz.Allowed(p, bare)}
	for _, l := range scopes {
		via := bare
		via.Resource.PropertyID = l.PropertyID
		via.Resource.OwnerID = l.CollectorStaffUserID.UUID
		if authz.Allowed(p, via) {
			out.leaseIDs = append(out.leaseIDs, l.ID)
		}
	}
	return out, nil
}

func loadArrears(ctx context.Context, q *dbgen.Queries, org uuid.UUID, leaseIDs []uuid.UUID, today time.Time) ([]TenantArrears, error) {
	rows, err := q.ListTenantArrears(ctx, dbgen.ListTenantArrearsParams{
		OrganizationID: org, LeaseIds: leaseIDs, Today: today, RowLimit: dashboardRows,
	})
	if err != nil {
		return nil, fmt.Errorf("listing tenants in arrears: %w", err)
	}
	out := make([]TenantArrears, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.TenantID
	}
	unitRows, err := q.ListArrearsUnits(ctx, dbgen.ListArrearsUnitsParams{
		OrganizationID: org, LeaseIds: leaseIDs, TenantIds: ids, Today: today,
	})
	if err != nil {
		return nil, fmt.Errorf("listing the units in arrears: %w", err)
	}
	units := map[uuid.UUID][]UnitRef{}
	for _, u := range unitRows {
		units[u.TenantID] = append(units[u.TenantID], UnitRef{PropertyName: u.PropertyName, UnitLabel: u.UnitLabel})
	}
	for i, r := range rows {
		out[i] = TenantArrears{
			TenantID: r.TenantID, TenantName: r.TenantName, Units: orEmpty(units[r.TenantID]),
			Overdue: r.Overdue, OldestUnpaidDueDate: r.OldestUnpaidDueDate, AgeDays: daysSince(r.OldestUnpaidDueDate, today),
		}
	}
	return out, nil
}

func loadRecentCorrections(ctx context.Context, q *dbgen.Queries, org uuid.UUID, vis visible) ([]DashboardCorrection, error) {
	rows, err := q.ListRecentCorrections(ctx, dbgen.ListRecentCorrectionsParams{
		OrganizationID: org, LeaseIds: vis.leaseIDs, NoLeaseVisible: vis.orgWide, RowLimit: dashboardRows,
	})
	if err != nil {
		return nil, fmt.Errorf("listing recent corrections: %w", err)
	}
	out := make([]DashboardCorrection, len(rows))
	for i, r := range rows {
		c := DashboardCorrection{
			CorrectionRecord: CorrectionRecord{
				ID: r.ID, InvoiceID: r.InvoiceID.UUID, PaymentID: r.PaymentID.UUID, AmountDelta: r.AmountDelta, Reason: r.Reason,
				CreatedBy: StaffRef{MemberID: r.CreatedByMemberID, Name: r.CreatedByName}, CreatedAt: r.CreatedAt,
			},
			TenantID: r.TenantID, TenantName: r.TenantName, Period: r.InvoicePeriod,
		}
		if r.PropertyName != nil && r.UnitLabel != nil {
			c.Unit = &UnitRef{PropertyName: *r.PropertyName, UnitLabel: *r.UnitLabel}
		}
		if r.ReceivedAt != nil {
			on := receivedOn(*r.ReceivedAt)
			c.ReceivedOn = &on
		}
		out[i] = c
	}
	return out, nil
}

func loadInvoicePage(ctx context.Context, q *dbgen.Queries, args dbgen.ListInvoicePageParams) ([]InvoiceListItem, error) {
	rows, err := q.ListInvoicePage(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("listing invoices: %w", err)
	}
	out := make([]InvoiceListItem, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.InvoiceID
	}
	lineRows, err := q.ListLinesOfInvoices(ctx, dbgen.ListLinesOfInvoicesParams{OrganizationID: args.OrganizationID, InvoiceIds: ids})
	if err != nil {
		return nil, fmt.Errorf("listing invoice lines: %w", err)
	}
	lines := map[uuid.UUID][]AccountLine{}
	for _, l := range lineRows {
		lines[l.InvoiceID] = append(lines[l.InvoiceID], AccountLine{ID: l.ID, Kind: l.Kind, Label: l.Label, Amount: l.Amount, MeteredChargeID: l.MeteredChargeID.UUID})
	}
	for i, r := range rows {
		item := InvoiceListItem{
			ID: r.InvoiceID, Kind: r.Kind, TenantID: r.TenantID, TenantName: r.TenantName, LeaseID: r.LeaseID,
			PropertyID: r.PropertyID, PropertyName: r.PropertyName, UnitLabel: r.UnitLabel,
			Period: r.Period, PeriodEnd: r.PeriodEnd, DueDate: r.DueDate, IssuedOn: receivedOn(r.CreatedAt),
			Amount: r.Amount, Billed: r.Billed, Paid: r.Paid, Outstanding: r.Outstanding, Status: r.Status,
			PastPlannedEnd: r.PastPlannedEnd, Lines: orEmpty(lines[r.InvoiceID]),
		}
		if r.Outstanding > 0 && r.DueDate.Before(args.Today) {
			item.OverdueDays = daysSince(r.DueDate, args.Today)
		}
		out[i] = item
	}
	return out, nil
}

func loadInvoiceSummary(ctx context.Context, q *dbgen.Queries, args dbgen.ListInvoicePageParams) (InvoiceSummary, error) {
	sum, err := q.SumMonthInvoices(ctx, dbgen.SumMonthInvoicesParams{
		OrganizationID: args.OrganizationID, LeaseIds: args.LeaseIds, MonthStart: args.MonthStart, MonthEnd: args.MonthEnd,
		PropertyID: args.PropertyID,
	})
	if err != nil {
		return InvoiceSummary{}, fmt.Errorf("summing the month's invoices: %w", err)
	}
	out := InvoiceSummary{Billed: sum.Billed, Paid: sum.Paid, Outstanding: sum.Outstanding, TenantsOutstanding: int(sum.TenantsOutstanding)}
	oldest, err := q.GetOldestOverdue(ctx, dbgen.GetOldestOverdueParams{
		OrganizationID: args.OrganizationID, LeaseIds: args.LeaseIds, Today: args.Today, PropertyID: args.PropertyID,
	})
	if err != nil {
		return InvoiceSummary{}, fmt.Errorf("finding the oldest debt: %w", err)
	}
	if len(oldest) > 0 {
		o := oldest[0]
		out.OldestOverdue = &OldestOverdue{
			TenantID: o.TenantID, TenantName: o.TenantName, PropertyName: o.PropertyName, UnitLabel: o.UnitLabel,
			OldestUnpaidDueDate: o.DueDate, AgeDays: daysSince(o.DueDate, args.Today),
		}
	}
	return out, nil
}

// parseMonth reads a YYYY-MM month as its first day (midnight UTC like a date column); empty is
// today's month. A bad value is an *InputError on period.
func parseMonth(period string, today time.Time) (time.Time, error) {
	if period == "" {
		return time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	}
	m, err := time.Parse("2006-01", period)
	if err != nil || len(period) != len("2006-01") {
		return time.Time{}, &InputError{Field: "period", Message: "Bulan harus berformat TTTT-BB, misalnya 2026-11."}
	}
	return m, nil
}

// daysSince counts the calendar days from day to today, both calendar days as midnight UTC.
func daysSince(day, today time.Time) int {
	return int(calendarDay(today).Sub(calendarDay(day)).Hours() / 24)
}
