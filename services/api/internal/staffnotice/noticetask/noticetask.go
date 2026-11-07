// Package noticetask sends staff notices through the queue (TG-5, decision 42: demo only): one
// staff:notice task per recorded notice, enqueued once the event's transaction commits, and the
// sweep's part that enqueues notices still pending (reminders:sweep runs it, see
// remindertask.Sweep). The rules live in package staffnotice; this package only moves ids between
// the queue and the worker.
package noticetask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/queue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/staffnotice"
)

const (
	// TypeNotice sends one staff notice (staffnotice.Sender.Send).
	TypeNotice = "staff:notice"
	// noticeRetries bounds the retries of a send that failed before its notice was marked (a
	// database error). Once it is marked, nothing is retried.
	noticeRetries = 5
)

// TaskID names a notice's task: staff:notice:<notice id>.
func TaskID(noticeID uuid.UUID) string { return TypeNotice + ":" + noticeID.String() }

type payload struct {
	OrganizationID uuid.UUID `json:"organizationId"`
	ID             uuid.UUID `json:"id"`
}

// Client enqueues staff:notice on asynq. It implements staffnotice.Queue.
type Client struct{ c *asynq.Client }

// NewClient returns a Client that uses c.
func NewClient(c *asynq.Client) Client { return Client{c: c} }

// EnqueueNotice implements staffnotice.Queue.
func (c Client) EnqueueNotice(ctx context.Context, org, noticeID uuid.UUID) error {
	body, err := json.Marshal(payload{OrganizationID: org, ID: noticeID})
	if err != nil {
		return fmt.Errorf("encoding %s payload: %w", TypeNotice, err)
	}
	id := TaskID(noticeID)
	_, err = c.c.EnqueueContext(ctx, asynq.NewTask(TypeNotice, body), asynq.TaskID(id), asynq.Queue(queue.Default),
		asynq.MaxRetry(noticeRetries))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil // the same task is already queued
	}
	if err != nil {
		return fmt.Errorf("enqueueing %s: %w", id, err)
	}
	return nil
}

// Sender sends notices and lists those still pending. staffnotice.Sender implements it.
type Sender interface {
	Send(ctx context.Context, org, noticeID uuid.UUID) (staffnotice.Outcome, error)
	Pending(ctx context.Context, org uuid.UUID) ([]uuid.UUID, error)
}

// Worker holds what the staff:notice handler needs.
type Worker struct {
	Sender Sender
	Log    *slog.Logger
}

// Register adds the staff:notice handler to mux.
func (w Worker) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeNotice, w.HandleNotice)
}

// HandleNotice sends one notice. A malformed payload or a notice that does not exist is never
// retried; an error before the notice is marked is.
func (w Worker) HandleNotice(ctx context.Context, t *asynq.Task) error {
	var p payload
	if err := json.Unmarshal(t.Payload(), &p); err != nil || p.OrganizationID == uuid.Nil || p.ID == uuid.Nil {
		return fmt.Errorf("%s without an organization or notice: %w", TypeNotice, asynq.SkipRetry)
	}
	_, err := w.Sender.Send(ctx, p.OrganizationID, p.ID)
	if errors.Is(err, staffnotice.ErrNoticeNotFound) {
		return fmt.Errorf("staff notice %s of organization %s: %w", p.ID, p.OrganizationID, asynq.SkipRetry)
	}
	if err != nil {
		return fmt.Errorf("sending staff notice %s: %w", p.ID, err)
	}
	return nil
}

// Sweeper enqueues an organization's notices still pending again. reminders:sweep runs it for
// every organization (remindertask.Sweep).
type Sweeper struct {
	Sender Sender
	Queue  staffnotice.Queue
}

// SweepOrg enqueues the tasks of org's notices still pending staffnotice.SweepAfter after they
// were recorded and returns how many it enqueued. A failed enqueue does not stop the others; they
// come back joined.
func (s Sweeper) SweepOrg(ctx context.Context, org uuid.UUID) (int, error) {
	ids, err := s.Sender.Pending(ctx, org)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, id := range ids {
		if err := s.Queue.EnqueueNotice(ctx, org, id); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}
