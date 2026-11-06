//go:build integration

package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func requirePaymentPromise(t *testing.T, resp result, body []byte) apigen.PaymentPromise {
	t.Helper()
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	return decode[apigen.PaymentPromise](t, body)
}

func hasAudit(rows []dbgen.AuditLog, actor uuid.UUID, action, decision string) bool {
	for _, r := range rows {
		if r.ActorStaffUserID.UUID == actor && r.Action == action && r.Decision == decision {
			return true
		}
	}
	return false
}

// TestPromisesOverHTTP covers makePaymentPromise, the promise fields of getPaymentPage, and
// listTenantPromises and makeTenantPromise end to end. The clock is 2 Nov 2026, 09:00 in
// Asia/Jakarta; every lease is monthly from that day, so its next invoice is due 2 Dec.
func TestPromisesOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	logs := &lockedBuffer{}
	srv := newLinkServer(t, env, logs)
	caller := &qrisCaller{base: srv.URL}

	a := newLinkOrg(t, env, "Griya Sejahtera", "0812961")
	b := newLinkOrg(t, env, "Kos Bulanan Sejahtera", "0812962")
	rina := a.rentWithLink(t, "Rina Wulandari")
	dewi := b.rentWithLink(t, "Dewi Lestari")
	budiID, _ := a.rent(t, "Budi Santoso", a.karyawan.MembershipID)
	tokens := []string{rina.token, dewi.token}

	admin, karyawan, supervisor, adminB := newClient(t, srv.URL), newClient(t, srv.URL), newClient(t, srv.URL), newClient(t, srv.URL)
	admin.signIn(env, a.o.Admin.Phone)
	karyawan.signIn(env, a.o.Karyawan.Phone)
	supervisor.signIn(env, a.o.Supervisor.Phone)
	adminB.signIn(env, b.o.Admin.Phone)
	promisesPath := func(org, tenantID uuid.UUID) string {
		return "/api/orgs/" + org.String() + "/tenants/" + tenantID.String() + "/promises"
	}

	t.Run("before any promise the page offers the dates and the next invoice", func(t *testing.T) {
		resp, body := getPage(t, srv.URL, rina.token, caller.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		page := decode[apigen.PaymentPage](t, body)
		require.Nil(t, page.Promise)
		require.Equal(t, &apigen.PromiseDates{Earliest: date(2026, 11, 3), Latest: date(2027, 1, 1)}, page.PromiseDates)
		require.Equal(t, ptrTo(date(2026, 12, 2)), page.NextInvoiceDueDate)
		require.Contains(t, string(body), `"promise":null`, "the field is there, null")
	})

	t.Run("the tenant makes a promise", func(t *testing.T) {
		resp, body := caller.post(t, rina.token, "/promise",
			`{"kind":"specific_date","promisedDate":"2026-11-10","promisedAmount":500000,"payingNow":200000}`)
		got := requirePaymentPromise(t, resp, body)
		require.Equal(t, apigen.PromiseStatusActive, got.Status)
		require.Equal(t, 1, got.Version)
		require.Equal(t, apigen.SpecificDate, got.Kind)
		require.Equal(t, date(2026, 11, 10), got.PromisedDate)
		require.Equal(t, ptrTo(int64(500_000)), got.PromisedAmount)
		require.Equal(t, int64(200_000), got.PayingNow)
	})

	t.Run("making it again is an edit with the server's next due date", func(t *testing.T) {
		resp, body := caller.post(t, rina.token, "/promise", `{"kind":"with_next_invoice"}`)
		got := requirePaymentPromise(t, resp, body)
		require.Equal(t, 2, got.Version)
		require.Equal(t, apigen.WithNextInvoice, got.Kind)
		require.Equal(t, date(2026, 12, 2), got.PromisedDate)
		require.Nil(t, got.PromisedAmount)
		require.Equal(t, int64(0), got.PayingNow)

		resp, body = getPage(t, srv.URL, rina.token, caller.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		page := decode[apigen.PaymentPage](t, body)
		require.NotNil(t, page.Promise)
		require.Equal(t, got, *page.Promise)
	})

	t.Run("refusals answer a code and the field, and save nothing", func(t *testing.T) {
		tests := []struct {
			name, body, code, field string
			status                  int
		}{
			{"no kind", `{}`, "invalid_input", "kind", http.StatusBadRequest},
			{"an unknown kind", `{"kind":"someday"}`, "invalid_input", "kind", http.StatusBadRequest},
			{"a specific date without one", `{"kind":"specific_date"}`, "invalid_input", "promisedDate", http.StatusBadRequest},
			{"a date sent for with_next_invoice", `{"kind":"with_next_invoice","promisedDate":"2026-12-02"}`, "invalid_input", "promisedDate", http.StatusBadRequest},
			{"a promised amount of zero", `{"kind":"with_next_invoice","promisedAmount":0}`, "invalid_input", "promisedAmount", http.StatusBadRequest},
			{"a negative payment now", `{"kind":"with_next_invoice","payingNow":-1}`, "invalid_input", "payingNow", http.StatusBadRequest},
			{"a note from the tenant page", `{"kind":"with_next_invoice","note":"nanti"}`, "invalid_json", "", http.StatusBadRequest},
			{"a date that is not a date", `{"kind":"specific_date","promisedDate":"10-11-2026"}`, "invalid_json", "", http.StatusBadRequest},
			{"today", `{"kind":"specific_date","promisedDate":"2026-11-02"}`, "promise_date_out_of_range", "promisedDate", http.StatusBadRequest},
			{"61 days ahead", `{"kind":"specific_date","promisedDate":"2027-01-02"}`, "promise_date_out_of_range", "promisedDate", http.StatusBadRequest},
			{"an amount above what is owed", `{"kind":"with_next_invoice","promisedAmount":700001}`, "promise_amount_above_outstanding", "promisedAmount", http.StatusBadRequest},
			{"paying more than is owed now", `{"kind":"with_next_invoice","payingNow":700001}`, "promise_amount_above_outstanding", "payingNow", http.StatusBadRequest},
			{"paying everything now", `{"kind":"with_next_invoice","payingNow":700000}`, "promise_nothing_owed", "", http.StatusConflict},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := caller.post(t, rina.token, "/promise", tc.body)
				requireAPIError(t, resp, body, tc.status, tc.code, tc.field)
			})
		}
		resp, body := admin.do(http.MethodGet, promisesPath(a.o.ID, rina.tenantID), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		list := decode[apigen.TenantPromiseList](t, body)
		require.Len(t, list.Items, 1)
		require.Len(t, list.Items[0].Versions, 2, "no refused request added a version")
	})

	t.Run("a tenant whose lease reaches its planned end has no next invoice", func(t *testing.T) {
		sari := a.rentWithLink(t, "Sari Puspita")
		tokens = append(tokens, sari.token)
		acct, err := env.Billing.Account(ctx, a.admin, sari.tenantID)
		require.NoError(t, err)
		end := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
		_, err = env.Tenants.SetPlannedEnd(ctx, a.admin, acct.Invoices[0].LeaseID, &end)
		require.NoError(t, err)

		resp, body := getPage(t, srv.URL, sari.token, caller.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Nil(t, decode[apigen.PaymentPage](t, body).NextInvoiceDueDate)
		resp, body = caller.post(t, sari.token, "/promise", `{"kind":"with_next_invoice"}`)
		requireAPIError(t, resp, body, http.StatusBadRequest, "promise_no_next_invoice", "kind")
	})

	t.Run("a tenant who owes nothing", func(t *testing.T) {
		tono := a.rentWithLink(t, "Tono Hartono")
		tokens = append(tokens, tono.token)
		_, err := env.Billing.RecordPayment(ctx, a.admin, billing.PaymentInput{
			RequestID: uuid.New(), TenantID: tono.tenantID, Amount: 700_000, Method: billing.MethodCash, ReceivedOn: tenant.Today(env.Clock),
		})
		require.NoError(t, err)
		resp, body := caller.post(t, tono.token, "/promise", `{"kind":"specific_date","promisedDate":"2026-11-10"}`)
		requireAPIError(t, resp, body, http.StatusConflict, "promise_nothing_owed", "")

		resp, body = getPage(t, srv.URL, tono.token, caller.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		page := decode[apigen.PaymentPage](t, body)
		require.Nil(t, page.PromiseDates)
		require.Nil(t, page.NextInvoiceDueDate)
	})

	t.Run("a bad token answers as getPaymentPage does", func(t *testing.T) {
		revoked := a.rentWithLink(t, "Yanti Kusuma")
		tokens = append(tokens, revoked.token)
		_, err := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin).RevokeAll(ctx, a.admin, revoked.invoiceID)
		require.NoError(t, err)
		for name, token := range map[string]string{
			"revoked": revoked.token, "unknown": strings.Repeat("A", 43), "malformed": "not-a-token",
		} {
			t.Run(name, func(t *testing.T) {
				pageResp, pageBody := getPage(t, srv.URL, token, caller.freshIP())
				resp, body := caller.post(t, token, "/promise", `{"kind":"with_next_invoice"}`)
				require.Equal(t, pageResp.StatusCode, resp.StatusCode)
				require.Equal(t, decode[apigen.Error](t, pageBody), decode[apigen.Error](t, body))
				requireAPIError(t, resp, body, http.StatusNotFound, "payment_link_not_found", "")
			})
		}
	})

	t.Run("another organization's token reaches only its own tenant", func(t *testing.T) {
		resp, body := caller.post(t, dewi.token, "/promise", `{"kind":"specific_date","promisedDate":"2026-11-15"}`)
		require.Equal(t, 1, requirePaymentPromise(t, resp, body).Version)

		resp, body = adminB.do(http.MethodGet, promisesPath(b.o.ID, dewi.tenantID), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		list := decode[apigen.TenantPromiseList](t, body)
		require.Equal(t, 1, list.Total)
		require.Equal(t, dewi.tenantID, list.Items[0].TenantId)

		resp, body = admin.do(http.MethodGet, promisesPath(a.o.ID, dewi.tenantID), nil)
		requireAPIError(t, resp, body, http.StatusNotFound, "not_found", "")
		resp, body = admin.do(http.MethodGet, promisesPath(a.o.ID, rina.tenantID), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, 1, decode[apigen.TenantPromiseList](t, body).Total, "a's tenant has only its own promise")
	})

	t.Run("an admin records an edit, audited, and lists every version", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, promisesPath(a.o.ID, rina.tenantID), map[string]any{
			"kind": "specific_date", "promisedDate": "2026-11-20", "promisedAmount": 300_000, "note": "  Janji lewat telepon  ",
		})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		got := decode[apigen.TenantPromise](t, body)
		require.Equal(t, rina.tenantID, got.TenantId)
		require.Equal(t, apigen.PromiseStatusActive, got.Status)
		require.Nil(t, got.ClosedAt)
		require.Len(t, got.Versions, 3)
		v := got.Versions[2]
		require.Equal(t, 3, v.Version)
		require.Equal(t, apigen.PromiseFromAdmin, v.Source)
		require.Equal(t, ptrTo("Janji lewat telepon"), v.Note)
		require.NotNil(t, v.CreatedBy)
		require.Equal(t, a.o.Admin.Name, v.CreatedBy.Name)
		require.Equal(t, int64(700_000), v.OutstandingAt)
		require.Equal(t, []uuid.UUID{rina.invoiceID}, v.CoveredInvoiceIds)
		require.Equal(t, apigen.PromiseFromTenantPage, got.Versions[0].Source)
		require.Nil(t, got.Versions[0].CreatedBy)
		require.Nil(t, got.Versions[0].Note)

		require.True(t, hasAudit(env.AuditLog(t, a.o.ID), a.o.Admin.UserID, authz.ActionPromiseCreate, "allow"),
			"the allowed promise is audited")

		resp, body = admin.do(http.MethodGet, promisesPath(a.o.ID, rina.tenantID)+"?page=1&pageSize=5", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		list := decode[apigen.TenantPromiseList](t, body)
		require.Equal(t, apigen.TenantPromiseList{Items: []apigen.TenantPromise{got}, Page: 1, PageSize: 5, Total: 1}, list)

		resp, body = admin.do(http.MethodGet, promisesPath(a.o.ID, rina.tenantID)+"?pageSize=101", nil)
		requireAPIError(t, resp, body, http.StatusBadRequest, "invalid_input", "pageSize")
		resp, body = admin.do(http.MethodGet, promisesPath(a.o.ID, uuid.New()), nil)
		requireAPIError(t, resp, body, http.StatusNotFound, "not_found", "")
		resp, body = admin.do(http.MethodPost, promisesPath(a.o.ID, rina.tenantID), map[string]any{"kind": "specific_date", "promisedDate": "2026-11-02"})
		requireAPIError(t, resp, body, http.StatusBadRequest, "promise_date_out_of_range", "promisedDate")
		resp, body = admin.do(http.MethodPost, promisesPath(a.o.ID, rina.tenantID), map[string]any{"kind": "with_next_invoice", "note": strings.Repeat("a", 501)})
		requireAPIError(t, resp, body, http.StatusBadRequest, "invalid_input", "note")
	})

	t.Run("a karyawan makes promises only for the tenants they collect from, and reads none", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodPost, promisesPath(a.o.ID, rina.tenantID), map[string]any{"kind": "with_next_invoice"})
		requireAPIError(t, resp, body, http.StatusForbidden, "forbidden", "")
		require.True(t, hasAudit(env.AuditLog(t, a.o.ID), a.o.Karyawan.UserID, authz.ActionPromiseCreate, "deny"),
			"the denial is audited")

		resp, body = karyawan.do(http.MethodPost, promisesPath(a.o.ID, budiID), map[string]any{"kind": "with_next_invoice"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, 1, decode[apigen.TenantPromise](t, body).Versions[0].Version)

		resp, body = karyawan.do(http.MethodGet, promisesPath(a.o.ID, budiID), nil)
		requireAPIError(t, resp, body, http.StatusForbidden, "forbidden", "")
	})

	t.Run("a supervisor reads promises and makes none", func(t *testing.T) {
		resp, body := supervisor.do(http.MethodPost, promisesPath(a.o.ID, rina.tenantID), map[string]any{"kind": "with_next_invoice"})
		requireAPIError(t, resp, body, http.StatusForbidden, "forbidden", "")
		require.True(t, hasAudit(env.AuditLog(t, a.o.ID), a.o.Supervisor.UserID, authz.ActionPromiseCreate, "deny"))

		resp, body = supervisor.do(http.MethodGet, promisesPath(a.o.ID, rina.tenantID), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Len(t, decode[apigen.TenantPromiseList](t, body).Items[0].Versions, 3)
	})

	t.Run("the promise shares the page's rate limit per token", func(t *testing.T) {
		lina := a.rentWithLink(t, "Lina Marlina")
		tokens = append(tokens, lina.token)
		for range 30 {
			resp, body := getPage(t, srv.URL, lina.token, caller.freshIP())
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		}
		resp, body := caller.post(t, lina.token, "/promise", `{"kind":"with_next_invoice"}`)
		requireAPIError(t, resp, body, http.StatusTooManyRequests, "rate_limited", "")
	})

	// Version 3 promised Rp300.000 by 20 Nov. A payment recorded through the API that reaches it
	// closes the promise as kept in the same request.
	t.Run("a manual payment that keeps the promise closes it as kept", func(t *testing.T) {
		pay := func(amount int64) {
			t.Helper()
			resp, body := admin.do(http.MethodPost, "/api/orgs/"+a.o.ID.String()+"/payments", map[string]any{
				"requestId": uuid.New(), "tenantId": rina.tenantID, "amount": amount, "method": "cash", "receivedOn": "2026-11-02",
			})
			require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		}
		latest := func() apigen.TenantPromise {
			t.Helper()
			resp, body := admin.do(http.MethodGet, promisesPath(a.o.ID, rina.tenantID), nil)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			return decode[apigen.TenantPromiseList](t, body).Items[0]
		}
		pay(299_999)
		require.Equal(t, apigen.PromiseStatusActive, latest().Status)
		pay(1)
		got := latest()
		require.Equal(t, apigen.PromiseStatusKept, got.Status)
		require.NotNil(t, got.ClosedAt)
		require.True(t, env.Clock.Now().Equal(*got.ClosedAt))
	})

	t.Run("no token reaches the logs", func(t *testing.T) {
		out := logs.String()
		require.NotEmpty(t, out)
		for _, token := range tokens {
			require.NotContains(t, out, token)
		}
	})
}

func ptrTo[T any](v T) *T { return &v }
