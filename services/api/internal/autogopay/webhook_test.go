package autogopay_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

const webhookKey = "agp-live-key-griya-sejahtera"

// sign is the lowercase hex HMAC-SHA256 of body keyed with key, as AutoGoPay sends it.
func sign(body []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// rawFixture reads a captured callback as the exact bytes AutoGoPay signed.
func rawFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return b
}

func TestValidSignature(t *testing.T) {
	t.Parallel()
	body := rawFixture(t, "webhook_paid.json")
	good := sign(body, webhookKey)
	changed := append([]byte(nil), body...)
	changed[len(changed)/2] ^= 0x01

	tests := []struct {
		name      string
		body      []byte
		key       secret.Value
		signature string
		want      bool
	}{
		{"the right lowercase hex passes", body, secret.NewValue(webhookKey), good, true},
		{"the right uppercase hex passes", body, secret.NewValue(webhookKey), strings.ToUpper(good), true},
		{"a body changed by one byte fails", changed, secret.NewValue(webhookKey), good, false},
		{"the wrong key fails", body, secret.NewValue("agp-live-key-kos-melati"), good, false},
		{"an empty signature fails", body, secret.NewValue(webhookKey), "", false},
		{"a blank signature fails", body, secret.NewValue(webhookKey), "   ", false},
		{"a signature that is not hex fails", body, secret.NewValue(webhookKey), strings.Repeat("zz", sha256.Size), false},
		{"one extra hex character fails", body, secret.NewValue(webhookKey), good + "0", false},
		{"one extra hex byte fails", body, secret.NewValue(webhookKey), good + "00", false},
		{"one missing hex character fails", body, secret.NewValue(webhookKey), good[:len(good)-1], false},
		{"one missing hex byte fails", body, secret.NewValue(webhookKey), good[:len(good)-2], false},
		{"an empty key fails even with the empty key's own HMAC", body, secret.Value{}, sign(body, ""), false},
		{"an empty key value fails", body, secret.NewValue(""), sign(body, ""), false},
		{"a base64 signature fails", body, secret.NewValue(webhookKey), "c2lnbmF0dXJl", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, autogopay.ValidSignature(tc.body, tc.key, tc.signature))
		})
	}
}

func TestParseCallbackFixtures(t *testing.T) {
	t.Parallel()

	t.Run("the challenge gives only its event", func(t *testing.T) {
		t.Parallel()
		cb, err := autogopay.ParseCallback(rawFixture(t, "webhook_challenge.json"))
		require.NoError(t, err)
		require.Equal(t, autogopay.Callback{Event: autogopay.EventChallenge}, cb)
	})

	t.Run("the paid callback gives the transaction, paid at in Asia/Jakarta", func(t *testing.T) {
		t.Parallel()
		cb, err := autogopay.ParseCallback(rawFixture(t, "webhook_paid.json"))
		require.NoError(t, err)
		require.Equal(t, autogopay.EventTransactionReceived, cb.Event)
		require.NotNil(t, cb.Transaction)
		tx := cb.Transaction
		require.Equal(t, "00000000-0000-4000-8000-0000000000a1", tx.ID)
		require.Equal(t, autogopay.StatusPaid, tx.Status)
		require.Equal(t, int64(1), tx.Amount)
		want := time.Date(2026, 11, 6, 8, 38, 21, 0, clock.Jakarta)
		require.True(t, want.Equal(tx.PaidAt), "paid_at %s, want %s", tx.PaidAt, want)
		require.Equal(t, clock.Jakarta, tx.PaidAt.Location())
	})
}

// paidBody is a transaction.received body whose transaction object is tx.
func paidBody(tx string) []byte {
	return []byte(`{"event":"transaction.received","timestamp":"2026-11-06 08:38:43","transaction":` + tx + `}`)
}

func TestParseCallbackRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body []byte
	}{
		{"a body that is not JSON", []byte(`event=transaction.received`)},
		{"an empty body", nil},
		{"a JSON array", []byte(`[]`)},
		{"transaction.received with no transaction", []byte(`{"event":"transaction.received"}`)},
		{"transaction.received with a null transaction", []byte(`{"event":"transaction.received","transaction":null}`)},
		{"an empty transaction id", paidBody(`{"transaction_id":"","amount":700000,"status":"PAID"}`)},
		{"a blank transaction id", paidBody(`{"transaction_id":"   ","amount":700000,"status":"PAID"}`)},
		{"no transaction id", paidBody(`{"amount":700000,"status":"PAID"}`)},
		{"amount 0", paidBody(`{"transaction_id":"tx-1","amount":0,"status":"PAID"}`)},
		{"a negative amount", paidBody(`{"transaction_id":"tx-1","amount":-700000,"status":"PAID"}`)},
		{"a fractional amount", paidBody(`{"transaction_id":"tx-1","amount":1.5,"status":"PAID"}`)},
		{"an amount as a string", paidBody(`{"transaction_id":"tx-1","amount":"1","status":"PAID"}`)},
		{"no amount", paidBody(`{"transaction_id":"tx-1","status":"PAID"}`)},
		{"a null amount", paidBody(`{"transaction_id":"tx-1","amount":null,"status":"PAID"}`)},
		{"an unknown status", paidBody(`{"transaction_id":"tx-1","amount":700000,"status":"REFUNDED"}`)},
		{"no status", paidBody(`{"transaction_id":"tx-1","amount":700000}`)},
		{"an unreadable paid_at", paidBody(`{"transaction_id":"tx-1","amount":700000,"status":"PAID","paid_at":"06/11/2026 08:38"}`)},
		{"a paid_at with a zone", paidBody(`{"transaction_id":"tx-1","amount":700000,"status":"PAID","paid_at":"2026-11-06T08:38:21Z"}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cb, err := autogopay.ParseCallback(tc.body)
			require.ErrorIs(t, err, autogopay.ErrBadCallback)
			require.Equal(t, autogopay.Callback{}, cb)
		})
	}
}

func TestParseCallbackAccepts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body []byte
		want autogopay.Callback
	}{
		{
			name: "an unknown event gives only its event",
			body: []byte(`{"event":"transaction.refunded","transaction":{"transaction_id":"","amount":"x"}}`),
			want: autogopay.Callback{Event: "transaction.refunded"},
		},
		{
			name: "no event gives an empty event",
			body: []byte(`{"timestamp":"2026-11-06 08:30:02"}`),
			want: autogopay.Callback{},
		},
		{
			name: "status settlement is paid",
			body: paidBody(`{"transaction_id":"tx-1","amount":700000,"status":"settlement"}`),
			want: autogopay.Callback{Event: autogopay.EventTransactionReceived,
				Transaction: &autogopay.PaidTransaction{ID: "tx-1", Status: autogopay.StatusPaid, Amount: 700_000}},
		},
		{
			name: "status pending is read as pending",
			body: paidBody(`{"transaction_id":"tx-1","amount":700000,"status":"pending"}`),
			want: autogopay.Callback{Event: autogopay.EventTransactionReceived,
				Transaction: &autogopay.PaidTransaction{ID: "tx-1", Status: autogopay.StatusPending, Amount: 700_000}},
		},
		{
			name: "extra unknown fields are ignored",
			body: []byte(`{"event":"transaction.received","merchant":{"name":"Griya Sejahtera"},"transaction":` +
				`{"transaction_id":"tx-1","amount":700000,"status":"PAID","fee":3500,"customer":{"name":"Rina Wulandari"}}}`),
			want: autogopay.Callback{Event: autogopay.EventTransactionReceived,
				Transaction: &autogopay.PaidTransaction{ID: "tx-1", Status: autogopay.StatusPaid, Amount: 700_000}},
		},
		{
			name: "an order_id in the generate format is never read",
			body: paidBody(`{"transaction_id":"tx-1","order_id":"AUTOGOPAY-1793000000-1","amount":700000,"status":"PAID"}`),
			want: autogopay.Callback{Event: autogopay.EventTransactionReceived,
				Transaction: &autogopay.PaidTransaction{ID: "tx-1", Status: autogopay.StatusPaid, Amount: 700_000}},
		},
		{
			name: "an order_id of another type is never read",
			body: paidBody(`{"transaction_id":"tx-1","order_id":{"nested":[1,2]},"amount":700000,"status":"PAID"}`),
			want: autogopay.Callback{Event: autogopay.EventTransactionReceived,
				Transaction: &autogopay.PaidTransaction{ID: "tx-1", Status: autogopay.StatusPaid, Amount: 700_000}},
		},
		{
			name: "a missing paid_at gives a zero time",
			body: paidBody(`{"transaction_id":"tx-1","amount":1250000,"status":"PAID"}`),
			want: autogopay.Callback{Event: autogopay.EventTransactionReceived,
				Transaction: &autogopay.PaidTransaction{ID: "tx-1", Status: autogopay.StatusPaid, Amount: 1_250_000}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cb, err := autogopay.ParseCallback(tc.body)
			require.NoError(t, err)
			require.Equal(t, tc.want, cb)
		})
	}
}

func TestValidWebhookID(t *testing.T) {
	t.Parallel()
	good := strings.Repeat("aB3_-", 8) + "xyz" // 43 characters of base64url
	require.Len(t, good, 43)

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"43 base64url characters", good, true},
		{"43 letters", strings.Repeat("Q", 43), true},
		{"empty", "", false},
		{"42 characters", good[:42], false},
		{"44 characters", good + "a", false},
		{"base64 padding", good[:42] + "=", false},
		{"a plus sign (standard base64)", good[:42] + "+", false},
		{"a slash", good[:42] + "/", false},
		{"a dot", good[:42] + ".", false},
		{"a path traversal", "../" + good[:40], false},
		{"a space", good[:42] + " ", false},
		{"a trailing newline", good + "\n", false},
		{"a non-ASCII letter", good[:41] + "é", false},
		{"a uuid", "00000000-0000-4000-8000-0000000000a1", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, autogopay.ValidWebhookID(tc.id))
		})
	}
}
