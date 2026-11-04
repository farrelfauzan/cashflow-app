package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// ErrMeteredChargeBilled means the month's metered amount is already on an invoice, so it can no
// longer change; a fix is a correction on that invoice.
var ErrMeteredChargeBilled = errors.New("billing: metered amount already billed")

// MeteredInput is one month's metered amount as a member records it. The meter numbers are
// optional and only shown; Amount is what is billed.
type MeteredInput struct {
	Amount     int64
	MeterStart *int64
	MeterEnd   *int64
}

// BilledRef is the invoice whose line bills a metered amount.
type BilledRef struct {
	InvoiceID uuid.UUID
	Kind      string // InvoiceRegular or InvoiceFinal
	Period    time.Time
}

// MeteredCharge is a lease's metered amount for one charge type and usage month.
type MeteredCharge struct {
	ID             uuid.UUID
	LeaseID        uuid.UUID
	ChargeTypeID   uuid.UUID
	ChargeTypeName string
	UsageMonth     time.Time // the first day of the month
	Amount         int64
	MeterStart     *int64
	MeterEnd       *int64
	RecordedBy     StaffRef // who saved it last
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Billed         *BilledRef // nil while no invoice bills it, and it may still change
}

// MeteredQuery filters a lease's metered amounts. Zero values mean every type and every month.
type MeteredQuery struct {
	ChargeTypeID uuid.UUID
	UsageMonth   time.Time // a first day of a month
	Page         listing.Page
}

// MeteredList is one page of a lease's metered amounts, and the invoice the next amount recorded
// would go on (nil when no further invoice will be generated).
type MeteredList struct {
	listing.Result[MeteredCharge]
	NextInvoice *NextInvoice
}

// MissingQuery is what the missing-readings list asks for. Month is a first day of a month,
// zero for last month in Asia/Jakarta.
type MissingQuery struct {
	Month      time.Time
	PropertyID uuid.UUID // uuid.Nil for every property
	Page       listing.Page
}

// MissingReading is an active lease that expects metered amounts for the month and has none
// for some of its types.
type MissingReading struct {
	LeaseID      uuid.UUID
	TenantID     uuid.UUID
	TenantName   string
	PropertyName string
	UnitLabel    string
	Missing      []tenant.MeteredType // by name
	NextInvoice  *NextInvoice         // where an amount recorded now goes; nil when none will come
}

// ValidMetered checks a metered amount: Rp1 to MaxAmount, meter numbers not negative and the end
// not before the start. Errors are *InputError.
func ValidMetered(in MeteredInput) error {
	switch {
	case in.Amount < 1 || in.Amount > MaxAmount:
		return &InputError{Field: "amount", Message: "Jumlah harus antara Rp1 dan Rp1.000.000.000.000."}
	case in.MeterStart != nil && *in.MeterStart < 0:
		return &InputError{Field: "meterStart", Message: "Angka meter awal tidak boleh negatif."}
	case in.MeterEnd != nil && *in.MeterEnd < 0:
		return &InputError{Field: "meterEnd", Message: "Angka meter akhir tidak boleh negatif."}
	case in.MeterStart != nil && in.MeterEnd != nil && *in.MeterEnd < *in.MeterStart:
		return &InputError{Field: "meterEnd", Message: "Angka meter akhir tidak boleh lebih kecil dari angka awal."}
	}
	return nil
}

// CheckUsageMonth checks the usage month of an amount for a lease that started on start: the
// first day of a month, not before the month the lease started and not after today's month
// (Asia/Jakarta). Errors are *InputError on usageMonth.
func CheckUsageMonth(month, start, today time.Time) error {
	switch {
	case month.IsZero() || month.Day() != 1:
		return &InputError{Field: "usageMonth", Message: "Bulan pemakaian harus berformat TTTT-BB, misalnya 2026-10."}
	case month.Before(firstOfMonth(start)):
		return &InputError{Field: "usageMonth", Message: "Bulan pemakaian tidak boleh sebelum bulan mulai sewa."}
	case month.After(firstOfMonth(today)):
		return &InputError{Field: "usageMonth", Message: "Bulan pemakaian tidak boleh setelah bulan ini."}
	}
	return nil
}

// LastMonth is the first day of the month before today's (today is an Asia/Jakarta day).
func LastMonth(today time.Time) time.Time {
	return firstOfMonth(today).AddDate(0, -1, 0)
}

func firstOfMonth(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// RecordMeteredCharge saves a lease's metered amount for one charge type and usage month: a new
// one, or a change to one no invoice bills yet (ErrMeteredChargeBilled once one does). It needs
// charge:record on the lease, in its property and assigned to its collector, with the amount
// (audited). The lease must be active (tenant.ErrLeaseEnded) and the type metered. The lease row
// is locked first, the lock invoice generation takes, so the amount lands either before an
// invoice is made, and is billed on it, or after, and waits for the next one.
func (s *Service) RecordMeteredCharge(ctx context.Context, p authz.Principal, leaseID, chargeTypeID uuid.UUID, usageMonth time.Time, in MeteredInput) (MeteredCharge, error) {
	if err := ValidMetered(in); err != nil {
		return MeteredCharge{}, err
	}
	today := tenant.Today(s.clock)
	org := p.OrganizationID
	var out MeteredCharge
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		l, err := lockLease(ctx, q, org, leaseID)
		if err != nil {
			return err
		}
		amount := in.Amount
		req := authz.Request{Action: authz.ActionChargeRecord, Resource: authz.Resource{
			Type: "lease", ID: leaseID.String(), PropertyID: l.PropertyID, OwnerID: l.CollectorUserID, Amount: &amount,
		}}
		if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
			return err
		}
		if l.Status != tenant.StatusActive {
			return tenant.ErrLeaseEnded
		}
		if err := checkMeteredType(ctx, q, org, chargeTypeID); err != nil {
			return err
		}
		if err := CheckUsageMonth(usageMonth, l.Dates.Start, today); err != nil {
			return err
		}
		id, err := saveMetered(ctx, q, org, p.UserID, leaseID, chargeTypeID, usageMonth, in, s.clock.Now())
		if err != nil {
			return err
		}
		r, err := q.GetMeteredCharge(ctx, dbgen.GetMeteredChargeParams{OrganizationID: org, ID: id})
		if err != nil {
			return fmt.Errorf("loading metered amount: %w", err)
		}
		out = toMeteredCharge(dbgen.ListLeaseMeteredChargePageRow(r))
		return nil
	})
	if err != nil {
		return MeteredCharge{}, err
	}
	return out, nil
}

// checkMeteredType makes sure the charge type exists in the organization and is metered. Errors
// are *InputError on chargeTypeId.
func checkMeteredType(ctx context.Context, q *dbgen.Queries, org, id uuid.UUID) error {
	rows, err := q.GetChargeTypes(ctx, dbgen.GetChargeTypesParams{OrganizationID: org, Ids: []uuid.UUID{id}})
	if err != nil {
		return fmt.Errorf("loading charge type: %w", err)
	}
	switch {
	case len(rows) == 0:
		return &InputError{Field: "chargeTypeId", Message: "Jenis biaya tidak ditemukan."}
	case !rows[0].Metered:
		return &InputError{Field: "chargeTypeId", Message: "Biaya " + rows[0].Name + " bukan biaya meteran."}
	}
	return nil
}

// saveMetered inserts the month's amount, or updates the one stored when no invoice bills it,
// and returns its id. The stored row is locked before it is read.
func saveMetered(ctx context.Context, q *dbgen.Queries, org, recordedBy, leaseID, chargeTypeID uuid.UUID, month time.Time, in MeteredInput, now time.Time) (uuid.UUID, error) {
	cur, err := q.LockMeteredCharge(ctx, dbgen.LockMeteredChargeParams{
		OrganizationID: org, LeaseID: leaseID, ChargeTypeID: chargeTypeID, UsageMonth: month,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		id := uuid.New()
		err := q.CreateMeteredCharge(ctx, dbgen.CreateMeteredChargeParams{
			ID: id, OrganizationID: org, LeaseID: leaseID, ChargeTypeID: chargeTypeID, UsageMonth: month,
			Amount: in.Amount, MeterStart: in.MeterStart, MeterEnd: in.MeterEnd, RecordedByStaffUserID: recordedBy, Now: now,
		})
		if err != nil {
			return uuid.Nil, fmt.Errorf("recording metered amount: %w", err)
		}
		return id, nil
	case err != nil:
		return uuid.Nil, fmt.Errorf("locking metered amount: %w", err)
	case cur.Billed:
		return uuid.Nil, ErrMeteredChargeBilled
	}
	n, err := q.UpdateMeteredCharge(ctx, dbgen.UpdateMeteredChargeParams{
		OrganizationID: org, ID: cur.ID, Amount: in.Amount, MeterStart: in.MeterStart, MeterEnd: in.MeterEnd,
		RecordedByStaffUserID: recordedBy, Now: now,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("changing metered amount: %w", err)
	}
	if n != 1 {
		return uuid.Nil, fmt.Errorf("changing metered amount %s: %d rows", cur.ID, n)
	}
	return cur.ID, nil
}

// ListLeaseMeteredCharges returns one page of a lease's metered amounts, the newest month first,
// then by type name, with the invoice the next amount recorded would go on. It needs charge:read
// on the lease, in its property and assigned to its collector; an ended lease is readable.
func (s *Service) ListLeaseMeteredCharges(ctx context.Context, p authz.Principal, leaseID uuid.UUID, mq MeteredQuery) (MeteredList, error) {
	if mq.Page == (listing.Page{}) {
		mq.Page = listing.FirstPage
	}
	today := tenant.Today(s.clock)
	org := p.OrganizationID
	out := MeteredList{Result: listing.Result[MeteredCharge]{Page: mq.Page, Items: []MeteredCharge{}}}
	// One snapshot, so the total, the rows and the next invoice agree.
	err := s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		l, err := q.GetLeaseDetail(ctx, dbgen.GetLeaseDetailParams{OrganizationID: org, ID: leaseID})
		if errors.Is(err, pgx.ErrNoRows) {
			return &NotFoundError{}
		}
		if err != nil {
			return fmt.Errorf("loading lease: %w", err)
		}
		req := authz.Request{Action: authz.ActionChargeRead, Resource: authz.Resource{
			Type: "lease", ID: leaseID.String(), PropertyID: l.PropertyID, OwnerID: l.CollectorStaffUserID.UUID,
		}}
		if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
			return err
		}
		args := dbgen.ListLeaseMeteredChargePageParams{
			OrganizationID: org, LeaseID: leaseID,
			ChargeTypeID: uuid.NullUUID{UUID: mq.ChargeTypeID, Valid: mq.ChargeTypeID != uuid.Nil},
			PageLimit:    mq.Page.Limit(), PageOffset: mq.Page.Offset(),
		}
		if !mq.UsageMonth.IsZero() {
			args.UsageMonth = &mq.UsageMonth
		}
		if out.Total, err = q.CountLeaseMeteredCharges(ctx, dbgen.CountLeaseMeteredChargesParams{
			OrganizationID: org, LeaseID: leaseID, ChargeTypeID: args.ChargeTypeID, UsageMonth: args.UsageMonth,
		}); err != nil {
			return fmt.Errorf("counting metered amounts: %w", err)
		}
		rows, err := q.ListLeaseMeteredChargePage(ctx, args)
		if err != nil {
			return fmt.Errorf("listing metered amounts: %w", err)
		}
		for _, r := range rows {
			out.Items = append(out.Items, toMeteredCharge(r))
		}
		if l.Status != tenant.StatusActive {
			return nil
		}
		next, err := nextInvoices(ctx, q, org, today, map[uuid.UUID]LeaseDates{leaseID: leaseDatesOf(l)})
		out.NextInvoice = next[leaseID]
		return err
	})
	if err != nil {
		return MeteredList{}, err
	}
	return out, nil
}

// ListMissingMeterReadings returns one page of the active leases that expect a metered amount for
// the month and have none for some of their types ("meteran belum dicatat"), by tenant name, then
// unit. A lease counts once it has started by the month's last day. It needs charge:read; a
// member who may read amounts only in some properties, or only of their own tenants, sees only
// those leases.
func (s *Service) ListMissingMeterReadings(ctx context.Context, p authz.Principal, mq MissingQuery) (listing.Result[MissingReading], error) {
	today := tenant.Today(s.clock)
	if mq.Month.IsZero() {
		mq.Month = LastMonth(today)
	}
	if mq.Month.Day() != 1 {
		return listing.Result[MissingReading]{}, &InputError{Field: "month", Message: "Bulan harus berformat TTTT-BB, misalnya 2026-10."}
	}
	if mq.Month.After(firstOfMonth(today)) {
		return listing.Result[MissingReading]{}, &InputError{Field: "month", Message: "Bulan tidak boleh setelah bulan ini."}
	}
	if mq.Page == (listing.Page{}) {
		mq.Page = listing.FirstPage
	}
	org := p.OrganizationID
	out := listing.Result[MissingReading]{Page: mq.Page, Items: []MissingReading{}}
	// One snapshot, so the total, the rows and the next invoice agree.
	err := s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		vis, err := s.visibleLeases(ctx, tx, q, p, authz.ActionChargeRead, "lease")
		if err != nil {
			return err
		}
		args := dbgen.ListMissingMeterLeasePageParams{
			OrganizationID: org, LeaseIds: vis.leaseIDs, MonthLast: mq.Month.AddDate(0, 1, -1),
			PropertyID: uuid.NullUUID{UUID: mq.PropertyID, Valid: mq.PropertyID != uuid.Nil},
			UsageMonth: mq.Month, PageLimit: mq.Page.Limit(), PageOffset: mq.Page.Offset(),
		}
		if out.Total, err = q.CountMissingMeterLeases(ctx, dbgen.CountMissingMeterLeasesParams{
			OrganizationID: org, LeaseIds: args.LeaseIds, MonthLast: args.MonthLast, PropertyID: args.PropertyID, UsageMonth: args.UsageMonth,
		}); err != nil {
			return fmt.Errorf("counting missing meter readings: %w", err)
		}
		out.Items, err = loadMissingPage(ctx, q, args, today)
		return err
	})
	if err != nil {
		return listing.Result[MissingReading]{}, err
	}
	return out, nil
}

func loadMissingPage(ctx context.Context, q *dbgen.Queries, args dbgen.ListMissingMeterLeasePageParams, today time.Time) ([]MissingReading, error) {
	rows, err := q.ListMissingMeterLeasePage(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("listing missing meter readings: %w", err)
	}
	out := make([]MissingReading, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(rows))
	dates := make(map[uuid.UUID]LeaseDates, len(rows))
	for i, r := range rows {
		ids[i] = r.LeaseID
		dates[r.LeaseID] = LeaseDates{
			Start: r.StartDate, EnteredOn: r.EnteredOn, BilledFrom: r.BilledFrom, TermMonths: int(r.TermMonths),
			PlannedEnd: r.PlannedEndDate, End: r.EndDate,
		}
	}
	typeRows, err := q.ListMissingMeterTypes(ctx, dbgen.ListMissingMeterTypesParams{
		OrganizationID: args.OrganizationID, LeaseIds: ids, UsageMonth: args.UsageMonth,
	})
	if err != nil {
		return nil, fmt.Errorf("listing missing meter types: %w", err)
	}
	missing := map[uuid.UUID][]tenant.MeteredType{}
	for _, t := range typeRows {
		missing[t.LeaseID] = append(missing[t.LeaseID], tenant.MeteredType{ChargeTypeID: t.ChargeTypeID, Name: t.ChargeTypeName})
	}
	next, err := nextInvoices(ctx, q, args.OrganizationID, today, dates)
	if err != nil {
		return nil, err
	}
	for i, r := range rows {
		out[i] = MissingReading{
			LeaseID: r.LeaseID, TenantID: r.TenantID, TenantName: r.TenantName, PropertyName: r.PropertyName,
			UnitLabel: r.UnitLabel, Missing: orEmpty(missing[r.LeaseID]), NextInvoice: next[r.LeaseID],
		}
	}
	return out, nil
}

// nextInvoices returns the next invoice of each lease (see NextInvoiceOf), with one query for
// their invoices; a lease with none to come is left out.
func nextInvoices(ctx context.Context, q *dbgen.Queries, org uuid.UUID, today time.Time, leases map[uuid.UUID]LeaseDates) (map[uuid.UUID]*NextInvoice, error) {
	ids := make([]uuid.UUID, 0, len(leases))
	for id := range leases {
		ids = append(ids, id)
	}
	rows, err := q.ListInvoicePeriodsOfLeases(ctx, dbgen.ListInvoicePeriodsOfLeasesParams{OrganizationID: org, LeaseIds: ids})
	if err != nil {
		return nil, fmt.Errorf("loading the leases' invoices: %w", err)
	}
	invoiced := map[uuid.UUID][]time.Time{}
	for _, r := range rows {
		invoiced[r.LeaseID] = append(invoiced[r.LeaseID], r.Period)
	}
	out := make(map[uuid.UUID]*NextInvoice, len(leases))
	for id, dates := range leases {
		if next, ok := NextInvoiceOf(dates, today, invoiced[id]); ok {
			out[id] = &next
		}
	}
	return out, nil
}

func leaseDatesOf(l dbgen.GetLeaseDetailRow) LeaseDates {
	return LeaseDates{
		Start: l.StartDate, EnteredOn: l.EnteredOn, BilledFrom: l.BilledFrom, TermMonths: int(l.TermMonths),
		PlannedEnd: l.PlannedEndDate, End: l.EndDate,
	}
}

func toMeteredCharge(r dbgen.ListLeaseMeteredChargePageRow) MeteredCharge {
	out := MeteredCharge{
		ID: r.ID, LeaseID: r.LeaseID, ChargeTypeID: r.ChargeTypeID, ChargeTypeName: r.ChargeTypeName,
		UsageMonth: r.UsageMonth, Amount: r.Amount, MeterStart: r.MeterStart, MeterEnd: r.MeterEnd,
		RecordedBy: StaffRef{MemberID: r.RecordedByMemberID, Name: r.RecordedByName},
		CreatedAt:  r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.BilledInvoiceID.Valid && r.BilledPeriod != nil && r.BilledKind != nil {
		out.Billed = &BilledRef{InvoiceID: r.BilledInvoiceID.UUID, Kind: *r.BilledKind, Period: *r.BilledPeriod}
	}
	return out
}
