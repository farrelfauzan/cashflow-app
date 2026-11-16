//go:build integration

package receipt_test

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

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/messagingtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/receipt"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/receipt/receipttask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder/remindertask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// counting is a provider that counts every call, accepted or refused, in front of a fake.
type counting struct {
	mu    sync.Mutex
	calls int
	fake  *messagingtest.Fake
}

func (c *counting) SendTemplate(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.fake.SendTemplate(ctx, m)
}

func (c *counting) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// queue is a fake receipt queue for billing and the sweep.
type queue struct {
	mu  sync.Mutex
	ids []uuid.UUID
}

func (q *queue) EnqueueReceipt(_ context.Context, _, id uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids = append(q.ids, id)
	return nil
}

func (q *queue) take() []uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.ids
	q.ids = nil
	return out
}

// reminderQueue is a fake remindertask.Queue; nothing in these tests is a reminder.
type reminderQueue struct{}

func (reminderQueue) EnqueueSend(context.Context, uuid.UUID, uuid.UUID) error    { return nil }
func (reminderQueue) EnqueueConfirm(context.Context, uuid.UUID, uuid.UUID) error { return nil }

// world is one organization with a kos, a tenant its karyawan collects from with an invoice of
// Rp700.000 due today, and the cash, receipt and sweep wiring.
type world struct {
	env      *testenv.Env
	org      testenv.Org
	admin    authz.Principal
	karyawan authz.Principal
	billing  *billing.Service
	queue    *queue
	provider *counting
	sender   *receipt.Sender
	logs     *bytes.Buffer
	tenantID uuid.UUID
	phone    string
}

func newWorld(t *testing.T, env *testenv.Env, name, prefix string) *world {
	t.Helper()
	ctx := context.Background()
	w := &world{env: env, org: env.CreateOrg(t, name, prefix), queue: &queue{},
		provider: &counting{fake: &messagingtest.Fake{}}, logs: &bytes.Buffer{}}
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	w.karyawan = env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	w.billing = billing.NewService(env.Store, env.Authz, env.Clock, env.Log, env.Promises, billing.SendReceipts(w.queue))
	w.sender = receipt.NewSender(receipt.SenderDeps{
		Store: env.Store, Provider: w.provider, Clock: env.Clock, Log: slog.New(slog.NewJSONHandler(w.logs, nil)),
	})
	kos, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	unit, err := env.Properties.CreateUnit(ctx, w.admin, kos.ID, property.UnitInput{Label: "Kamar 3", Terms: testenv.Monthly(700_000)})
	require.NoError(t, err)
	w.phone = prefix + "5001"
	tn, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Wati Susanti", Phone: w.phone})
	require.NoError(t, err)
	w.tenantID = tn.ID
	_, err = env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: tenant.Today(env.Clock), CollectorMemberID: w.karyawan.MembershipID}})
	require.NoError(t, err)
	_, err = env.Billing.GenerateOrg(ctx, w.org.ID)
	require.NoError(t, err)
	return w
}

// cash records amount of cash from the tenant as the karyawan.
func (w *world) cash(t *testing.T, amount int64) billing.CashResult {
	t.Helper()
	res, err := w.billing.RecordCashPayment(context.Background(), w.karyawan, billing.CashInput{
		RequestID: uuid.New(), TenantID: w.tenantID, Amount: amount,
		PhotoUploadID: w.env.CashPhoto(t, w.org.ID, w.karyawan.UserID, w.tenantID),
	})
	require.NoError(t, err)
	return res
}

// status is a receipt's status and sent_at.
func (w *world) status(t *testing.T, paymentID uuid.UUID) (string, *time.Time) {
	t.Helper()
	var st string
	var at *time.Time
	require.NoError(t, w.env.Store.WithOrgTx(context.Background(), w.org.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), "SELECT status, sent_at FROM payment_receipt WHERE payment_id = $1", paymentID).Scan(&st, &at)
	}))
	return st, at
}

func (w *world) send(t *testing.T, paymentID uuid.UUID) receipt.Outcome {
	t.Helper()
	out, err := w.sender.Send(context.Background(), w.org.ID, paymentID)
	require.NoError(t, err)
	return out
}

func setNow(env *testenv.Env, m time.Month, d, hour, minute int) {
	env.Clock.Set(time.Date(2026, m, d, hour, minute, 0, 0, clock.Jakarta))
}

func TestSendReceipt(t *testing.T) {
	env := testenv.New(t)
	setNow(env, 11, 14, 19, 30)
	w := newWorld(t, env, "Griya Sejahtera", "0812871")

	t.Run("the receipt goes to the tenant once, with what the cash paid", func(t *testing.T) {
		res := w.cash(t, 300_000)
		require.Equal(t, []uuid.UUID{res.Payment.ID}, w.queue.take(), "enqueued once the payment committed")
		st, _ := w.status(t, res.Payment.ID)
		require.Equal(t, billing.ReceiptPending, st)

		require.Equal(t, receipt.OutcomeSent, w.send(t, res.Payment.ID), "sent at once, even in the evening")
		sent := w.provider.fake.Sent()
		require.Len(t, sent, 1)
		m := sent[0]
		require.Equal(t, messaging.TemplatePaymentReceipt, m.Template)
		require.Equal(t, w.org.ID, m.OrganizationID)
		require.Equal(t, w.tenantID, m.TenantID, "the router can send it to a linked Telegram chat")
		require.Equal(t, "+62"+strings.TrimPrefix(w.phone, "0"), m.To)
		require.Empty(t, m.Buttons)
		require.Equal(t, map[string]string{
			"organization": "Griya Sejahtera", "name": "Wati Susanti", "amount": "Rp300.000", "date": "14 November 2026",
			"paid": "Tagihan November 2026 Rp300.000, sisa Rp400.000", "balance": "Sisa tagihan Anda sekarang Rp400.000.",
		}, m.Params)
		st, at := w.status(t, res.Payment.ID)
		require.Equal(t, billing.ReceiptSent, st)
		require.NotNil(t, at)
		require.True(t, env.Clock.Now().Equal(*at))

		require.Equal(t, receipt.OutcomeSettled, w.send(t, res.Payment.ID), "the task run twice sends once")
		require.Equal(t, 1, w.provider.count())
		require.NotContains(t, w.logs.String(), "Wati", "no message text in the log")
		require.NotContains(t, w.logs.String(), w.phone[1:], "no phone in the log")
	})

	t.Run("a refused send is not tried again", func(t *testing.T) {
		res := w.cash(t, 100_000)
		before := w.provider.count()
		w.provider.fake.FailWith(messaging.ErrUndeliverable)
		require.Equal(t, receipt.OutcomeUndelivered, w.send(t, res.Payment.ID))
		w.provider.fake.FailWith(nil)
		st, _ := w.status(t, res.Payment.ID)
		require.Equal(t, billing.ReceiptSent, st, "marked sent before the provider was called")
		require.Equal(t, receipt.OutcomeSettled, w.send(t, res.Payment.ID))
		require.Equal(t, before+1, w.provider.count(), "one call in all")
		w.queue.take()
	})

	t.Run("a crash after marking it sent never sends it twice", func(t *testing.T) {
		res := w.cash(t, 50_000)
		w.queue.take()
		// The sending transaction committed the mark, then the worker died before the provider call.
		require.NoError(t, env.Store.WithOrgTx(context.Background(), w.org.ID, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(), "UPDATE payment_receipt SET status = 'sent', sent_at = now() WHERE payment_id = $1", res.Payment.ID)
			return err
		}))
		before := w.provider.count()
		require.Equal(t, receipt.OutcomeSettled, w.send(t, res.Payment.ID))
		require.Equal(t, before, w.provider.count())
	})

	t.Run("a payment corrected to zero before the send is skipped", func(t *testing.T) {
		res := w.cash(t, 10_000)
		w.queue.take()
		delta := int64(-10_000)
		_, err := env.Billing.CreateCorrection(context.Background(), w.admin, billing.CorrectionInput{
			RequestID: uuid.New(), PaymentID: res.Payment.ID, AmountDelta: &delta, Reason: "salah catat",
		})
		require.NoError(t, err)
		before := w.provider.count()
		require.Equal(t, receipt.OutcomeSkipped, w.send(t, res.Payment.ID))
		st, at := w.status(t, res.Payment.ID)
		require.Equal(t, billing.ReceiptSkipped, st)
		require.Nil(t, at)
		require.Equal(t, before, w.provider.count())
	})

	t.Run("an unknown receipt is not found", func(t *testing.T) {
		_, err := w.sender.Send(context.Background(), w.org.ID, uuid.New())
		require.ErrorIs(t, err, receipt.ErrReceiptNotFound)
	})
}

// TestSweepReEnqueuesPendingReceipts: a receipt whose enqueue was lost is enqueued again by
// reminders:sweep once it has been pending for receipt.SweepAfter, and not after it is sent.
func TestSweepReEnqueuesPendingReceipts(t *testing.T) {
	env := testenv.New(t)
	setNow(env, 11, 14, 10, 0)
	w := newWorld(t, env, "Griya Sejahtera", "0812872")
	res := w.cash(t, 700_000)
	w.queue.take() // the enqueue after the commit is "lost"

	links := paylink.NewService(env.Store, env.Authz, env.Clock, "https://app.test")
	reminders := reminder.NewSender(reminder.SenderDeps{Store: env.Store, Links: links, Provider: w.provider, Clock: env.Clock, Log: env.Log})
	sweep := func() []uuid.UUID {
		t.Helper()
		_, err := remindertask.Sweep(context.Background(), env.Store, reminders, reminderQueue{},
			receipttask.Sweeper{Sender: w.sender, Queue: w.queue})
		require.NoError(t, err)
		return w.queue.take()
	}

	require.Empty(t, sweep(), "not yet: its task may still be on its way")
	setNow(env, 11, 14, 10, 16)
	require.Equal(t, []uuid.UUID{res.Payment.ID}, sweep())
	require.Equal(t, receipt.OutcomeSent, w.send(t, res.Payment.ID))
	require.Empty(t, sweep(), "a sent receipt is not enqueued again")
	require.Len(t, w.provider.fake.Sent(), 1)
}

// TestReceiptIsolation: one organization's sender never reaches another's receipt.
func TestReceiptIsolation(t *testing.T) {
	env := testenv.New(t)
	setNow(env, 11, 14, 10, 0)
	a := newWorld(t, env, "Griya Sejahtera", "0812873")
	b := newWorld(t, env, "Kos Bulanan Sejahtera", "0812874")
	pb := b.cash(t, 100_000)
	setNow(env, 11, 14, 10, 30)

	_, err := a.sender.Send(context.Background(), a.org.ID, pb.Payment.ID)
	require.ErrorIs(t, err, receipt.ErrReceiptNotFound, "b's receipt does not exist in a's scope")
	pending, err := a.sender.Pending(context.Background(), a.org.ID)
	require.NoError(t, err)
	require.Empty(t, pending)
	pending, err = b.sender.Pending(context.Background(), b.org.ID)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{pb.Payment.ID}, pending)
	require.Zero(t, a.provider.count())
	st, _ := b.status(t, pb.Payment.ID)
	require.Equal(t, billing.ReceiptPending, st)
}
