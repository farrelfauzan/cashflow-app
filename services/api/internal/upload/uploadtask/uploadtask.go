// Package uploadtask runs upload scans through the queue (S6-1): one upload:scan task per
// completed upload, enqueued by completeUpload once its transaction commits, and upload:reconcile
// every 10 minutes, which enqueues the scan of every upload completed a while ago and still
// pending (a lost enqueue, or a task that failed every retry). The rules live in package upload;
// this package only moves ids between the queue and the worker.
package uploadtask

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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

const (
	// TypeScan scans one upload (upload.Checker.Scan).
	TypeScan = "upload:scan"
	// TypeReconcile enqueues the scans of every organization's stale uploads.
	TypeReconcile = "upload:reconcile"
	// ReconcileCron runs TypeReconcile every 10 minutes; the scheduler runs in Asia/Jakarta.
	ReconcileCron = "*/10 * * * *"
	// slot is the period one reconcile run covers, which names the scan tasks it enqueues.
	slot = 10 * time.Minute

	// scanRetries bounds the retries of a scan that failed on our side (storage, the scanner or
	// the database); the reconcile enqueues it again later.
	scanRetries = 8
	// scanTimeout bounds one scan: reading and scanning at most upload.MaxSize.
	scanTimeout = 3 * time.Minute
)

// TaskID names an upload's scan task as completeUpload enqueues it: upload:scan:<upload id>. A
// repeated complete finds the first task still queued.
func TaskID(uploadID uuid.UUID) string { return TypeScan + ":" + uploadID.String() }

// ReconcileTaskID names the scan task the reconcile enqueues for an upload in the 10 minutes that
// hold at: upload:scan:<upload id>:<YYYY-MM-DDTHH:MM> in Asia/Jakarta. A task id outlives its
// task when the task fails every retry (asynq keeps it archived), so the reconcile cannot reuse
// TaskID; a scan that runs twice changes nothing.
func ReconcileTaskID(uploadID uuid.UUID, at time.Time) string {
	return TaskID(uploadID) + ":" + at.In(clock.Jakarta).Truncate(slot).Format("2006-01-02T15:04")
}

type payload struct {
	OrganizationID uuid.UUID `json:"organizationId"`
	UploadID       uuid.UUID `json:"uploadId"`
}

// Client enqueues upload:scan on asynq. It implements upload.ScanQueue.
type Client struct{ c *asynq.Client }

// NewClient returns a Client that uses c.
func NewClient(c *asynq.Client) Client { return Client{c: c} }

// EnqueueScan implements upload.ScanQueue.
func (c Client) EnqueueScan(ctx context.Context, org, uploadID uuid.UUID) error {
	return c.EnqueueScanAs(ctx, org, uploadID, TaskID(uploadID))
}

// EnqueueScanAs enqueues an upload's scan under taskID. A task already queued under it counts as
// enqueued.
func (c Client) EnqueueScanAs(ctx context.Context, org, uploadID uuid.UUID, taskID string) error {
	body, err := json.Marshal(payload{OrganizationID: org, UploadID: uploadID})
	if err != nil {
		return fmt.Errorf("encoding %s payload: %w", TypeScan, err)
	}
	_, err = c.c.EnqueueContext(ctx, asynq.NewTask(TypeScan, body), asynq.TaskID(taskID),
		asynq.Queue(queue.Default), asynq.MaxRetry(scanRetries), asynq.Timeout(scanTimeout))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("enqueueing %s: %w", taskID, err)
	}
	return nil
}

// Enqueuer puts a scan task on the queue under a given id. Client implements it.
type Enqueuer interface {
	EnqueueScanAs(ctx context.Context, org, uploadID uuid.UUID, taskID string) error
}

// Checker scans uploads and lists the stale ones. upload.Checker implements it.
type Checker interface {
	Scan(ctx context.Context, org, id uuid.UUID) (upload.Result, error)
	Stale(ctx context.Context, org uuid.UUID) ([]uuid.UUID, error)
}

// OrgLister lists the organizations the reconcile covers. The store implements it.
type OrgLister interface {
	ActiveOrganizationIDs(ctx context.Context) ([]uuid.UUID, error)
}

// Worker holds what the handlers need.
type Worker struct {
	Checker  Checker
	Orgs     OrgLister
	Enqueuer Enqueuer
	Clock    clock.Clock
	Log      *slog.Logger
}

// Register adds the handlers to mux and the 10-minute reconcile to the scheduler, which must run
// in Asia/Jakarta (queue.NewScheduler with clock.Jakarta).
func (w Worker) Register(mux *asynq.ServeMux, sched *asynq.Scheduler) error {
	mux.HandleFunc(TypeScan, w.HandleScan)
	mux.HandleFunc(TypeReconcile, w.HandleReconcile)
	// A run that fails is not retried: the next one comes in 10 minutes.
	if _, err := sched.Register(ReconcileCron, asynq.NewTask(TypeReconcile, nil),
		asynq.Queue(queue.Default), asynq.MaxRetry(0), asynq.Timeout(slot)); err != nil {
		return fmt.Errorf("scheduling the upload reconcile: %w", err)
	}
	return nil
}

// HandleScan scans one upload. A malformed payload or an upload that does not exist is never
// retried; a storage, scanner or database error is.
func (w Worker) HandleScan(ctx context.Context, t *asynq.Task) error {
	var p payload
	if err := json.Unmarshal(t.Payload(), &p); err != nil || p.OrganizationID == uuid.Nil || p.UploadID == uuid.Nil {
		return fmt.Errorf("%s without an organization or upload: %w", TypeScan, asynq.SkipRetry)
	}
	res, err := w.Checker.Scan(ctx, p.OrganizationID, p.UploadID)
	if errors.Is(err, upload.ErrNotFound) {
		return fmt.Errorf("upload %s of organization %s: %w", p.UploadID, p.OrganizationID, asynq.SkipRetry)
	}
	if err != nil {
		return fmt.Errorf("scanning upload %s: %w", p.UploadID, err)
	}
	if res.Changed {
		w.Log.InfoContext(ctx, "upload scanned", "organization", p.OrganizationID, "upload", p.UploadID, "status", res.Status)
	}
	return nil
}

// HandleReconcile enqueues the scan of every stale upload of every organization.
func (w Worker) HandleReconcile(ctx context.Context, _ *asynq.Task) error {
	n, err := Reconcile(ctx, w.Orgs, w.Checker, w.Enqueuer, w.Clock.Now())
	if n > 0 {
		w.Log.InfoContext(ctx, "stale uploads enqueued", "uploads", n)
	}
	if err != nil {
		return fmt.Errorf("reconciling uploads: %w", err)
	}
	return nil
}

// Reconcile enqueues the scan of every organization's stale uploads under ReconcileTaskID for at,
// and returns how many it enqueued. A failure for one organization or upload does not stop the
// others; they come back joined.
func Reconcile(ctx context.Context, orgs OrgLister, c Checker, enq Enqueuer, at time.Time) (int, error) {
	ids, err := orgs.ActiveOrganizationIDs(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, org := range ids {
		stale, err := c.Stale(ctx, org)
		if err != nil {
			errs = append(errs, fmt.Errorf("organization %s: %w", org, err))
			continue
		}
		for _, id := range stale {
			if err := enq.EnqueueScanAs(ctx, org, id, ReconcileTaskID(id, at)); err != nil {
				errs = append(errs, err)
				continue
			}
			n++
		}
	}
	return n, errors.Join(errs...)
}
