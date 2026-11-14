package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// FieldDueWithinDays is how far ahead "Tagihan hari ini" looks: a tenant is on it when they owe on
// an invoice that is overdue, due today, or due within this many days (decision 46.9).
const FieldDueWithinDays = 2

// Reasons a tenant is on "Tagihan hari ini": the first three from the oldest invoice they still
// owe on, the last for a tenant who owes nothing due in the window but paid the member cash today.
const (
	FieldOverdue   = "overdue"     // its due date has passed (debt from H+1)
	FieldDueToday  = "due_today"   // it is due today
	FieldDueInDays = "due_in_days" // it is due within FieldDueWithinDays days
	FieldPaidToday = "paid_today"  // nothing due in the window; the member recorded cash from them today
)

// FieldReason says why a tenant is on "Tagihan hari ini".
type FieldReason struct {
	Code string // FieldOverdue, FieldDueToday, FieldDueInDays or FieldPaidToday
	// Days is how many days overdue (1 on the day after the due date) for FieldOverdue, how many
	// days until it is due (1 or 2) for FieldDueInDays, and 0 for FieldDueToday and FieldPaidToday.
	Days int
}

// FieldReasonAt is the reason for a tenant whose oldest unpaid invoice is due on due (a calendar
// day, midnight UTC like a date column), at the moment now: the day is the Asia/Jakarta day of
// now. ok is false when the invoice is due later than FieldDueWithinDays days ahead.
func FieldReasonAt(due, now time.Time) (FieldReason, bool) {
	today := tenant.DayOf(now)
	days := daysBetween(today, calendarDay(due))
	switch {
	case days < 0:
		return FieldReason{Code: FieldOverdue, Days: -days}, true
	case days == 0:
		return FieldReason{Code: FieldDueToday}, true
	case days <= FieldDueWithinDays:
		return FieldReason{Code: FieldDueInDays, Days: days}, true
	default:
		return FieldReason{}, false
	}
}

// daysBetween counts the calendar days from a to b, both midnight UTC: negative when b is before a.
func daysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours() / 24)
}

// FieldUnit is a unit of a tenant on "Tagihan hari ini", with its property's address for
// directions.
type FieldUnit struct {
	UnitLabel       string
	PropertyName    string
	PropertyAddress string // empty when the property has none
}

// CashTotal is an amount of cash and how many cash payments make it up.
type CashTotal struct {
	Amount int64
	Count  int
}

// FieldCashPayment is one cash payment a member recorded from a tenant today.
type FieldCashPayment struct {
	PaymentID  uuid.UUID
	Amount     int64 // net of its corrections
	RecordedAt time.Time
}

// FieldCash is the cash a member recorded from one tenant today: the total, how many payments,
// and each payment, oldest first.
type FieldCash struct {
	CashTotal
	Payments []FieldCashPayment
}

// FieldTenant is one row of "Tagihan hari ini".
type FieldTenant struct {
	TenantID   uuid.UUID
	TenantName string
	// Units are those of the tenant's active leases the member collects, by property then unit.
	Units  []FieldUnit
	Reason FieldReason
	// OldestUnpaidDueDate is the due date of the oldest invoice the tenant still owes on; nil when
	// they owe nothing (a FieldPaidToday row may).
	OldestUnpaidDueDate *time.Time
	// Outstanding is everything the tenant owes now, over all their invoices; Overdue the part of
	// it that is debt today.
	Outstanding int64
	Overdue     int64
	// CashToday is the cash the member recorded from the tenant today (Asia/Jakarta), net of any
	// correction.
	CashToday FieldCash
}

// FieldToday is "Tagihan hari ini" for member p, of the tenants p collects from on an active lease:
// first those who owe on an invoice that is overdue, due today or due within FieldDueWithinDays
// days (Asia/Jakarta, the injected clock), oldest unpaid due date first, with that reason even when
// they also paid cash today; then the others from whom p recorded cash today (FieldPaidToday,
// "Tunai diterima"), by name, so a tenant stays on the list for the rest of the day after paying
// everything. The tenant id breaks ties.
//
// It needs invoice:read. The collection is authorized like the dashboard's (a member with no
// access at all is denied and audited); then a tenant is listed only when p may read invoices
// through every lease the tenant has or had, in the lease's property and assigned to its
// collector, since the amount owed spans all of them. A member with the organization-wide grant
// (an admin) still sees only the tenants they collect from: this is the member's own field list.
// Total counts only the rows p may see.
func (s *Service) FieldToday(ctx context.Context, p authz.Principal, page listing.Page) (listing.Result[FieldTenant], error) {
	org := p.OrganizationID
	now := s.clock.Now()
	today := tenant.DayOf(now)
	dayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, clock.Jakarta)
	out := listing.Result[FieldTenant]{Items: []FieldTenant{}, Page: page}
	// One snapshot, so the total, the rows and their units agree.
	err := s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		vis, err := s.visibleLeases(ctx, tx, q, p, authz.ActionInvoiceRead, "invoice")
		if err != nil {
			return err
		}
		collector := uuid.NullUUID{UUID: p.UserID, Valid: true}
		until := today.AddDate(0, 0, FieldDueWithinDays)
		total, err := q.CountFieldToday(ctx, dbgen.CountFieldTodayParams{
			OrganizationID: org, Today: today, Until: until, CollectorID: collector, LeaseIds: vis.leaseIDs,
			DayStart: dayStart, DayEnd: dayStart.AddDate(0, 0, 1),
		})
		if err != nil {
			return fmt.Errorf("counting today's field list: %w", err)
		}
		rows, err := q.ListFieldToday(ctx, dbgen.ListFieldTodayParams{
			OrganizationID: org, Today: today, Until: until, CollectorID: collector, LeaseIds: vis.leaseIDs,
			DayStart: dayStart, DayEnd: dayStart.AddDate(0, 0, 1),
			PageLimit: page.Limit(), PageOffset: page.Offset(),
		})
		if err != nil {
			return fmt.Errorf("listing today's field list: %w", err)
		}
		items, err := fieldTenants(ctx, q, org, collector, rows, now, dayStart)
		if err != nil {
			return err
		}
		out.Items, out.Total = items, total
		return nil
	})
	if err != nil {
		return listing.Result[FieldTenant]{}, err
	}
	return out, nil
}

// fieldTenants turns a page of ListFieldToday into rows with their units and today's cash payments.
func fieldTenants(ctx context.Context, q *dbgen.Queries, org uuid.UUID, collector uuid.NullUUID, rows []dbgen.ListFieldTodayRow, now, dayStart time.Time) ([]FieldTenant, error) {
	out := make([]FieldTenant, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.TenantID
	}
	unitRows, err := q.ListFieldTodayUnits(ctx, dbgen.ListFieldTodayUnitsParams{OrganizationID: org, TenantIds: ids, CollectorID: collector})
	if err != nil {
		return nil, fmt.Errorf("listing the units of today's field list: %w", err)
	}
	units := map[uuid.UUID][]FieldUnit{}
	for _, u := range unitRows {
		units[u.TenantID] = append(units[u.TenantID], FieldUnit{UnitLabel: u.UnitLabel, PropertyName: u.PropertyName, PropertyAddress: u.PropertyAddress})
	}
	cashRows, err := q.ListFieldCashToday(ctx, dbgen.ListFieldCashTodayParams{
		OrganizationID: org, TenantIds: ids, CollectorID: collector, DayStart: dayStart, DayEnd: dayStart.AddDate(0, 0, 1),
	})
	if err != nil {
		return nil, fmt.Errorf("listing today's cash of the field list: %w", err)
	}
	cash := map[uuid.UUID][]FieldCashPayment{}
	for _, c := range cashRows {
		cash[c.TenantID] = append(cash[c.TenantID], FieldCashPayment{PaymentID: c.ID, Amount: c.Net, RecordedAt: c.CreatedAt})
	}
	for _, r := range rows {
		item, err := fieldTenant(r, now)
		if err != nil {
			return nil, err
		}
		item.Units = units[r.TenantID]
		if item.Units == nil {
			item.Units = []FieldUnit{}
		}
		item.CashToday.Payments = cash[r.TenantID]
		if item.CashToday.Payments == nil {
			item.CashToday.Payments = []FieldCashPayment{}
		}
		out = append(out, item)
	}
	return out, nil
}

// fieldTenant is one row of ListFieldToday with its reason, without its units and payments.
func fieldTenant(r dbgen.ListFieldTodayRow, now time.Time) (FieldTenant, error) {
	item := FieldTenant{
		TenantID: r.TenantID, TenantName: r.TenantName, Outstanding: r.Outstanding, Overdue: r.Overdue,
		CashToday: FieldCash{CashTotal: CashTotal{Amount: r.CashToday, Count: int(r.CashTodayCount)}},
	}
	if r.Outstanding > 0 {
		// The query reads 0001-01-01 for a tenant who owes nothing; this one owes.
		due := r.OldestUnpaidDueDate
		item.OldestUnpaidDueDate = &due
	}
	if !r.Due {
		item.Reason = FieldReason{Code: FieldPaidToday}
		return item, nil
	}
	reason, ok := FieldReasonAt(r.OldestUnpaidDueDate, now)
	if !ok {
		return FieldTenant{}, fmt.Errorf("tenant %s on today's field list is due after the window", r.TenantID)
	}
	item.Reason = reason
	return item, nil
}

// FieldCash is the cash member p holds: every cash payment p recorded, net of its corrections,
// and how many. Cash deposits (S6-3) will leave out what a deposit already covers.
//
// It is p's own cash, so it is checked as cash:deposit on a record owned by p (the karyawan's
// "self"); the check is not audited when allowed, as it reads and changes nothing, and a denial is
// audited.
func (s *Service) FieldCash(ctx context.Context, p authz.Principal) (CashTotal, error) {
	req := authz.Request{Action: authz.ActionCashDeposit, Resource: authz.Resource{
		Type: "cash", ID: p.UserID.String(), OwnerID: p.UserID,
	}}
	if err := s.authz.Check(ctx, p, req); err != nil {
		return CashTotal{}, err
	}
	var out CashTotal
	err := s.store.WithOrgSnapshot(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetFieldCash(ctx, dbgen.GetFieldCashParams{
			OrganizationID: p.OrganizationID, StaffUserID: uuid.NullUUID{UUID: p.UserID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("reading the member's cash: %w", err)
		}
		out = CashTotal{Amount: row.Amount, Count: int(row.Payments)}
		return nil
	})
	if err != nil {
		return CashTotal{}, err
	}
	return out, nil
}
