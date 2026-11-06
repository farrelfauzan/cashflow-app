// Package remindertask decides reminders through the queue (phase B, slice 4): every day at 08:00
// Asia/Jakarta (decision 41.6) the scheduler enqueues one reminders:decide task per organization,
// and the worker records what that organization's tenants should be told today
// (reminder.Decider.DecideOrg). It sends nothing; sending is S4-5. The rules live in package
// reminder; this package only moves organization ids between the scheduler and the worker.
//
// A task carries the organization id and nothing else. Its id names the organization and the day,
// so a run still queued keeps a duplicate out; deciding itself is idempotent (every stage, H-1 and
// broken notice is recorded once, and a tenant has at most one open visit task per reason).
package remindertask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/queue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

const (
	// TypeDecide records one organization's reminders for today.
	TypeDecide = "reminders:decide"
	// TypeDaily enqueues TypeDecide for every organization.
	TypeDaily = "reminders:daily"
	// DailyCron runs TypeDaily at 08:00 every day, when reminders go out (decision 41.6), after
	// the night's promises:close (00:30) so a promise broken overnight is noticed the same morning.
	// The scheduler runs in Asia/Jakarta.
	DailyCron = "0 8 * * *"
)

// Task is one reminders:decide task to enqueue.
type Task struct {
	ID             string
	OrganizationID uuid.UUID
}

// TaskID names an organization's reminders:decide task for a day (a calendar day in
// Asia/Jakarta): reminders:decide:<org>:<YYYY-MM-DD>.
func TaskID(org uuid.UUID, day time.Time) string {
	return fmt.Sprintf("%s:%s:%s", TypeDecide, org, day.Format(time.DateOnly))
}

// Enqueuer puts a reminders:decide task on the queue. A task whose id is already queued counts as
// enqueued.
type Enqueuer interface {
	Enqueue(ctx context.Context, t Task) error
}

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
		return fmt.Errorf("encoding reminders:decide payload: %w", err)
	}
	_, err = c.c.EnqueueContext(ctx, asynq.NewTask(TypeDecide, body), asynq.TaskID(t.ID), asynq.Queue(queue.Default))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil // the same task is already queued
	}
	if err != nil {
		return fmt.Errorf("enqueueing %s: %w", t.ID, err)
	}
	return nil
}

// OrgLister lists the organizations the daily run covers. The store implements it.
type OrgLister interface {
	ActiveOrganizationIDs(ctx context.Context) ([]uuid.UUID, error)
}

// EnqueueDaily enqueues one reminders:decide task per organization for day, named TaskID, and
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
		if err := enq.Enqueue(ctx, Task{ID: TaskID(id, day), OrganizationID: id}); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}

// Decider records one organization's reminders for today. reminder.Decider implements it.
type Decider interface {
	DecideOrg(ctx context.Context, org uuid.UUID) (reminder.Result, error)
}

// Worker holds what the worker's handlers need.
type Worker struct {
	Decider  Decider
	Orgs     OrgLister
	Enqueuer Enqueuer
	Clock    clock.Clock
	Log      *slog.Logger
}

// Register adds the handlers to mux and the daily run to the scheduler, which must run in
// Asia/Jakarta (queue.NewScheduler with clock.Jakarta).
func (w Worker) Register(mux *asynq.ServeMux, sched *asynq.Scheduler) error {
	mux.HandleFunc(TypeDecide, w.HandleDecide)
	mux.HandleFunc(TypeDaily, w.HandleDaily)
	if _, err := sched.Register(DailyCron, asynq.NewTask(TypeDaily, nil), asynq.Queue(queue.Default)); err != nil {
		return fmt.Errorf("scheduling the daily reminders: %w", err)
	}
	return nil
}

// HandleDecide records one organization's reminders. A payload without an organization is never
// retried.
func (w Worker) HandleDecide(ctx context.Context, t *asynq.Task) error {
	var p payload
	if err := json.Unmarshal(t.Payload(), &p); err != nil || p.OrganizationID == uuid.Nil {
		return fmt.Errorf("reminders:decide without an organization: %w", asynq.SkipRetry)
	}
	res, err := w.Decider.DecideOrg(ctx, p.OrganizationID)
	w.Log.InfoContext(ctx, "reminders decided", "organization", p.OrganizationID, "tenants", res.Tenants,
		"reminders", res.Reminders, "stages", res.Stages, "promiseH1", res.PromiseH1,
		"brokenNotices", res.BrokenNotices, "visitTasks", res.VisitTasks, "failed", res.Failed)
	if err != nil {
		return fmt.Errorf("deciding reminders for organization %s: %w", p.OrganizationID, err)
	}
	return nil
}

// HandleDaily enqueues every organization's reminders:decide task for today in Asia/Jakarta.
func (w Worker) HandleDaily(ctx context.Context, _ *asynq.Task) error {
	day := tenant.Today(w.Clock)
	n, err := EnqueueDaily(ctx, w.Orgs, w.Enqueuer, day)
	w.Log.InfoContext(ctx, "daily reminders enqueued", "day", day.Format(time.DateOnly), "organizations", n)
	if err != nil {
		return fmt.Errorf("enqueueing the daily reminders: %w", err)
	}
	return nil
}
