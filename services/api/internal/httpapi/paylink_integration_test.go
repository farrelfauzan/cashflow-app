//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

const linkOrigin = "https://app.test"

// lockedBuffer is a log sink the server writes to while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// linkOrg is an organization with a kos, its three members as principals, and helpers to rent
// a room and to sign its members in.
type linkOrg struct {
	env                         *testenv.Env
	o                           testenv.Org
	admin, karyawan, supervisor authz.Principal
	kos                         property.Property
	prefix                      string
	n                           int
}

func newLinkOrg(t *testing.T, env *testenv.Env, name, prefix string) *linkOrg {
	t.Helper()
	lo := &linkOrg{env: env, o: env.CreateOrg(t, name, prefix), prefix: prefix}
	lo.admin = env.Principal(t, lo.o.ID, lo.o.Admin.UserID)
	lo.karyawan = env.Principal(t, lo.o.ID, lo.o.Karyawan.UserID)
	lo.supervisor = env.Principal(t, lo.o.ID, lo.o.Supervisor.UserID)
	var err error
	lo.kos, err = env.Properties.Create(context.Background(), lo.admin, property.PropertyInput{Name: "Kos " + name, Kind: property.KindKos})
	require.NoError(t, err)
	return lo
}

// rent lets a new room to a new tenant from today, collected by collector (a member id or
// uuid.Nil), generates the first invoice and returns the tenant and invoice ids.
func (lo *linkOrg) rent(t *testing.T, tenantName string, collector uuid.UUID) (tenantID, invoiceID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	lo.n++
	unit, err := lo.env.Properties.CreateUnit(ctx, lo.admin, lo.kos.ID, property.UnitInput{
		Label: fmt.Sprintf("Kamar %d", lo.n), Terms: testenv.Monthly(700_000),
	})
	require.NoError(t, err)
	tn, err := lo.env.Tenants.Create(ctx, lo.admin, tenant.Input{Name: tenantName, Phone: fmt.Sprintf("%s%04d", lo.prefix, 5000+lo.n)})
	require.NoError(t, err)
	_, err = lo.env.Tenants.CreateLease(ctx, lo.admin, tenant.NewLease{
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: tenant.Today(lo.env.Clock), CollectorMemberID: collector},
	})
	require.NoError(t, err)
	_, err = lo.env.Billing.GenerateOrg(ctx, lo.o.ID)
	require.NoError(t, err)
	acct, err := lo.env.Billing.Account(ctx, lo.admin, tn.ID)
	require.NoError(t, err)
	require.Len(t, acct.Invoices, 1)
	return tn.ID, acct.Invoices[0].ID
}

// newLinkServer serves the API with the payment link service, logging to logs.
func newLinkServer(t *testing.T, env *testenv.Env, logs io.Writer) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing,
		PayLinks:      paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin),
		PublicLimiter: env.Limiter,
		CookieSecure:  false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// getPage opens a payment page with no session, as a tenant's phone would, from the address ip
// (sent as Caddy would, in X-Forwarded-For).
func getPage(t *testing.T, base, token, ip string) (result, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/api/t/"+token, nil)
	require.NoError(t, err)
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return result{StatusCode: resp.StatusCode, Header: resp.Header}, data
}

func requirePublicHeaders(t *testing.T, resp result) {
	t.Helper()
	require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	require.Equal(t, "no-referrer", resp.Header.Get("Referrer-Policy"))
}

// TestPaymentLinksOverHTTP covers createPaymentLink, revokePaymentLinks and getPaymentPage end
// to end, with the token kept out of the logs.
func TestPaymentLinksOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	logs := &lockedBuffer{}
	srv := newLinkServer(t, env, logs)

	a := newLinkOrg(t, env, "Griya Sejahtera", "0812621")
	b := newLinkOrg(t, env, "Kos Bulanan Sejahtera", "0812622")
	_, rinaInvoice := a.rent(t, "Rina Wulandari", a.karyawan.MembershipID)
	budi, budiInvoice := a.rent(t, "Budi Santoso", uuid.Nil)
	_, dewiInvoice := b.rent(t, "Dewi Lestari", uuid.Nil)
	_, err := paylink.NewService(env.Store, env.Authz, env.Clock, "").SetBankAccount(ctx, a.o.ID,
		paylink.BankAccount{BankName: "BCA", AccountNumber: "123 456 7890", AccountHolder: "Suhwendi"})
	require.NoError(t, err)

	admin, karyawan, supervisor, adminB := newClient(t, srv.URL), newClient(t, srv.URL), newClient(t, srv.URL), newClient(t, srv.URL)
	admin.signIn(env, a.o.Admin.Phone)
	karyawan.signIn(env, a.o.Karyawan.Phone)
	supervisor.signIn(env, a.o.Supervisor.Phone)
	adminB.signIn(env, b.o.Admin.Phone)
	linksPath := func(org uuid.UUID, invoice string) string {
		return "/api/orgs/" + org.String() + "/invoices/" + invoice + "/payment-links"
	}
	mint := func(t *testing.T, c *client, org, invoice uuid.UUID) string {
		t.Helper()
		resp, body := c.do(http.MethodPost, linksPath(org, invoice.String()), nil)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
		token, ok := strings.CutPrefix(decode[apigen.PaymentLink](t, body).Url, linkOrigin+"/t/")
		require.True(t, ok, string(body))
		require.Regexp(t, `^[A-Za-z0-9_-]{43}$`, token)
		return token
	}
	var tokens []string // every token made, to look for in the logs at the end

	first := mint(t, admin, a.o.ID, rinaInvoice)
	second := mint(t, admin, a.o.ID, rinaInvoice)
	tokens = append(tokens, first, second)

	t.Run("the page shows the tenant, the unit, what is owed and the bank account", func(t *testing.T) {
		resp, body := getPage(t, srv.URL, first, "203.0.113.10")
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		requirePublicHeaders(t, resp)
		page := decode[apigen.PaymentPage](t, body)
		require.Equal(t, "Griya Sejahtera", page.OrganizationName)
		require.Equal(t, "Rina Wulandari", page.TenantName)
		require.Equal(t, rinaInvoice, page.Invoice.Id)
		require.Equal(t, "Kamar 1", page.Invoice.UnitLabel)
		require.Equal(t, "Kos Griya Sejahtera", page.Invoice.PropertyName)
		require.Equal(t, apigen.Regular, page.Invoice.Kind)
		require.Equal(t, "2026-11-02", page.Invoice.Period.String())
		require.Equal(t, "2026-12-01", page.Invoice.PeriodEnd.String())
		require.Equal(t, "2026-11-02", page.Invoice.DueDate.String())
		require.EqualValues(t, 700_000, page.Invoice.Amount)
		require.EqualValues(t, 700_000, page.Invoice.Balance)
		require.EqualValues(t, 700_000, page.Outstanding)
		require.Zero(t, page.Credit)
		require.False(t, page.Paid)
		require.Len(t, page.OpenInvoices, 1)
		require.NotEmpty(t, page.OpenInvoices[0].Lines)
		require.NotNil(t, page.BankAccount)
		require.Equal(t, apigen.BankAccount{BankName: "BCA", AccountNumber: "123 456 7890", AccountHolder: "Suhwendi"}, *page.BankAccount)

		// Nothing beyond what the page needs: no phone, no staff, no other tenant.
		for _, leak := range []string{"phone", "0812621", a.o.Admin.Name, a.o.Karyawan.Name, "Budi", "note", "payments"} {
			require.NotContains(t, string(body), leak)
		}
	})

	t.Run("a second link works alongside the first", func(t *testing.T) {
		for _, token := range []string{first, second} {
			resp, body := getPage(t, srv.URL, token, "203.0.113.10")
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		}
	})

	t.Run("an organization without a bank account answers null", func(t *testing.T) {
		token := mint(t, adminB, b.o.ID, dewiInvoice)
		tokens = append(tokens, token)
		resp, body := getPage(t, srv.URL, token, "203.0.113.10")
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"bankAccount":null`)
		page := decode[apigen.PaymentPage](t, body)
		require.Equal(t, "Dewi Lestari", page.TenantName)
		require.Equal(t, "Kos Bulanan Sejahtera", page.OrganizationName)
		require.NotContains(t, string(body), "Griya", "organization B's link shows nothing of A")
		require.NotContains(t, string(body), "Rina")
	})

	t.Run("who may make and revoke links", func(t *testing.T) {
		tests := []struct {
			name    string
			c       *client
			invoice uuid.UUID
			status  int
			code    string
		}{
			{"karyawan, a tenant they collect from", karyawan, rinaInvoice, http.StatusCreated, ""},
			{"karyawan, a tenant they do not collect from", karyawan, budiInvoice, http.StatusForbidden, "forbidden"},
			{"supervisor", supervisor, rinaInvoice, http.StatusForbidden, "forbidden"},
			{"another organization's invoice", admin, dewiInvoice, http.StatusNotFound, "not_found"},
			{"an invoice that does not exist", admin, uuid.New(), http.StatusNotFound, "not_found"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := tc.c.do(http.MethodPost, linksPath(a.o.ID, tc.invoice.String()), nil)
				require.Equal(t, tc.status, resp.StatusCode, string(body))
				if tc.status == http.StatusCreated {
					tokens = append(tokens, strings.TrimPrefix(decode[apigen.PaymentLink](t, body).Url, linkOrigin+"/t/"))
					return
				}
				require.Equal(t, tc.code, decode[apigen.Error](t, body).Error.Code)
				resp, body = tc.c.do(http.MethodPost, linksPath(a.o.ID, tc.invoice.String())+"/revoke", nil)
				require.Equal(t, tc.status, resp.StatusCode, string(body))
				require.Equal(t, tc.code, decode[apigen.Error](t, body).Error.Code)
			})
		}
		resp, body := admin.do(http.MethodPost, linksPath(a.o.ID, "not-a-uuid"), nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	})

	var notFound []byte
	t.Run("revoking stops every link of the invoice", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, linksPath(a.o.ID, rinaInvoice.String())+"/revoke", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, 3, decode[apigen.PaymentLinksRevoked](t, body).Revoked, "two by the admin, one by the karyawan")
		for _, token := range []string{first, second} {
			resp, body := getPage(t, srv.URL, token, "203.0.113.10")
			require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
			requirePublicHeaders(t, resp)
			e := decode[apigen.Error](t, body)
			require.Equal(t, "payment_link_not_found", e.Error.Code)
			require.Equal(t, "Link pembayaran tidak ditemukan atau sudah tidak berlaku.", e.Error.Message)
			notFound = body
		}
		resp, body = admin.do(http.MethodPost, linksPath(a.o.ID, rinaInvoice.String())+"/revoke", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Zero(t, decode[apigen.PaymentLinksRevoked](t, body).Revoked)
	})

	t.Run("garbage, unknown and another organization's revoked tokens answer the same 404", func(t *testing.T) {
		otherRevoked := mint(t, adminB, b.o.ID, dewiInvoice)
		tokens = append(tokens, otherRevoked)
		resp, body := adminB.do(http.MethodPost, linksPath(b.o.ID, dewiInvoice.String())+"/revoke", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		for name, token := range map[string]string{
			"garbage":                             "not-a-token",
			"well-formed but unknown":             strings.Repeat("A", 43),
			"too long":                            strings.Repeat("A", 500),
			"another organization's revoked link": otherRevoked,
		} {
			t.Run(name, func(t *testing.T) {
				resp, body := getPage(t, srv.URL, token, "203.0.113.10")
				require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
				requirePublicHeaders(t, resp)
				require.Equal(t, string(notFound), string(body), "identical to a revoked link's answer")
			})
		}
	})

	t.Run("a link expires 30 days after its invoice is paid", func(t *testing.T) {
		token := mint(t, admin, a.o.ID, budiInvoice)
		tokens = append(tokens, token)
		resp, body := admin.do(http.MethodPost, "/api/orgs/"+a.o.ID.String()+"/payments", map[string]any{
			"requestId": uuid.New(), "tenantId": budi, "amount": 700_000, "method": "transfer", "receivedOn": "2026-11-02",
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		paidBy := time.Now()

		resp, body = getPage(t, srv.URL, token, "203.0.113.11")
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.True(t, decode[apigen.PaymentPage](t, body).Paid, "a paid invoice's link still works")

		env.Clock.Set(paidBy.Add(30*24*time.Hour + time.Hour).In(clock.Jakarta))
		resp, body = getPage(t, srv.URL, token, "203.0.113.11")
		require.Equal(t, http.StatusGone, resp.StatusCode, string(body))
		requirePublicHeaders(t, resp)
		e := decode[apigen.Error](t, body)
		require.Equal(t, "payment_link_expired", e.Error.Code)
		require.Equal(t, "Link pembayaran ini sudah kedaluwarsa. Minta link baru ke pengelola.", e.Error.Message)

		admin.signIn(env, a.o.Admin.Phone) // the session lasts 30 days too
		resp, body = admin.do(http.MethodPost, linksPath(a.o.ID, budiInvoice.String()), nil)
		require.Equal(t, http.StatusConflict, resp.StatusCode, string(body))
		require.Equal(t, "payment_link_expired_invoice", decode[apigen.Error](t, body).Error.Code)
	})

	t.Run("mint and revoke are audited", func(t *testing.T) {
		var allowed int
		for _, row := range env.AuditLog(t, a.o.ID) {
			if row.Action == authz.ActionInvoiceShare && row.Decision == "allow" {
				allowed++
			}
		}
		// Mints: two by the admin, one by the karyawan, one for Budi; revokes: two.
		require.Equal(t, 6, allowed)
	})

	srv.Close() // waits for every request, so every log line is written
	out := logs.String()
	require.Contains(t, out, `"route":"/api/t/{token}"`, "the request log names the route")
	for _, token := range tokens {
		require.NotContains(t, out, token, "a token reached the logs")
	}
}

// TestPaymentPageRateLimits: the page answers 429 past 30 requests a minute for one token and
// past 120 a minute from one address, before the token is looked up.
func TestPaymentPageRateLimits(t *testing.T) {
	env := testenv.New(t)
	srv := newLinkServer(t, env, io.Discard)
	a := newLinkOrg(t, env, "Griya Sejahtera", "0812623")
	_, invoice := a.rent(t, "Rina Wulandari", uuid.Nil)
	admin := newClient(t, srv.URL)
	admin.signIn(env, a.o.Admin.Phone)
	resp, body := admin.do(http.MethodPost, "/api/orgs/"+a.o.ID.String()+"/invoices/"+invoice.String()+"/payment-links", nil)
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	token := strings.TrimPrefix(decode[apigen.PaymentLink](t, body).Url, linkOrigin+"/t/")

	t.Run("per token", func(t *testing.T) {
		for i := range 30 {
			resp, body := getPage(t, srv.URL, token, fmt.Sprintf("198.51.100.%d", i)) // a new address each time
			require.Equal(t, http.StatusOK, resp.StatusCode, "request %d: %s", i+1, body)
		}
		resp, body := getPage(t, srv.URL, token, "198.51.100.200")
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, string(body))
		requirePublicHeaders(t, resp)
		require.Equal(t, "rate_limited", decode[apigen.Error](t, body).Error.Code)

		env.Clock.Advance(time.Minute)
		resp, body = getPage(t, srv.URL, token, "198.51.100.201")
		require.Equal(t, http.StatusOK, resp.StatusCode, "a new window: %s", body)
	})

	t.Run("per address, counting unknown tokens too", func(t *testing.T) {
		const ip = "203.0.113.99"
		for i := range 120 {
			resp, body := getPage(t, srv.URL, fmt.Sprintf("guess%038d", i), ip)
			require.Equal(t, http.StatusNotFound, resp.StatusCode, "request %d: %s", i+1, body)
		}
		resp, body := getPage(t, srv.URL, strings.Repeat("B", 43), ip)
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, string(body))
		resp, body = getPage(t, srv.URL, strings.Repeat("B", 43), "203.0.113.100")
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "another address is counted apart: %s", body)
		resp, body = getPage(t, srv.URL, strings.Repeat("B", 43), "1.2.3.4, "+ip)
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "a forged left entry does not escape the limit: %s", body)
	})
}
