package autogopay_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

const (
	testKey = "agp_test_key_not_real_7c1f"
	txID    = "00000000-0000-4000-8000-0000000000a1"
)

var key = secret.NewValue(testKey)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(b)
}

// reply is what the fake AutoGoPay answers.
type reply struct {
	status int
	body   string
	delay  time.Duration
}

// server is an httptest AutoGoPay that checks every request and counts them.
type server struct {
	requests atomic.Int32
	url      string
}

// newServer starts a fake AutoGoPay that expects a POST to wantPath with wantBody (JSON) and the
// test key, and answers with rep.
func newServer(t *testing.T, wantPath, wantBody string, rep reply) *server {
	t.Helper()
	s := &server{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		body, err := io.ReadAll(r.Body)
		if !assertRequest(t, r, body, wantPath, wantBody) || err != nil {
			http.Error(w, "unexpected request", http.StatusTeapot)
			return
		}
		if rep.delay > 0 {
			select {
			case <-time.After(rep.delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rep.status)
		_, _ = io.WriteString(w, rep.body)
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

func assertRequest(t *testing.T, r *http.Request, body []byte, wantPath, wantBody string) bool {
	t.Helper()
	ok := true
	check := func(cond bool, format string, args ...any) {
		if !cond {
			t.Errorf(format, args...)
			ok = false
		}
	}
	check(r.Method == http.MethodPost, "method = %s, want POST", r.Method)
	check(r.URL.Path == wantPath, "path = %s, want %s", r.URL.Path, wantPath)
	check(r.Header.Get("Authorization") == "Bearer "+testKey, "wrong Authorization header")
	check(r.Header.Get("Content-Type") == "application/json", "Content-Type = %q", r.Header.Get("Content-Type"))
	var got, want any
	check(json.Unmarshal(body, &got) == nil, "body is not JSON: %s", body)
	check(json.Unmarshal([]byte(wantBody), &want) == nil, "bad wantBody")
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	check(string(gotJSON) == string(wantJSON), "body = %s, want %s", body, wantBody)
	return ok
}

func newClient(t *testing.T, baseURL string, timeout time.Duration) *autogopay.Client {
	t.Helper()
	c, err := autogopay.NewClient(autogopay.Config{BaseURL: baseURL, Timeout: timeout})
	require.NoError(t, err)
	return c
}

func jakarta(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, clock.Jakarta)
	if err != nil {
		panic(err)
	}
	return t
}

func TestGenerate(t *testing.T) {
	t.Parallel()
	s := newServer(t, "/qris/generate", `{"amount":1}`, reply{status: http.StatusOK, body: fixture(t, "generate.json")})
	got, err := newClient(t, s.url, 0).Generate(t.Context(), key, 1)
	require.NoError(t, err)

	require.Equal(t, autogopay.QRIS{
		TransactionID: txID,
		OrderID:       "AUTOGOPAY-1762231189-1",
		Amount:        1,
		QRString:      "00020101021226TEST-NOT-A-REAL-QRIS6304ABCD",
		Status:        autogopay.StatusPending,
		CreatedAt:     jakarta("2026-11-04 11:39:49"),
		ExpiresAt:     jakarta("2026-11-04 11:54:49"),
	}, got)
	require.Equal(t, time.Date(2026, 11, 4, 4, 39, 49, 0, time.UTC), got.CreatedAt.UTC(), "WIB is UTC+7")
	require.Equal(t, 15*time.Minute, got.ExpiresAt.Sub(got.CreatedAt))
	require.Equal(t, int32(1), s.requests.Load())
}

func TestGenerateFailures(t *testing.T) {
	t.Parallel()
	generate := fixture(t, "generate.json")
	tests := []struct {
		name    string
		rep     reply
		timeout time.Duration
		wantErr error
		wantMsg string // must appear in the error text
	}{
		{name: "success false is rejected", rep: reply{status: 200, body: fixture(t, "generate_rejected.json")},
			wantErr: autogopay.ErrRejected, wantMsg: "Saldo merchant tidak aktif"},
		{name: "401 invalid key is rejected", rep: reply{status: 401, body: fixture(t, "error_401.json")},
			wantErr: autogopay.ErrRejected, wantMsg: "API key is required"},
		{name: "403 IP not allowed is rejected", rep: reply{status: 403, body: fixture(t, "error_403.json")},
			wantErr: autogopay.ErrRejected, wantMsg: "IP not allowed: 6.6.6.6"},
		{name: "404 is rejected", rep: reply{status: 404, body: "not found"}, wantErr: autogopay.ErrRejected},
		{name: "long 4xx message is truncated and cleaned",
			rep:     reply{status: 400, body: `{"success":false,"message":"bad\nline\t` + strings.Repeat("x", 300) + `"}`},
			wantErr: autogopay.ErrRejected, wantMsg: "badline" + strings.Repeat("x", 113) + "…"},
		{name: "500 HTML is unavailable", rep: reply{status: 500, body: "<html>oops</html>"}, wantErr: autogopay.ErrUnavailable},
		{name: "502 is unavailable", rep: reply{status: 502, body: "Bad Gateway"}, wantErr: autogopay.ErrUnavailable},
		{name: "429 is unavailable", rep: reply{status: 429, body: `{"success":false,"message":"slow down"}`}, wantErr: autogopay.ErrUnavailable},
		{name: "200 not JSON is unavailable", rep: reply{status: 200, body: "<html>maintenance</html>"}, wantErr: autogopay.ErrUnavailable},
		{name: "body over 1 MiB is unavailable",
			rep:     reply{status: 200, body: `{"success":true,"message":"` + strings.Repeat("a", 1<<20) + `"}`},
			wantErr: autogopay.ErrUnavailable},
		{name: "timeout is unavailable", rep: reply{status: 200, body: generate, delay: 2 * time.Second},
			timeout: 50 * time.Millisecond, wantErr: autogopay.ErrUnavailable},
		{name: "missing qr_string is a bad response",
			rep:     reply{status: 200, body: strings.Replace(generate, "00020101021226TEST-NOT-A-REAL-QRIS6304ABCD", "", 1)},
			wantErr: autogopay.ErrBadResponse},
		{name: "missing transaction_id is a bad response",
			rep:     reply{status: 200, body: strings.Replace(generate, `"transaction_id": "`+txID+`"`, `"transaction_id": ""`, 1)},
			wantErr: autogopay.ErrBadResponse},
		{name: "unreadable expiry_time is a bad response",
			rep:     reply{status: 200, body: strings.Replace(generate, "2026-11-04 11:54:49", "soon", 1)},
			wantErr: autogopay.ErrBadResponse},
		{name: "status other than pending is a bad response",
			rep:     reply{status: 200, body: strings.Replace(generate, `"pending"`, `"settlement"`, 1)},
			wantErr: autogopay.ErrBadResponse},
		{name: "another amount is a bad response",
			rep:     reply{status: 200, body: strings.Replace(generate, `"amount": 1,`, `"amount": 2,`, 1)},
			wantErr: autogopay.ErrBadResponse},
		{name: "3xx is not followed", rep: reply{status: 302, body: ""}, wantErr: autogopay.ErrBadResponse},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newServer(t, "/qris/generate", `{"amount":1}`, tc.rep)
			_, err := newClient(t, s.url, tc.timeout).Generate(t.Context(), key, 1)
			require.ErrorIs(t, err, tc.wantErr)
			if tc.wantMsg != "" {
				require.Contains(t, err.Error(), tc.wantMsg)
			}
			require.NotContains(t, err.Error(), testKey)
			require.Equal(t, int32(1), s.requests.Load(), "generate is sent exactly once, never retried")
		})
	}
}

func TestTransaction(t *testing.T) {
	t.Parallel()
	body := `{"transaction_id":"` + txID + `"}`
	tests := []struct {
		name       string
		cancel     bool
		file       string
		wantStatus autogopay.TransactionStatus
		wantErr    error
	}{
		{name: "status pending", file: "status_pending.json", wantStatus: autogopay.StatusPending},
		{name: "cancel answers cancelled", cancel: true, file: "cancel.json", wantStatus: autogopay.StatusCancelled},
		{name: "status after cancel", file: "status_after_cancel.json", wantStatus: autogopay.StatusCancelled},
		{name: "settlement is paid", file: "status_settlement.json", wantStatus: autogopay.StatusPaid},
		{name: "PAID is paid", file: "status_paid.json", wantStatus: autogopay.StatusPaid},
		{name: "cancel of a paid QR reports paid", cancel: true, file: "status_settlement.json", wantStatus: autogopay.StatusPaid},
		{name: "expire is expired", file: "status_expire.json", wantStatus: autogopay.StatusExpired},
		{name: "unknown id is not found", file: "status_unknown.json", wantErr: autogopay.ErrTransactionNotFound},
		{name: "unknown id on cancel is not found", cancel: true, file: "status_unknown.json", wantErr: autogopay.ErrTransactionNotFound},
		{name: "unknown status is a bad response", file: "status_unknown_value.json", wantErr: autogopay.ErrBadResponse},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, call := "/qris/status", (*autogopay.Client).Status
			if tc.cancel {
				path, call = "/qris/cancel", (*autogopay.Client).Cancel
			}
			s := newServer(t, path, body, reply{status: http.StatusOK, body: fixture(t, tc.file)})
			got, err := call(newClient(t, s.url, 0), t.Context(), key, txID)
			require.Equal(t, int32(1), s.requests.Load())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.NotContains(t, err.Error(), testKey)
				return
			}
			require.NoError(t, err)
			require.Equal(t, autogopay.Transaction{
				ID: txID, Status: tc.wantStatus, Time: jakarta("2026-11-04 11:39:49"),
			}, got)
		})
	}
}

func TestTransactionFailures(t *testing.T) {
	t.Parallel()
	body := `{"transaction_id":"` + txID + `"}`
	tests := []struct {
		name    string
		rep     reply
		wantErr error
	}{
		{name: "403 is rejected", rep: reply{status: 403, body: fixture(t, "error_403.json")}, wantErr: autogopay.ErrRejected},
		{name: "503 is unavailable", rep: reply{status: 503, body: ""}, wantErr: autogopay.ErrUnavailable},
		{name: "answer for another id is a bad response",
			rep:     reply{status: 200, body: strings.ReplaceAll(fixture(t, "status_pending.json"), txID, "00000000-0000-4000-8000-0000000000b2")},
			wantErr: autogopay.ErrBadResponse},
		{name: "no data is a bad response", rep: reply{status: 200, body: `{"success":false,"message":"x"}`}, wantErr: autogopay.ErrBadResponse},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newServer(t, "/qris/status", body, tc.rep)
			_, err := newClient(t, s.url, 0).Status(t.Context(), key, txID)
			require.ErrorIs(t, err, tc.wantErr)
			require.NotContains(t, err.Error(), testKey)
		})
	}
}

func TestRefusedBeforeAnyRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		call    func(*autogopay.Client) error
		wantErr error
	}{
		{name: "amount 0", wantErr: autogopay.ErrAmountOutOfRange, call: func(c *autogopay.Client) error {
			_, err := c.Generate(context.Background(), key, 0)
			return err
		}},
		{name: "amount above Rp10.000.000", wantErr: autogopay.ErrAmountOutOfRange, call: func(c *autogopay.Client) error {
			_, err := c.Generate(context.Background(), key, autogopay.MaxAmount+1)
			return err
		}},
		{name: "negative amount", wantErr: autogopay.ErrAmountOutOfRange, call: func(c *autogopay.Client) error {
			_, err := c.Generate(context.Background(), key, -5)
			return err
		}},
		{name: "generate with no key", wantErr: autogopay.ErrInvalidAPIKey, call: func(c *autogopay.Client) error {
			_, err := c.Generate(context.Background(), secret.Value{}, 1)
			return err
		}},
		{name: "status with no key", wantErr: autogopay.ErrInvalidAPIKey, call: func(c *autogopay.Client) error {
			_, err := c.Status(context.Background(), secret.Value{}, txID)
			return err
		}},
		{name: "status with empty id", wantErr: autogopay.ErrInvalidTransactionID, call: func(c *autogopay.Client) error {
			_, err := c.Status(context.Background(), key, "")
			return err
		}},
		{name: "cancel with blank id", wantErr: autogopay.ErrInvalidTransactionID, call: func(c *autogopay.Client) error {
			_, err := c.Cancel(context.Background(), key, "  ")
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
			t.Cleanup(srv.Close)
			err := tc.call(newClient(t, srv.URL, 0))
			require.ErrorIs(t, err, tc.wantErr)
			require.Zero(t, requests.Load(), "no request may be sent")
		})
	}
}

func TestGenerateAcceptsTheAmountLimits(t *testing.T) {
	t.Parallel()
	generate := fixture(t, "generate.json")
	for _, amount := range []int64{autogopay.MinAmount, autogopay.MaxAmount} {
		n := strconv.FormatInt(amount, 10)
		s := newServer(t, "/qris/generate", `{"amount":`+n+`}`,
			reply{status: http.StatusOK, body: strings.Replace(generate, `"amount": 1,`, `"amount": `+n+`,`, 1)})
		q, err := newClient(t, s.url, 0).Generate(t.Context(), key, amount)
		require.NoError(t, err)
		require.Equal(t, amount, q.Amount)
	}
}

func TestCallerCancellationIsContextCanceled(t *testing.T) {
	t.Parallel()
	s := newServer(t, "/qris/status", `{"transaction_id":"`+txID+`"}`, reply{status: 200, body: "{}", delay: 5 * time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := newClient(t, s.url, 0).Status(ctx, key, txID)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, autogopay.ErrUnavailable, "the caller gave up; AutoGoPay is not at fault")
}

func TestErrorsNeverContainTheKey(t *testing.T) {
	t.Parallel()
	// AutoGoPay echoing the key back in its message must not put it into our error.
	echo := `{"success":false,"message":"invalid key ` + testKey + `"}`
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
		s := newServer(t, "/qris/generate", `{"amount":1}`, reply{status: status, body: echo})
		_, err := newClient(t, s.url, 0).Generate(t.Context(), key, 1)
		require.ErrorIs(t, err, autogopay.ErrRejected)
		require.NotContains(t, err.Error(), testKey)
		require.Contains(t, err.Error(), "[redacted]")
	}
}

func TestNewClient(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     autogopay.Config
		wantErr bool
	}{
		{name: "defaults", cfg: autogopay.Config{}},
		{name: "explicit https", cfg: autogopay.Config{BaseURL: "https://example.test", Timeout: time.Second}},
		{name: "relative URL", cfg: autogopay.Config{BaseURL: "/qris"}, wantErr: true},
		{name: "other scheme", cfg: autogopay.Config{BaseURL: "ftp://example.test"}, wantErr: true},
		{name: "negative timeout", cfg: autogopay.Config{Timeout: -time.Second}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := autogopay.NewClient(tc.cfg)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestErrorsAreDistinct(t *testing.T) {
	t.Parallel()
	all := []error{autogopay.ErrUnavailable, autogopay.ErrRejected, autogopay.ErrTransactionNotFound,
		autogopay.ErrBadResponse, autogopay.ErrAmountOutOfRange, autogopay.ErrInvalidTransactionID}
	for i, a := range all {
		for j, b := range all {
			if i != j {
				require.False(t, errors.Is(a, b), "%v must not match %v", a, b)
			}
		}
	}
}
