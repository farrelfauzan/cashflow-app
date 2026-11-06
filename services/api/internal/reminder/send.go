package reminder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// Statuses of a reminder or a promise confirmation.
const (
	StatusPending = "pending"
	StatusSent    = "sent"
	StatusSkipped = "skipped"
)

// SweepAfter is how long a reminder or a promise confirmation may stay pending before the sweep
// enqueues its task again: its first enqueue was lost (the worker or Redis went away between the
// commit and the enqueue).
const SweepAfter = 15 * time.Minute

// sweepBatch bounds how many rows one sweep enqueues per organization and table.
const sweepBatch = 500

var (
	// ErrReminderNotFound means the organization has no such reminder.
	ErrReminderNotFound = errors.New("reminder: reminder not found")
	// ErrConfirmationNotFound means the organization has no confirmation for that promise version.
	ErrConfirmationNotFound = errors.New("reminder: promise confirmation not found")
)

// Outcome is what one send did.
type Outcome string

// Outcomes of Send and Confirm.
const (
	// OutcomeSent: the row is marked sent and the provider accepted the message.
	OutcomeSent Outcome = "sent"
	// OutcomeUndelivered: the row is marked sent, then the provider refused the message. It is
	// not tried again (at most once); the error is logged.
	OutcomeUndelivered Outcome = "undelivered"
	// OutcomeSkipped: nothing it carried still applies, or it is from an earlier day; the row is
	// marked skipped and nothing is sent.
	OutcomeSkipped Outcome = "skipped"
	// OutcomeHeld: the pacing guard (decision 41.6) held it back; the row is marked skipped and
	// nothing is sent.
	OutcomeHeld Outcome = "held"
	// OutcomeSettled: the row was already sent or skipped by an earlier delivery; nothing is done.
	OutcomeSettled Outcome = "settled"
)

// SendsThrough reports whether reminders and promise confirmations actually go out through the
// configured WhatsApp provider. Until slice 5 paces sends on the shared number, only devlog (the
// log) and twilio (the Twilio sandbox, development only) send them; with gowa they are recorded
// and marked skipped (decision 41.6). Login codes and member invitations are not affected.
func SendsThrough(provider string) bool {
	return provider == "devlog" || provider == "twilio"
}

// LinkMinter mints a payment link inside a transaction for the platform's own messages.
// paylink.Service implements it.
type LinkMinter interface {
	MintForPlatformTx(ctx context.Context, tx pgx.Tx, org, invoiceID uuid.UUID) (paylink.Link, error)
}

// SenderDeps are what a Sender needs.
type SenderDeps struct {
	Store    *store.Store
	Links    LinkMinter
	Provider messaging.Provider
	// ProviderName is WHATSAPP_PROVIDER, for the pacing guard (SendsThrough).
	ProviderName string
	// Clock decides today in Asia/Jakarta and sent_at.
	Clock clock.Clock
	Log   *slog.Logger
}

// Sender sends recorded reminders (reminder:send) and promise confirmations (promise:confirm)
// (slice 4, S4-5).
//
// Like deciding, sending is the platform's own work: it checks no member's permission and writes
// no audit row; the reminder or confirmation row is the record. Every read and write runs in the
// row's organization scope (WithOrgTx).
//
// Sending is at most once: in one transaction the row is checked, a fresh payment link is minted
// (decision 39) and the row is marked sent, and only after that commits is the provider called. A
// crash after the commit loses the message instead of sending it twice, and a provider error is
// logged, never retried. Slice 5's message log, with delivery status and pacing, replaces this.
//
// Nothing here logs the payment link, its token or the message text.
type Sender struct {
	store    *store.Store
	links    LinkMinter
	provider messaging.Provider
	sends    bool
	clock    clock.Clock
	log      *slog.Logger
}

// NewSender returns a Sender.
func NewSender(d SenderDeps) *Sender {
	return &Sender{
		store: d.Store, links: d.Links, provider: d.Provider, sends: SendsThrough(d.ProviderName),
		clock: d.Clock, log: d.Log,
	}
}

// prepared is a message ready to go once its row is marked sent.
type prepared struct {
	outcome Outcome // OutcomeSent when msg is to be sent; otherwise why not
	reason  string  // why it was skipped, for the log
	msg     messaging.Message
}

// Send sends one reminder of org. A reminder that is not pending does nothing (OutcomeSettled), so
// a second delivery of its task sends nothing.
//
// At send time it keeps only what still applies: an invoice stage whose invoice still owes
// something; a promise H-1 whose promise is still active, still at that version, with a covered
// invoice owing; a broken-promise notice while the tenant has no new active promise and owes
// something. A reminder recorded on an earlier day is not sent late: the next run decides afresh.
// With nothing left the reminder is marked skipped.
//
// One reminder is one message, chosen by priority: the broken notice (it asks for a new promise),
// then the promise H-1, then the invoice stages (invoiceMessage). The others it carried are not
// sent separately: the message's link opens the page with everything the tenant owes.
func (s *Sender) Send(ctx context.Context, org, reminderID uuid.UUID) (Outcome, error) {
	var p prepared
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.LockReminder(ctx, dbgen.LockReminderParams{OrganizationID: org, ID: reminderID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrReminderNotFound
		}
		if err != nil {
			return fmt.Errorf("reminder: loading the reminder: %w", err)
		}
		if row.Status != StatusPending {
			p = prepared{outcome: OutcomeSettled}
			return nil
		}
		p, err = s.prepareReminder(ctx, tx, org, row)
		if err != nil {
			return err
		}
		return s.settle(ctx, tx, &p, func(status string, at *time.Time) (int64, error) {
			return q.MarkReminder(ctx, dbgen.MarkReminderParams{OrganizationID: org, ID: reminderID, Status: status, SentAt: at})
		})
	})
	if err != nil {
		return "", err
	}
	return s.deliver(ctx, org, p, "reminder", reminderID), nil
}

// Confirm sends the "janji tercatat" confirmation of a promise version of org. A confirmation
// that is not pending does nothing (OutcomeSettled). It is skipped when the version is no longer
// the newest of an active promise (edited again, or kept already), when the tenant owes nothing,
// or when it was saved on an earlier day.
func (s *Sender) Confirm(ctx context.Context, org, versionID uuid.UUID) (Outcome, error) {
	var p prepared
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.LockPromiseConfirmation(ctx, dbgen.LockPromiseConfirmationParams{OrganizationID: org, PromiseVersionID: versionID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConfirmationNotFound
		}
		if err != nil {
			return fmt.Errorf("reminder: loading the promise confirmation: %w", err)
		}
		if row.Status != StatusPending {
			p = prepared{outcome: OutcomeSettled}
			return nil
		}
		p, err = s.prepareConfirmation(ctx, tx, org, row)
		if err != nil {
			return err
		}
		return s.settle(ctx, tx, &p, func(status string, at *time.Time) (int64, error) {
			return q.MarkPromiseConfirmation(ctx, dbgen.MarkPromiseConfirmationParams{
				OrganizationID: org, PromiseVersionID: versionID, Status: status, SentAt: at,
			})
		})
	})
	if err != nil {
		return "", err
	}
	return s.deliver(ctx, org, p, "promise confirmation", versionID), nil
}

// settle marks the row: skipped when p is not to be sent, held back (skipped) by the guard, or
// sent with the clock's now. The row is locked and pending, so exactly one row changes.
func (s *Sender) settle(ctx context.Context, tx pgx.Tx, p *prepared, mark func(status string, at *time.Time) (int64, error)) error {
	if p.outcome == OutcomeSent && !s.sends {
		*p = prepared{outcome: OutcomeHeld, reason: "held back by the pacing guard until slice 5 (decision 41.6)"}
	}
	status, at := StatusSkipped, (*time.Time)(nil)
	if p.outcome == OutcomeSent {
		now := s.clock.Now()
		status, at = StatusSent, &now
	}
	n, err := mark(status, at)
	if err != nil {
		return fmt.Errorf("reminder: marking it %s: %w", status, err)
	}
	if n != 1 {
		return fmt.Errorf("reminder: marking it %s changed %d rows", status, n)
	}
	return nil
}

// deliver calls the provider for a prepared message whose row is committed as sent, and logs what
// happened. It never logs the message or its link.
func (s *Sender) deliver(ctx context.Context, org uuid.UUID, p prepared, kind string, id uuid.UUID) Outcome {
	switch p.outcome {
	case OutcomeSettled:
		s.log.InfoContext(ctx, kind+" already settled", "id", id, "organization", org)
		return p.outcome
	case OutcomeSkipped, OutcomeHeld:
		s.log.InfoContext(ctx, kind+" skipped", "id", id, "organization", org, "reason", p.reason)
		return p.outcome
	}
	_, err := s.provider.SendTemplate(ctx, p.msg)
	if err == nil {
		s.log.InfoContext(ctx, kind+" sent", "id", id, "organization", org, "template", p.msg.Template)
		return OutcomeSent
	}
	// The adapters' errors carry the provider's code, never the message text or its link.
	attrs := []any{"id", id, "organization", org, "template", p.msg.Template, "error", err}
	switch {
	case errors.Is(err, messaging.ErrUndeliverable):
		s.log.WarnContext(ctx, kind+" undeliverable: the tenant's number cannot receive WhatsApp", attrs...)
	case errors.Is(err, messaging.ErrRateLimited):
		s.log.WarnContext(ctx, kind+" not sent: WhatsApp asked us to slow down", attrs...)
	case errors.Is(err, messaging.ErrNotConnected):
		s.log.ErrorContext(ctx, kind+" not sent: the WhatsApp session is not connected; an operator must re-link it", attrs...)
	default:
		s.log.ErrorContext(ctx, kind+" not sent", attrs...)
	}
	return OutcomeUndelivered
}

// prepareReminder decides, inside tx under the tenant's allocation lock, what the locked pending
// reminder still says, and builds its message with a fresh link when something is left.
func (s *Sender) prepareReminder(ctx context.Context, tx pgx.Tx, org uuid.UUID, row dbgen.LockReminderRow) (prepared, error) {
	q := dbgen.New(tx)
	today := tenant.Today(s.clock)
	if !row.RunDay.Equal(today) {
		return prepared{outcome: OutcomeSkipped, reason: "decided on an earlier day"}, nil
	}
	if err := billing.LockTenant(ctx, q, org, row.TenantID); err != nil {
		return prepared{}, err
	}
	owing, err := loadOwing(ctx, q, org, row.TenantID)
	if err != nil {
		return prepared{}, err
	}
	if len(owing) == 0 {
		return prepared{outcome: OutcomeSkipped, reason: "the tenant owes nothing now"}, nil
	}
	rcpt, err := loadRecipient(ctx, q, org, row.TenantID)
	if err != nil {
		return prepared{}, err
	}

	var template string
	var params map[string]string
	button := messaging.ButtonPay
	if row.BrokenPromiseID.Valid {
		template, params, err = s.brokenNotice(ctx, q, org, row, rcpt, owing)
		button = messaging.ButtonNewPromise
	}
	if template == "" && err == nil && row.PromiseVersionID.Valid {
		template, params, err = s.promiseH1(ctx, q, org, row.PromiseVersionID.UUID, rcpt, owing)
		button = messaging.ButtonChangePromise
	}
	if template == "" && err == nil {
		template, params, err = s.invoiceStages(ctx, q, org, row.ID, rcpt, owing, today)
		button = messaging.ButtonPay
	}
	if err != nil {
		return prepared{}, err
	}
	if template == "" {
		return prepared{outcome: OutcomeSkipped, reason: "nothing it carried still applies"}, nil
	}
	return s.build(ctx, tx, org, rcpt, owing, template, params, button)
}

// brokenNotice is the broken-promise notice, or "" when it no longer applies: the tenant made a
// new promise since.
func (s *Sender) brokenNotice(ctx context.Context, q *dbgen.Queries, org uuid.UUID, row dbgen.LockReminderRow, rcpt Recipient, owing []OwingInvoice) (string, map[string]string, error) {
	active, err := q.TenantHasActivePromise(ctx, dbgen.TenantHasActivePromiseParams{OrganizationID: org, TenantID: row.TenantID})
	if err != nil {
		return "", nil, fmt.Errorf("reminder: reading the tenant's active promise: %w", err)
	}
	if active {
		return "", nil, nil
	}
	date, err := q.GetBrokenPromiseDate(ctx, dbgen.GetBrokenPromiseDateParams{OrganizationID: org, PromiseID: row.BrokenPromiseID.UUID})
	if err != nil {
		return "", nil, fmt.Errorf("reminder: reading the broken promise's date: %w", err)
	}
	return messaging.TemplatePromiseBroken, map[string]string{
		"name": rcpt.Name, "organization": rcpt.Organization, "date": messaging.Date(date), "total": messaging.Rupiah(total(owing)),
	}, nil
}

// promiseH1 is the H-1 of a promise version, or "" when it no longer applies: the promise closed
// or was edited, or nothing it covers owes anything.
func (s *Sender) promiseH1(ctx context.Context, q *dbgen.Queries, org, versionID uuid.UUID, rcpt Recipient, owing []OwingInvoice) (string, map[string]string, error) {
	v, err := q.GetPromiseVersionState(ctx, dbgen.GetPromiseVersionStateParams{OrganizationID: org, ID: versionID})
	if err != nil {
		return "", nil, fmt.Errorf("reminder: reading the promise version: %w", err)
	}
	if v.PromiseStatus != "active" || !v.Latest {
		return "", nil, nil
	}
	covered, err := q.ListPromiseCoveredInvoices(ctx, dbgen.ListPromiseCoveredInvoicesParams{OrganizationID: org, VersionIds: []uuid.UUID{versionID}})
	if err != nil {
		return "", nil, fmt.Errorf("reminder: reading the covered invoices: %w", err)
	}
	owes := map[uuid.UUID]bool{}
	for _, inv := range owing {
		owes[inv.ID] = true
	}
	for _, c := range covered {
		if owes[c.InvoiceID] {
			return messaging.TemplatePromiseReminder, map[string]string{
				"name": rcpt.Name, "organization": rcpt.Organization,
				"amount": promisedAmount(v.PromisedAmount), "date": messaging.Date(v.PromisedDate),
			}, nil
		}
	}
	return "", nil, nil
}

// invoiceStages is the message of the reminder's invoice stages whose invoices still owe, or ""
// when none does.
func (s *Sender) invoiceStages(ctx context.Context, q *dbgen.Queries, org, reminderID uuid.UUID, rcpt Recipient, owing []OwingInvoice, today time.Time) (string, map[string]string, error) {
	rows, err := q.ListReminderStages(ctx, dbgen.ListReminderStagesParams{OrganizationID: org, ReminderID: reminderID})
	if err != nil {
		return "", nil, fmt.Errorf("reminder: loading its invoice stages: %w", err)
	}
	owes := map[uuid.UUID]bool{}
	for _, inv := range owing {
		owes[inv.ID] = true
	}
	var stages []InvoiceStage
	for _, r := range rows {
		if owes[r.InvoiceID] {
			stages = append(stages, InvoiceStage{InvoiceID: r.InvoiceID, Stage: r.Stage})
		}
	}
	if len(stages) == 0 {
		return "", nil, nil
	}
	lines := map[uuid.UUID][]Line{}
	if len(owing) == 1 {
		rows, err := q.ListInvoiceLinesFor(ctx, dbgen.ListInvoiceLinesForParams{OrganizationID: org, InvoiceIds: []uuid.UUID{owing[0].ID}})
		if err != nil {
			return "", nil, fmt.Errorf("reminder: loading the invoice lines: %w", err)
		}
		for _, r := range rows {
			lines[r.InvoiceID] = append(lines[r.InvoiceID], Line{Label: r.Label, Amount: r.Amount})
		}
	}
	template, params := invoiceMessage(rcpt, stages, owing, lines, today)
	return template, params, nil
}

// prepareConfirmation decides, inside tx under the tenant's allocation lock, whether the locked
// pending confirmation still applies, and builds its message with a fresh link if so.
func (s *Sender) prepareConfirmation(ctx context.Context, tx pgx.Tx, org uuid.UUID, row dbgen.LockPromiseConfirmationRow) (prepared, error) {
	q := dbgen.New(tx)
	if !tenant.DayOf(row.CreatedAt).Equal(tenant.Today(s.clock)) {
		return prepared{outcome: OutcomeSkipped, reason: "saved on an earlier day"}, nil
	}
	if err := billing.LockTenant(ctx, q, org, row.TenantID); err != nil {
		return prepared{}, err
	}
	v, err := q.GetPromiseVersionState(ctx, dbgen.GetPromiseVersionStateParams{OrganizationID: org, ID: row.PromiseVersionID})
	if err != nil {
		return prepared{}, fmt.Errorf("reminder: reading the promise version: %w", err)
	}
	if v.PromiseStatus != "active" || !v.Latest {
		return prepared{outcome: OutcomeSkipped, reason: "the promise was edited again or closed"}, nil
	}
	owing, err := loadOwing(ctx, q, org, row.TenantID)
	if err != nil {
		return prepared{}, err
	}
	if len(owing) == 0 {
		return prepared{outcome: OutcomeSkipped, reason: "the tenant owes nothing now"}, nil
	}
	rcpt, err := loadRecipient(ctx, q, org, row.TenantID)
	if err != nil {
		return prepared{}, err
	}
	return s.build(ctx, tx, org, rcpt, owing, messaging.TemplatePromiseRecorded, map[string]string{
		"name": rcpt.Name, "organization": rcpt.Organization,
		"amount": promisedAmount(v.PromisedAmount), "date": messaging.Date(v.PromisedDate),
	}, messaging.ButtonChangePromise)
}

// build mints one fresh link to the tenant's oldest owing invoice (the page shows everything they
// owe) and makes the message, checked with messaging.Render so a mistake fails here, before the
// row is marked, rather than in the tenant's chat. With the guard holding sends back, no link is
// minted.
func (s *Sender) build(ctx context.Context, tx pgx.Tx, org uuid.UUID, rcpt Recipient, owing []OwingInvoice, template string, params map[string]string, button string) (prepared, error) {
	msg := messaging.Message{OrganizationID: org, To: rcpt.Phone, Template: template, Params: params}
	if !s.sends {
		return prepared{outcome: OutcomeSent, msg: msg}, nil // settle turns it into OutcomeHeld
	}
	link, err := s.links.MintForPlatformTx(ctx, tx, org, owing[0].ID)
	if err != nil {
		return prepared{}, fmt.Errorf("reminder: minting the payment link: %w", err)
	}
	msg.Buttons = []messaging.Button{{Title: button, Payload: "invoice:" + org.String() + ":" + owing[0].ID.String(), URL: link.URL}}
	if _, err := messaging.Render(msg); err != nil {
		return prepared{}, fmt.Errorf("reminder: building the %s message: %w", template, err)
	}
	return prepared{outcome: OutcomeSent, msg: msg}, nil
}

// loadOwing reads the tenant's invoices that still owe something, oldest period first.
func loadOwing(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID) ([]OwingInvoice, error) {
	rows, err := q.ListTenantOwingInvoices(ctx, dbgen.ListTenantOwingInvoicesParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("reminder: loading the owing invoices: %w", err)
	}
	out := make([]OwingInvoice, len(rows))
	for i, r := range rows {
		out[i] = OwingInvoice{ID: r.InvoiceID, Period: r.Period, DueDate: r.DueDate, Billed: r.Billed, Paid: r.Paid, Outstanding: r.Outstanding}
	}
	return out, nil
}

func loadRecipient(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID) (Recipient, error) {
	r, err := q.GetMessageRecipient(ctx, dbgen.GetMessageRecipientParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return Recipient{}, fmt.Errorf("reminder: loading the recipient: %w", err)
	}
	return Recipient{Name: r.Name, Phone: r.Phone, Organization: r.DisplayName}, nil
}

func total(owing []OwingInvoice) int64 {
	var t int64
	for _, inv := range owing {
		t += inv.Outstanding
	}
	return t
}

// Pending lists org's reminders and promise confirmations still pending SweepAfter after they were
// recorded, oldest first and at most sweepBatch of each: the sweep enqueues their tasks again.
func (s *Sender) Pending(ctx context.Context, org uuid.UUID) (reminders, confirmations []uuid.UUID, err error) {
	before := s.clock.Now().Add(-SweepAfter)
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		var err error
		reminders, err = q.ListPendingReminders(ctx, dbgen.ListPendingRemindersParams{OrganizationID: org, Before: before, RowLimit: sweepBatch})
		if err != nil {
			return fmt.Errorf("reminder: listing pending reminders: %w", err)
		}
		confirmations, err = q.ListPendingPromiseConfirmations(ctx, dbgen.ListPendingPromiseConfirmationsParams{
			OrganizationID: org, Before: before, RowLimit: sweepBatch,
		})
		if err != nil {
			return fmt.Errorf("reminder: listing pending promise confirmations: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return reminders, confirmations, nil
}
