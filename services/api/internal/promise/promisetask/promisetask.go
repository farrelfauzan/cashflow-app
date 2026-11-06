// Package promisetask closes payment promises through the queue (phase B, slice 4): every day at
// 00:30 Asia/Jakarta the scheduler enqueues one promises:close task per organization, and the
// worker closes that organization's active promises whose promised day has ended
// (promise.Closer.CloseDue). The rules live in package promise; this package only moves
// organization ids between the scheduler and the worker.
//
// A task carries the organization id and nothing else. Its id names the organization and the day,
// so a run still queued keeps a duplicate out; closing itself is idempotent (only active promises
// are closed, a closed one is final, and a tenant has at most one open visit task per reason).
package promisetask

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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

const (
	// TypeClose closes one organization's promises whose day has ended.
	TypeClose = "promises:close"
	// TypeDaily enqueues TypeClose for every organization.
	TypeDaily = "promises:daily"
	// DailyCron runs TypeDaily at 00:30 every day, once the promised day has ended; the scheduler
	// runs in Asia/Jakarta.
	DailyCron = "30 0 * * *"
)

// Task is one promises:close task to enqueue.
type Task struct {
	ID             string
	OrganizationID uuid.UUID
}

// TaskID names an organization's promises:close task for a day (a calendar day in Asia/Jakarta):
// promises:close:<org>:<YYYY-MM-DD>.
func TaskID(org uuid.UUID, day time.Time) string {
	return fmt.Sprintf("%s:%s:%s", TypeClose, org, day.Format(time.DateOnly))
}

// Enqueuer puts a promises:close task on the queue. A task whose id is already queued counts as
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
		return fmt.Errorf("encoding promises:close payload: %w", err)
	}
	_, err = c.c.EnqueueContext(ctx, asynq.NewTask(TypeClose, body), asynq.TaskID(t.ID), asynq.Queue(queue.Default))
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

// EnqueueDaily enqueues one promises:close task per organization for day, named TaskID, and
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

// Closer closes one organization's promises whose day has ended. promise.Closer implements it.
type Closer interface {
	CloseDue(ctx context.Context, org uuid.UUID) (promise.CloseResult, error)
}

// Worker holds what the worker's handlers need.
type Worker struct {
	Closer   Closer
	Orgs     OrgLister
	Enqueuer Enqueuer
	Clock    clock.Clock
	Log      *slog.Logger
}

// Register adds the handlers to mux and the daily run to the scheduler, which must run in
// Asia/Jakarta (queue.NewScheduler with clock.Jakarta).
func (w Worker) Register(mux *asynq.ServeMux, sched *asynq.Scheduler) error {
	mux.HandleFunc(TypeClose, w.HandleClose)
	mux.HandleFunc(TypeDaily, w.HandleDaily)
	if _, err := sched.Register(DailyCron, asynq.NewTask(TypeDaily, nil), asynq.Queue(queue.Default)); err != nil {
		return fmt.Errorf("scheduling the daily promise close: %w", err)
	}
	return nil
}

// HandleClose closes one organization's promises. A payload without an organization is never
// retried.
func (w Worker) HandleClose(ctx context.Context, t *asynq.Task) error {
	var p payload
	if err := json.Unmarshal(t.Payload(), &p); err != nil || p.OrganizationID == uuid.Nil {
		return fmt.Errorf("promises:close without an organization: %w", asynq.SkipRetry)
	}
	res, err := w.Closer.CloseDue(ctx, p.OrganizationID)
	w.Log.InfoContext(ctx, "payment promises closed", "organization", p.OrganizationID, "due", res.Due,
		"kept", res.Kept, "partiallyKept", res.PartiallyKept, "broken", res.Broken,
		"visitTasks", res.VisitTasks, "failed", res.Failed)
	if err != nil {
		return fmt.Errorf("closing payment promises for organization %s: %w", p.OrganizationID, err)
	}
	return nil
}

// HandleDaily enqueues every organization's promises:close task for today in Asia/Jakarta.
func (w Worker) HandleDaily(ctx context.Context, _ *asynq.Task) error {
	day := tenant.Today(w.Clock)
	n, err := EnqueueDaily(ctx, w.Orgs, w.Enqueuer, day)
	w.Log.InfoContext(ctx, "daily promise close enqueued", "day", day.Format(time.DateOnly), "organizations", n)
	if err != nil {
		return fmt.Errorf("enqueueing the daily promise close: %w", err)
	}
	return nil
}
