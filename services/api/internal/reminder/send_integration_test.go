//go:build integration

package reminder_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/messagingtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder/remindertask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// queued is a fake send and confirm queue: it records the ids it is asked to enqueue.
type queued struct {
	mu            sync.Mutex
	sends         []uuid.UUID
	confirmations []uuid.UUID
}

func (q *queued) EnqueueSend(_ context.Context, _, id uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.sends = append(q.sends, id)
	return nil
}

func (q *queued) EnqueueConfirm(_ context.Context, _, id uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.confirmations = append(q.confirmations, id)
	return nil
}

// logBuffer is a JSON log the test reads back, to check the payment link never reaches it.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// sending is a world's sender with its provider, queue and log.
type sending struct {
	sender *reminder.Sender
	fake   *messagingtest.Fake
	queue  *queued
	logs   *logBuffer
}

// newSending wires a Sender for w through provider (a messagingtest.Fake when nil) as if
// WHATSAPP_PROVIDER were providerName, and gives w a decider and promise service that enqueue
// into its queue.
func newSending(t *testing.T, w *world, providerName string, provider messaging.Provider) *sending {
	t.Helper()
	s := &sending{fake: &messagingtest.Fake{}, queue: &queued{}, logs: &logBuffer{}}
	if provider == nil {
		provider = s.fake
	}
	log := slog.New(slog.NewJSONHandler(s.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	links := paylink.NewService(w.env.Store, w.env.Authz, w.env.Clock, "https://app.test")
	s.sender = reminder.NewSender(reminder.SenderDeps{
		Store: w.env.Store, Links: links, Provider: provider, ProviderName: providerName, Clock: w.env.Clock, Log: log,
	})
	w.decider = reminder.NewDecider(w.env.Store, w.env.Clock, log, s.queue)
	w.promises = promise.NewService(promise.Deps{
		Store: w.env.Store, Links: links, Tenants: w.env.Billing, Clock: w.env.Clock, Confirms: s.queue, Log: log,
	})
	return s
}

// requireNoLink checks that no payment link token, and no link at all, reached the log.
func (s *sending) requireNoLink(t *testing.T, msgs ...messaging.Message) {
	t.Helper()
	logs := s.logs.String()
	require.NotEmpty(t, logs)
	require.NotContains(t, logs, "/t/", "no payment link in the log")
	for _, m := range msgs {
		for _, b := range m.Buttons {
			_, token, ok := strings.Cut(b.URL, "/t/")
			require.True(t, ok)
			require.NotContains(t, logs, token)
		}
		for _, v := range m.Params {
			if strings.Contains(v, " ") { // a whole text parameter, not a single word
				require.NotContains(t, logs, v, "no message text in the log")
			}
		}
	}
}

// status is a reminder's status and sent_at.
func (w *world) status(t *testing.T, reminderID uuid.UUID) (string, *time.Time) {
	t.Helper()
	var st string
	var at *time.Time
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT status, sent_at FROM reminder WHERE id = $1", reminderID).Scan(&st, &at)
	}))
	return st, at
}

// confirmationStatus is a promise version's confirmation status.
func (w *world) confirmationStatus(t *testing.T, versionID uuid.UUID) string {
	t.Helper()
	var st string
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT status FROM promise_confirmation WHERE promise_version_id = $1", versionID).Scan(&st)
	}))
	return st
}

// links counts the organization's payment links.
func (w *world) links(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM payment_link").Scan(&n)
	}))
	return n
}

// phone is a tenant's phone number.
func (w *world) phone(t *testing.T, tenantID uuid.UUID) string {
	t.Helper()
	var p string
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT phone FROM tenant WHERE id = $1", tenantID).Scan(&p)
	}))
	return p
}

// requireFreshLink checks that m's one button carries a link minted by the platform for invoiceID:
// a payment_link row with the token's hash and no member.
func (w *world) requireFreshLink(t *testing.T, m messaging.Message, title string, invoiceID uuid.UUID) {
	t.Helper()
	require.Len(t, m.Buttons, 1)
	require.Equal(t, title, m.Buttons[0].Title)
	token, ok := strings.CutPrefix(m.Buttons[0].URL, "https://app.test/t/")
	require.True(t, ok, "the link opens our payment page")
	hash, ok := paylink.TokenHash(token)
	require.True(t, ok)
	var linkInvoice uuid.UUID
	var by uuid.NullUUID
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT invoice_id, created_by_staff_user_id FROM payment_link WHERE token_hash = $1", hash).Scan(&linkInvoice, &by)
	}))
	require.Equal(t, invoiceID, linkInvoice)
	require.False(t, by.Valid, "a reminder's link is the platform's, made by no member")
}

func send(t *testing.T, s *sending, w *world, reminderID uuid.UUID) reminder.Outcome {
	t.Helper()
	out, err := s.sender.Send(context.Background(), w.org.ID, reminderID)
	require.NoError(t, err)
	return out
}

// TestSendInvoiceReminders: a single invoice gets its stage's message with a fresh link, two
// invoices the rollover, an invoice paid between deciding and sending nothing; a second delivery
// sends nothing; an earlier day's row is not sent late.
func TestSendInvoiceReminders(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812981")
	s := newSending(t, w, "devlog", nil)
	due := day(2026, 11, 10)
	rina := w.renter(t, "Rina Wulandari", due)
	siti := w.renter(t, "Siti Rahayu", due, due)
	dewi := w.renter(t, "Dewi Lestari", due)
	env.Clock.Set(wib(time.November, 3, 9, 0))
	w.generate(t)
	rinaInv, sitiInv := w.invoices(t, rina), w.invoices(t, siti)

	require.Equal(t, reminder.Result{Tenants: 3, Reminders: 3, Stages: 4}, w.decide(t, wib(time.November, 7, 8, 0)))
	rinaRem, sitiRem, dewiRem := w.reminders(t, rina)[0].ID, w.reminders(t, siti)[0].ID, w.reminders(t, dewi)[0].ID
	require.ElementsMatch(t, []uuid.UUID{rinaRem, sitiRem, dewiRem}, s.queue.sends, "each row's send task, after its commit")

	env.Clock.Set(wib(time.November, 7, 8, 1))
	w.pay(t, dewi, 700_000)
	linksBefore := w.links(t)

	t.Run("H-3 of a single invoice: invoice_due_soon with a fresh platform link", func(t *testing.T) {
		env.Clock.Set(wib(time.November, 7, 8, 2))
		require.Equal(t, reminder.OutcomeSent, send(t, s, w, rinaRem))
		sent := s.fake.Sent()
		require.Len(t, sent, 1)
		m := sent[0]
		require.Equal(t, messaging.TemplateInvoiceDueSoon, m.Template)
		require.Equal(t, w.org.ID, m.OrganizationID)
		require.Equal(t, w.phone(t, rina), m.To)
		require.Equal(t, "Rina Wulandari", m.Params["name"])
		require.Equal(t, "Griya Sejahtera", m.Params["organization"])
		require.Equal(t, "November 2026", m.Params["month"])
		require.Equal(t, "Rp700.000", m.Params["amount"])
		require.Equal(t, "10 November 2026", m.Params["due_date"])
		require.Contains(t, m.Params["lines"], "Rp700.000")
		w.requireFreshLink(t, m, messaging.ButtonPay, rinaInv[0])
		require.Equal(t, linksBefore+1, w.links(t))
		st, at := w.status(t, rinaRem)
		require.Equal(t, reminder.StatusSent, st)
		require.NotNil(t, at)
		require.True(t, wib(time.November, 7, 8, 2).Equal(*at))
	})

	t.Run("a second delivery of the same task sends nothing", func(t *testing.T) {
		require.Equal(t, reminder.OutcomeSettled, send(t, s, w, rinaRem))
		require.Len(t, s.fake.Sent(), 1)
		require.Equal(t, linksBefore+1, w.links(t), "and mints no link")
	})

	t.Run("two invoices: one rollover message with their sum and one link to the oldest", func(t *testing.T) {
		require.Equal(t, reminder.OutcomeSent, send(t, s, w, sitiRem))
		sent := s.fake.Sent()
		require.Len(t, sent, 2)
		m := sent[1]
		require.Equal(t, messaging.TemplateInvoiceRollover, m.Template)
		require.Equal(t, "Tagihan November 2026 Rp1.400.000", m.Params["summary"])
		require.Equal(t, "Rp1.400.000", m.Params["total"])
		w.requireFreshLink(t, m, messaging.ButtonPay, sitiInv[0])
	})

	t.Run("an invoice paid between deciding and sending: skipped, nothing sent, no link", func(t *testing.T) {
		require.Equal(t, reminder.OutcomeSkipped, send(t, s, w, dewiRem))
		require.Len(t, s.fake.Sent(), 2)
		st, at := w.status(t, dewiRem)
		require.Equal(t, reminder.StatusSkipped, st)
		require.Nil(t, at)
		require.Equal(t, linksBefore+2, w.links(t))
	})

	t.Run("a partly paid invoice at H+3 is sent with what is left", func(t *testing.T) {
		env.Clock.Set(wib(time.November, 11, 9, 0))
		w.pay(t, rina, 200_000)
		w.decide(t, wib(time.November, 13, 8, 0))
		got := w.reminders(t, rina)
		require.Len(t, got, 2)
		require.Equal(t, reminder.OutcomeSent, send(t, s, w, got[1].ID))
		m := s.fake.Sent()[2]
		require.Equal(t, messaging.TemplateInvoiceOverdue, m.Template)
		require.Equal(t, "Rp500.000", m.Params["amount"])
		require.Equal(t, "3", m.Params["days_late"])
		require.Contains(t, m.Params["lines"], "sudah dibayar Rp200.000")
	})

	t.Run("a row from an earlier day is not sent late", func(t *testing.T) {
		got := w.reminders(t, siti)
		require.Len(t, got, 2, "H-3 and H+3")
		env.Clock.Set(wib(time.November, 14, 8, 0))
		require.Equal(t, reminder.OutcomeSkipped, send(t, s, w, got[1].ID))
		require.Len(t, s.fake.Sent(), 3)
	})

	s.requireNoLink(t, s.fake.Sent()...)
}

// TestSendPromiseMessages: the confirmation of each saved version once, the H-1 of a promise, and
// a broken notice ahead of the invoice stage it shares a row with.
func TestSendPromiseMessages(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812982")
	s := newSending(t, w, "devlog", nil)
	due := day(2026, 11, 10)
	rina := w.renter(t, "Rina Wulandari", due)
	dewi := w.renter(t, "Dewi Lestari", due)
	env.Clock.Set(wib(time.November, 3, 9, 0))
	w.generate(t)
	dewiInv := w.invoices(t, dewi)[0]

	env.Clock.Set(wib(time.November, 11, 9, 0))
	dewiPromise := w.promise(t, dewi, day(2026, 11, 12))
	amount := int64(300_000)
	d20 := day(2026, 11, 20)
	rinaV1, err := w.promises.MakeForTenant(ctx, w.admin, rina, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: &d20, PromisedAmount: &amount})
	require.NoError(t, err)
	rinaV2 := w.promise(t, rina, day(2026, 11, 21))
	require.Equal(t, []uuid.UUID{dewiPromise.Latest().ID, rinaV1.Latest().ID, rinaV2.Latest().ID}, s.queue.confirmations,
		"each saved version enqueues its confirmation after its commit")

	t.Run("promise_recorded: once per version", func(t *testing.T) {
		out, err := s.sender.Confirm(ctx, w.org.ID, dewiPromise.Latest().ID)
		require.NoError(t, err)
		require.Equal(t, reminder.OutcomeSent, out)
		out, err = s.sender.Confirm(ctx, w.org.ID, dewiPromise.Latest().ID)
		require.NoError(t, err)
		require.Equal(t, reminder.OutcomeSettled, out, "a second delivery sends nothing")
		sent := s.fake.Sent()
		require.Len(t, sent, 1)
		m := sent[0]
		require.Equal(t, messaging.TemplatePromiseRecorded, m.Template)
		require.Equal(t, w.phone(t, dewi), m.To)
		require.Equal(t, messaging.AllOutstanding, m.Params["amount"])
		require.Equal(t, "12 November 2026", m.Params["date"])
		w.requireFreshLink(t, m, messaging.ButtonChangePromise, dewiInv)
		require.Equal(t, reminder.StatusSent, w.confirmationStatus(t, dewiPromise.Latest().ID))
	})

	t.Run("an edited version's confirmation is skipped; the newest is sent", func(t *testing.T) {
		out, err := s.sender.Confirm(ctx, w.org.ID, rinaV1.Latest().ID)
		require.NoError(t, err)
		require.Equal(t, reminder.OutcomeSkipped, out)
		require.Equal(t, reminder.StatusSkipped, w.confirmationStatus(t, rinaV1.Latest().ID))
		out, err = s.sender.Confirm(ctx, w.org.ID, rinaV2.Latest().ID)
		require.NoError(t, err)
		require.Equal(t, reminder.OutcomeSent, out)
		sent := s.fake.Sent()
		require.Len(t, sent, 2)
		require.Equal(t, "21 November 2026", sent[1].Params["date"])
	})

	t.Run("H-1 of a promise: promise_reminder with Ubah janji", func(t *testing.T) {
		w.decide(t, wib(time.November, 12, 8, 0))
		got := w.reminders(t, dewi)
		require.Len(t, got, 1)
		require.Equal(t, uuid.NullUUID{UUID: dewiPromise.Latest().ID, Valid: true}, got[0].PromiseVersion)
		require.Equal(t, reminder.OutcomeSent, send(t, s, w, got[0].ID))
		m := s.fake.Sent()[2]
		require.Equal(t, messaging.TemplatePromiseReminder, m.Template)
		require.Equal(t, messaging.AllOutstanding, m.Params["amount"])
		require.Equal(t, "12 November 2026", m.Params["date"])
		w.requireFreshLink(t, m, messaging.ButtonChangePromise, dewiInv)
	})

	t.Run("a broken notice and an H+3 on one row: one message, the notice", func(t *testing.T) {
		env.Clock.Set(wib(time.November, 13, 0, 30))
		closed, err := env.Promises.CloseDue(ctx, w.org.ID)
		require.NoError(t, err)
		require.Equal(t, 1, closed.Broken)
		w.decide(t, wib(time.November, 13, 8, 0))
		got := w.reminders(t, dewi)
		require.Len(t, got, 2)
		require.Equal(t, uuid.NullUUID{UUID: dewiPromise.ID, Valid: true}, got[1].BrokenPromise)
		require.Len(t, got[1].Stages, 1, "the H+3 rides on the same row")
		require.Equal(t, reminder.OutcomeSent, send(t, s, w, got[1].ID))
		sent := s.fake.Sent()
		require.Len(t, sent, 4)
		m := sent[3]
		require.Equal(t, messaging.TemplatePromiseBroken, m.Template)
		require.Equal(t, "12 November 2026", m.Params["date"])
		require.Equal(t, "Rp700.000", m.Params["total"])
		w.requireFreshLink(t, m, messaging.ButtonNewPromise, dewiInv)
	})

	t.Run("an H-1 whose promise was kept before the send is skipped", func(t *testing.T) {
		w.decide(t, wib(time.November, 20, 8, 0))
		got := w.reminders(t, rina)
		h1 := got[len(got)-1]
		require.Equal(t, uuid.NullUUID{UUID: rinaV2.Latest().ID, Valid: true}, h1.PromiseVersion)
		env.Clock.Set(wib(time.November, 20, 8, 1))
		w.pay(t, rina, 700_000)
		require.Equal(t, reminder.OutcomeSkipped, send(t, s, w, h1.ID))
		require.Len(t, s.fake.Sent(), 4)
	})

	s.requireNoLink(t, s.fake.Sent()...)
}

// TestSendGuardHoldsBackGowa: with gowa configured, reminders and confirmations are marked
// skipped and nothing is sent or minted (decision 41.6).
func TestSendGuardHoldsBackGowa(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812983")
	s := newSending(t, w, "gowa", nil)
	rina := w.renter(t, "Rina Wulandari", day(2026, 11, 10))
	env.Clock.Set(wib(time.November, 3, 9, 0))
	w.generate(t)
	w.decide(t, wib(time.November, 7, 8, 0))
	rem := w.reminders(t, rina)[0].ID
	links := w.links(t)

	require.Equal(t, reminder.OutcomeHeld, send(t, s, w, rem))
	st, _ := w.status(t, rem)
	require.Equal(t, reminder.StatusSkipped, st)
	require.Equal(t, reminder.OutcomeSettled, send(t, s, w, rem))

	env.Clock.Set(wib(time.November, 7, 9, 0))
	p := w.promise(t, rina, day(2026, 11, 12))
	out, err := s.sender.Confirm(ctx, w.org.ID, p.Latest().ID)
	require.NoError(t, err)
	require.Equal(t, reminder.OutcomeHeld, out)
	require.Equal(t, reminder.StatusSkipped, w.confirmationStatus(t, p.Latest().ID))

	require.Empty(t, s.fake.Sent())
	require.Equal(t, links, w.links(t), "no link is minted for a message held back")
	require.Contains(t, s.logs.String(), "pacing guard")
}

// failingProvider records every message it is asked to send, with the reminder's status at that
// moment, and refuses them all.
type failingProvider struct {
	mu       sync.Mutex
	w        *world
	calls    []messaging.Message
	statuses []string
	err      error
}

func (p *failingProvider) SendTemplate(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, m)
	var st string
	err := p.w.env.Store.WithOrgTx(ctx, p.w.org.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT status FROM reminder WHERE status <> 'pending' ORDER BY sent_at DESC NULLS LAST LIMIT 1").Scan(&st)
	})
	if err != nil {
		st = "unreadable: " + err.Error()
	}
	p.statuses = append(p.statuses, st)
	return messaging.Receipt{}, p.err
}

// TestSendIsAtMostOnce: the row is committed as sent before the provider is called, so a provider
// error leaves it sent and a retry sends nothing; the error is logged without the link.
func TestSendIsAtMostOnce(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantLog string
	}{
		{name: "undeliverable", err: messaging.ErrUndeliverable, wantLog: "undeliverable"},
		{name: "rate limited", err: messaging.ErrRateLimited, wantLog: "slow down"},
		{name: "not connected", err: messaging.ErrNotConnected, wantLog: "re-link"},
	}
	env := testenv.New(t)
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env.Clock.Set(wib(time.November, 2, 9, 0))
			w := newWorld(t, env, "Griya Sejahtera "+tc.name, "081298"+string(rune('4'+i)))
			p := &failingProvider{w: w, err: tc.err}
			s := newSending(t, w, "twilio", p)
			rina := w.renter(t, "Rina Wulandari", day(2026, 11, 10))
			env.Clock.Set(wib(time.November, 3, 9, 0))
			w.generate(t)
			w.decide(t, wib(time.November, 7, 8, 0))
			rem := w.reminders(t, rina)[0].ID

			require.Equal(t, reminder.OutcomeUndelivered, send(t, s, w, rem))
			require.Len(t, p.calls, 1)
			require.Equal(t, []string{reminder.StatusSent}, p.statuses, "the row was committed as sent before the provider call")
			st, _ := w.status(t, rem)
			require.Equal(t, reminder.StatusSent, st)

			require.Equal(t, reminder.OutcomeSettled, send(t, s, w, rem), "a retry")
			require.Len(t, p.calls, 1, "nothing is sent twice")

			require.Contains(t, s.logs.String(), tc.wantLog)
			s.requireNoLink(t, p.calls...)
		})
	}
}

// TestSweepFindsLostEnqueues: a reminder or confirmation whose task was never enqueued is found
// by the sweep once it has been pending SweepAfter, in its own organization only.
func TestSweepFindsLostEnqueues(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	a := newWorld(t, env, "Griya Sejahtera", "0812987")
	b := newWorld(t, env, "Kos Bulanan Sejahtera", "0812988")
	sa, sb := newSending(t, a, "devlog", nil), newSending(t, b, "devlog", nil)
	rina := a.renter(t, "Rina Wulandari", day(2026, 11, 10))
	dewi := b.renter(t, "Dewi Lestari", day(2026, 11, 10))
	env.Clock.Set(wib(time.November, 3, 9, 0))
	a.generate(t)
	b.generate(t)

	// a's enqueues are lost: its decider and promises have no queue.
	a.decider = reminder.NewDecider(env.Store, env.Clock, env.Log, nil)
	a.promises = promise.NewService(promise.Deps{Store: env.Store, Links: paylink.NewService(env.Store, env.Authz, env.Clock, "https://app.test"),
		Tenants: env.Billing, Clock: env.Clock})
	a.decide(t, wib(time.November, 7, 8, 0))
	b.decide(t, wib(time.November, 7, 8, 0))
	env.Clock.Set(wib(time.November, 7, 8, 5))
	p := a.promise(t, rina, day(2026, 11, 12))
	rem := a.reminders(t, rina)[0].ID

	sweep := func(at time.Time) *queued {
		env.Clock.Set(at)
		q := &queued{}
		_, err := remindertask.Sweep(ctx, env.Store, sa.sender, q)
		require.NoError(t, err)
		return q
	}

	q := sweep(wib(time.November, 7, 8, 14))
	require.Empty(t, q.sends, "the reminders are 14 minutes old")
	require.Empty(t, q.confirmations)

	q = sweep(wib(time.November, 7, 8, 16))
	require.Contains(t, q.sends, rem)
	require.Empty(t, q.confirmations, "the promise is 11 minutes old")
	bRem := b.reminders(t, dewi)[0].ID
	require.Contains(t, q.sends, bRem, "b's pending row is found through b's own scope")

	q = sweep(wib(time.November, 7, 8, 21))
	require.Equal(t, []uuid.UUID{p.Latest().ID}, q.confirmations)

	t.Run("the swept task sends", func(t *testing.T) {
		require.Equal(t, reminder.OutcomeSent, send(t, sa, a, rem))
		q := sweep(wib(time.November, 7, 8, 30))
		require.NotContains(t, q.sends, rem, "a sent row is not swept again")
	})

	t.Run("isolation: a's sender cannot reach b's reminder", func(t *testing.T) {
		_, err := sa.sender.Send(ctx, a.org.ID, bRem)
		require.ErrorIs(t, err, reminder.ErrReminderNotFound)
		st, _ := b.status(t, bRem)
		require.Equal(t, reminder.StatusPending, st)
		ra, rc, err := sa.sender.Pending(ctx, a.org.ID)
		require.NoError(t, err)
		require.NotContains(t, ra, bRem)
		require.Equal(t, []uuid.UUID{p.Latest().ID}, rc)
		require.Equal(t, reminder.OutcomeSent, send(t, sb, b, bRem))
		require.Len(t, sb.fake.Sent(), 1)
		require.Equal(t, b.org.ID, sb.fake.Sent()[0].OrganizationID)
		require.Len(t, sa.fake.Sent(), 1)
	})
}
