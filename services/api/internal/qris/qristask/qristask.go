// Package qristask runs QRIS reconcile through the queue (phase B, slice 3): every 30 minutes the
// scheduler enqueues one qris:reconcile task per organization, and the worker asks AutoGoPay about
// that organization's pending attempts (qris.Service.ReconcileOrg). It covers a webhook that never
// arrived and a tenant who said they paid. The rules live in package qris; this package only
// moves organization ids between the scheduler and the worker.
//
// A task carries the organization id and nothing else. Its id names the half hour it was
// scheduled for, so a run still queued keeps a duplicate out; reconcile itself is idempotent (the
// payment is unique per transaction id, and a closed or paid attempt is skipped).
package qristask

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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
)

const (
	// TypeReconcile reconciles one organization's pending attempts.
	TypeReconcile = "qris:reconcile"
	// TypeReconcileAll enqueues TypeReconcile for every organization.
	TypeReconcileAll = "qris:reconcile-all"
	// ReconcileCron runs TypeReconcileAll every 30 minutes, the night included; the scheduler
	// runs in Asia/Jakarta.
	ReconcileCron = "*/30 * * * *"
	// slot is the period one run covers, which names its tasks.
	slot = 30 * time.Minute
)

// TaskID names an organization's reconcile task for the half hour that holds at:
// qris:reconcile:<org>:<YYYY-MM-DDTHH:MM> in Asia/Jakarta.
func TaskID(org uuid.UUID, at time.Time) string {
	return fmt.Sprintf("%s:%s:%s", TypeReconcile, org, at.In(clock.Jakarta).Truncate(slot).Format("2006-01-02T15:04"))
}

// Task is one qris:reconcile task to enqueue.
type Task struct {
	ID             string
	OrganizationID uuid.UUID
}

// Enqueuer puts a qris:reconcile task on the queue. A task whose id is already queued counts as
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
		return fmt.Errorf("encoding qris:reconcile payload: %w", err)
	}
	// A run that fails is not retried: the next one comes in 30 minutes.
	_, err = c.c.EnqueueContext(ctx, asynq.NewTask(TypeReconcile, body),
		asynq.TaskID(t.ID), asynq.Queue(queue.Default), asynq.MaxRetry(0), asynq.Timeout(slot))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("enqueueing %s: %w", t.ID, err)
	}
	return nil
}

// OrgLister lists the organizations reconcile covers. The store implements it.
type OrgLister interface {
	ActiveOrganizationIDs(ctx context.Context) ([]uuid.UUID, error)
}

// Reconciler reconciles one organization. qris.Service implements it.
type Reconciler interface {
	ReconcileOrg(ctx context.Context, org uuid.UUID) (qris.ReconcileResult, error)
}

// EnqueueAll enqueues one reconcile task per organization for the half hour that holds at, and
// returns how many it enqueued. A failure for one organization does not stop the others; they
// come back joined.
func EnqueueAll(ctx context.Context, orgs OrgLister, enq Enqueuer, at time.Time) (int, error) {
	ids, err := orgs.ActiveOrganizationIDs(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, id := range ids {
		if err := enq.Enqueue(ctx, Task{ID: TaskID(id, at), OrganizationID: id}); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}

// Worker holds what the worker's handlers need.
type Worker struct {
	Reconciler Reconciler
	Orgs       OrgLister
	Enqueuer   Enqueuer
	Clock      clock.Clock
	Log        *slog.Logger
}

// Register adds the handlers to mux and the 30-minute run to the scheduler, which must run in
// Asia/Jakarta (queue.NewScheduler with clock.Jakarta).
func (w Worker) Register(mux *asynq.ServeMux, sched *asynq.Scheduler) error {
	mux.HandleFunc(TypeReconcile, w.HandleReconcile)
	mux.HandleFunc(TypeReconcileAll, w.HandleReconcileAll)
	if _, err := sched.Register(ReconcileCron, asynq.NewTask(TypeReconcileAll, nil),
		asynq.Queue(queue.Default), asynq.MaxRetry(0)); err != nil {
		return fmt.Errorf("scheduling QRIS reconcile: %w", err)
	}
	return nil
}

// HandleReconcile reconciles one organization. A payload without an organization is never
// retried.
func (w Worker) HandleReconcile(ctx context.Context, t *asynq.Task) error {
	var p payload
	if err := json.Unmarshal(t.Payload(), &p); err != nil || p.OrganizationID == uuid.Nil {
		return fmt.Errorf("qris:reconcile without an organization: %w", asynq.SkipRetry)
	}
	res, err := w.Reconciler.ReconcileOrg(ctx, p.OrganizationID)
	w.Log.InfoContext(ctx, "QRIS reconciled", "organization", p.OrganizationID, "checked", res.Checked,
		"paid", res.Paid, "closed", res.Closed, "busy", res.Busy, "failed", res.Failed)
	if err != nil {
		return fmt.Errorf("reconciling QRIS for organization %s: %w", p.OrganizationID, err)
	}
	return nil
}

// HandleReconcileAll enqueues every organization's reconcile task for this half hour.
func (w Worker) HandleReconcileAll(ctx context.Context, _ *asynq.Task) error {
	now := w.Clock.Now()
	n, err := EnqueueAll(ctx, w.Orgs, w.Enqueuer, now)
	w.Log.InfoContext(ctx, "QRIS reconcile enqueued", "organizations", n)
	if err != nil {
		return fmt.Errorf("enqueueing QRIS reconcile: %w", err)
	}
	return nil
}
