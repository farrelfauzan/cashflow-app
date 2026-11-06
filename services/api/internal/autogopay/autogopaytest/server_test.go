package autogopaytest_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay/autogopaytest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
)

// stand is the HTTP stand-in on a fake clock, with the real client pointed at it.
type stand struct {
	url    string
	clock  *clock.Fake
	client *autogopay.Client
}

func newStand(t *testing.T) stand {
	t.Helper()
	clk := clock.NewFake(start)
	srv := httptest.NewServer(autogopaytest.NewServer(autogopaytest.New(clk, autogopaytest.WithRandomIDs())))
	t.Cleanup(srv.Close)
	c, err := autogopay.NewClient(autogopay.Config{BaseURL: srv.URL})
	require.NoError(t, err)
	return stand{url: srv.URL, clock: clk, client: c}
}

type controlTx struct {
	TransactionID string    `json:"transactionId"`
	Amount        int64     `json:"amount"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

func (s stand) control(t *testing.T, path string, body any) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	resp := do(t, http.MethodPost, s.url+"/_control/"+path, &buf)
	defer resp.Body.Close()
	var out bytes.Buffer
	_, err := out.ReadFrom(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, out.Bytes()
}

func (s stand) transactions(t *testing.T) []controlTx {
	t.Helper()
	resp := do(t, http.MethodGet, s.url+"/_control/transactions", nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Transactions []controlTx `json:"transactions"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out.Transactions
}

// do sends one request with a JSON body (nil for none).
func do(t *testing.T, method, url string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// The real client reads every answer the stand-in gives, through a QR's whole life.
func TestServerSpeaksAutoGoPay(t *testing.T) {
	t.Parallel()
	s := newStand(t)
	ctx := t.Context()

	q, err := s.client.Generate(ctx, keyA, 150_000)
	require.NoError(t, err)
	require.Equal(t, int64(150_000), q.Amount)
	require.Equal(t, autogopay.StatusPending, q.Status)
	require.NotEmpty(t, q.QRString)
	require.True(t, q.CreatedAt.Equal(start))
	require.Equal(t, autogopaytest.QRValidity, q.ExpiresAt.Sub(q.CreatedAt))
	require.NotEqual(t, "00000000-0000-4000-8000-000000000001", q.TransactionID, "random ids, so a restart never repeats one")

	got, err := s.client.Status(ctx, keyA, q.TransactionID)
	require.NoError(t, err)
	require.Equal(t, autogopay.StatusPending, got.Status)

	_, err = s.client.Status(ctx, keyB, q.TransactionID)
	require.ErrorIs(t, err, autogopay.ErrTransactionNotFound, "another key does not see the QR")

	code, body := s.control(t, "pay", nil)
	require.Equal(t, http.StatusOK, code, string(body))
	got, err = s.client.Status(ctx, keyA, q.TransactionID)
	require.NoError(t, err)
	require.Equal(t, autogopay.StatusPaid, got.Status, "a paid QR reads settlement")
	got, err = s.client.Cancel(ctx, keyA, q.TransactionID)
	require.NoError(t, err)
	require.Equal(t, autogopay.StatusPaid, got.Status, "cancelling a paid QR reports paid")

	q2, err := s.client.Generate(ctx, keyA, 75_000)
	require.NoError(t, err)
	got, err = s.client.Cancel(ctx, keyA, q2.TransactionID)
	require.NoError(t, err)
	require.Equal(t, autogopay.StatusCancelled, got.Status)
	got, err = s.client.Status(ctx, keyA, q2.TransactionID)
	require.NoError(t, err)
	require.Equal(t, autogopay.StatusCancelled, got.Status)

	q3, err := s.client.Generate(ctx, keyA, 50_000)
	require.NoError(t, err)
	s.clock.Advance(autogopaytest.QRValidity)
	got, err = s.client.Status(ctx, keyA, q3.TransactionID)
	require.NoError(t, err)
	require.Equal(t, autogopay.StatusExpired, got.Status, "a QR expires after 15 minutes")

	_, err = s.client.Status(ctx, keyA, "00000000-0000-4000-8000-0000000000ff")
	require.ErrorIs(t, err, autogopay.ErrTransactionNotFound)
}

func TestServerRefusesAMissingKey(t *testing.T) {
	t.Parallel()
	s := newStand(t)
	for _, path := range []string{"/qris/generate", "/qris/status", "/qris/cancel"} {
		resp := do(t, http.MethodPost, s.url+path, bytes.NewBufferString(`{"amount":1}`))
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, path)
	}
}

func TestControlPayAndExpire(t *testing.T) {
	t.Parallel()
	s := newStand(t)
	ctx := t.Context()
	older, err := s.client.Generate(ctx, keyA, 100_000)
	require.NoError(t, err)
	newer, err := s.client.Generate(ctx, keyB, 200_000)
	require.NoError(t, err)

	all := s.transactions(t)
	require.Len(t, all, 2)
	require.Equal(t, newer.TransactionID, all[0].TransactionID, "newest first")
	require.Equal(t, controlTx{
		TransactionID: older.TransactionID, Amount: 100_000, Status: "pending",
		CreatedAt: all[1].CreatedAt, ExpiresAt: all[1].ExpiresAt,
	}, all[1])
	require.True(t, all[1].CreatedAt.Equal(start))
	require.True(t, all[1].ExpiresAt.Equal(start.Add(autogopaytest.QRValidity)))

	code, body := s.control(t, "expire", map[string]string{"transactionId": older.TransactionID})
	require.Equal(t, http.StatusOK, code, string(body))
	var tx controlTx
	require.NoError(t, json.Unmarshal(body, &tx))
	require.Equal(t, "expired", tx.Status)

	code, body = s.control(t, "pay", nil)
	require.Equal(t, http.StatusOK, code, string(body))
	require.NoError(t, json.Unmarshal(body, &tx))
	require.Equal(t, newer.TransactionID, tx.TransactionID, "no id pays the newest pending QR")
	require.Equal(t, "paid", tx.Status)

	code, _ = s.control(t, "pay", nil)
	require.Equal(t, http.StatusNotFound, code, "no pending QR left")
	code, _ = s.control(t, "expire", map[string]string{"transactionId": "nope"})
	require.Equal(t, http.StatusNotFound, code)
	code, _ = s.control(t, "pay", map[string]string{"id": "x"})
	require.Equal(t, http.StatusBadRequest, code, "unknown field")

	code, _ = s.control(t, "reset", nil)
	require.Equal(t, http.StatusNoContent, code)
	require.Empty(t, s.transactions(t))
}

func TestControlFailNext(t *testing.T) {
	t.Parallel()
	s := newStand(t)
	ctx := t.Context()

	code, body := s.control(t, "fail-next", map[string]any{"method": "generate", "amount": 950_123})
	require.Equal(t, http.StatusNoContent, code, string(body))
	_, err := s.client.Generate(ctx, keyA, 100_000)
	require.NoError(t, err, "another amount is not hit")
	_, err = s.client.Generate(ctx, keyA, 950_123)
	require.ErrorIs(t, err, autogopay.ErrUnavailable)
	q, err := s.client.Generate(ctx, keyA, 950_123)
	require.NoError(t, err, "the failure happens once")

	code, _ = s.control(t, "fail-next", map[string]string{"method": "status"})
	require.Equal(t, http.StatusNoContent, code)
	_, err = s.client.Status(ctx, keyA, q.TransactionID)
	require.ErrorIs(t, err, autogopay.ErrUnavailable)
	_, err = s.client.Status(ctx, keyA, q.TransactionID)
	require.NoError(t, err)

	code, _ = s.control(t, "fail-next", map[string]string{"method": "generate"})
	require.Equal(t, http.StatusNoContent, code)
	_, err = s.client.Generate(ctx, keyA, 1)
	require.ErrorIs(t, err, autogopay.ErrUnavailable, "with no amount, any generate")
	require.Len(t, s.transactions(t), 2, "a failed generate makes no QR")

	for name, body := range map[string]any{
		"unknown method":         map[string]string{"method": "transactions"},
		"no method":              map[string]string{},
		"amount with status":     map[string]any{"method": "status", "amount": 1},
		"negative amount":        map[string]any{"method": "generate", "amount": -1},
		"unknown field":          map[string]any{"method": "generate", "count": 2},
		"amount is not a number": map[string]any{"method": "generate", "amount": "1"},
	} {
		code, _ := s.control(t, "fail-next", body)
		require.Equal(t, http.StatusBadRequest, code, name)
	}
}
