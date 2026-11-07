package remindertask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/queue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
)

// Sending (slice 4, S4-5): one reminder:send task per reminder row, enqueued once the decide run's
// tenant transaction commits, and one promise:confirm task per saved promise version, enqueued by
// the API once the version commits. reminders:sweep enqueues both again for rows still pending
// reminder.SweepAfter later, so a lost enqueue is caught the same day.
const (
	// TypeSend sends one reminder (reminder.Sender.Send).
	TypeSend = "reminder:send"
	// TypeConfirm sends one promise version's confirmation (reminder.Sender.Confirm).
	TypeConfirm = "promise:confirm"
	// TypeSweep enqueues TypeSend and TypeConfirm again for every organization's rows still
	// pending.
	TypeSweep = "reminders:sweep"
	// SweepCron runs TypeSweep every 15 minutes from 08:00 to 21:45, the hours messages go out in;
	// the scheduler runs in Asia/Jakarta. A row from an earlier day is skipped when its task runs,
	// so the first sweep of a morning settles what a night left pending without sending it.
	SweepCron = "*/15 8-21 * * *"
	// sendRetries bounds the retries of a send that failed before its row was marked (a database
	// error). Once the row is marked, nothing is retried.
	sendRetries = 5
)

// SendTaskID names a reminder's send task: reminder:send:<reminder id>.
func SendTaskID(reminderID uuid.UUID) string { return TypeSend + ":" + reminderID.String() }

// ConfirmTaskID names a promise version's confirmation task: promise:confirm:<version id>.
func ConfirmTaskID(versionID uuid.UUID) string { return TypeConfirm + ":" + versionID.String() }

type sendPayload struct {
	OrganizationID uuid.UUID `json:"organizationId"`
	ID             uuid.UUID `json:"id"`
}

// SendClient enqueues reminder:send and promise:confirm on asynq. It implements
// reminder.SendQueue and promise.ConfirmQueue.
type SendClient struct{ c *asynq.Client }

// NewSendClient returns a SendClient that uses c.
func NewSendClient(c *asynq.Client) SendClient { return SendClient{c: c} }

// EnqueueSend implements reminder.SendQueue.
func (c SendClient) EnqueueSend(ctx context.Context, org, reminderID uuid.UUID) error {
	return c.enqueue(ctx, TypeSend, SendTaskID(reminderID), org, reminderID)
}

// EnqueueConfirm implements promise.ConfirmQueue.
func (c SendClient) EnqueueConfirm(ctx context.Context, org, versionID uuid.UUID) error {
	return c.enqueue(ctx, TypeConfirm, ConfirmTaskID(versionID), org, versionID)
}

func (c SendClient) enqueue(ctx context.Context, typ, id string, org, rowID uuid.UUID) error {
	body, err := json.Marshal(sendPayload{OrganizationID: org, ID: rowID})
	if err != nil {
		return fmt.Errorf("encoding %s payload: %w", typ, err)
	}
	_, err = c.c.EnqueueContext(ctx, asynq.NewTask(typ, body), asynq.TaskID(id), asynq.Queue(queue.Default),
		asynq.MaxRetry(sendRetries))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil // the same task is already queued
	}
	if err != nil {
		return fmt.Errorf("enqueueing %s: %w", id, err)
	}
	return nil
}

// Queue is what the sweep enqueues through. SendClient implements it.
type Queue interface {
	EnqueueSend(ctx context.Context, org, reminderID uuid.UUID) error
	EnqueueConfirm(ctx context.Context, org, versionID uuid.UUID) error
}

// Sender sends reminders and confirmations and lists those still pending. reminder.Sender
// implements it.
type Sender interface {
	Send(ctx context.Context, org, reminderID uuid.UUID) (reminder.Outcome, error)
	Confirm(ctx context.Context, org, versionID uuid.UUID) (reminder.Outcome, error)
	Pending(ctx context.Context, org uuid.UUID) (reminders, confirmations []uuid.UUID, err error)
}

// OrgSweep is one more kind of pending row the sweep enqueues again for each organization, such
// as staff notices (TG-5, noticetask.Sweeper).
type OrgSweep interface {
	// SweepOrg enqueues org's rows still pending and returns how many it enqueued.
	SweepOrg(ctx context.Context, org uuid.UUID) (int, error)
}

// SendWorker holds what the sending handlers need.
type SendWorker struct {
	Sender Sender
	Orgs   OrgLister
	Queue  Queue
	// Also are swept with the reminders and confirmations; empty when there are none (staff
	// notices are off without TELEGRAM_MESSAGES=linked).
	Also []OrgSweep
	Log  *slog.Logger
}

// Register adds the sending handlers to mux and the sweep to the scheduler, which must run in
// Asia/Jakarta.
func (w SendWorker) Register(mux *asynq.ServeMux, sched *asynq.Scheduler) error {
	mux.HandleFunc(TypeSend, w.HandleSend)
	mux.HandleFunc(TypeConfirm, w.HandleConfirm)
	mux.HandleFunc(TypeSweep, w.HandleSweep)
	if _, err := sched.Register(SweepCron, asynq.NewTask(TypeSweep, nil), asynq.Queue(queue.Default), asynq.MaxRetry(0)); err != nil {
		return fmt.Errorf("scheduling the reminder sweep: %w", err)
	}
	return nil
}

// HandleSend sends one reminder. A malformed payload or a reminder that does not exist is never
// retried; an error before the row is marked is.
func (w SendWorker) HandleSend(ctx context.Context, t *asynq.Task) error {
	p, err := decode(t)
	if err != nil {
		return err
	}
	_, err = w.Sender.Send(ctx, p.OrganizationID, p.ID)
	if errors.Is(err, reminder.ErrReminderNotFound) {
		return fmt.Errorf("reminder %s of organization %s: %w", p.ID, p.OrganizationID, asynq.SkipRetry)
	}
	if err != nil {
		return fmt.Errorf("sending reminder %s: %w", p.ID, err)
	}
	return nil
}

// HandleConfirm sends one promise version's confirmation, like HandleSend.
func (w SendWorker) HandleConfirm(ctx context.Context, t *asynq.Task) error {
	p, err := decode(t)
	if err != nil {
		return err
	}
	_, err = w.Sender.Confirm(ctx, p.OrganizationID, p.ID)
	if errors.Is(err, reminder.ErrConfirmationNotFound) {
		return fmt.Errorf("promise confirmation %s of organization %s: %w", p.ID, p.OrganizationID, asynq.SkipRetry)
	}
	if err != nil {
		return fmt.Errorf("sending promise confirmation %s: %w", p.ID, err)
	}
	return nil
}

func decode(t *asynq.Task) (sendPayload, error) {
	var p sendPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil || p.OrganizationID == uuid.Nil || p.ID == uuid.Nil {
		return sendPayload{}, fmt.Errorf("%s without an organization or row: %w", t.Type(), asynq.SkipRetry)
	}
	return p, nil
}

// HandleSweep enqueues the send task of every organization's reminders and confirmations still
// pending reminder.SweepAfter after they were recorded.
func (w SendWorker) HandleSweep(ctx context.Context, _ *asynq.Task) error {
	n, err := Sweep(ctx, w.Orgs, w.Sender, w.Queue, w.Also...)
	if n > 0 || err != nil {
		w.Log.InfoContext(ctx, "pending reminders enqueued again", "tasks", n)
	}
	if err != nil {
		return fmt.Errorf("sweeping pending reminders: %w", err)
	}
	return nil
}

// Sweep enqueues, for every organization, the tasks of its reminders and promise confirmations
// still pending, and those of each of also, and returns how many it enqueued. A failure on one
// organization or task does not stop the others; they come back joined.
func Sweep(ctx context.Context, orgs OrgLister, s Sender, q Queue, also ...OrgSweep) (int, error) {
	ids, err := orgs.ActiveOrganizationIDs(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, org := range ids {
		reminders, confirmations, err := s.Pending(ctx, org)
		if err != nil {
			errs = append(errs, fmt.Errorf("organization %s: %w", org, err))
			continue
		}
		for _, id := range reminders {
			if err := q.EnqueueSend(ctx, org, id); err != nil {
				errs = append(errs, err)
				continue
			}
			n++
		}
		for _, id := range confirmations {
			if err := q.EnqueueConfirm(ctx, org, id); err != nil {
				errs = append(errs, err)
				continue
			}
			n++
		}
		for _, a := range also {
			k, err := a.SweepOrg(ctx, org)
			n += k
			if err != nil {
				errs = append(errs, fmt.Errorf("organization %s: %w", org, err))
			}
		}
	}
	return n, errors.Join(errs...)
}
