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

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// getPreview asks what paying amount (the raw query value) through token would pay, with no
// session, from the address ip.
func getPreview(t *testing.T, base, token, amount, ip string) (result, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/api/t/"+token+"/preview?amount="+amount, nil)
	require.NoError(t, err)
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return result{StatusCode: resp.StatusCode, Header: resp.Header}, data
}

// rentSince lets a new room to a new tenant from start (before today: entered late, so every
// period since start is invoiced), generates the invoices and returns the tenant and its oldest
// invoice.
func (lo *linkOrg) rentSince(t *testing.T, tenantName string, start time.Time) (tenantID, oldest uuid.UUID) {
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
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: start},
	})
	require.NoError(t, err)
	_, err = lo.env.Billing.GenerateOrg(ctx, lo.o.ID)
	require.NoError(t, err)
	acct, err := lo.env.Billing.Account(ctx, lo.admin, tn.ID)
	require.NoError(t, err)
	require.NotEmpty(t, acct.Invoices)
	return tn.ID, acct.Invoices[0].ID
}

// TestPaymentPreviewOverHTTP covers getPaymentPreview: the oldest invoice is paid first, nothing
// above what is owed, link errors and limits as getPaymentPage, one organization's link sees only
// its own tenant, and the token stays out of the logs.
func TestPaymentPreviewOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	logs := &lockedBuffer{}
	srv := newLinkServer(t, env, logs)

	a := newLinkOrg(t, env, "Griya Sejahtera", "0812631")
	b := newLinkOrg(t, env, "Kos Bulanan Sejahtera", "0812632")
	// Rina rents since 2 Oct, entered today: October and November are both invoiced, 700.000 each.
	rina, rinaOldest := a.rentSince(t, "Rina Wulandari", time.Date(2026, 10, 2, 0, 0, 0, 0, clock.Jakarta))
	_, dewiInvoice := b.rent(t, "Dewi Lestari", uuid.Nil)

	admin, adminB := newClient(t, srv.URL), newClient(t, srv.URL)
	admin.signIn(env, a.o.Admin.Phone)
	adminB.signIn(env, b.o.Admin.Phone)
	mint := func(t *testing.T, c *client, org, invoice uuid.UUID) string {
		t.Helper()
		resp, body := c.do(http.MethodPost, "/api/orgs/"+org.String()+"/invoices/"+invoice.String()+"/payment-links", nil)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		return strings.TrimPrefix(decode[apigen.PaymentLink](t, body).Url, linkOrigin+"/t/")
	}
	var tokens []string

	// 200.000 already paid leaves October owing 500.000 and November 700.000.
	resp, body := admin.do(http.MethodPost, "/api/orgs/"+a.o.ID.String()+"/payments", map[string]any{
		"requestId": uuid.New(), "tenantId": rina, "amount": 200_000, "method": "cash", "receivedOn": "2026-11-02",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))

	rinaToken := mint(t, admin, a.o.ID, rinaOldest)
	tokens = append(tokens, rinaToken)
	resp, body = getPage(t, srv.URL, rinaToken, "203.0.113.20")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	page := decode[apigen.PaymentPage](t, body)
	require.Len(t, page.OpenInvoices, 2)
	require.EqualValues(t, 1_200_000, page.Outstanding)
	oct, nov := page.OpenInvoices[0], page.OpenInvoices[1]
	require.Equal(t, "2026-10-02", oct.Period.String())
	require.Equal(t, "2026-11-02", nov.Period.String())
	require.EqualValues(t, 500_000, oct.Balance)
	require.EqualValues(t, 700_000, nov.Balance)

	// item is what the preview should list for an invoice of the page.
	item := func(inv apigen.PaymentPageInvoice, applied int64) apigen.PaymentPreviewItem {
		return apigen.PaymentPreviewItem{
			InvoiceId: inv.Id, Kind: inv.Kind, UnitLabel: inv.UnitLabel, PropertyName: inv.PropertyName,
			Period: inv.Period, PeriodEnd: inv.PeriodEnd, DueDate: inv.DueDate,
			Balance: inv.Balance, Applied: applied, BalanceAfter: inv.Balance - applied, PaidInFull: applied == inv.Balance,
		}
	}

	t.Run("previews oldest invoice first", func(t *testing.T) {
		tests := []struct {
			name   string
			amount int64
			want   apigen.PaymentPreview
		}{
			{
				name: "part of the oldest invoice only", amount: 300_000,
				want: apigen.PaymentPreview{Amount: 300_000, OutstandingBefore: 1_200_000, OutstandingAfter: 900_000,
					Items: []apigen.PaymentPreviewItem{item(oct, 300_000)}},
			},
			{
				name: "spanning both: October paid in full, November in part", amount: 800_000,
				want: apigen.PaymentPreview{Amount: 800_000, OutstandingBefore: 1_200_000, OutstandingAfter: 400_000,
					Items: []apigen.PaymentPreviewItem{item(oct, 500_000), item(nov, 300_000)}},
			},
			{
				name: "everything owed", amount: 1_200_000,
				want: apigen.PaymentPreview{Amount: 1_200_000, OutstandingBefore: 1_200_000, OutstandingAfter: 0,
					Items: []apigen.PaymentPreviewItem{item(oct, 500_000), item(nov, 700_000)}},
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := getPreview(t, srv.URL, rinaToken, fmt.Sprint(tc.amount), "203.0.113.20")
				require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
				requirePublicHeaders(t, resp)
				require.Equal(t, tc.want, decode[apigen.PaymentPreview](t, body))
				require.Equal(t, "Kamar 1", tc.want.Items[0].UnitLabel)
				require.Equal(t, "Kos Griya Sejahtera", tc.want.Items[0].PropertyName)
			})
		}
	})

	t.Run("previewing records nothing", func(t *testing.T) {
		resp, body := getPage(t, srv.URL, rinaToken, "203.0.113.20")
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.EqualValues(t, 1_200_000, decode[apigen.PaymentPage](t, body).Outstanding)
	})

	t.Run("bad amounts", func(t *testing.T) {
		tests := []struct {
			name, amount, code, message string
		}{
			{"above what is owed", "1200001", "amount_above_outstanding", "Nominal melebihi sisa tagihan."},
			{"not a number", "sejuta", "invalid_input", "Jumlah harus angka bulat lebih dari Rp0."},
			{"zero", "0", "invalid_input", "Jumlah harus angka bulat lebih dari Rp0."},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := getPreview(t, srv.URL, rinaToken, tc.amount, "203.0.113.20")
				require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
				requirePublicHeaders(t, resp)
				e := decode[apigen.Error](t, body)
				require.Equal(t, tc.code, e.Error.Code)
				require.Equal(t, tc.message, e.Error.Message)
				require.NotNil(t, e.Error.Field)
				require.Equal(t, "amount", *e.Error.Field)
			})
		}
	})

	t.Run("another organization's link sees only its own tenant", func(t *testing.T) {
		dewiToken := mint(t, adminB, b.o.ID, dewiInvoice)
		tokens = append(tokens, dewiToken)
		resp, body := getPreview(t, srv.URL, dewiToken, "700000", "203.0.113.21")
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		p := decode[apigen.PaymentPreview](t, body)
		require.EqualValues(t, 700_000, p.OutstandingBefore)
		require.Len(t, p.Items, 1)
		require.Equal(t, dewiInvoice, p.Items[0].InvoiceId)
		require.NotContains(t, string(body), "Griya")
		require.NotContains(t, string(body), rinaOldest.String())

		resp, body = getPreview(t, srv.URL, dewiToken, "700001", "203.0.113.21")
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "Rina's debt is not Dewi's: %s", body)
		require.Equal(t, "amount_above_outstanding", decode[apigen.Error](t, body).Error.Code)
	})

	t.Run("revoked and garbage tokens answer exactly like the page", func(t *testing.T) {
		revoked := mint(t, admin, a.o.ID, rinaOldest)
		tokens = append(tokens, revoked)
		resp, body := admin.do(http.MethodPost, "/api/orgs/"+a.o.ID.String()+"/invoices/"+rinaOldest.String()+"/payment-links/revoke", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		for name, token := range map[string]string{
			"revoked":                 revoked,
			"garbage":                 "not-a-token",
			"well-formed but unknown": strings.Repeat("A", 43),
		} {
			t.Run(name, func(t *testing.T) {
				pageResp, pageBody := getPage(t, srv.URL, token, "203.0.113.22")
				resp, body := getPreview(t, srv.URL, token, "1000", "203.0.113.22")
				require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
				require.Equal(t, pageResp.StatusCode, resp.StatusCode)
				require.Equal(t, string(pageBody), string(body))
				requirePublicHeaders(t, resp)
				require.Equal(t, "payment_link_not_found", decode[apigen.Error](t, body).Error.Code)
			})
		}
	})

	t.Run("shares the per-token limit with the page", func(t *testing.T) {
		_, invoice := a.rent(t, "Budi Santoso", uuid.Nil)
		token := mint(t, admin, a.o.ID, invoice)
		tokens = append(tokens, token)
		for i := range 30 {
			resp, body := getPreview(t, srv.URL, token, "1000", fmt.Sprintf("198.51.100.%d", i))
			require.Equal(t, http.StatusOK, resp.StatusCode, "request %d: %s", i+1, body)
		}
		resp, body := getPage(t, srv.URL, token, "198.51.100.200")
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, string(body))
		resp, body = getPreview(t, srv.URL, token, "1000", "198.51.100.201")
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, string(body))
		requirePublicHeaders(t, resp)
		require.Equal(t, "rate_limited", decode[apigen.Error](t, body).Error.Code)
		env.Clock.Advance(time.Minute)
	})

	// The preview promises "this is what your money will pay": the payment's real allocation must
	// match it, so a change to allocation's order cannot leave the preview behind unnoticed.
	t.Run("the payment then pays exactly what the preview showed", func(t *testing.T) {
		token := mint(t, admin, a.o.ID, rinaOldest) // an earlier case revoked Rina's first links
		tokens = append(tokens, token)
		resp, body := getPreview(t, srv.URL, token, "800000", "203.0.113.24")
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		preview := decode[apigen.PaymentPreview](t, body)
		require.Len(t, preview.Items, 2, "800.000 reaches both invoices")

		resp, body = admin.do(http.MethodPost, "/api/orgs/"+a.o.ID.String()+"/payments", map[string]any{
			"requestId": uuid.New(), "tenantId": rina, "amount": 800_000, "method": "cash", "receivedOn": "2026-11-02",
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		paid := decode[apigen.PaymentResult](t, body)

		want := make([]apigen.PaymentAllocation, len(preview.Items))
		for i, it := range preview.Items {
			want[i] = apigen.PaymentAllocation{InvoiceId: it.InvoiceId, Amount: it.Applied}
		}
		require.Equal(t, want, paid.Payment.Allocations, "the allocation follows the preview, in order")
		require.Equal(t, preview.OutstandingAfter, paid.Outstanding)
	})

	t.Run("an expired link answers the page's 410", func(t *testing.T) {
		paid, invoice := a.rent(t, "Sari Puspita", uuid.Nil)
		token := mint(t, admin, a.o.ID, invoice)
		tokens = append(tokens, token)
		resp, body := admin.do(http.MethodPost, "/api/orgs/"+a.o.ID.String()+"/payments", map[string]any{
			"requestId": uuid.New(), "tenantId": paid, "amount": 700_000, "method": "transfer", "receivedOn": "2026-11-02",
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))

		resp, body = getPreview(t, srv.URL, token, "1", "203.0.113.23")
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "a paid tenant owes nothing: %s", body)
		require.Equal(t, "amount_above_outstanding", decode[apigen.Error](t, body).Error.Code)

		env.Clock.Set(time.Now().Add(30*24*time.Hour + time.Hour).In(clock.Jakarta))
		pageResp, pageBody := getPage(t, srv.URL, token, "203.0.113.23")
		resp, body = getPreview(t, srv.URL, token, "1", "203.0.113.23")
		require.Equal(t, http.StatusGone, resp.StatusCode, string(body))
		require.Equal(t, pageResp.StatusCode, resp.StatusCode)
		require.Equal(t, string(pageBody), string(body))
		requirePublicHeaders(t, resp)
		require.Equal(t, "payment_link_expired", decode[apigen.Error](t, body).Error.Code)
	})

	srv.Close() // waits for every request, so every log line is written
	out := logs.String()
	require.Contains(t, out, `"route":"/api/t/{token}/preview"`, "the request log names the route")
	for _, token := range tokens {
		require.NotContains(t, out, token, "a token reached the logs")
	}
}
