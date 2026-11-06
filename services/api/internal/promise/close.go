package promise

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

// VisitReasonBrokenPromises is the reason of the visit task a tenant gets once their broken
// promises reach the organization's threshold (visit_task.reason).
const VisitReasonBrokenPromises = "broken_promises"

// Closer closes payment promises (decision 41.3): as kept the moment a payment keeps one
// (PaymentRecorded, CloseIfKept), and with their outcome once their promised day has ended
// (CloseDue, the daily promises:close task). A closed promise is final.
//
// Closing is the platform's own work, like invoice generation: it checks no member's permission.
// It writes no audit row either: a promise is not money, and its status and closed_at are the
// record.
type Closer struct {
	store *store.Store
	clock clock.Clock
	log   *slog.Logger
}

// NewCloser returns a Closer. c decides today in Asia/Jakarta and the closing time.
func NewCloser(st *store.Store, c clock.Clock, log *slog.Logger) *Closer {
	return &Closer{store: st, clock: c, log: log}
}

// PaymentRecorded implements billing.PaymentHook: after a manual payment is inserted and
// allocated, in the same transaction, it closes the tenant's active promise when the payment kept
// it.
func (c *Closer) PaymentRecorded(ctx context.Context, tx pgx.Tx, org, tenantID uuid.UUID) error {
	_, err := CloseIfKept(ctx, tx, org, tenantID, c.clock.Now())
	return err
}

// CloseIfKept closes the tenant's active promise as kept, at now, when its newest version is kept
// now (see Progress.Outcome), and reports whether it did. It never closes one as partially kept or
// broken: those wait for the end of the promised day.
//
// It runs inside tx, scoped to org, in the transaction that inserted and allocated a payment
// (billing.RecordPayment through PaymentRecorded, and the QRIS settle), so the promise turns kept
// with the payment that kept it. It takes the tenant's allocation lock, which that transaction
// already holds.
func CloseIfKept(ctx context.Context, tx pgx.Tx, org, tenantID uuid.UUID, now time.Time) (bool, error) {
	q := dbgen.New(tx)
	if err := billing.LockTenant(ctx, q, org, tenantID); err != nil {
		return false, err
	}
	active, err := q.GetActivePromise(ctx, dbgen.GetActivePromiseParams{OrganizationID: org, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("promise: loading the active promise: %w", err)
	}
	p, err := progressOf(ctx, q, org, tenantID, active)
	if err != nil {
		return false, err
	}
	if p.Outcome() != StatusKept {
		return false, nil
	}
	if err := closePromise(ctx, q, org, active.PromiseID, StatusKept, now); err != nil {
		return false, err
	}
	return true, nil
}

// CloseResult counts what one CloseDue run did.
type CloseResult struct {
	Due           int // tenants whose active promise's day had ended
	Kept          int
	PartiallyKept int
	Broken        int
	VisitTasks    int // broken_promises visit tasks opened
	Failed        int // tenants left as they were because of an error (returned)
}

// CloseDue is the promises:close task for one organization: every active promise whose newest
// version promised a day before today (Asia/Jakarta) closes with its outcome, each tenant in a
// transaction of its own. A tenant whose broken promises then reach the organization's threshold
// gets an open broken_promises visit task, unless one is already open.
//
// Running it again changes nothing: only active promises are closed, and a closed one is final. A
// failure on one tenant does not stop the others; the run returns them joined, so the queue
// retries it.
func (c *Closer) CloseDue(ctx context.Context, org uuid.UUID) (CloseResult, error) {
	today := tenant.Today(c.clock)
	var tenants []uuid.UUID
	err := c.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		tenants, err = dbgen.New(tx).ListTenantsWithDuePromise(ctx, dbgen.ListTenantsWithDuePromiseParams{OrganizationID: org, Today: today})
		return err
	})
	if err != nil {
		return CloseResult{}, fmt.Errorf("promise: listing the promises to close: %w", err)
	}
	res := CloseResult{Due: len(tenants)}
	var errs []error
	for _, tenantID := range tenants {
		status, opened, err := c.closeDueTenant(ctx, org, tenantID, today)
		if err != nil {
			res.Failed++
			c.log.ErrorContext(ctx, "closing a payment promise", "organization", org, "tenant", tenantID, "error", err)
			errs = append(errs, fmt.Errorf("tenant %s: %w", tenantID, err))
			continue
		}
		switch status {
		case StatusKept:
			res.Kept++
		case StatusPartiallyKept:
			res.PartiallyKept++
		case StatusBroken:
			res.Broken++
		}
		if opened {
			res.VisitTasks++
		}
	}
	return res, errors.Join(errs...)
}

// closeDueTenant closes the tenant's active promise when its day ended before today, under the
// tenant's allocation lock, and returns the status it closed with ("" when there was nothing to
// close: an edit moved the date, or it closed meanwhile) and whether it opened a visit task.
func (c *Closer) closeDueTenant(ctx context.Context, org, tenantID uuid.UUID, today time.Time) (string, bool, error) {
	var status string
	var opened bool
	err := c.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := billing.LockTenant(ctx, q, org, tenantID); err != nil {
			return err
		}
		active, err := q.GetActivePromise(ctx, dbgen.GetActivePromiseParams{OrganizationID: org, TenantID: tenantID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("promise: loading the active promise: %w", err)
		}
		if !active.PromisedDate.Before(today) {
			return nil
		}
		p, err := progressOf(ctx, q, org, tenantID, active)
		if err != nil {
			return err
		}
		now := c.clock.Now()
		outcome := p.Outcome()
		if err := closePromise(ctx, q, org, active.PromiseID, outcome, now); err != nil {
			return err
		}
		status = outcome
		if outcome == StatusBroken {
			opened, err = openVisitIfThreshold(ctx, q, org, tenantID, now)
		}
		return err
	})
	if err != nil {
		return "", false, err
	}
	return status, opened, nil
}

// openVisitIfThreshold opens the tenant's broken_promises visit task when their broken promises
// since their last resolved broken_promises task (all of them when none was resolved) reach the
// organization's threshold. It reports whether it
// opened one; an open one already there is left as it is.
func openVisitIfThreshold(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, now time.Time) (bool, error) {
	broken, err := q.CountTenantBrokenPromisesSinceVisit(ctx, dbgen.CountTenantBrokenPromisesSinceVisitParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return false, fmt.Errorf("promise: counting broken promises: %w", err)
	}
	threshold, err := q.GetBrokenPromiseThreshold(ctx, org)
	if err != nil {
		return false, fmt.Errorf("promise: reading the broken-promise threshold: %w", err)
	}
	if broken < int64(threshold) {
		return false, nil
	}
	n, err := q.OpenVisitTask(ctx, dbgen.OpenVisitTaskParams{
		ID: uuid.New(), OrganizationID: org, TenantID: tenantID, Reason: VisitReasonBrokenPromises, CreatedAt: now,
	})
	if err != nil {
		return false, fmt.Errorf("promise: opening the visit task: %w", err)
	}
	return n == 1, nil
}

// progressOf reads where the active promise's newest version v stands.
func progressOf(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, v dbgen.GetActivePromiseRow) (Progress, error) {
	paid, err := q.SumTenantPaymentsSince(ctx, dbgen.SumTenantPaymentsSinceParams{
		OrganizationID: org, TenantID: tenantID, SeqMark: v.PaymentSeqMark, MadeAt: v.CreatedAt,
	})
	if err != nil {
		return Progress{}, fmt.Errorf("promise: summing the payments since the version: %w", err)
	}
	owing, err := q.CountCoveredInvoicesOwing(ctx, dbgen.CountCoveredInvoicesOwingParams{OrganizationID: org, PromiseVersionID: v.ID})
	if err != nil {
		return Progress{}, fmt.Errorf("promise: reading the covered invoices: %w", err)
	}
	return Progress{PayingNow: v.PayingNow, PromisedAmount: v.PromisedAmount, Paid: paid, CoveredSettled: owing == 0}, nil
}

func closePromise(ctx context.Context, q *dbgen.Queries, org, promiseID uuid.UUID, status string, now time.Time) error {
	n, err := q.ClosePromise(ctx, dbgen.ClosePromiseParams{Status: status, ClosedAt: &now, OrganizationID: org, ID: promiseID})
	if err != nil {
		return fmt.Errorf("promise: closing promise %s as %s: %w", promiseID, status, err)
	}
	if n != 1 {
		// The tenant's lock is held and the promise was read active in this transaction.
		return fmt.Errorf("promise: promise %s was not active when closed", promiseID)
	}
	return nil
}
