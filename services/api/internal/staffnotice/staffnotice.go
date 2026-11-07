// Package staffnotice tells the staff who look after a tenant what happened to them (TG-5,
// decision 42: demo only): the tenant made or edited a promise on the payment page, a promise
// closed as broken, or the tenant joined the visit list. A notice goes only to members with a
// live Telegram link, and only with TELEGRAM_MESSAGES=linked; nothing goes to staff on WhatsApp
// (decision 41(5): that comes with slice 5). With the setting off nothing is recorded: the
// services that make the events get no Notices (nil is off).
//
// A notice is recorded in the transaction that makes its event (RecordTx), so it cannot be lost
// between the event and the queue, and its staff:notice task is enqueued once that commits
// (Enqueue). The task sends it (Sender.Send) at most once, like a promise confirmation: its row is
// marked and committed before any message goes out. The reminders:sweep re-enqueues a notice
// still pending after a lost enqueue.
//
// Who a notice goes to is decided when it is sent: the collectors of the tenant's active leases
// and every active member whom authz allows payment:record on the tenant through one of those
// leases (Recipients). That is the platform's own decision, so it uses authz.Evaluate, which
// records nothing, never Authorize or Check.
//
// Logs carry the notice id, its kind, the organization and counts, never a chat id, a phone or
// the message text.
package staffnotice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// Kinds of notice (staff_notice.kind).
const (
	KindPromiseMade   = "promise_made"
	KindPromiseBroken = "promise_broken"
	KindVisitTask     = "visit_task"
)

// Statuses of a notice.
const (
	StatusPending = "pending"
	StatusSent    = "sent"
	StatusSkipped = "skipped"
)

// ErrNoticeNotFound means the organization has no such notice.
var ErrNoticeNotFound = errors.New("staffnotice: notice not found")

// Event is something staff are told about: a kind, its organization and tenant, and the row it
// is about (Source). Make one with PromiseMade, PromiseBroken or VisitTaskOpened.
type Event struct {
	Kind           string
	OrganizationID uuid.UUID
	TenantID       uuid.UUID
	// Source is the promise version (KindPromiseMade), the promise (KindPromiseBroken) or the
	// visit task (KindVisitTask). Each is noticed at most once.
	Source uuid.UUID
}

// PromiseMade is a promise version the tenant saved on the payment page.
func PromiseMade(org, tenantID, versionID uuid.UUID) Event {
	return Event{Kind: KindPromiseMade, OrganizationID: org, TenantID: tenantID, Source: versionID}
}

// PromiseBroken is a promise that closed as broken.
func PromiseBroken(org, tenantID, promiseID uuid.UUID) Event {
	return Event{Kind: KindPromiseBroken, OrganizationID: org, TenantID: tenantID, Source: promiseID}
}

// VisitTaskOpened is a visit task that was just opened.
func VisitTaskOpened(org, tenantID, visitTaskID uuid.UUID) Event {
	return Event{Kind: KindVisitTask, OrganizationID: org, TenantID: tenantID, Source: visitTaskID}
}

// Queue enqueues a notice's staff:notice task. A task already queued counts as enqueued.
// noticetask.Client implements it.
type Queue interface {
	EnqueueNotice(ctx context.Context, org, noticeID uuid.UUID) error
}

// Notices records notices and enqueues their tasks. The promise service, the promise closer and
// the reminder decider get one only when TELEGRAM_MESSAGES=linked.
type Notices struct {
	clock clock.Clock
	queue Queue
	log   *slog.Logger
}

// NewNotices returns Notices that stamp notices with c's time and enqueue them on q (nil enqueues
// nothing; the sweep still finds them).
func NewNotices(c clock.Clock, q Queue, log *slog.Logger) *Notices {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Notices{clock: c, queue: q, log: log}
}

// RecordTx records a notice of e inside tx, the transaction that makes the event, which must be
// scoped to e's organization. It returns the notice's id, or uuid.Nil when e's source already has
// one (nothing is recorded then). A nil *Notices records nothing.
func (n *Notices) RecordTx(ctx context.Context, tx pgx.Tx, e Event) (uuid.UUID, error) {
	if n == nil {
		return uuid.Nil, nil
	}
	p := dbgen.InsertStaffNoticeParams{
		ID: uuid.New(), OrganizationID: e.OrganizationID, TenantID: e.TenantID, Kind: e.Kind, CreatedAt: n.clock.Now(),
	}
	src := uuid.NullUUID{UUID: e.Source, Valid: true}
	switch e.Kind {
	case KindPromiseMade:
		p.PromiseVersionID = src
	case KindPromiseBroken:
		p.PromiseID = src
	case KindVisitTask:
		p.VisitTaskID = src
	default:
		return uuid.Nil, fmt.Errorf("staffnotice: unknown kind %q", e.Kind)
	}
	rows, err := dbgen.New(tx).InsertStaffNotice(ctx, p)
	if err != nil {
		return uuid.Nil, fmt.Errorf("staffnotice: recording a %s notice: %w", e.Kind, err)
	}
	if rows == 0 {
		return uuid.Nil, nil
	}
	return p.ID, nil
}

// Enqueue enqueues the task of a notice whose transaction has committed. A failure is logged,
// not returned: the event is saved, and the sweep enqueues a notice still pending. A nil
// *Notices, or uuid.Nil, does nothing.
func (n *Notices) Enqueue(ctx context.Context, org, noticeID uuid.UUID) {
	if n == nil || n.queue == nil || noticeID == uuid.Nil {
		return
	}
	if err := n.queue.EnqueueNotice(ctx, org, noticeID); err != nil {
		n.log.WarnContext(ctx, "enqueueing a staff notice; the sweep retries it",
			"organization", org, "notice", noticeID, "error", err)
	}
}
