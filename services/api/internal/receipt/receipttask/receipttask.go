// Package receipttask sends cash payment receipts through the queue (slice 6, S6-2): one
// receipt:send task per cash payment, enqueued by the API once the payment commits, and the
// sweep's part that enqueues receipts still pending (reminders:sweep runs it, see
// remindertask.Sweep). The rules live in package receipt; this package only moves ids between the
// queue and the worker.
package receipttask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/queue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/receipt"
)

const (
	// TypeSend sends one cash payment's receipt (receipt.Sender.Send).
	TypeSend = "receipt:send"
	// sendRetries bounds the retries of a send that failed before its receipt was marked (a
	// database error). Once it is marked, nothing is retried.
	sendRetries = 5
)

// TaskID names a payment's receipt task: receipt:send:<payment id>.
func TaskID(paymentID uuid.UUID) string { return TypeSend + ":" + paymentID.String() }

type payload struct {
	OrganizationID uuid.UUID `json:"organizationId"`
	ID             uuid.UUID `json:"id"`
}

// Client enqueues receipt:send on asynq. It implements billing.ReceiptQueue.
type Client struct{ c *asynq.Client }

// NewClient returns a Client that uses c.
func NewClient(c *asynq.Client) Client { return Client{c: c} }

// EnqueueReceipt implements billing.ReceiptQueue.
func (c Client) EnqueueReceipt(ctx context.Context, org, paymentID uuid.UUID) error {
	body, err := json.Marshal(payload{OrganizationID: org, ID: paymentID})
	if err != nil {
		return fmt.Errorf("encoding %s payload: %w", TypeSend, err)
	}
	id := TaskID(paymentID)
	_, err = c.c.EnqueueContext(ctx, asynq.NewTask(TypeSend, body), asynq.TaskID(id), asynq.Queue(queue.Default),
		asynq.MaxRetry(sendRetries))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil // the same task is already queued
	}
	if err != nil {
		return fmt.Errorf("enqueueing %s: %w", id, err)
	}
	return nil
}

// Queue enqueues a receipt's task. Client implements it.
type Queue interface {
	EnqueueReceipt(ctx context.Context, org, paymentID uuid.UUID) error
}

// Sender sends receipts and lists those still pending. receipt.Sender implements it.
type Sender interface {
	Send(ctx context.Context, org, paymentID uuid.UUID) (receipt.Outcome, error)
	Pending(ctx context.Context, org uuid.UUID) ([]uuid.UUID, error)
}

// Worker holds what the receipt:send handler needs.
type Worker struct {
	Sender Sender
	Log    *slog.Logger
}

// Register adds the receipt:send handler to mux.
func (w Worker) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeSend, w.HandleSend)
}

// HandleSend sends one receipt. A malformed payload or a receipt that does not exist is never
// retried; an error before the receipt is marked is.
func (w Worker) HandleSend(ctx context.Context, t *asynq.Task) error {
	var p payload
	if err := json.Unmarshal(t.Payload(), &p); err != nil || p.OrganizationID == uuid.Nil || p.ID == uuid.Nil {
		return fmt.Errorf("%s without an organization or payment: %w", TypeSend, asynq.SkipRetry)
	}
	_, err := w.Sender.Send(ctx, p.OrganizationID, p.ID)
	if errors.Is(err, receipt.ErrReceiptNotFound) {
		return fmt.Errorf("receipt of payment %s of organization %s: %w", p.ID, p.OrganizationID, asynq.SkipRetry)
	}
	if err != nil {
		return fmt.Errorf("sending the receipt of payment %s: %w", p.ID, err)
	}
	return nil
}

// Sweeper enqueues an organization's receipts still pending again. reminders:sweep runs it for
// every organization (remindertask.Sweep).
type Sweeper struct {
	Sender Sender
	Queue  Queue
}

// SweepOrg enqueues the tasks of org's receipts still pending receipt.SweepAfter after they were
// recorded and returns how many it enqueued. A failed enqueue does not stop the others; they come
// back joined.
func (s Sweeper) SweepOrg(ctx context.Context, org uuid.UUID) (int, error) {
	ids, err := s.Sender.Pending(ctx, org)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, id := range ids {
		if err := s.Queue.EnqueueReceipt(ctx, org, id); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}
