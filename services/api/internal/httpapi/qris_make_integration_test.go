//go:build integration

package httpapi_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay/autogopaytest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// qrisCaller sends the tenant payment page's POST requests with no session. Unless a test names
// an address, each request comes from an address of its own, so the per-address limit stays out
// of the way of tests about something else.
type qrisCaller struct {
	base string
	sent int
}

func (c *qrisCaller) freshIP() string {
	c.sent++
	return fmt.Sprintf("10.1.%d.%d", c.sent/250, c.sent%250+1)
}

// send posts body (raw, so a test can send what is not JSON) to path with an optional Origin,
// from ip.
func (c *qrisCaller) send(t *testing.T, ip, origin, path, body string) (result, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, c.base+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", ip)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return result{StatusCode: resp.StatusCode, Header: resp.Header}, data
}

// post sends body to /api/t/{token}{route} from a fresh address, with no Origin, and requires the
// public page headers on whatever comes back.
func (c *qrisCaller) post(t *testing.T, token, route, body string) (result, []byte) {
	t.Helper()
	resp, data := c.send(t, c.freshIP(), "", "/api/t/"+token+route, body)
	requirePublicHeaders(t, resp)
	return resp, data
}

func amountBody(amount int64) string { return fmt.Sprintf(`{"amount":%d}`, amount) }

// requireAPIError checks an error answer's status, code and field ("" means no field).
func requireAPIError(t *testing.T, resp result, body []byte, status int, code, field string) {
	t.Helper()
	require.Equal(t, status, resp.StatusCode, string(body))
	e := decode[apigen.Error](t, body)
	require.Equal(t, code, e.Error.Code, string(body))
	require.NotEmpty(t, e.Error.Message)
	if field == "" {
		require.Nil(t, e.Error.Field, string(body))
		return
	}
	require.NotNil(t, e.Error.Field, string(body))
	require.Equal(t, field, *e.Error.Field)
}

// requireQris checks a 200 answer and returns what it says.
func requireQris(t *testing.T, resp result, body []byte, status apigen.QrisStatusStatus, amount int64) apigen.QrisStatus {
	t.Helper()
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	got := decode[apigen.QrisStatus](t, body)
	require.Equal(t, status, got.Status, string(body))
	require.NotNil(t, got.Amount, string(body))
	require.Equal(t, amount, *got.Amount)
	require.NotNil(t, got.ExpiresAt, string(body))
	if status == apigen.QrisStatusStatusChecking {
		require.NotNil(t, got.CheckedAt, "checkedAt is present while checking: %s", body)
	} else {
		require.Nil(t, got.CheckedAt, string(body))
		require.NotContains(t, string(body), "checkedAt", "checkedAt is left out unless checking")
	}
	return got
}

// gatewayCalls counts the fake's calls of method that used key.
func gatewayCalls(gw *autogopaytest.Fake, method autogopaytest.Method, key string) int {
	n := 0
	for _, c := range gw.Calls() {
		if c.Method == method && c.Key == key {
			n++
		}
	}
	return n
}

// TestQrisMakeAndCheckOverHTTP covers createPaymentQris and checkPaymentQris end to end against
// the fake AutoGoPay, with the token kept out of the logs.
func TestQrisMakeAndCheckOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 5, 10, 0, 0, 0, clock.Jakarta))
	logs := &lockedBuffer{}
	gw := autogopaytest.New(env.Clock)
	srv := newLinkServerWith(t, env, logs, gw)
	c := &qrisCaller{base: srv.URL}

	const keyA = "agp-key-griya-sejahtera"
	a := newLinkOrg(t, env, "Griya Sejahtera", "0812651")
	b := newLinkOrg(t, env, "Kos Bulanan Sejahtera", "0812652") // no AutoGoPay account
	env.SetAutoGoPayKey(t, a.o.ID, keyA)

	var tokens []string // every token made, to look for in the logs at the end
	renter := func(org *linkOrg, name string) qrisRenter {
		r := org.rentWithLink(t, name)
		tokens = append(tokens, r.token)
		return r
	}
	rina := renter(a, "Rina Wulandari")

	t.Run("makes a QR, shows it and reuses it for the same amount", func(t *testing.T) {
		now := env.Clock.Now()
		resp, body := c.post(t, rina.token, "/qris", amountBody(700_000))
		got := requireQris(t, resp, body, apigen.QrisStatusStatusPending, 700_000)
		require.True(t, now.Add(autogopaytest.QRValidity).Equal(*got.ExpiresAt), "expires %s", got.ExpiresAt)
		require.Equal(t, 1, gatewayCalls(gw, autogopaytest.MethodGenerate, keyA))
		require.NotContains(t, string(body), "FAKE-QRIS", "the QR string is only in the image")
		require.NotContains(t, string(body), "00000000-0000-4000-8000", "nor is the transaction id in the answer")

		resp, data := getPublic(t, srv.URL, "/api/t/"+rina.token+"/qris.png", c.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
		requirePublicHeaders(t, resp)
		require.Equal(t, "image/png", resp.Header.Get("Content-Type"))

		resp, data = getPublic(t, srv.URL, "/api/t/"+rina.token+"/status", c.freshIP())
		requireQris(t, resp, data, apigen.QrisStatusStatusPending, 700_000)

		resp, body = c.post(t, rina.token, "/qris", amountBody(700_000))
		again := requireQris(t, resp, body, apigen.QrisStatusStatusPending, 700_000)
		require.True(t, got.ExpiresAt.Equal(*again.ExpiresAt), "the same QR, not a new one")
		require.Equal(t, 1, gatewayCalls(gw, autogopaytest.MethodGenerate, keyA), "the fake recorded one generate")
	})

	t.Run("an amount that is not allowed is a 400 on the amount", func(t *testing.T) {
		tests := []struct {
			name   string
			body   string
			code   string
			status int
		}{
			{"zero", amountBody(0), "invalid_input", http.StatusBadRequest},
			{"missing", `{}`, "invalid_input", http.StatusBadRequest},
			{"negative", amountBody(-5_000), "invalid_input", http.StatusBadRequest},
			{"above the QRIS limit", amountBody(10_000_001), "qris_amount_above_limit", http.StatusBadRequest},
			{"above what is owed", amountBody(700_001), "qris_amount_above_outstanding", http.StatusBadRequest},
		}
		r := renter(a, "Budi Santoso")
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				before := len(gw.Calls())
				resp, body := c.post(t, r.token, "/qris", tc.body)
				requireAPIError(t, resp, body, tc.status, tc.code, "amount")
				require.Len(t, gw.Calls(), before, "AutoGoPay is never asked")
			})
		}
	})

	t.Run("a body that is not a valid request is a 400 invalid_json", func(t *testing.T) {
		tests := []struct {
			name string
			body string
		}{
			{"unknown field", `{"amount":700000,"note":"lunas"}`},
			{"amount with decimals", `{"amount":700000.5}`},
			{"amount as text", `{"amount":"700000"}`},
			{"not JSON", `amount=700000`},
			{"empty body", ``},
			{"two objects", `{"amount":700000}{"amount":1}`},
			{"an array", `[700000]`},
		}
		r := renter(a, "Siti Rahayu")
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				before := len(gw.Calls())
				resp, body := c.post(t, r.token, "/qris", tc.body)
				requireAPIError(t, resp, body, http.StatusBadRequest, "invalid_json", "")
				require.Len(t, gw.Calls(), before)
			})
		}
	})

	t.Run("a tenant who owes nothing gets 409 nothing_to_pay", func(t *testing.T) {
		r := renter(a, "Agus Prasetyo")
		_, err := env.Billing.RecordPayment(ctx, a.admin, billing.PaymentInput{
			RequestID: uuid.New(), TenantID: r.tenantID, Amount: 700_000, Method: billing.MethodTransfer,
			ReceivedOn: tenant.Today(env.Clock), Note: "Lunas November",
		})
		require.NoError(t, err)
		before := len(gw.Calls())
		resp, body := c.post(t, r.token, "/qris", amountBody(700_000))
		requireAPIError(t, resp, body, http.StatusConflict, "nothing_to_pay", "")
		require.Len(t, gw.Calls(), before)
	})

	t.Run("QRIS is unavailable without an AutoGoPay account", func(t *testing.T) {
		r := renter(b, "Dewi Lestari")
		before := len(gw.Calls())
		resp, body := c.post(t, r.token, "/qris", amountBody(700_000))
		requireAPIError(t, resp, body, http.StatusServiceUnavailable, "qris_unavailable", "")
		require.Equal(t, "QRIS sedang gangguan. Silakan bayar lewat transfer bank.", decode[apigen.Error](t, body).Error.Message)
		require.Len(t, gw.Calls(), before, "there is no key to call AutoGoPay with")
	})

	t.Run("an AutoGoPay failure is a 503 that does not stick", func(t *testing.T) {
		r := renter(a, "Hendra Gunawan")
		gw.FailNext(autogopaytest.MethodGenerate, autogopay.ErrUnavailable)
		before := gatewayCalls(gw, autogopaytest.MethodGenerate, keyA)
		resp, body := c.post(t, r.token, "/qris", amountBody(700_000))
		requireAPIError(t, resp, body, http.StatusServiceUnavailable, "qris_unavailable", "")
		require.Equal(t, before+1, gatewayCalls(gw, autogopaytest.MethodGenerate, keyA))

		resp, body = getPublic(t, srv.URL, "/api/t/"+r.token+"/status", c.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.JSONEq(t, `{"status":"none"}`, string(body), "no attempt was saved")

		resp, body = c.post(t, r.token, "/qris", amountBody(700_000))
		requireQris(t, resp, body, apigen.QrisStatusStatusPending, 700_000)
	})

	t.Run("link errors on both routes answer the link's 404", func(t *testing.T) {
		r := renter(a, "Maya Kusuma")
		_, err := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin).RevokeAll(ctx, a.admin, r.invoiceID)
		require.NoError(t, err)
		unknown := strings.Repeat("A", 43)
		tokens = append(tokens, unknown)
		tests := []struct{ name, token string }{
			{"garbage", "not-a-token"},
			{"well-formed but unknown", unknown},
			{"revoked", r.token},
		}
		routes := []struct{ route, body string }{
			{"/qris", amountBody(700_000)},
			{"/check", ``},
		}
		for _, tc := range tests {
			for _, rt := range routes {
				t.Run(tc.name+rt.route, func(t *testing.T) {
					before := len(gw.Calls())
					resp, body := c.post(t, tc.token, rt.route, rt.body)
					requireAPIError(t, resp, body, http.StatusNotFound, "payment_link_not_found", "")
					require.Equal(t, "Link pembayaran tidak ditemukan atau sudah tidak berlaku.", decode[apigen.Error](t, body).Error.Message)
					require.Len(t, gw.Calls(), before)
				})
			}
		}
	})

	t.Run("origin is checked on both routes", func(t *testing.T) {
		r := renter(a, "Lestari Putri")
		resp, body := c.post(t, r.token, "/qris", amountBody(700_000))
		requireQris(t, resp, body, apigen.QrisStatusStatusPending, 700_000)
		tests := []struct {
			name   string
			origin string
			status int
		}{
			{"an unknown origin", "https://evil.example", http.StatusForbidden},
			{"the allowed host over another scheme", "https://app.test", http.StatusForbidden},
			{"the allowed origin", origin, http.StatusOK},
			{"no Origin", "", http.StatusOK},
		}
		// A check that gets through marks the QR as being checked, which is what it answers.
		for _, rt := range []struct {
			route, body string
			want        apigen.QrisStatusStatus
		}{
			{"/qris", amountBody(700_000), apigen.QrisStatusStatusPending},
			{"/check", ``, apigen.QrisStatusStatusChecking},
		} {
			for _, tc := range tests {
				t.Run(tc.name+rt.route, func(t *testing.T) {
					before := len(gw.Calls())
					resp, body := c.send(t, c.freshIP(), tc.origin, "/api/t/"+r.token+rt.route, rt.body)
					require.Equal(t, tc.status, resp.StatusCode, string(body))
					require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
					require.Equal(t, "no-referrer", resp.Header.Get("Referrer-Policy"))
					if tc.status == http.StatusForbidden {
						requireAPIError(t, resp, body, http.StatusForbidden, "bad_origin", "")
						require.Len(t, gw.Calls(), before, "refused before anything runs")
						return
					}
					requireQris(t, resp, body, rt.want, 700_000)
				})
			}
		}
	})

	t.Run("check: no QR, then checking, then paid", func(t *testing.T) {
		r := renter(a, "Fitri Handayani")
		resp, body := c.post(t, r.token, "/check", ``)
		requireAPIError(t, resp, body, http.StatusNotFound, "qris_not_found", "")

		resp, body = c.post(t, r.token, "/qris", amountBody(700_000))
		requireQris(t, resp, body, apigen.QrisStatusStatusPending, 700_000)

		checkedAt := env.Clock.Now()
		resp, body = c.post(t, r.token, "/check", ``)
		checking := requireQris(t, resp, body, apigen.QrisStatusStatusChecking, 700_000)
		require.True(t, checkedAt.Equal(*checking.CheckedAt), "checkedAt %s, asked at %s", checking.CheckedAt, checkedAt)
		resp, body = getPublic(t, srv.URL, "/api/t/"+r.token+"/status", c.freshIP())
		later := requireQris(t, resp, body, apigen.QrisStatusStatusChecking, 700_000)
		require.True(t, checking.CheckedAt.Equal(*later.CheckedAt), "GET status shows the same checkedAt")
		var txID string
		for _, call := range gw.Calls() {
			if call.Method == autogopaytest.MethodStatus {
				txID = call.TransactionID // the last one is this tenant's
			}
		}
		require.NotEmpty(t, txID, "the check asked AutoGoPay once")
		acct, err := env.Billing.Account(ctx, a.admin, r.tenantID)
		require.NoError(t, err)
		require.Empty(t, acct.Payments, "still pending at AutoGoPay: nothing recorded")

		require.NoError(t, gw.Pay(txID))
		resp, body = c.post(t, r.token, "/check", ``)
		requireQris(t, resp, body, apigen.QrisStatusStatusPaid, 700_000)
		acct, err = env.Billing.Account(ctx, a.admin, r.tenantID)
		require.NoError(t, err)
		require.Len(t, acct.Payments, 1)
		require.EqualValues(t, 700_000, acct.Payments[0].Amount)
		require.Equal(t, billing.MethodQRIS, acct.Payments[0].Method)
		require.Zero(t, acct.Outstanding)
		resp, body = getPublic(t, srv.URL, "/api/t/"+r.token+"/status", c.freshIP())
		requireQris(t, resp, body, apigen.QrisStatusStatusPaid, 700_000)

		t.Run("and asking again records nothing new", func(t *testing.T) {
			resp, body := c.post(t, r.token, "/check", ``)
			requireQris(t, resp, body, apigen.QrisStatusStatusPaid, 700_000)
			acct, err := env.Billing.Account(ctx, a.admin, r.tenantID)
			require.NoError(t, err)
			require.Len(t, acct.Payments, 1)

			resp, body = c.post(t, r.token, "/qris", amountBody(700_000))
			requireAPIError(t, resp, body, http.StatusConflict, "nothing_to_pay", "")
		})
	})

	t.Run("check: a QR that expired at AutoGoPay closes", func(t *testing.T) {
		r := renter(a, "Joko Widodo")
		resp, body := c.post(t, r.token, "/qris", amountBody(300_000))
		requireQris(t, resp, body, apigen.QrisStatusStatusPending, 300_000)
		resp, body = c.post(t, r.token, "/check", ``)
		requireQris(t, resp, body, apigen.QrisStatusStatusChecking, 300_000)
		var txID string
		for _, call := range gw.Calls() {
			if call.Method == autogopaytest.MethodStatus {
				txID = call.TransactionID
			}
		}
		require.NoError(t, gw.Expire(txID))
		resp, body = c.post(t, r.token, "/check", ``)
		requireQris(t, resp, body, apigen.QrisStatusStatusExpired, 300_000)
		acct, err := env.Billing.Account(ctx, a.admin, r.tenantID)
		require.NoError(t, err)
		require.Empty(t, acct.Payments)
	})

	t.Run("both routes count against the page's limit per token", func(t *testing.T) {
		r := renter(a, "Sari Indah")
		for i := range 30 {
			if i%2 == 0 {
				resp, body := c.post(t, r.token, "/qris", amountBody(0))
				requireAPIError(t, resp, body, http.StatusBadRequest, "invalid_input", "amount")
				continue
			}
			resp, body := c.post(t, r.token, "/check", ``)
			requireAPIError(t, resp, body, http.StatusNotFound, "qris_not_found", "")
		}
		for _, route := range []string{"/qris", "/check"} {
			resp, body := c.post(t, r.token, route, amountBody(700_000))
			requireAPIError(t, resp, body, http.StatusTooManyRequests, "rate_limited", "")
		}
		resp, body := getPublic(t, srv.URL, "/api/t/"+r.token+"/status", c.freshIP())
		requireAPIError(t, resp, body, http.StatusTooManyRequests, "rate_limited", "")
		require.Equal(t, "no-store", resp.Header.Get("Cache-Control"), "the bucket is shared with the GET routes")
	})

	t.Run("both routes count against the page's limit per address", func(t *testing.T) {
		const ip = "198.51.100.77"
		for i := range 120 {
			token := fmt.Sprintf("%043d", i) // well-formed, unknown, and never over the per-token limit
			route := []string{"/qris", "/check"}[i%2]
			resp, body := c.send(t, ip, "", "/api/t/"+token+route, amountBody(700_000))
			requireAPIError(t, resp, body, http.StatusNotFound, "payment_link_not_found", "")
		}
		resp, body := c.send(t, ip, "", "/api/t/"+fmt.Sprintf("%043d", 999)+"/check", ``)
		requirePublicHeaders(t, resp)
		requireAPIError(t, resp, body, http.StatusTooManyRequests, "rate_limited", "")
		resp, body = c.send(t, "198.51.100.78", "", "/api/t/"+fmt.Sprintf("%043d", 999)+"/check", ``)
		requireAPIError(t, resp, body, http.StatusNotFound, "payment_link_not_found", "")
	})

	t.Run("a tenant gets 5 new QRs in 24 hours, and reusing one is free", func(t *testing.T) {
		r := renter(a, "Anton Wijaya")
		before := gatewayCalls(gw, autogopaytest.MethodGenerate, keyA)
		for i := int64(1); i <= qris.MaxGeneratesPerDay; i++ {
			amount := i * 100_000
			// Attempts are ordered by their creation time, so each one needs a moment of its own.
			env.Clock.Advance(time.Second)
			resp, body := c.post(t, r.token, "/qris", amountBody(amount))
			requireQris(t, resp, body, apigen.QrisStatusStatusPending, amount)
		}
		require.Equal(t, before+qris.MaxGeneratesPerDay, gatewayCalls(gw, autogopaytest.MethodGenerate, keyA))

		resp, body := c.post(t, r.token, "/qris", amountBody(500_000))
		requireQris(t, resp, body, apigen.QrisStatusStatusPending, 500_000)

		resp, body = c.post(t, r.token, "/qris", amountBody(600_000))
		requireAPIError(t, resp, body, http.StatusTooManyRequests, "qris_limit", "")
		require.Equal(t, before+qris.MaxGeneratesPerDay, gatewayCalls(gw, autogopaytest.MethodGenerate, keyA), "the sixth never reached AutoGoPay")

		resp, body = getPublic(t, srv.URL, "/api/t/"+r.token+"/status", c.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, apigen.QrisStatusStatusPending, decode[apigen.QrisStatus](t, body).Status,
			"at the cap the live QR is not touched, so the tenant can still pay it")

		env.Clock.Advance(qris.GenerateWindow + time.Minute)
		resp, body = c.post(t, r.token, "/qris", amountBody(600_000))
		requireQris(t, resp, body, apigen.QrisStatusStatusPending, 600_000)
	})

	srv.Close() // waits for every request, so every log line is written
	out := logs.String()
	require.Contains(t, out, `"route":"/api/t/{token}/qris"`)
	require.Contains(t, out, `"route":"/api/t/{token}/check"`)
	for _, token := range tokens {
		require.NotContains(t, out, token, "a token reached the logs")
	}
}

// TestPaymentPageQrisAvailableOverHTTP covers PaymentPage.qrisAvailable: whether the page offers
// "Bayar QRIS" follows the organization's AutoGoPay account.
func TestPaymentPageQrisAvailableOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 5, 10, 0, 0, 0, clock.Jakarta))
	srv := newLinkServer(t, env, &lockedBuffer{})
	c := &qrisCaller{base: srv.URL}
	org := newLinkOrg(t, env, "Kontrakan Mawar", "0812661")
	r := org.rentWithLink(t, "Nur Hidayati")
	other := newLinkOrg(t, env, "Ruko Sentosa", "0812662")
	otherRenter := other.rentWithLink(t, "Hartono Setiawan")

	available := func(t *testing.T, token string) bool {
		t.Helper()
		resp, body := getPage(t, srv.URL, token, c.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"qrisAvailable"`, "the field is required, so it is always there")
		return decode[apigen.PaymentPage](t, body).QrisAvailable
	}

	require.False(t, available(t, r.token), "no AutoGoPay account yet")
	env.SetAutoGoPayKey(t, org.o.ID, "agp-key-kontrakan-mawar")
	require.True(t, available(t, r.token), "set up")
	require.False(t, available(t, otherRenter.token), "another organization's account does not count")
}
