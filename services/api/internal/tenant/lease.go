package tenant

import (
	"context"
	"errors"
	"fmt"
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
	ID            uuid.UUID
	TenantID      uuid.UUID
	UnitID        uuid.UUID
	UnitLabel     string
	PropertyID    uuid.UUID
	PropertyName  string
	StartDate     time.Time
	EndDate       *time.Time // last day, set by move-out
	RentAmount    int64
	DueDay        int
	DepositAmount int64
	Status        string
	// Collector is the member who collects from this tenant, if any.
	CollectorMemberID uuid.UUID
	CollectorUserID   uuid.UUID
	CollectorName     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Terms are the editable terms of a lease.
type Terms struct {
	StartDate     time.Time
	RentAmount    int64
	DueDay        int
	DepositAmount int64
	// CollectorMemberID is a member id (not a staff user id); uuid.Nil means no collector.
	CollectorMemberID uuid.UUID
}

// NewLease is the input for renting a unit to a tenant.
type NewLease struct {
	TenantID uuid.UUID
	UnitID   uuid.UUID
	Terms
}

// CreateLease rents a unit to a tenant. It needs lease:manage on the unit's property, and the
// unit must not have an active lease.
func (s *Service) CreateLease(ctx context.Context, p authz.Principal, in NewLease) (Lease, error) {
	terms, err := validTerms(in.Terms)
	if err != nil {
		return Lease{}, err
	}
	var out Lease
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.GetTenant(ctx, dbgen.GetTenantParams{OrganizationID: p.OrganizationID, ID: in.TenantID}); err != nil {
			return lookupError(err, "tenantId", "Penyewa tidak ditemukan.")
		}
		unit, err := q.GetUnit(ctx, dbgen.GetUnitParams{OrganizationID: p.OrganizationID, ID: in.UnitID})
		if err != nil {
			return lookupError(err, "unitId", "Unit tidak ditemukan.")
		}
		collector, err := collectorUser(ctx, q, p.OrganizationID, terms.CollectorMemberID)
		if err != nil {
			return err
		}
		id := uuid.New()
		req := authz.Request{Action: authz.ActionLeaseManage, Resource: authz.Resource{
			Type: "lease", ID: id.String(), PropertyID: unit.PropertyID, OwnerID: collector.UUID,
		}}
		if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
			return err
		}
		err = q.CreateLease(ctx, dbgen.CreateLeaseParams{
			ID: id, OrganizationID: p.OrganizationID, UnitID: in.UnitID, TenantID: in.TenantID,
			StartDate: terms.StartDate, RentAmount: terms.RentAmount, DueDay: int16(terms.DueDay), //nolint:gosec // validTerms keeps it in 1..28
			DepositAmount: terms.DepositAmount, CollectorStaffUserID: collector,
		})
		if isUniqueViolation(err, "lease_one_active_per_unit") {
			return ErrUnitOccupied
		}
		if err != nil {
			return fmt.Errorf("creating lease: %w", err)
		}
		out, err = loadLease(ctx, q, p.OrganizationID, id)
		return err
	})
	return out, err
}

// UpdateLease changes the terms of an active lease. Each change is audited; a new rent applies
// from the next invoice.
func (s *Service) UpdateLease(ctx context.Context, p authz.Principal, id uuid.UUID, t Terms) (Lease, error) {
	terms, err := validTerms(t)
	if err != nil {
		return Lease{}, err
	}
	var out Lease
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := s.loadActiveForChange(ctx, tx, q, p, id); err != nil {
			return err
		}
		collector, err := collectorUser(ctx, q, p.OrganizationID, terms.CollectorMemberID)
		if err != nil {
			return err
		}
		err = q.UpdateLeaseTerms(ctx, dbgen.UpdateLeaseTermsParams{
			OrganizationID: p.OrganizationID, ID: id, StartDate: terms.StartDate, RentAmount: terms.RentAmount,
			DueDay:        int16(terms.DueDay), //nolint:gosec // validTerms keeps it in 1..28
			DepositAmount: terms.DepositAmount, CollectorStaffUserID: collector,
		})
		if err != nil {
			return fmt.Errorf("updating lease: %w", err)
		}
		out, err = loadLease(ctx, q, p.OrganizationID, id)
		return err
	})
	return out, err
}

// MoveOut ends an active lease. endDate is the last day: not before the start and not after
// today in Asia/Jakarta. The unit is free for a new lease right away. What happens to arrears is
// an organization setting applied in slice 2, when invoices exist.
func (s *Service) MoveOut(ctx context.Context, p authz.Principal, id uuid.UUID, endDate time.Time) (Lease, error) {
	endDate = calendarDay(endDate)
	var out Lease
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		current, err := s.loadActiveForChange(ctx, tx, q, p, id)
		if err != nil {
			return err
		}
		switch {
		case endDate.Before(current.StartDate):
			return &InputError{Field: "endDate", Message: "Tanggal keluar tidak boleh sebelum tanggal mulai sewa."}
		case endDate.After(today(s.clock)):
			return &InputError{Field: "endDate", Message: "Tanggal keluar tidak boleh setelah hari ini."}
		}
		if err := q.EndLease(ctx, dbgen.EndLeaseParams{OrganizationID: p.OrganizationID, ID: id, EndDate: &endDate}); err != nil {
			return fmt.Errorf("ending lease: %w", err)
		}
		out, err = loadLease(ctx, q, p.OrganizationID, id)
		return err
	})
	return out, err
}

// loadActiveForChange loads a lease, authorizes lease:manage on it and checks that it is
// still active.
func (s *Service) loadActiveForChange(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, p authz.Principal, id uuid.UUID) (Lease, error) {
	current, err := loadLease(ctx, q, p.OrganizationID, id)
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

func loadLease(ctx context.Context, q *dbgen.Queries, org, id uuid.UUID) (Lease, error) {
	r, err := q.GetLeaseDetail(ctx, dbgen.GetLeaseDetailParams{OrganizationID: org, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Lease{}, ErrNotFound
	}
	if err != nil {
		return Lease{}, fmt.Errorf("loading lease: %w", err)
	}
	return toLease(r), nil
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
		StartDate: r.StartDate, EndDate: r.EndDate, RentAmount: r.RentAmount, DueDay: int(r.DueDay),
		DepositAmount: r.DepositAmount, Status: r.Status,
		CollectorMemberID: r.CollectorMemberID.UUID, CollectorUserID: r.CollectorStaffUserID.UUID,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.CollectorName != nil {
		l.CollectorName = *r.CollectorName
	}
	return l
}

func validTerms(t Terms) (Terms, error) {
	t.StartDate = calendarDay(t.StartDate)
	switch {
	case t.StartDate.IsZero():
		return t, &InputError{Field: "startDate", Message: "Tanggal mulai sewa wajib diisi."}
	case t.RentAmount <= 0:
		return t, &InputError{Field: "rentAmount", Message: "Harga sewa harus lebih dari Rp0."}
	case t.DueDay < 1 || t.DueDay > 28:
		return t, &InputError{Field: "dueDay", Message: "Tanggal jatuh tempo harus antara 1 dan 28."}
	case t.DepositAmount < 0:
		return t, &InputError{Field: "depositAmount", Message: "Deposit tidak boleh negatif."}
	}
	return t, nil
}

// calendarDay keeps only the year, month and day of t, as midnight UTC, the way PostgreSQL
// returns a date column.
func calendarDay(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// today is the current calendar day in Asia/Jakarta.
func today(c clock.Clock) time.Time {
	return calendarDay(c.Now().In(clock.Jakarta))
}
