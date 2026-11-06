//go:build integration

package qris_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
)

// enqueued is one EnqueuePaid call.
type enqueued struct {
	Org           uuid.UUID
	TransactionID string
	Amount        int64
}

// paidQueue is a fake qris.PaidQueue that records what it accepted and refuses with err.
type paidQueue struct {
	mu    sync.Mutex
	calls []enqueued
	err   error
}

func (q *paidQueue) EnqueuePaid(_ context.Context, org uuid.UUID, transactionID string, amount int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.calls = append(q.calls, enqueued{Org: org, TransactionID: transactionID, Amount: amount})
	return nil
}

func (q *paidQueue) Calls() []enqueued {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]enqueued(nil), q.calls...)
}

// hexHMAC is X-Signature for body under key: lowercase hex HMAC-SHA256.
func hexHMAC(body []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// callbackFixture reads a captured AutoGoPay callback as the exact bytes it signed.
func callbackFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "autogopay", "testdata", name))
	require.NoError(t, err)
	return b
}

// paidCallback is a transaction.received body for txID at amount with status.
func paidCallback(txID string, amount int64, status string) []byte {
	return []byte(fmt.Sprintf(`{"event":"transaction.received","timestamp":"2026-11-06 08:38:43","transaction":`+
		`{"amount":%d,"order_id":"AUTOGOPAY-00000000-0000-4000-8000-0000000000b2","paid_at":"2026-11-06 08:38:21",`+
		`"payment_method":"QRIS","settlement_time":"2026-11-06 08:38:41","status":%q,"transaction_id":%q}}`,
		amount, status, txID))
}

// webhookID returns f's webhook id. Setting the same key again keeps the id.
func (f *flow) webhookID(t *testing.T) string {
	t.Helper()
	require.NotEmpty(t, f.key, "the organization needs an AutoGoPay key")
	id, err := f.env.AutoGoPay.SetAPIKey(context.Background(), f.org.ID, f.key)
	require.NoError(t, err)
	return id
}

func TestWebhookHandle(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	a := s.newFlow(t, true)
	b := s.newFlow(t, true)
	aID, bID := a.webhookID(t), b.webhookID(t)
	require.NotEqual(t, aID, bID)
	paid := callbackFixture(t, "webhook_paid.json")
	challenge := callbackFixture(t, "webhook_challenge.json")
	const fixtureTx = "00000000-0000-4000-8000-0000000000a1"
	log := slog.New(slog.DiscardHandler)

	// A's id with its first character replaced by a different one; always a real change, even when
	// the random id starts with 'x'.
	first := "x"
	if aID[0] == 'x' {
		first = "y"
	}
	changedID := first + aID[1:]

	refused := []struct {
		name      string
		webhookID string
		signature string
		body      []byte
	}{
		{"an unknown webhook id", strings.Repeat("Q", 43), hexHMAC(paid, a.key), paid},
		{"a malformed webhook id", "griya-sejahtera", hexHMAC(paid, a.key), paid},
		{"an empty webhook id", "", hexHMAC(paid, a.key), paid},
		{"A's id with one character changed", changedID, hexHMAC(paid, a.key), paid},
		{"a bad signature", aID, hexHMAC(paid, "agp-live-key-palsu"), paid},
		{"a missing signature", aID, "", paid},
		{"a signature over another body", aID, hexHMAC(challenge, a.key), paid},
		{"B's key on A's webhook id", aID, hexHMAC(paid, b.key), paid},
		{"A's key on B's webhook id", bID, hexHMAC(paid, a.key), paid},
		{"a signed body that is not JSON", aID, hexHMAC([]byte("not json"), a.key), []byte("not json")},
		{"a signed paid callback with no transaction", aID,
			hexHMAC([]byte(`{"event":"transaction.received"}`), a.key), []byte(`{"event":"transaction.received"}`)},
		{"a signed paid callback with amount 0", aID,
			hexHMAC(paidCallback(fixtureTx, 0, "PAID"), a.key), paidCallback(fixtureTx, 0, "PAID")},
	}
	for _, tc := range refused {
		t.Run(tc.name+" is refused and enqueues nothing", func(t *testing.T) {
			q := &paidQueue{}
			err := qris.NewWebhook(s.env.AutoGoPay, q, log).Handle(ctx, tc.webhookID, tc.signature, tc.body)
			require.ErrorIs(t, err, qris.ErrWebhookRefused)
			require.Empty(t, q.Calls())
		})
	}

	accepted := []struct {
		name      string
		webhookID string
		key       string
		body      []byte
		want      []enqueued
	}{
		{"a signed paid callback enqueues its transaction for A", aID, a.key, paid,
			[]enqueued{{Org: a.org.ID, TransactionID: fixtureTx, Amount: 1}}},
		{"the same callback to B's id with B's key enqueues for B", bID, b.key, paid,
			[]enqueued{{Org: b.org.ID, TransactionID: fixtureTx, Amount: 1}}},
		{"status settlement counts as paid", aID, a.key, paidCallback("tx-settlement", 700_000, "settlement"),
			[]enqueued{{Org: a.org.ID, TransactionID: "tx-settlement", Amount: 700_000}}},
		{"the challenge is accepted with nothing enqueued", aID, a.key, challenge, nil},
		{"a pending status is accepted and ignored", aID, a.key, paidCallback(fixtureTx, 700_000, "pending"), nil},
		{"an expired status is accepted and ignored", aID, a.key, paidCallback(fixtureTx, 700_000, "expire"), nil},
		{"an unknown event is accepted and ignored", aID, a.key, []byte(`{"event":"transaction.refunded"}`), nil},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			q := &paidQueue{}
			require.NoError(t, qris.NewWebhook(s.env.AutoGoPay, q, log).Handle(ctx, tc.webhookID, hexHMAC(tc.body, tc.key), tc.body))
			require.Equal(t, tc.want, q.Calls())
		})
	}

	t.Run("an uppercase hex signature is accepted", func(t *testing.T) {
		q := &paidQueue{}
		require.NoError(t, qris.NewWebhook(s.env.AutoGoPay, q, log).Handle(ctx, aID, strings.ToUpper(hexHMAC(paid, a.key)), paid))
		require.Len(t, q.Calls(), 1)
	})

	t.Run("a queue failure is our error, not a refusal", func(t *testing.T) {
		cause := errors.New("redis down")
		q := &paidQueue{err: cause}
		err := qris.NewWebhook(s.env.AutoGoPay, q, log).Handle(ctx, aID, hexHMAC(paid, a.key), paid)
		require.Error(t, err)
		require.ErrorIs(t, err, cause)
		require.NotErrorIs(t, err, qris.ErrWebhookRefused)
	})

}

// mismatchAudits are org's audit rows for the amount mismatch of attempt.
func (f *flow) mismatchAudits(t *testing.T, attempt uuid.UUID) int {
	t.Helper()
	n := 0
	for _, row := range f.env.AuditLog(t, f.org.ID) {
		if row.Action == authz.ActionPaymentRecord && row.Resource == "qris_amount_mismatch/"+attempt.String() {
			require.Equal(t, "allow", row.Decision)
			require.False(t, row.ActorStaffUserID.Valid, "the system records it, no member")
			n++
		}
	}
	return n
}

func TestSettlePaid(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	t.Run("records one payment allocated FIFO and marks the attempt paid, however often", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Rina Wulandari")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)

		for range 3 {
			require.NoError(t, f.svc.SettlePaid(ctx, f.org.ID, a.TransactionID, 700_000))
		}
		got := f.only(t, r)
		require.Equal(t, qris.AttemptPaid, got.Status)
		require.NotNil(t, got.PaidAmount)
		require.Equal(t, int64(700_000), *got.PaidAmount)
		require.False(t, got.AmountMismatch)
		f.requireQRISPayment(t, r, a.TransactionID, 700_000)
		require.Zero(t, f.mismatchAudits(t, a.ID))
	})

	t.Run("a partial QR pays the invoice in part", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Agus Pratama")
		f.makeQR(t, r, 250_000)
		a := f.only(t, r)

		require.NoError(t, f.svc.SettlePaid(ctx, f.org.ID, a.TransactionID, 250_000))
		f.requireQRISPayment(t, r, a.TransactionID, 250_000)
		require.False(t, f.only(t, r).AmountMismatch)
	})

	t.Run("after Check recorded it, still one payment", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Budi Santoso")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		require.NoError(t, f.fake.Pay(a.TransactionID))
		require.Equal(t, qris.StatusPaid, f.check(t, r).Status)

		require.NoError(t, f.svc.SettlePaid(ctx, f.org.ID, a.TransactionID, 700_000))
		f.requireQRISPayment(t, r, a.TransactionID, 700_000)
		require.Zero(t, f.mismatchAudits(t, a.ID))
	})

	t.Run("after ReconcileOrg recorded it, still one payment", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Siti Rahayu")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		require.NoError(t, f.fake.Pay(a.TransactionID))
		s.env.Clock.Advance(16 * time.Minute)
		require.Equal(t, qris.ReconcileResult{Checked: 1, Paid: 1}, f.reconcile(t))

		require.NoError(t, f.svc.SettlePaid(ctx, f.org.ID, a.TransactionID, 700_000))
		f.requireQRISPayment(t, r, a.TransactionID, 700_000)
		require.Equal(t, qris.AttemptPaid, f.only(t, r).Status)
	})

	t.Run("another amount is recorded as paid, flagged and audited", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Dewi Lestari")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)

		require.NoError(t, f.svc.SettlePaid(ctx, f.org.ID, a.TransactionID, 650_000))
		got := f.only(t, r)
		require.Equal(t, qris.AttemptPaid, got.Status)
		require.Equal(t, int64(700_000), got.Amount)
		require.Equal(t, int64(650_000), *got.PaidAmount)
		require.True(t, got.AmountMismatch)
		f.requireQRISPayment(t, r, a.TransactionID, 650_000)
		require.Equal(t, 1, f.mismatchAudits(t, a.ID))

		require.NoError(t, f.svc.SettlePaid(ctx, f.org.ID, a.TransactionID, 650_000))
		f.requireQRISPayment(t, r, a.TransactionID, 650_000)
		require.Equal(t, 1, f.mismatchAudits(t, a.ID), "a repeated webhook at the same amount audits nothing new")
	})

	t.Run("already recorded at the attempt's amount, another reported amount changes nothing but is audited", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Wati Kusuma")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		require.NoError(t, f.fake.Pay(a.TransactionID))
		require.Equal(t, qris.StatusPaid, f.check(t, r).Status)

		require.NoError(t, f.svc.SettlePaid(ctx, f.org.ID, a.TransactionID, 650_000))
		got := f.only(t, r)
		require.Equal(t, int64(700_000), *got.PaidAmount, "the recorded payment stays")
		require.False(t, got.AmountMismatch, "paid_amount still equals the attempt's amount")
		f.requireQRISPayment(t, r, a.TransactionID, 700_000)
		require.Equal(t, 1, f.mismatchAudits(t, a.ID))
	})

	t.Run("an unknown transaction id is ErrUnknownTransaction", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Hendra Gunawan")
		f.makeQR(t, r, 700_000)

		err := f.svc.SettlePaid(ctx, f.org.ID, uuid.NewString(), 700_000)
		require.ErrorIs(t, err, qris.ErrUnknownTransaction)
		f.requireNoPayment(t, r)
		require.Equal(t, qris.AttemptPending, f.only(t, r).Status)
	})

	t.Run("another organization's transaction id is ErrUnknownTransaction", func(t *testing.T) {
		a := s.newFlow(t, true)
		b := s.newFlow(t, true)
		r := a.rent(t, "Rina Wulandari")
		a.makeQR(t, r, 700_000)
		at := a.only(t, r)

		err := b.svc.SettlePaid(ctx, b.org.ID, at.TransactionID, 700_000)
		require.ErrorIs(t, err, qris.ErrUnknownTransaction)
		// The same service with A's id, but scoped to B, finds nothing either.
		err = a.svc.SettlePaid(ctx, b.org.ID, at.TransactionID, 700_000)
		require.ErrorIs(t, err, qris.ErrUnknownTransaction)
		a.requireNoPayment(t, r)
		require.Equal(t, qris.AttemptPending, a.only(t, r).Status)
	})
}
