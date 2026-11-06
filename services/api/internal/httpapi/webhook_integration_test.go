//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay/autogopaytest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// webhookEnqueued is one EnqueuePaid call.
type webhookEnqueued struct {
	Org           uuid.UUID
	TransactionID string
	Amount        int64
}

// webhookQueue is a fake qris.PaidQueue that records what it accepted.
type webhookQueue struct {
	mu    sync.Mutex
	calls []webhookEnqueued
	err   error // refuses every call when set
}

func (q *webhookQueue) EnqueuePaid(_ context.Context, org uuid.UUID, transactionID string, amount int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.calls = append(q.calls, webhookEnqueued{Org: org, TransactionID: transactionID, Amount: amount})
	return nil
}

func (q *webhookQueue) Calls() []webhookEnqueued {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]webhookEnqueued(nil), q.calls...)
}

// newWebhookServer is newLinkServerWith plus Deps.Webhooks on the real AutoGoPay accounts and
// the recording queue q. Every log line, the webhook's included, goes to logs.
func newWebhookServer(t *testing.T, env *testenv.Env, logs io.Writer, q qris.PaidQueue) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	links := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin)
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing,
		PayLinks: links,
		QRIS: qris.NewService(qris.Deps{
			Store: env.Store, Links: links, Clock: env.Clock, Gateway: autogopaytest.New(env.Clock),
			Keys: env.AutoGoPay, Limiter: env.Limiter, Audit: env.Authz, Log: log,
		}),
		Webhooks:      qris.NewWebhook(env.AutoGoPay, q, log),
		PublicLimiter: env.Limiter,
		CookieSecure:  false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// webhookCall is one answer from the webhook.
type webhookCall struct {
	status      int
	contentType string
	body        string
}

// postWebhook posts body to webhookID with signature (no header when empty) and extra headers.
func postWebhook(t *testing.T, base, webhookID, signature string, body []byte, headers map[string]string) webhookCall {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+autogopay.WebhookPath(webhookID), bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set(autogopay.SignatureHeader, signature)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return webhookCall{status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), body: string(data)}
}

func signHex(body []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func webhookFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "autogopay", "testdata", name))
	require.NoError(t, err)
	return b
}

// requireAnswer checks the status and that the body is exactly {success, message} JSON.
func requireAnswer(t *testing.T, got webhookCall, status int, success bool) {
	t.Helper()
	require.Equal(t, status, got.status, got.body)
	require.Equal(t, "application/json", got.contentType)
	var answer map[string]any
	require.NoError(t, json.Unmarshal([]byte(got.body), &answer))
	require.Len(t, answer, 2, "only success and message: %s", got.body)
	require.Equal(t, success, answer["success"])
	require.IsType(t, "", answer["message"])
	for _, internal := range []string{"autogopay:", "qris:", "redis", "sql", "error:"} {
		require.NotContains(t, strings.ToLower(got.body), internal, "no internal text in the answer")
	}
}

// TestAutoGoPayWebhookOverHTTP covers POST /webhooks/autogopay/{webhookId} end to end, with the
// webhook id and the key kept out of the answers and the logs.
func TestAutoGoPayWebhookOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	logs := &lockedBuffer{}
	q := &webhookQueue{}
	srv := newWebhookServer(t, env, logs, q)

	a := env.CreateOrg(t, "Griya Sejahtera", "0812631")
	b := env.CreateOrg(t, "Kos Melati Indah", "0812632")
	const aKey, bKey = "agp-live-key-griya-sejahtera-7f3a", "agp-live-key-kos-melati-91c2"
	aID, err := env.AutoGoPay.SetAPIKey(ctx, a.ID, aKey)
	require.NoError(t, err)
	bID, err := env.AutoGoPay.SetAPIKey(ctx, b.ID, bKey)
	require.NoError(t, err)

	paid := webhookFixture(t, "webhook_paid.json")
	challenge := webhookFixture(t, "webhook_challenge.json")
	const fixtureTx = "00000000-0000-4000-8000-0000000000a1"

	t.Run("the challenge is answered 200 success true", func(t *testing.T) {
		got := postWebhook(t, srv.URL, aID, signHex(challenge, aKey), challenge, nil)
		requireAnswer(t, got, http.StatusOK, true)
		require.Empty(t, q.Calls())
	})

	t.Run("a signed paid callback is enqueued once per POST", func(t *testing.T) {
		before := len(q.Calls())
		got := postWebhook(t, srv.URL, aID, signHex(paid, aKey), paid, nil)
		requireAnswer(t, got, http.StatusOK, true)
		want := webhookEnqueued{Org: a.ID, TransactionID: fixtureTx, Amount: 1}
		require.Equal(t, []webhookEnqueued{want}, q.Calls()[before:])

		// A duplicate enqueues the same transaction again; the task id dedupes it in production
		// and the payment's unique key makes a late one harmless.
		again := postWebhook(t, srv.URL, aID, signHex(paid, aKey), paid, nil)
		requireAnswer(t, again, http.StatusOK, true)
		require.Equal(t, []webhookEnqueued{want, want}, q.Calls()[before:])
	})

	t.Run("an Origin header does not matter", func(t *testing.T) {
		before := len(q.Calls())
		got := postWebhook(t, srv.URL, aID, signHex(paid, aKey), paid, map[string]string{"Origin": "https://evil.example"})
		requireAnswer(t, got, http.StatusOK, true)
		require.Len(t, q.Calls()[before:], 1)
	})

	t.Run("B's paid callback is enqueued for B", func(t *testing.T) {
		before := len(q.Calls())
		got := postWebhook(t, srv.URL, bID, signHex(paid, bKey), paid, nil)
		requireAnswer(t, got, http.StatusOK, true)
		require.Equal(t, []webhookEnqueued{{Org: b.ID, TransactionID: fixtureTx, Amount: 1}}, q.Calls()[before:])
	})

	refused := []struct {
		name      string
		webhookID string
		signature string
		body      []byte
	}{
		{"an unknown webhook id", strings.Repeat("Q", 43), signHex(paid, aKey), paid},
		{"a malformed webhook id", "griya-sejahtera", signHex(paid, aKey), paid},
		{"a bad signature", aID, signHex(paid, "agp-live-key-palsu"), paid},
		{"a missing signature", aID, "", paid},
		{"B's key on A's URL", aID, signHex(paid, bKey), paid},
		{"a signed body that cannot be read", aID, signHex([]byte(`{"event":"transaction.received"}`), aKey),
			[]byte(`{"event":"transaction.received"}`)},
	}
	var first string
	for _, tc := range refused {
		t.Run(tc.name+" is the same 401", func(t *testing.T) {
			before := len(q.Calls())
			got := postWebhook(t, srv.URL, tc.webhookID, tc.signature, tc.body, nil)
			requireAnswer(t, got, http.StatusUnauthorized, false)
			if first == "" {
				first = got.body
			}
			require.Equal(t, first, got.body, "every refusal answers the identical body")
			require.Len(t, q.Calls(), before, "nothing enqueued")
		})
	}

	t.Run("a body over 64 KiB is 413", func(t *testing.T) {
		before := len(q.Calls())
		big := []byte(`{"event":"verification.challenge","pad":"` + strings.Repeat("x", 64<<10) + `"}`)
		got := postWebhook(t, srv.URL, aID, signHex(big, aKey), big, nil)
		requireAnswer(t, got, http.StatusRequestEntityTooLarge, false)
		require.Len(t, q.Calls(), before)
	})

	t.Run("a body of exactly 64 KiB is read", func(t *testing.T) {
		head, tail := `{"event":"verification.challenge","pad":"`, `"}`
		body := []byte(head + strings.Repeat("x", 64<<10-len(head)-len(tail)) + tail)
		require.Len(t, body, 64<<10)
		got := postWebhook(t, srv.URL, aID, signHex(body, aKey), body, nil)
		requireAnswer(t, got, http.StatusOK, true)
	})

	t.Run("GET is not handled", func(t *testing.T) {
		resp, err := http.Get(srv.URL + autogopay.WebhookPath(aID))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Contains(t, []int{http.StatusMethodNotAllowed, http.StatusNotFound}, resp.StatusCode)
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NotContains(t, string(data), `"success"`)
	})

	t.Run("a queue failure is a generic 500", func(t *testing.T) {
		failing := newWebhookServer(t, env, logs, &webhookQueue{err: errors.New("redis down: dial tcp 10.0.0.7:6379")})
		got := postWebhook(t, failing.URL, aID, signHex(paid, aKey), paid, nil)
		requireAnswer(t, got, http.StatusInternalServerError, false)
		failing.Close()
	})

	srv.Close() // waits for every request, so every log line is written
	out := logs.String()
	require.NotEmpty(t, out)
	for _, secretText := range []string{aID, bID, aKey, bKey} {
		require.NotContains(t, out, secretText, "the webhook id and the key never reach the logs")
	}
	require.Contains(t, out, "/webhooks/autogopay/{webhookID}", "the request log records the route pattern")
}
