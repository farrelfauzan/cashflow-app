package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// Lock order. Every transaction that touches a lease's money locks rows in this order, so two of
// them never wait on each other in a cycle:
//
//  1. the lease row, FOR NO KEY UPDATE (generation: LockLeaseForBilling; the lease service:
//     LockLease, for an update, a planned end date or a move-out);
//  2. the unit row, FOR SHARE, only by generation before it reads the price (a price being
//     recorded holds it FOR NO KEY UPDATE and touches no lease or tenant);
//  3. the tenant row, FOR NO KEY UPDATE, last (billing.Allocate, and the trigger that checks a
//     correction).
//
// Inserting an invoice or a correction takes FOR KEY SHARE on the lease, tenant and invoice it
// refers to; FOR NO KEY UPDATE does not conflict with that, which is why neither the lease nor the
// tenant is ever locked FOR UPDATE. Generation runs one transaction per lease, so it never holds
// one lease's tenant lock while it waits for another lease of the same tenant.

// Service generates invoices and settles the money side of a lease's end. Generation is the
// platform's own work: it checks no member's permission and audits each invoice as a system
// action. The lease hooks (PlannedEndChanged, MovedOut) run as the acting member and authorize
// each correction.
type Service struct {
	store *store.Store
	authz *authz.Service
	clock clock.Clock
	log   *slog.Logger
}

// NewService returns a Service. c decides "today" in Asia/Jakarta.
func NewService(st *store.Store, az *authz.Service, c clock.Clock, log *slog.Logger) *Service {
	return &Service{store: st, authz: az, clock: c, log: log}
}

// Result counts what one generation run did.
type Result struct {
	Leases   int // active leases looked at
	Invoices int // invoices created
	Skipped  int // leases left without an invoice because no price is in force (logged)
}

// GenerateOrg is the invoices:generate task for one organization: for each of its active
// leases, in a transaction of its own, it creates every missing invoice due within the lead time
// (see DuePeriods) and allocates the tenant's credit to them. Running it again creates nothing.
// A lease whose term has no price in force is skipped and logged as an error. A failure on one
// lease does not stop the others; the run returns them joined, so the queue retries it.
func (s *Service) GenerateOrg(ctx context.Context, org uuid.UUID) (Result, error) {
	today := tenant.Today(s.clock)
	var ids []uuid.UUID
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		ids, err = dbgen.New(tx).ListActiveLeaseIDs(ctx, org)
		return err
	})
	if err != nil {
		return Result{}, fmt.Errorf("listing leases to invoice: %w", err)
	}
	res := Result{Leases: len(ids)}
	var errs []error
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return res, fmt.Errorf("generating invoices: %w", err)
		}
		var n int
		err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			var err error
			n, err = s.GenerateLease(ctx, tx, org, id, today)
			return err
		})
		switch {
		case errors.Is(err, tenant.ErrNoPrice):
			res.Skipped++
		case err != nil:
			errs = append(errs, fmt.Errorf("lease %s: %w", id, err))
		default:
			res.Invoices += n
		}
	}
	return res, errors.Join(errs...)
}

// GenerateLease creates one lease's missing invoices due on today within the lead time, inside
// the caller's organization transaction, and allocates the tenant's credit to them. It returns
// how many it created. It locks the lease first (see the lock order above) and reads it after
// the lock, so it sees a planned end date or a move-out committed while it waited. An ended
// lease gets nothing here (MovedOut bills what it started). With no price in force for the
// lease's term it creates nothing, logs an error and returns an error wrapping tenant.ErrNoPrice.
func (s *Service) GenerateLease(ctx context.Context, tx pgx.Tx, org, leaseID uuid.UUID, today time.Time) (int, error) {
	q := dbgen.New(tx)
	l, ok, err := lockActiveLease(ctx, q, org, leaseID)
	if err != nil || !ok {
		return 0, err
	}
	n, err := s.createMissing(ctx, q, tx, org, l, today)
	if err != nil || n == 0 {
		return 0, err
	}
	if _, err := Allocate(ctx, tx, org, l.TenantID); err != nil {
		return 0, err
	}
	return n, nil
}

// PreviewLease returns the invoices GenerateLease would create for one lease on today, without
// creating them: how many, and what they bill in total. The tenant import's dry run shows it, so
// a sheet without "Mulai tagih dari" cannot create years of debt unnoticed. It runs inside the
// caller's organization transaction and takes the same locks as generation. An ended or missing
// lease previews nothing; with no price in force it returns an error wrapping tenant.ErrNoPrice.
func PreviewLease(ctx context.Context, tx pgx.Tx, org, leaseID uuid.UUID, today time.Time) (count int, total int64, err error) {
	q := dbgen.New(tx)
	l, ok, err := lockActiveLease(ctx, q, org, leaseID)
	if err != nil || !ok {
		return 0, 0, err
	}
	planned, err := planMissing(ctx, q, org, l, today)
	if err != nil {
		return 0, 0, err
	}
	for _, p := range planned {
		total += p.Amount()
	}
	return len(planned), total, nil
}

// lockActiveLease locks a lease for billing (see the lock order above) and reads it after the
// lock. ok is false when the lease is missing or no longer active.
func lockActiveLease(ctx context.Context, q *dbgen.Queries, org, leaseID uuid.UUID) (billedLease, bool, error) {
	r, err := q.LockLeaseForBilling(ctx, dbgen.LockLeaseForBillingParams{OrganizationID: org, ID: leaseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return billedLease{}, false, nil
	}
	if err != nil {
		return billedLease{}, false, fmt.Errorf("locking lease for billing: %w", err)
	}
	if r.Status != tenant.StatusActive {
		return billedLease{}, false, nil
	}
	return billedLease{
		ID: r.ID, TenantID: r.TenantID, UnitID: r.UnitID, PropertyID: r.PropertyID,
		Dates: LeaseDates{
			Start: r.StartDate, EnteredOn: r.EnteredOn, BilledFrom: r.BilledFrom, TermMonths: int(r.TermMonths),
			PlannedEnd: r.PlannedEndDate, End: r.EndDate,
		},
	}, true, nil
}

// billedLease is what generation needs to know about a lease.
type billedLease struct {
	ID, TenantID, UnitID, PropertyID uuid.UUID
	Dates                            LeaseDates
}

// plannedInvoice is one invoice generation would insert: a period with its lines.
type plannedInvoice struct {
	Period tenant.Period
	Lines  []Line
}

// Amount is what the invoice bills: the sum of its lines.
func (p plannedInvoice) Amount() int64 {
	var total int64
	for _, ln := range p.Lines {
		total += ln.Amount
	}
	return total
}

// planMissing returns the lease's invoices that DuePeriods asks for and that do not exist yet,
// each with its lines: the price in force today for the lease's term and the lease's fixed
// charges. Generation inserts exactly these, and the import's dry run shows them, so the two
// never differ. With no price in force it returns an error wrapping tenant.ErrNoPrice. The
// caller holds the lease row lock; planMissing locks the unit FOR SHARE before reading the
// price, so a price recorded at the same moment cannot contradict the invoice.
func planMissing(ctx context.Context, q *dbgen.Queries, org uuid.UUID, l billedLease, today time.Time) ([]plannedInvoice, error) {
	due := DuePeriods(l.Dates, today)
	if len(due) == 0 {
		return nil, nil
	}
	existing, err := q.ListLeaseInvoicePeriods(ctx, dbgen.ListLeaseInvoicePeriodsParams{OrganizationID: org, LeaseID: l.ID})
	if err != nil {
		return nil, fmt.Errorf("loading the lease's invoices: %w", err)
	}
	have := make(map[string]bool, len(existing))
	for _, d := range existing {
		have[d.Format(time.DateOnly)] = true
	}
	var missing []tenant.Period
	for _, p := range due {
		if !have[p.From.Format(time.DateOnly)] {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}

	if _, err := q.LockUnitForBilling(ctx, dbgen.LockUnitForBillingParams{OrganizationID: org, ID: l.UnitID}); err != nil {
		return nil, fmt.Errorf("locking unit for billing: %w", err)
	}
	rent, err := tenant.PriceInForce(ctx, q, org, l.UnitID, l.Dates.TermMonths, today)
	if errors.Is(err, tenant.ErrNoPrice) {
		return nil, fmt.Errorf("lease %s: %w", l.ID, err)
	}
	if err != nil {
		return nil, err
	}
	charges, err := fixedCharges(ctx, q, org, l.ID)
	if err != nil {
		return nil, err
	}
	out := make([]plannedInvoice, len(missing))
	for i, p := range missing {
		out[i] = plannedInvoice{Period: p, Lines: InvoiceLines(p, l.Dates.TermMonths, rent, charges)}
	}
	return out, nil
}

// createMissing inserts the lease's invoices that planMissing returns, each with its lines, and
// audits each one. The caller holds the lease row lock and runs allocation afterwards. It
// returns how many it inserted.
func (s *Service) createMissing(ctx context.Context, q *dbgen.Queries, tx pgx.Tx, org uuid.UUID, l billedLease, today time.Time) (int, error) {
	planned, err := planMissing(ctx, q, org, l, today)
	if errors.Is(err, tenant.ErrNoPrice) {
		s.log.ErrorContext(ctx, "no price in force for a lease's term; no invoice generated",
			"organization", org, "lease", l.ID, "unit", l.UnitID, "termMonths", l.Dates.TermMonths)
	}
	if err != nil || len(planned) == 0 {
		return 0, err
	}
	now := s.clock.Now()
	created := 0
	for _, p := range planned {
		ok, err := s.insertInvoice(ctx, q, tx, org, l, p.Period, p.Lines, now)
		if err != nil {
			return 0, err
		}
		if ok {
			created++
		}
	}
	return created, nil
}

// insertInvoice inserts one invoice and, only when it was inserted, its lines and its audit
// row. It reports false when the period already had an invoice.
func (s *Service) insertInvoice(ctx context.Context, q *dbgen.Queries, tx pgx.Tx, org uuid.UUID, l billedLease, p tenant.Period, lines []Line, now time.Time) (bool, error) {
	var amount int64
	for _, ln := range lines {
		amount += ln.Amount
	}
	id, err := q.InsertInvoiceIfMissing(ctx, dbgen.InsertInvoiceIfMissingParams{
		ID: uuid.New(), OrganizationID: org, LeaseID: l.ID, TenantID: l.TenantID,
		Period: p.From, PeriodEnd: p.To, DueDate: p.Due, Amount: amount, CreatedAt: now,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inserting invoice: %w", err)
	}
	for _, ln := range lines {
		_, err := q.CreateInvoiceLine(ctx, dbgen.CreateInvoiceLineParams{
			ID: uuid.New(), OrganizationID: org, InvoiceID: id, Kind: ln.Kind,
			ChargeTypeID: uuid.NullUUID{UUID: ln.ChargeTypeID, Valid: ln.ChargeTypeID != uuid.Nil},
			Label:        ln.Label, Amount: ln.Amount,
		})
		if err != nil {
			return false, fmt.Errorf("inserting invoice line: %w", err)
		}
	}
	res := authz.Resource{Type: "invoice", ID: id.String(), PropertyID: l.PropertyID, Amount: &amount}
	if err := s.authz.RecordSystemAction(ctx, tx, org, authz.ActionInvoiceCreate, res); err != nil {
		return false, err
	}
	return true, nil
}

// fixedCharges loads a lease's fixed monthly extras, by name.
func fixedCharges(ctx context.Context, q *dbgen.Queries, org, leaseID uuid.UUID) ([]FixedCharge, error) {
	rows, err := q.ListLeaseCharges(ctx, dbgen.ListLeaseChargesParams{OrganizationID: org, LeaseIds: []uuid.UUID{leaseID}})
	if err != nil {
		return nil, fmt.Errorf("loading the lease's charges: %w", err)
	}
	out := make([]FixedCharge, len(rows))
	for i, r := range rows {
		out[i] = FixedCharge{ChargeTypeID: r.ChargeTypeID, Name: r.ChargeTypeName, Monthly: r.Amount}
	}
	return out, nil
}
