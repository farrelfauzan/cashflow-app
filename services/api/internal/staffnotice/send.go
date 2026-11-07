package staffnotice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// SweepAfter is how long a notice may stay pending before the sweep enqueues its task again, as
// for reminders (reminder.SweepAfter).
const SweepAfter = 15 * time.Minute

// sweepBatch bounds how many notices one sweep enqueues per organization.
const sweepBatch = 500

// Sending hours in Asia/Jakarta: notices go out from 08:00 until 21:59, the hours reminders:sweep
// runs in. A notice whose task runs at night (a promise closed broken at 00:30, a promise made at
// 23:00) stays pending, and the first sweep of the morning sends it.
const (
	firstSendHour = 8
	lastSendHour  = 21
)

// InSendingHours reports whether t is within the sending hours in Asia/Jakarta.
func InSendingHours(t time.Time) bool {
	h := t.In(clock.Jakarta).Hour()
	return h >= firstSendHour && h <= lastSendHour
}

// Visit task reasons (visit_task.reason), as reminder and promise open them.
const (
	visitReasonH7             = "h7"
	visitReasonBrokenPromises = "broken_promises"
	visitReasonUndeliverable  = "undeliverable"
)

// noUnit names the tenant's unit when they have no lease at all.
const noUnit = "tanpa unit"

// Outcome is what one Send did.
type Outcome string

// Outcomes of Send.
const (
	// OutcomeSent: the notice is marked sent and a message went to each recipient through the
	// router; a recipient with no live Telegram link got nothing, and a refused send is logged,
	// never retried.
	OutcomeSent Outcome = "sent"
	// OutcomeSkipped: the event no longer applies (the promise was edited again or closed, the
	// visit task was resolved) or nobody looks after the tenant; the notice is marked skipped.
	OutcomeSkipped Outcome = "skipped"
	// OutcomeSettled: an earlier delivery already sent or skipped it; nothing is done.
	OutcomeSettled Outcome = "settled"
	// OutcomeDeferred: outside the sending hours; the notice stays pending for the sweep.
	OutcomeDeferred Outcome = "deferred"
)

// Principals lists the active members of an organization inside a transaction scoped to it.
// authz.Service implements it.
type Principals interface {
	ActivePrincipalsTx(ctx context.Context, tx pgx.Tx, org uuid.UUID) ([]authz.Principal, error)
}

// SenderDeps are what a Sender needs.
type SenderDeps struct {
	Store      *store.Store
	Principals Principals
	// Provider is the router (TELEGRAM_MESSAGES=linked): it sends a TelegramOnly message to the
	// staff user's linked chat or nowhere. A WhatsApp adapter would ignore TelegramOnly, so the
	// worker builds a Sender only when the router is in front.
	Provider messaging.Provider
	// Clock decides the sending hours and sent_at.
	Clock clock.Clock
	Log   *slog.Logger
}

// Sender sends recorded staff notices (the staff:notice task).
//
// Sending is the platform's own work: it checks no member's permission and writes no audit row;
// the notice row is the record. Every read and write runs in the notice's organization scope.
// It is at most once: in one transaction the notice is checked, its recipients are decided and it
// is marked sent, and only after that commits do the messages go out.
type Sender struct {
	store      *store.Store
	principals Principals
	provider   messaging.Provider
	clock      clock.Clock
	log        *slog.Logger
}

// NewSender returns a Sender.
func NewSender(d SenderDeps) *Sender {
	return &Sender{store: d.Store, principals: d.Principals, provider: d.Provider, clock: d.Clock, log: d.Log}
}

// prepared is a notice ready to go once its row is marked sent.
type prepared struct {
	outcome    Outcome
	reason     string // why it was skipped, for the log
	kind       string
	msg        messaging.Message
	recipients []uuid.UUID
}

// Send sends one notice of org. A notice that is not pending does nothing (OutcomeSettled), so a
// second delivery of its task sends nothing; outside the sending hours nothing is read or changed
// (OutcomeDeferred).
func (s *Sender) Send(ctx context.Context, org, noticeID uuid.UUID) (Outcome, error) {
	if !InSendingHours(s.clock.Now()) {
		s.log.InfoContext(ctx, "staff notice held until the morning", "notice", noticeID, "organization", org)
		return OutcomeDeferred, nil
	}
	var p prepared
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.LockStaffNotice(ctx, dbgen.LockStaffNoticeParams{OrganizationID: org, ID: noticeID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoticeNotFound
		}
		if err != nil {
			return fmt.Errorf("staffnotice: loading the notice: %w", err)
		}
		if row.Status != StatusPending {
			p = prepared{outcome: OutcomeSettled, kind: row.Kind}
			return nil
		}
		p, err = s.prepare(ctx, tx, org, row)
		if err != nil {
			return err
		}
		status, at := StatusSkipped, (*time.Time)(nil)
		if p.outcome == OutcomeSent {
			now := s.clock.Now()
			status, at = StatusSent, &now
		}
		n, err := q.MarkStaffNotice(ctx, dbgen.MarkStaffNoticeParams{OrganizationID: org, ID: noticeID, Status: status, SentAt: at})
		if err != nil {
			return fmt.Errorf("staffnotice: marking the notice %s: %w", status, err)
		}
		if n != 1 {
			return fmt.Errorf("staffnotice: marking the notice %s changed %d rows", status, n)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	s.deliver(ctx, org, noticeID, p)
	return p.outcome, nil
}

// prepare decides, inside tx, whether the locked pending notice still applies and to whom it goes,
// and builds its message.
func (s *Sender) prepare(ctx context.Context, tx pgx.Tx, org uuid.UUID, row dbgen.LockStaffNoticeRow) (prepared, error) {
	q := dbgen.New(tx)
	template, params, skip, err := s.event(ctx, q, org, row)
	if err != nil {
		return prepared{}, err
	}
	if skip != "" {
		return prepared{outcome: OutcomeSkipped, kind: row.Kind, reason: skip}, nil
	}

	leaseRows, err := q.ListNoticeLeases(ctx, dbgen.ListNoticeLeasesParams{OrganizationID: org, TenantID: row.TenantID})
	if err != nil {
		return prepared{}, fmt.Errorf("staffnotice: loading the tenant's leases: %w", err)
	}
	members, err := s.principals.ActivePrincipalsTx(ctx, tx, org)
	if err != nil {
		return prepared{}, fmt.Errorf("staffnotice: loading the members: %w", err)
	}
	leases := make([]Lease, len(leaseRows))
	for i, l := range leaseRows {
		leases[i] = Lease{PropertyID: l.PropertyID, CollectorUserID: l.CollectorStaffUserID.UUID, Active: l.Status == "active"}
	}
	recipients := Recipients(row.TenantID, leases, members)
	if len(recipients) == 0 {
		return prepared{outcome: OutcomeSkipped, kind: row.Kind, reason: "no member looks after the tenant"}, nil
	}

	who, err := q.GetMessageRecipient(ctx, dbgen.GetMessageRecipientParams{OrganizationID: org, TenantID: row.TenantID})
	if err != nil {
		return prepared{}, fmt.Errorf("staffnotice: loading the tenant: %w", err)
	}
	params["organization"], params["tenant"], params["unit"] = who.DisplayName, who.Name, units(leaseRows)
	msg := messaging.Message{OrganizationID: org, TelegramOnly: true, Template: template, Params: params}
	if _, err := messaging.Render(msg); err != nil {
		return prepared{}, fmt.Errorf("staffnotice: building the %s message: %w", template, err)
	}
	return prepared{outcome: OutcomeSent, kind: row.Kind, msg: msg, recipients: recipients}, nil
}

// event reads what the notice says, or why it no longer applies (skip). params lacks the
// organization, tenant and unit, which prepare adds.
func (s *Sender) event(ctx context.Context, q *dbgen.Queries, org uuid.UUID, row dbgen.LockStaffNoticeRow) (template string, params map[string]string, skip string, err error) {
	switch row.Kind {
	case KindPromiseMade:
		v, err := q.GetNoticePromiseVersion(ctx, dbgen.GetNoticePromiseVersionParams{OrganizationID: org, ID: row.PromiseVersionID.UUID})
		if err != nil {
			return "", nil, "", fmt.Errorf("staffnotice: reading the promise version: %w", err)
		}
		if v.PromiseStatus != "active" || !v.Latest {
			return "", nil, "the promise was edited again or closed", nil
		}
		action := messaging.StaffPromiseMade
		if v.Version > 1 {
			action = messaging.StaffPromiseEdited
		}
		amount := messaging.AllOutstanding
		if v.PromisedAmount != nil {
			amount = messaging.Rupiah(*v.PromisedAmount)
		}
		return messaging.TemplateStaffPromiseMade, map[string]string{
			"action": action, "amount": amount, "date": messaging.Date(v.PromisedDate),
		}, "", nil
	case KindPromiseBroken:
		// A closed promise is final, so a broken one always still applies.
		date, err := q.GetBrokenPromiseDate(ctx, dbgen.GetBrokenPromiseDateParams{OrganizationID: org, PromiseID: row.PromiseID.UUID})
		if err != nil {
			return "", nil, "", fmt.Errorf("staffnotice: reading the broken promise's date: %w", err)
		}
		return messaging.TemplateStaffPromiseBroken, map[string]string{"date": messaging.Date(date)}, "", nil
	case KindVisitTask:
		vt, err := q.GetNoticeVisitTask(ctx, dbgen.GetNoticeVisitTaskParams{OrganizationID: org, ID: row.VisitTaskID.UUID})
		if err != nil {
			return "", nil, "", fmt.Errorf("staffnotice: reading the visit task: %w", err)
		}
		if vt.ResolvedAt != nil {
			return "", nil, "the visit task was resolved", nil
		}
		reason, err := visitReason(ctx, q, org, row.TenantID, vt.Reason)
		if err != nil {
			return "", nil, "", err
		}
		return messaging.TemplateStaffVisitTask, map[string]string{"reason": reason}, "", nil
	default:
		return "", nil, "", fmt.Errorf("staffnotice: unknown kind %q", row.Kind)
	}
}

// visitReason says in Bahasa Indonesia why the tenant joined the visit list.
func visitReason(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, reason string) (string, error) {
	switch reason {
	case visitReasonH7:
		return "tunggakan lewat 7 hari", nil
	case visitReasonBrokenPromises:
		n, err := q.CountTenantBrokenPromisesSinceVisit(ctx, dbgen.CountTenantBrokenPromisesSinceVisitParams{OrganizationID: org, TenantID: tenantID})
		if err != nil {
			return "", fmt.Errorf("staffnotice: counting the broken promises: %w", err)
		}
		return brokenPromisesReason(n), nil
	case visitReasonUndeliverable:
		return "nomor WhatsApp-nya tidak bisa dihubungi", nil
	default:
		return "", fmt.Errorf("staffnotice: unknown visit task reason %q", reason)
	}
}

// brokenPromisesReason is the reason of a broken_promises visit task, with the number of promises
// broken since the tenant's last resolved one when it is known.
func brokenPromisesReason(n int64) string {
	if n < 1 {
		return "beberapa janji bayar tidak ditepati"
	}
	return "janji bayar tidak ditepati " + strconv.FormatInt(n, 10) + " kali"
}

// units names the tenant's units, with their property, for the notice: those of the active leases,
// or of every lease when none is active. leases come active first (ListNoticeLeases).
func units(leases []dbgen.ListNoticeLeasesRow) string {
	var names []string
	for _, l := range leases {
		if l.Status != "active" && len(names) > 0 {
			break
		}
		name := l.UnitLabel + ", " + l.PropertyName
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return noUnit
	}
	return strings.Join(names, "; ")
}

// deliver sends a notice whose row is committed as sent, one message per recipient, and logs what
// happened: counts, never a chat id, a phone or the text.
func (s *Sender) deliver(ctx context.Context, org, noticeID uuid.UUID, p prepared) {
	attrs := []any{"notice", noticeID, "kind", p.kind, "organization", org}
	switch p.outcome {
	case OutcomeSettled:
		s.log.InfoContext(ctx, "staff notice already settled", attrs...)
		return
	case OutcomeSkipped:
		s.log.InfoContext(ctx, "staff notice skipped", append(attrs, "reason", p.reason)...)
		return
	}
	var delivered, notLinked, failed int
	for _, user := range p.recipients {
		m := p.msg
		m.StaffUserID = user
		_, err := s.provider.SendTemplate(ctx, m)
		switch {
		case err == nil:
			delivered++
		case errors.Is(err, messaging.ErrNoTelegramChat):
			notLinked++
			s.log.DebugContext(ctx, "staff notice recipient not linked to Telegram", append(attrs, "staff_user_id", user)...)
		default:
			failed++
			// The router's and the Telegram client's errors carry codes, never the chat or text.
			s.log.WarnContext(ctx, "staff notice not delivered to a recipient", append(attrs, "staff_user_id", user, "error", err)...)
		}
	}
	s.log.InfoContext(ctx, "staff notice sent", append(attrs,
		"recipients", len(p.recipients), "delivered", delivered, "notLinked", notLinked, "failed", failed)...)
}

// Pending lists org's notices still pending SweepAfter after they were recorded, oldest first and
// at most sweepBatch: the sweep enqueues their tasks again.
func (s *Sender) Pending(ctx context.Context, org uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		ids, err = dbgen.New(tx).ListPendingStaffNotices(ctx, dbgen.ListPendingStaffNoticesParams{
			OrganizationID: org, Before: s.clock.Now().Add(-SweepAfter), RowLimit: sweepBatch,
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("staffnotice: listing pending notices: %w", err)
	}
	return ids, nil
}
