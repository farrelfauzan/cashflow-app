package tenant

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// Lease statuses.
const (
	StatusActive = "active"
	StatusEnded  = "ended"
)

var (
	// ErrUnitOccupied means the unit already has an active lease.
	ErrUnitOccupied = errors.New("tenant: unit already has an active lease")
	// ErrLeaseEnded means the lease has ended and can no longer change.
	ErrLeaseEnded = errors.New("tenant: lease has ended")
)

// Lease is a tenant renting a unit. Dates are calendar days (midnight UTC, no time zone
// meaning); amounts are rupiah.
type Lease struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	UnitID       uuid.UUID
	UnitLabel    string
	PropertyID   uuid.UUID
	PropertyName string
	// StartDate is the first day; rent is due on its day of the month once per term (decisions
	// 24 and 32).
	StartDate time.Time
	EndDate   *time.Time // last day, set by move-out
	// EnteredOn is the Asia/Jakarta day the lease was saved. A start date before it means the
	// lease was entered late.
	EnteredOn time.Time
	// BilledFrom ("Mulai tagih dari") is the first period the app invoices, a period start of
	// the lease's term.
	BilledFrom time.Time
	// PlannedEndDate ("Rencana keluar") is the tenant's planned last day, or nil.
	PlannedEndDate *time.Time
	// TermMonths is the billing term, fixed when the lease is created (decision 32).
	TermMonths int
	// PeriodRent is the unit's price for the lease's term in force today, for display, or nil
	// when none is in force. Each invoice takes the price in force on the day it is made.
	PeriodRent    *int64
	DepositAmount int64
	Status        string
	// Collector is the member who collects from this tenant, if any. A collector who is
	// disabled stays on the lease, with CollectorActive false, until an admin picks another.
	CollectorMemberID uuid.UUID
	CollectorUserID   uuid.UUID
	CollectorName     string
	CollectorActive   bool
	// Charges are the fixed monthly extras on top of rent, by name; never nil once loaded.
	Charges   []Charge
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Terms are the editable terms of a lease.
type Terms struct {
	StartDate     time.Time
	DepositAmount int64
	// CollectorMemberID is a member id (not a staff user id); uuid.Nil means no collector.
	CollectorMemberID uuid.UUID
	// BilledFrom ("Mulai tagih dari") skips the periods the owner collected by hand. The zero
	// time means the start date for a new lease and the stored value for an update.
	BilledFrom time.Time
}

// NewLease is the input for renting a unit to a tenant.
type NewLease struct {
	TenantID uuid.UUID
	UnitID   uuid.UUID
	// TermMonths is one of the terms the unit offers, with a price in force today. It never
	// changes: another term is a new lease.
	TermMonths int
	Terms
}

// CreateLease rents a unit to a tenant. It needs lease:manage on the unit's property, the unit
// must not have an active lease, and it must offer the term with a price in force today
// (otherwise a *TermError on termMonths with ErrTermNotOffered or ErrNoPrice).
func (s *Service) CreateLease(ctx context.Context, p authz.Principal, in NewLease) (Lease, error) {
	var out Lease
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		var err error
		out, err = s.CreateLeaseTx(ctx, tx, p, in)
		return err
	})
	return out, err
}

// CreateLeaseTx is CreateLease inside the caller's organization transaction.
func (s *Service) CreateLeaseTx(ctx context.Context, tx pgx.Tx, p authz.Principal, in NewLease) (Lease, error) {
	if !ValidTerm(in.TermMonths) {
		return Lease{}, &InputError{Field: "termMonths", Message: termMessage}
	}
	terms, err := ValidTerms(in.Terms, in.TermMonths)
	if err != nil {
		return Lease{}, err
	}
	q := dbgen.New(tx)
	if _, err := q.GetTenant(ctx, dbgen.GetTenantParams{OrganizationID: p.OrganizationID, ID: in.TenantID}); err != nil {
		return Lease{}, lookupError(err, "tenantId", "Penyewa tidak ditemukan.")
	}
	unit, err := q.GetUnit(ctx, dbgen.GetUnitParams{OrganizationID: p.OrganizationID, ID: in.UnitID})
	if err != nil {
		return Lease{}, lookupError(err, "unitId", "Unit tidak ditemukan.")
	}
	enteredOn := Today(s.clock)
	if terms.BilledFrom.IsZero() {
		terms.BilledFrom = terms.StartDate
	}
	if err := checkBilledFrom(terms, in.TermMonths, enteredOn); err != nil {
		return Lease{}, err
	}
	collector, err := collectorUser(ctx, q, p.OrganizationID, terms.CollectorMemberID)
	if err != nil {
		return Lease{}, err
	}
	id := uuid.New()
	req := authz.Request{Action: authz.ActionLeaseManage, Resource: authz.Resource{
		Type: "lease", ID: id.String(), PropertyID: unit.PropertyID, OwnerID: collector.UUID,
	}}
	if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
		return Lease{}, err
	}
	if !slices.Contains(unit.Terms, int16(in.TermMonths)) { //nolint:gosec // a valid term is 1 to 12
		return Lease{}, &TermError{Err: ErrTermNotOffered, Field: "termMonths", Months: in.TermMonths}
	}
	if _, err := PriceInForce(ctx, q, p.OrganizationID, in.UnitID, in.TermMonths, enteredOn); err != nil {
		if errors.Is(err, ErrNoPrice) {
			return Lease{}, &TermError{Err: ErrNoPrice, Field: "termMonths", Months: in.TermMonths}
		}
		return Lease{}, err
	}
	err = q.CreateLease(ctx, dbgen.CreateLeaseParams{
		ID: id, OrganizationID: p.OrganizationID, UnitID: in.UnitID, TenantID: in.TenantID,
		StartDate: terms.StartDate, TermMonths: int16(in.TermMonths), //nolint:gosec // a valid term is 1 to 12
		DepositAmount: terms.DepositAmount, CollectorStaffUserID: collector,
		EnteredOn: enteredOn, BilledFrom: terms.BilledFrom,
	})
	if isUniqueViolation(err, "lease_one_active_per_unit") {
		return Lease{}, ErrUnitOccupied
	}
	if err != nil {
		return Lease{}, fmt.Errorf("creating lease: %w", err)
	}
	return loadLease(ctx, q, p.OrganizationID, id, enteredOn)
}

// ActiveLeaseForUnit returns the unit's active lease, or ErrNotFound when the unit is free. It
// does no permission check. day is the day its PeriodRent is in force on.
func ActiveLeaseForUnit(ctx context.Context, tx pgx.Tx, org, unitID uuid.UUID, day time.Time) (Lease, error) {
	q := dbgen.New(tx)
	r, err := q.GetActiveLeaseForUnit(ctx, dbgen.GetActiveLeaseForUnitParams{OrganizationID: org, UnitID: unitID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Lease{}, ErrNotFound
	}
	if err != nil {
		return Lease{}, fmt.Errorf("finding active lease: %w", err)
	}
	return loadLease(ctx, q, org, r.ID, day)
}

// UpdateLease changes the terms of an active lease. Each change is audited. The billing term
// never changes. Terms without a BilledFrom keep the stored one, so an edit of another
// term never undoes a skipped billing start; when the start date changes and the kept value no
// longer fits it, the update is refused on billedFrom. Terms that name the lease's current
// collector keep them even when they have been disabled, so the lease stays editable until an
// admin picks another; any other collector must be active. Slice 2 adds the rule that the start
// date and BilledFrom lock once the lease has an invoice.
func (s *Service) UpdateLease(ctx context.Context, p authz.Principal, id uuid.UUID, t Terms) (Lease, error) {
	terms, err := ValidTerms(t, 0) // the term is the lease's own, checked once it is loaded
	if err != nil {
		return Lease{}, err
	}
	var out Lease
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		current, err := s.loadActiveForChange(ctx, tx, q, p, id)
		if err != nil {
			return err
		}
		if terms.BilledFrom.IsZero() {
			terms.BilledFrom = current.BilledFrom
		}
		if err := checkBilledFrom(terms, current.TermMonths, current.EnteredOn); err != nil {
			return err
		}
		if current.PlannedEndDate != nil && current.PlannedEndDate.Before(terms.StartDate) {
			return &InputError{Field: "startDate", Message: "Tanggal mulai tidak boleh setelah rencana keluar. Ubah rencana keluar dulu."}
		}
		// The current collector is kept even when disabled; only a new one must be active.
		collector := uuid.NullUUID{UUID: current.CollectorUserID, Valid: current.CollectorUserID != uuid.Nil}
		if terms.CollectorMemberID != current.CollectorMemberID {
			if collector, err = collectorUser(ctx, q, p.OrganizationID, terms.CollectorMemberID); err != nil {
				return err
			}
		}
		n, err := q.UpdateLeaseTerms(ctx, dbgen.UpdateLeaseTermsParams{
			OrganizationID: p.OrganizationID, ID: id, StartDate: terms.StartDate,
			DepositAmount: terms.DepositAmount, CollectorStaffUserID: collector, BilledFrom: terms.BilledFrom,
		})
		if err != nil {
			return fmt.Errorf("updating lease: %w", err)
		}
		if n == 0 {
			return ErrLeaseEnded // the row lock makes this unreachable; kept as a guard
		}
		out, err = loadLease(ctx, q, p.OrganizationID, id, Today(s.clock))
		return err
	})
	return out, err
}

// SetPlannedEnd sets, changes or clears (nil) an active lease's planned end date ("Rencana
// keluar", decision 25): the tenant's last day, on or after today in Asia/Jakarta and not before
// the start date. It needs lease:manage, which is audited. Slice 2 adds what the date triggers:
// cancelling invoices for periods that start after it, which also needs correction:create.
func (s *Service) SetPlannedEnd(ctx context.Context, p authz.Principal, id uuid.UUID, date *time.Time) (Lease, error) {
	if date != nil {
		d := calendarDay(*date)
		date = &d
	}
	var out Lease
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		current, err := s.loadActiveForChange(ctx, tx, q, p, id)
		if err != nil {
			return err
		}
		if date != nil {
			if err := checkPlannedEnd(current.StartDate, *date, Today(s.clock)); err != nil {
				return err
			}
		}
		n, err := q.SetLeasePlannedEnd(ctx, dbgen.SetLeasePlannedEndParams{OrganizationID: p.OrganizationID, ID: id, PlannedEndDate: date})
		if err != nil {
			return fmt.Errorf("setting planned end date: %w", err)
		}
		if n == 0 {
			return ErrLeaseEnded // the row lock makes this unreachable; kept as a guard
		}
		out, err = loadLease(ctx, q, p.OrganizationID, id, Today(s.clock))
		return err
	})
	return out, err
}

// checkPlannedEnd applies the planned end date rules.
func checkPlannedEnd(start, end, now time.Time) error {
	switch {
	case end.Before(start):
		return &InputError{Field: "plannedEndDate", Message: "Rencana keluar tidak boleh sebelum tanggal mulai sewa."}
	case end.Before(now):
		return &InputError{Field: "plannedEndDate", Message: "Rencana keluar tidak boleh sebelum hari ini."}
	}
	return nil
}

// checkBilledFrom checks that "Mulai tagih dari" is a period start of the lease's term, and no
// later than the first period start after the later of the start date and the day the lease was
// entered, so an admin can skip at most the periods up to the current one.
func checkBilledFrom(t Terms, term int, enteredOn time.Time) error {
	if err := checkPeriodStart(t, term); err != nil {
		return err
	}
	from := t.StartDate
	if enteredOn.After(from) {
		from = enteredOn
	}
	if latest := NextPeriodStart(t.StartDate, term, from); t.BilledFrom.After(latest) {
		return &InputError{Field: "billedFrom", Message: "Mulai tagih paling lambat " + latest.Format("02/01/2006") + "."}
	}
	return nil
}

// MoveOut ends an active lease. endDate is the last day: not before the start and not after
// today in Asia/Jakarta. A lease that has not started (a booking) is cancelled by ending it on
// its start date. The unit is free for a new lease right away, and the planned end date is
// cleared. What happens to arrears is an organization setting applied in slice 2, when invoices
// exist.
func (s *Service) MoveOut(ctx context.Context, p authz.Principal, id uuid.UUID, endDate time.Time) (Lease, error) {
	endDate = calendarDay(endDate)
	var out Lease
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		current, err := s.loadActiveForChange(ctx, tx, q, p, id)
		if err != nil {
			return err
		}
		if err := checkEndDate(current.StartDate, endDate, Today(s.clock)); err != nil {
			return err
		}
		n, err := q.EndLease(ctx, dbgen.EndLeaseParams{OrganizationID: p.OrganizationID, ID: id, EndDate: &endDate})
		if err != nil {
			return fmt.Errorf("ending lease: %w", err)
		}
		if n == 0 {
			return ErrLeaseEnded // the row lock makes this unreachable; kept as a guard
		}
		out, err = loadLease(ctx, q, p.OrganizationID, id, Today(s.clock))
		return err
	})
	return out, err
}

// checkEndDate applies the move-out date rules. A booking that has not started can only be
// cancelled, which ends it on its start date; slice 2 never invoices such a lease.
func checkEndDate(start, end, now time.Time) error {
	if start.After(now) {
		if !end.Equal(start) {
			return &InputError{Field: "endDate", Message: "Sewa ini belum mulai. Untuk membatalkannya, isi tanggal keluar sama dengan tanggal mulai sewa."}
		}
		return nil
	}
	switch {
	case end.Before(start):
		return &InputError{Field: "endDate", Message: "Tanggal keluar tidak boleh sebelum tanggal mulai sewa."}
	case end.After(now):
		return &InputError{Field: "endDate", Message: "Tanggal keluar tidak boleh setelah hari ini."}
	}
	return nil
}

// loadActiveForChange locks a lease row for the rest of the transaction, loads it, authorizes
// lease:manage on it and checks that it is still active. The lock makes a concurrent change or
// move-out wait, so it sees the committed status rather than the one before.
func (s *Service) loadActiveForChange(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, p authz.Principal, id uuid.UUID) (Lease, error) {
	_, err := q.LockLease(ctx, dbgen.LockLeaseParams{OrganizationID: p.OrganizationID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Lease{}, ErrNotFound
	}
	if err != nil {
		return Lease{}, fmt.Errorf("locking lease: %w", err)
	}
	current, err := loadLease(ctx, q, p.OrganizationID, id, Today(s.clock))
	if err != nil {
		return Lease{}, err
	}
	req := authz.Request{Action: authz.ActionLeaseManage, Resource: authz.Resource{
		Type: "lease", ID: id.String(), PropertyID: current.PropertyID, OwnerID: current.CollectorUserID,
	}}
	if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
		return Lease{}, err
	}
	if current.Status != StatusActive {
		return Lease{}, ErrLeaseEnded
	}
	return current, nil
}

// collectorUser turns a member id into the staff user the lease stores. The member must be
// active in this organization.
func collectorUser(ctx context.Context, q *dbgen.Queries, org, memberID uuid.UUID) (uuid.NullUUID, error) {
	if memberID == uuid.Nil {
		return uuid.NullUUID{}, nil
	}
	m, err := q.GetMembership(ctx, dbgen.GetMembershipParams{OrganizationID: org, ID: memberID})
	if err != nil {
		return uuid.NullUUID{}, lookupError(err, "collectorMemberId", "Penagih tidak ditemukan.")
	}
	if m.Status != "active" {
		return uuid.NullUUID{}, &InputError{Field: "collectorMemberId", Message: "Penagih sudah tidak aktif."}
	}
	return uuid.NullUUID{UUID: m.StaffUserID, Valid: true}, nil
}

// loadLease loads one lease with its extras and its PeriodRent in force on day.
func loadLease(ctx context.Context, q *dbgen.Queries, org, id uuid.UUID, day time.Time) (Lease, error) {
	r, err := q.GetLeaseDetail(ctx, dbgen.GetLeaseDetailParams{OrganizationID: org, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Lease{}, ErrNotFound
	}
	if err != nil {
		return Lease{}, fmt.Errorf("loading lease: %w", err)
	}
	out := []Lease{toLease(r)}
	if err := attachDetails(ctx, q, org, day, out); err != nil {
		return Lease{}, err
	}
	return out[0], nil
}

// attachDetails fills in each lease's extras and PeriodRent in force on day, with one query
// each for all of them.
func attachDetails(ctx context.Context, q *dbgen.Queries, org uuid.UUID, day time.Time, leases []Lease) error {
	if err := attachCharges(ctx, q, org, leases); err != nil {
		return err
	}
	return attachPeriodRents(ctx, q, org, day, leases)
}

// lookupError turns a missing referenced record into an input error on the field that named it.
func lookupError(err error, field, message string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &InputError{Field: field, Message: message}
	}
	return fmt.Errorf("loading %s: %w", field, err)
}

func toLease(r dbgen.GetLeaseDetailRow) Lease {
	l := Lease{
		ID: r.ID, TenantID: r.TenantID, UnitID: r.UnitID, UnitLabel: r.UnitLabel,
		PropertyID: r.PropertyID, PropertyName: r.PropertyName,
		StartDate: r.StartDate, EndDate: r.EndDate, EnteredOn: r.EnteredOn, BilledFrom: r.BilledFrom,
		PlannedEndDate: r.PlannedEndDate, TermMonths: int(r.TermMonths),
		DepositAmount: r.DepositAmount, Status: r.Status, Charges: []Charge{},
		CollectorMemberID: r.CollectorMemberID.UUID, CollectorUserID: r.CollectorStaffUserID.UUID,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.CollectorName != nil {
		l.CollectorName = *r.CollectorName
	}
	l.CollectorActive = r.CollectorStatus != nil && *r.CollectorStatus == "active"
	return l
}

// termMessage is the message for a term that is not one of TermMonths.
const termMessage = "Jangka sewa harus 1, 3, 6 atau 12 bulan."

// ValidTerms checks lease terms for a lease billed every term months and returns them with the
// dates as calendar days. Errors are *InputError. A BilledFrom given must be a period start of
// the term; a zero one is left zero, for the service to fill in (the start date on create, the
// stored value on update), and the latest allowed one depends on the day the lease was entered,
// so the service checks that. A term of 0 means it is not known yet (an update before the lease
// is loaded, an import row whose term comes from its unit): the period start is then checked
// by the service.
func ValidTerms(t Terms, term int) (Terms, error) {
	t.StartDate = calendarDay(t.StartDate)
	t.BilledFrom = calendarDay(t.BilledFrom)
	switch {
	case t.StartDate.IsZero():
		return t, &InputError{Field: "startDate", Message: "Tanggal mulai sewa wajib diisi."}
	case t.DepositAmount < 0:
		return t, &InputError{Field: "depositAmount", Message: "Deposit tidak boleh negatif."}
	}
	if t.BilledFrom.IsZero() || term == 0 {
		return t, nil
	}
	return t, checkPeriodStart(t, term)
}

// checkPeriodStart checks that BilledFrom is the first day of one of the lease's periods.
func checkPeriodStart(t Terms, term int) error {
	if IsPeriodStart(t.StartDate, term, t.BilledFrom) {
		return nil
	}
	if term == 1 {
		return &InputError{Field: "billedFrom", Message: fmt.Sprintf(
			"Mulai tagih harus jatuh pada tanggal tagih sewa ini (tanggal %d) dan tidak sebelum tanggal mulai.", t.StartDate.Day())}
	}
	return &InputError{Field: "billedFrom", Message: fmt.Sprintf(
		"Mulai tagih harus jatuh pada awal periode sewa ini (tiap %d bulan sejak tanggal mulai).", term)}
}

// calendarDay keeps only the year, month and day of t, as midnight UTC, the way PostgreSQL
// returns a date column.
func calendarDay(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Today is the current calendar day in Asia/Jakarta, as midnight UTC like a date column.
func Today(c clock.Clock) time.Time {
	return calendarDay(c.Now().In(clock.Jakarta))
}
