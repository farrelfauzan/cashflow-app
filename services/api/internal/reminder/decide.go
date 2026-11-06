package reminder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// ErrNothingToRecord means a reminder was about to be recorded with no invoice stage, no H-1 and
// no broken notice. Decider never records one; the database refuses one too, at commit
// (reminder_carries_something).
var ErrNothingToRecord = errors.New("reminder: nothing to record")

// Decider records the day's reminder decisions of an organization's tenants (the
// reminders:decide task). It sends nothing.
//
// Deciding is the platform's own work, like closing promises: it checks no member's permission
// and writes no audit row; the reminder rows are the record.
type Decider struct {
	store *store.Store
	clock clock.Clock
	log   *slog.Logger
}

// NewDecider returns a Decider. c decides today in Asia/Jakarta and the time rows are made.
func NewDecider(st *store.Store, c clock.Clock, log *slog.Logger) *Decider {
	return &Decider{store: st, clock: c, log: log}
}

// Result counts what one DecideOrg run did.
type Result struct {
	Tenants       int // tenants looked at
	Reminders     int // reminder rows recorded (one message each)
	Stages        int // invoice stages on them
	PromiseH1     int // promise H-1 reminders on them
	BrokenNotices int // broken-promise notices on them
	VisitTasks    int // h7 visit tasks opened
	Failed        int // tenants left as they were because of an error (returned)
}

// DecideOrg runs the rules (Decide) for every tenant of org that owes on an invoice due within 3
// days or earlier, has an active promise, or had one break lately, each in a transaction of its
// own under the tenant's allocation lock, and records what they decide: one reminder row with its
// invoice stages per tenant that has something to be told, and an h7 visit task when one is due.
//
// Running it again on the same day records nothing new: every stage, H-1 and broken notice is
// recorded once, and a tenant has at most one open visit task per reason. A failure on one tenant
// does not stop the others; the run returns them joined, so the queue retries it.
func (d *Decider) DecideOrg(ctx context.Context, org uuid.UUID) (Result, error) {
	today := tenant.Today(d.clock)
	var tenants []uuid.UUID
	err := d.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		tenants, err = dbgen.New(tx).ListTenantsToRemind(ctx, dbgen.ListTenantsToRemindParams{
			OrganizationID: org, Horizon: addDays(today, 3), BrokenSince: brokenSince(today),
		})
		return err
	})
	if err != nil {
		return Result{}, fmt.Errorf("reminder: listing the tenants to remind: %w", err)
	}
	res := Result{Tenants: len(tenants)}
	var errs []error
	for _, tenantID := range tenants {
		dec, opened, err := d.decideTenant(ctx, org, tenantID, today)
		if err != nil {
			res.Failed++
			d.log.ErrorContext(ctx, "deciding reminders", "organization", org, "tenant", tenantID, "error", err)
			errs = append(errs, fmt.Errorf("tenant %s: %w", tenantID, err))
			continue
		}
		if dec.HasMessage() {
			res.Reminders++
			res.Stages += len(dec.Stages)
			if dec.PromiseH1 != uuid.Nil {
				res.PromiseH1++
			}
			if dec.BrokenNotice != uuid.Nil {
				res.BrokenNotices++
			}
		}
		if opened {
			res.VisitTasks++
		}
	}
	return res, errors.Join(errs...)
}

// brokenSince is the start of the Asia/Jakarta day BrokenNoticeDays before today: a promise
// closed broken since then may still be notified.
func brokenSince(today time.Time) time.Time {
	return time.Date(today.Year(), today.Month(), today.Day()-BrokenNoticeDays, 0, 0, 0, 0, clock.Jakarta)
}

// decideTenant reads the tenant's state under its allocation lock, runs the rules and records
// their decision in the same transaction, so the reminder row and its stages commit together or
// not at all. It returns the decision and whether it opened an h7 visit task.
//
// The lock is the one payments take, so the run never decides on an invoice a payment is paying
// meanwhile, and two runs for the same tenant (a retry and the next day's, say) take turns: the
// second reads what the first recorded. The unique keys back that up: a stage, H-1 or notice
// recorded twice fails the transaction instead of half-recording a message.
func (d *Decider) decideTenant(ctx context.Context, org, tenantID uuid.UUID, today time.Time) (Decision, bool, error) {
	var dec Decision
	var opened bool
	err := d.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := billing.LockTenant(ctx, q, org, tenantID); err != nil {
			return err
		}
		in, err := loadInput(ctx, q, org, tenantID, today)
		if err != nil {
			return err
		}
		dec = Decide(in)
		now := d.clock.Now()
		if dec.HasMessage() {
			if err := record(ctx, q, org, tenantID, today, now, dec); err != nil {
				return err
			}
		}
		if dec.OpenH7Visit {
			n, err := q.OpenVisitTask(ctx, dbgen.OpenVisitTaskParams{
				ID: uuid.New(), OrganizationID: org, TenantID: tenantID, Reason: VisitReasonH7, CreatedAt: now,
			})
			if err != nil {
				return fmt.Errorf("reminder: opening the h7 visit task: %w", err)
			}
			opened = n == 1
		}
		return nil
	})
	if err != nil {
		return Decision{}, false, err
	}
	return dec, opened, nil
}

// loadInput reads what the rules need about the tenant.
func loadInput(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, today time.Time) (Input, error) {
	in := Input{Today: today, Recorded: map[InvoiceStage]bool{}}

	rows, err := q.ListTenantOwingInvoices(ctx, dbgen.ListTenantOwingInvoicesParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return Input{}, fmt.Errorf("reminder: loading the owing invoices: %w", err)
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.InvoiceID
		in.Invoices = append(in.Invoices, Invoice{ID: r.InvoiceID, DueDate: r.DueDate, Outstanding: r.Outstanding})
	}
	if len(ids) > 0 {
		recorded, err := q.ListRecordedReminderStages(ctx, dbgen.ListRecordedReminderStagesParams{OrganizationID: org, InvoiceIds: ids})
		if err != nil {
			return Input{}, fmt.Errorf("reminder: loading the recorded stages: %w", err)
		}
		for _, r := range recorded {
			in.Recorded[InvoiceStage{InvoiceID: r.InvoiceID, Stage: r.Stage}] = true
		}
	}

	if err := loadActivePromise(ctx, q, org, tenantID, &in); err != nil {
		return Input{}, err
	}

	broken, err := q.ListTenantBrokenPromisesSince(ctx, dbgen.ListTenantBrokenPromisesSinceParams{
		OrganizationID: org, TenantID: tenantID, ClosedSince: brokenSince(today),
	})
	if err != nil {
		return Input{}, fmt.Errorf("reminder: loading the broken promises: %w", err)
	}
	for _, b := range broken {
		if b.ClosedAt == nil {
			continue // a broken promise always has closed_at (payment_promise_closed)
		}
		in.Broken = append(in.Broken, BrokenPromise{ID: b.ID, ClosedOn: tenant.DayOf(*b.ClosedAt), Notified: b.Notified})
	}

	resolved, err := q.LastResolvedVisitAt(ctx, dbgen.LastResolvedVisitAtParams{OrganizationID: org, TenantID: tenantID, Reason: VisitReasonH7})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return Input{}, fmt.Errorf("reminder: reading the last resolved h7 visit: %w", err)
	case resolved != nil:
		on := tenant.DayOf(*resolved)
		in.LastH7VisitResolvedOn = &on
	}
	return in, nil
}

// loadActivePromise sets in.Promise from the tenant's active promise, if any, and marks the
// invoices its newest version covers.
func loadActivePromise(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, in *Input) error {
	active, err := q.GetActivePromise(ctx, dbgen.GetActivePromiseParams{OrganizationID: org, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reminder: loading the active promise: %w", err)
	}
	reminded, err := q.PromiseVersionReminded(ctx, dbgen.PromiseVersionRemindedParams{OrganizationID: org, PromiseVersionID: active.ID})
	if err != nil {
		return fmt.Errorf("reminder: reading the promise's H-1: %w", err)
	}
	in.Promise = &ActivePromise{VersionID: active.ID, PromisedDate: active.PromisedDate, Reminded: reminded}

	covered, err := q.ListPromiseCoveredInvoices(ctx, dbgen.ListPromiseCoveredInvoicesParams{
		OrganizationID: org, VersionIds: []uuid.UUID{active.ID},
	})
	if err != nil {
		return fmt.Errorf("reminder: loading the covered invoices: %w", err)
	}
	isCovered := make(map[uuid.UUID]bool, len(covered))
	for _, c := range covered {
		isCovered[c.InvoiceID] = true
	}
	for i := range in.Invoices {
		in.Invoices[i].Covered = isCovered[in.Invoices[i].ID]
	}
	return nil
}

// record inserts one reminder row for the decision, decided on today, with its invoice stages. A
// decision with nothing to tell is ErrNothingToRecord, and nothing is written.
func record(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, today, now time.Time, dec Decision) error {
	if !dec.HasMessage() {
		return ErrNothingToRecord
	}
	id := uuid.New()
	if err := q.InsertReminder(ctx, dbgen.InsertReminderParams{
		ID: id, OrganizationID: org, TenantID: tenantID, RunDay: today,
		PromiseVersionID: nullID(dec.PromiseH1), BrokenPromiseID: nullID(dec.BrokenNotice), CreatedAt: now,
	}); err != nil {
		return fmt.Errorf("reminder: recording the reminder: %w", err)
	}
	if len(dec.Stages) == 0 {
		return nil
	}
	invoices := make([]uuid.UUID, len(dec.Stages))
	stages := make([]string, len(dec.Stages))
	for i, s := range dec.Stages {
		invoices[i], stages[i] = s.InvoiceID, s.Stage
	}
	if err := q.InsertReminderInvoices(ctx, dbgen.InsertReminderInvoicesParams{
		OrganizationID: org, ReminderID: id, InvoiceIds: invoices, TenantID: tenantID, Stages: stages,
	}); err != nil {
		return fmt.Errorf("reminder: recording the invoice stages: %w", err)
	}
	return nil
}

func nullID(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil} }
