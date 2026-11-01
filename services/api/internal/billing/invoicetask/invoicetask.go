// Package invoicetask runs invoice generation through the queue: the invoices:generate task for
// one organization, the daily run at 05:00 Asia/Jakarta that enqueues it for every
// organization, and the enqueue after a lease is saved. The billing rules live in package
// billing; this package only moves organization ids between the API, the scheduler and the
// worker.
//
// A task carries the organization id and nothing else, so no personal data sits in Redis. Task
// ids only keep a duplicate out while the first task is queued; generation itself is idempotent
// (a unique key per lease and period), so a duplicate run creates nothing.
package invoicetask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/queue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

const (
	// TypeGenerate generates one organization's invoices (billing.Service.GenerateOrg).
	TypeGenerate = "invoices:generate"
	// TypeDaily enqueues TypeGenerate for every organization.
	TypeDaily = "invoices:daily"
	// DailyCron runs TypeDaily at 05:00 every day; the scheduler runs in Asia/Jakarta.
	DailyCron = "0 5 * * *"
)

// enqueueTimeout bounds one enqueue made after a request has answered.
const enqueueTimeout = 5 * time.Second

// Task is one invoices:generate task to enqueue.
type Task struct {
	ID             string
	OrganizationID uuid.UUID
}

// DailyTaskID names the daily run's task for an organization on a day (a calendar day in
// Asia/Jakarta): invoices:generate:<org>:<YYYY-MM-DD>.
func DailyTaskID(org uuid.UUID, day time.Time) string {
	return fmt.Sprintf("%s:%s:%s", TypeGenerate, org, day.Format(time.DateOnly))
}

// LeaseTaskID names the task enqueued after a lease is saved: invoices:generate:<org>:<lease>.
func LeaseTaskID(org, lease uuid.UUID) string {
	return fmt.Sprintf("%s:%s:%s", TypeGenerate, org, lease)
}

// LeaseDatesTaskID names the task enqueued after a lease's start date or billing start changed:
// invoices:generate:<org>:<lease>:<start>:<billedFrom>. It differs from LeaseTaskID, so a task
// still running from the lease's creation does not swallow it, and it names the new dates, so
// each change gets its own task.
func LeaseDatesTaskID(org, lease uuid.UUID, start, billedFrom time.Time) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", TypeGenerate, org, lease, start.Format(time.DateOnly), billedFrom.Format(time.DateOnly))
}

// Enqueuer puts an invoices:generate task on the queue. A task whose id is already queued
// counts as enqueued.
type Enqueuer interface {
	Enqueue(ctx context.Context, t Task) error
}

// payload is what a generate task carries: the organization id only.
type payload struct {
	OrganizationID uuid.UUID `json:"organizationId"`
}

// Client is the Enqueuer on asynq.
type Client struct{ c *asynq.Client }

// NewClient returns an Enqueuer that uses c.
func NewClient(c *asynq.Client) Client { return Client{c: c} }

// Enqueue implements Enqueuer.
func (c Client) Enqueue(ctx context.Context, t Task) error {
	body, err := json.Marshal(payload{OrganizationID: t.OrganizationID})
	if err != nil {
		return fmt.Errorf("encoding invoices:generate payload: %w", err)
	}
	_, err = c.c.EnqueueContext(ctx, asynq.NewTask(TypeGenerate, body), asynq.TaskID(t.ID), asynq.Queue(queue.Default))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil // the same task is already queued
	}
	if err != nil {
		return fmt.Errorf("enqueueing %s: %w", t.ID, err)
	}
	return nil
}

// LeaseQueue implements tenant.InvoiceQueue: after a lease is saved it enqueues the
// organization's generate task, named after the lease, and logs a failure instead of returning
// it, since the daily run makes the invoices anyway.
type LeaseQueue struct {
	enq Enqueuer
	log *slog.Logger
}

// NewLeaseQueue returns a LeaseQueue.
func NewLeaseQueue(enq Enqueuer, log *slog.Logger) *LeaseQueue {
	return &LeaseQueue{enq: enq, log: log}
}

// LeaseSaved implements tenant.InvoiceQueue. It does not wait on the request's cancellation:
// the lease is already committed, so the enqueue is worth finishing.
func (q *LeaseQueue) LeaseSaved(ctx context.Context, org, lease uuid.UUID) {
	q.enqueue(ctx, Task{ID: LeaseTaskID(org, lease), OrganizationID: org}, lease)
}

// LeaseDatesChanged implements tenant.InvoiceQueue, as LeaseSaved under LeaseDatesTaskID.
func (q *LeaseQueue) LeaseDatesChanged(ctx context.Context, org, lease uuid.UUID, start, billedFrom time.Time) {
	q.enqueue(ctx, Task{ID: LeaseDatesTaskID(org, lease, start, billedFrom), OrganizationID: org}, lease)
}

func (q *LeaseQueue) enqueue(ctx context.Context, t Task, lease uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), enqueueTimeout)
	defer cancel()
	if err := q.enq.Enqueue(ctx, t); err != nil {
		q.log.ErrorContext(ctx, "enqueueing invoice generation after a lease was saved; the daily run will make its invoices",
			"organization", t.OrganizationID, "lease", lease, "error", err)
	}
}

// OrgLister lists the organizations the daily run covers. The store implements it.
type OrgLister interface {
	ActiveOrganizationIDs(ctx context.Context) ([]uuid.UUID, error)
}

// EnqueueDaily enqueues one generate task per organization for day, named DailyTaskID, and
// returns how many it enqueued. A failure for one organization does not stop the others; they
// come back joined.
func EnqueueDaily(ctx context.Context, orgs OrgLister, enq Enqueuer, day time.Time) (int, error) {
	ids, err := orgs.ActiveOrganizationIDs(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, id := range ids {
		if err := enq.Enqueue(ctx, Task{ID: DailyTaskID(id, day), OrganizationID: id}); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}

// Generator generates one organization's invoices. billing.Service implements it.
type Generator interface {
	GenerateOrg(ctx context.Context, org uuid.UUID) (billing.Result, error)
}

// Worker holds what the worker's handlers need.
type Worker struct {
	Generator Generator
	Orgs      OrgLister
	Enqueuer  Enqueuer
	Clock     clock.Clock
	Log       *slog.Logger
}

// Register adds the handlers to mux and the daily run to the scheduler, which must run in
// Asia/Jakarta (queue.NewScheduler with clock.Jakarta).
func (w Worker) Register(mux *asynq.ServeMux, sched *asynq.Scheduler) error {
	mux.HandleFunc(TypeGenerate, w.HandleGenerate)
	mux.HandleFunc(TypeDaily, w.HandleDaily)
	if _, err := sched.Register(DailyCron, asynq.NewTask(TypeDaily, nil), asynq.Queue(queue.Default)); err != nil {
		return fmt.Errorf("scheduling the daily invoice run: %w", err)
	}
	return nil
}

// HandleGenerate runs one organization's generation. A payload without an organization is
// never retried.
func (w Worker) HandleGenerate(ctx context.Context, t *asynq.Task) error {
	var p payload
	if err := json.Unmarshal(t.Payload(), &p); err != nil || p.OrganizationID == uuid.Nil {
		return fmt.Errorf("invoices:generate without an organization: %w", asynq.SkipRetry)
	}
	res, err := w.Generator.GenerateOrg(ctx, p.OrganizationID)
	w.Log.InfoContext(ctx, "invoices generated", "organization", p.OrganizationID,
		"leases", res.Leases, "invoices", res.Invoices, "skipped", res.Skipped)
	if err != nil {
		return fmt.Errorf("generating invoices for organization %s: %w", p.OrganizationID, err)
	}
	return nil
}

// HandleDaily enqueues every organization's generate task for today in Asia/Jakarta.
func (w Worker) HandleDaily(ctx context.Context, _ *asynq.Task) error {
	day := tenant.Today(w.Clock)
	n, err := EnqueueDaily(ctx, w.Orgs, w.Enqueuer, day)
	w.Log.InfoContext(ctx, "daily invoice run enqueued", "day", day.Format(time.DateOnly), "organizations", n)
	if err != nil {
		return fmt.Errorf("enqueueing the daily invoice run: %w", err)
	}
	return nil
}
