//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// qrisRenter is a tenant with one invoice and a working link to it.
type qrisRenter struct {
	tenantID, invoiceID, linkID uuid.UUID
	token                       string
}

// rentWithLink rents a room to a new tenant and mints a link to its first invoice.
func (lo *linkOrg) rentWithLink(t *testing.T, tenantName string) qrisRenter {
	t.Helper()
	tenantID, invoiceID := lo.rent(t, tenantName, uuid.Nil)
	r := qrisRenter{tenantID: tenantID, invoiceID: invoiceID}
	r.linkID, r.token = lo.mintLink(t, invoiceID)
	return r
}

func (lo *linkOrg) mintLink(t *testing.T, invoiceID uuid.UUID) (uuid.UUID, string) {
	t.Helper()
	link, err := paylink.NewService(lo.env.Store, lo.env.Authz, lo.env.Clock, linkOrigin).Mint(context.Background(), lo.admin, invoiceID)
	require.NoError(t, err)
	token, ok := strings.CutPrefix(link.URL, linkOrigin+"/t/")
	require.True(t, ok)
	return link.ID, token
}

// attemptState is how a test attempt is stored.
type attemptState struct {
	status     string
	pastExpiry bool  // expires_at is a minute ago instead of 14 minutes ahead
	checkAsked bool  // check_requested_at is set
	amount     int64 // 0 means Rp700.000
}

// insertAttempt stores an attempt for r, made a minute ago (or 16 minutes ago when pastExpiry),
// as the generate endpoint will.
func (lo *linkOrg) insertAttempt(t *testing.T, r qrisRenter, s attemptState) dbgen.InsertQrisAttemptParams {
	t.Helper()
	now := lo.env.Clock.Now()
	created := now.Add(-time.Minute)
	if s.pastExpiry {
		created = now.Add(-16 * time.Minute)
	}
	p := dbgen.InsertQrisAttemptParams{
		ID: uuid.New(), OrganizationID: lo.o.ID, TenantID: r.tenantID, PaymentLinkID: r.linkID,
		Amount: s.amount, TransactionID: uuid.NewString(), OrderID: "AUTOGOPAY-1793000000-1",
		QrString: "00020101021226610016ID.CO.QRIS.WWW0118936009140000000000215ID10200000000000303UME5204549953033605802ID",
		Status:   s.status, ExpiresAt: created.Add(15 * time.Minute), CreatedAt: created,
	}
	if p.Amount == 0 {
		p.Amount = 700_000
	}
	if s.status != qris.AttemptPending {
		p.ClosedAt = &now
	}
	if s.status == qris.AttemptPaid {
		p.PaidAmount = &p.Amount
	}
	if s.checkAsked {
		p.CheckRequestedAt = &now
	}
	require.NoError(t, lo.env.Store.WithOrgTx(context.Background(), lo.o.ID, func(tx pgx.Tx) error {
		return dbgen.New(tx).InsertQrisAttempt(context.Background(), p)
	}))
	return p
}

// getPublic sends a GET with no session to path, from the address ip.
func getPublic(t *testing.T, base, path, ip string) (result, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+path, nil)
	require.NoError(t, err)
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return result{StatusCode: resp.StatusCode, Header: resp.Header}, data
}

// TestQrisOverHTTP covers getPaymentStatus and getPaymentQRImage, with the token kept out of the
// logs.
func TestQrisOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 5, 10, 0, 0, 0, clock.Jakarta))
	logs := &lockedBuffer{}
	srv := newLinkServer(t, env, logs)
	a := newLinkOrg(t, env, "Griya Sejahtera", "0812641")
	b := newLinkOrg(t, env, "Kos Bulanan Sejahtera", "0812642")
	var tokens []string
	const ip = "203.0.113.20"

	// One tenant per state, each with its own link.
	type stateCase struct {
		name     string
		state    *attemptState // nil: no attempt
		status   apigen.QrisStatusStatus
		pngCode  int
		pngError string
	}
	cases := []stateCase{
		{"no attempt", nil, apigen.QrisStatusStatusNone, http.StatusNotFound, "qris_not_found"},
		{"pending", &attemptState{status: qris.AttemptPending, amount: 650_000}, apigen.QrisStatusStatusPending, http.StatusOK, ""},
		{"pending with a check asked", &attemptState{status: qris.AttemptPending, checkAsked: true}, apigen.QrisStatusStatusChecking, http.StatusOK, ""},
		{"pending past expiry", &attemptState{status: qris.AttemptPending, pastExpiry: true}, apigen.QrisStatusStatusExpired, http.StatusGone, "qris_expired"},
		{"paid", &attemptState{status: qris.AttemptPaid}, apigen.QrisStatusStatusPaid, http.StatusGone, "qris_expired"},
		{"expired", &attemptState{status: qris.AttemptExpired}, apigen.QrisStatusStatusExpired, http.StatusGone, "qris_expired"},
		{"cancelled", &attemptState{status: qris.AttemptCancelled}, apigen.QrisStatusStatusExpired, http.StatusGone, "qris_expired"},
	}
	for i, tc := range cases {
		r := a.rentWithLink(t, fmt.Sprintf("Penyewa %d", i+1))
		tokens = append(tokens, r.token)
		var stored *dbgen.InsertQrisAttemptParams
		if tc.state != nil {
			p := a.insertAttempt(t, r, *tc.state)
			stored = &p
		}

		t.Run("status: "+tc.name, func(t *testing.T) {
			resp, body := getPublic(t, srv.URL, "/api/t/"+r.token+"/status", ip)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			requirePublicHeaders(t, resp)
			got := decode[apigen.QrisStatus](t, body)
			require.Equal(t, tc.status, got.Status)
			if stored == nil {
				require.Nil(t, got.Amount)
				require.Nil(t, got.ExpiresAt)
				require.JSONEq(t, `{"status":"none"}`, string(body))
				return
			}
			require.NotNil(t, got.Amount)
			require.Equal(t, stored.Amount, *got.Amount)
			require.NotNil(t, got.ExpiresAt)
			require.True(t, stored.ExpiresAt.Equal(*got.ExpiresAt))
			require.NotContains(t, string(body), stored.QrString, "the QR string is only in the image")
			require.NotContains(t, string(body), stored.TransactionID)
		})

		t.Run("image: "+tc.name, func(t *testing.T) {
			resp, body := getPublic(t, srv.URL, "/api/t/"+r.token+"/qris.png", ip)
			require.Equal(t, tc.pngCode, resp.StatusCode, string(body))
			requirePublicHeaders(t, resp)
			if tc.pngCode == http.StatusOK {
				require.Equal(t, "image/png", resp.Header.Get("Content-Type"))
				img, err := png.Decode(bytes.NewReader(body))
				require.NoError(t, err)
				require.Positive(t, img.Bounds().Dx())
				return
			}
			e := decode[apigen.Error](t, body)
			require.Equal(t, tc.pngError, e.Error.Code)
			want := map[string]string{
				"qris_not_found": "Belum ada QR untuk tagihan ini.",
				"qris_expired":   "QR sudah tidak berlaku. Buat QR baru.",
			}[tc.pngError]
			require.Equal(t, want, e.Error.Message)
		})
	}

	t.Run("attempts belong to the tenant: every working link shows the latest one", func(t *testing.T) {
		r := a.rentWithLink(t, "Rina Wulandari")
		_, second := a.mintLink(t, r.invoiceID)
		tokens = append(tokens, r.token, second)
		a.insertAttempt(t, r, attemptState{status: qris.AttemptExpired, pastExpiry: true, amount: 100_000})
		env.Clock.Advance(time.Second)
		latest := a.insertAttempt(t, r, attemptState{status: qris.AttemptPending, amount: 250_000})
		for _, token := range []string{r.token, second} {
			resp, body := getPublic(t, srv.URL, "/api/t/"+token+"/status", ip)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			got := decode[apigen.QrisStatus](t, body)
			require.Equal(t, apigen.QrisStatusStatusPending, got.Status)
			require.Equal(t, latest.Amount, *got.Amount)
			resp, body = getPublic(t, srv.URL, "/api/t/"+token+"/qris.png", ip)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		}

		t.Run("and the QR stops at its expiry", func(t *testing.T) {
			env.Clock.Set(latest.ExpiresAt)
			resp, body := getPublic(t, srv.URL, "/api/t/"+r.token+"/qris.png", ip)
			require.Equal(t, http.StatusGone, resp.StatusCode, string(body))
			resp, body = getPublic(t, srv.URL, "/api/t/"+r.token+"/status", ip)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			require.Equal(t, apigen.QrisStatusStatusExpired, decode[apigen.QrisStatus](t, body).Status)
		})
	})

	t.Run("another organization's tenant sees only their own attempt", func(t *testing.T) {
		r := b.rentWithLink(t, "Dewi Lestari")
		tokens = append(tokens, r.token)
		resp, body := getPublic(t, srv.URL, "/api/t/"+r.token+"/status", ip)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.JSONEq(t, `{"status":"none"}`, string(body))
	})

	t.Run("garbage, unknown and revoked tokens answer the link's 404", func(t *testing.T) {
		r := a.rentWithLink(t, "Budi Santoso")
		tokens = append(tokens, r.token)
		a.insertAttempt(t, r, attemptState{status: qris.AttemptPending})
		_, err := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin).RevokeAll(ctx, a.admin, r.invoiceID)
		require.NoError(t, err)
		for name, token := range map[string]string{
			"garbage":                 "not-a-token",
			"well-formed but unknown": strings.Repeat("A", 43),
			"revoked":                 r.token,
		} {
			for _, suffix := range []string{"/status", "/qris.png"} {
				t.Run(name+suffix, func(t *testing.T) {
					resp, body := getPublic(t, srv.URL, "/api/t/"+token+suffix, ip)
					require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
					requirePublicHeaders(t, resp)
					require.Equal(t, "payment_link_not_found", decode[apigen.Error](t, body).Error.Code)
				})
			}
		}
	})

	t.Run("status and image count against the payment page's limits", func(t *testing.T) {
		r := a.rentWithLink(t, "Sari Indah")
		tokens = append(tokens, r.token)
		for i := range 30 {
			path := "/api/t/" + r.token + []string{"", "/status", "/qris.png"}[i%3]
			resp, body := getPublic(t, srv.URL, path, fmt.Sprintf("198.51.100.%d", i))
			require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode, "request %d: %s", i+1, body)
		}
		for _, suffix := range []string{"/status", "/qris.png"} {
			resp, body := getPublic(t, srv.URL, "/api/t/"+r.token+suffix, "198.51.100.250")
			require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, string(body))
			requirePublicHeaders(t, resp)
			require.Equal(t, "rate_limited", decode[apigen.Error](t, body).Error.Code)
		}
	})

	srv.Close() // waits for every request, so every log line is written
	out := logs.String()
	require.Contains(t, out, `"route":"/api/t/{token}/status"`)
	require.Contains(t, out, `"route":"/api/t/{token}/qris.png"`)
	for _, token := range tokens {
		require.NotContains(t, out, token, "a token reached the logs")
	}
}
