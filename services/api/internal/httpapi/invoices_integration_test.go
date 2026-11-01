//go:build integration

package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// TestInvoicedLeasesOverHTTP covers the lease and price endpoints whose answers change once a
// lease has invoices (slice 2).
func TestInvoicedLeasesOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 10, 15, 9, 0, 0, 0, clock.Jakarta))
	o := env.CreateOrg(t, "Griya Sejahtera", "0812841")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)

	resp, body := admin.do(http.MethodPost, orgPath+"/properties", apigen.PropertyInput{Name: "Kos Melati", Kind: apigen.Kos})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	melati := decode[apigen.Property](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/properties/"+melati.Id.String()+"/units", apigen.UnitInput{Label: "Kamar 1", Terms: monthly(700_000)})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	kamar1 := decode[apigen.Unit](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: "Rina Wulandari", Phone: "0812 8418 5001"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	rina := decode[apigen.Tenant](t, body)

	resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
		"tenantId": rina.Id, "unitId": kamar1.Id, "termMonths": 1, "startDate": "2026-10-15", "depositAmount": 200_000,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	lease := decode[apigen.Lease](t, body)
	leasePath := orgPath + "/leases/" + lease.Id.String()
	require.Contains(t, env.Queue.Tasks(), invoicetask.Task{
		ID: invoicetask.LeaseTaskID(o.ID, lease.Id), OrganizationID: o.ID,
	}, "createLease enqueues the organization's generate task after it commits")

	requireError := func(t *testing.T, resp result, body []byte, status int, code, field string) apigen.Error {
		t.Helper()
		require.Equal(t, status, resp.StatusCode, string(body))
		e := decode[apigen.Error](t, body)
		require.Equal(t, code, e.Error.Code)
		require.NotNil(t, e.Error.Field)
		require.Equal(t, field, *e.Error.Field)
		return e
	}
	generate := func(t *testing.T) {
		t.Helper()
		_, err := env.Billing.GenerateOrg(context.Background(), o.ID)
		require.NoError(t, err)
	}

	t.Run("updateLease takes a new start date until the first invoice", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, leasePath, map[string]any{"startDate": "2026-10-16", "billedFrom": "2026-10-16", "depositAmount": 200_000})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		resp, body = admin.do(http.MethodPut, leasePath, map[string]any{"startDate": "2026-10-15", "billedFrom": "2026-10-15", "depositAmount": 200_000})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	})

	t.Run("once invoiced, a new start date or billedFrom answers 409 lease_has_invoices", func(t *testing.T) {
		generate(t)
		resp, body := admin.do(http.MethodPut, leasePath, map[string]any{"startDate": "2026-10-16", "billedFrom": "2026-10-16"})
		e := requireError(t, resp, body, http.StatusConflict, "lease_has_invoices", "startDate")
		require.Contains(t, e.Error.Message, "tagihan")

		resp, body = admin.do(http.MethodPut, leasePath, map[string]any{"startDate": "2026-10-15", "billedFrom": "2026-11-15"})
		requireError(t, resp, body, http.StatusConflict, "lease_has_invoices", "billedFrom")

		resp, body = admin.do(http.MethodPut, leasePath, map[string]any{"startDate": "2026-10-15", "depositAmount": 250_000})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	})

	t.Run("a price for the day the unit was invoiced answers 409 price_too_early, with the date", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/units/"+kamar1.Id.String()+"/prices",
			map[string]any{"months": 1, "amount": 750_000, "effectiveFrom": "2026-10-15"})
		e := requireError(t, resp, body, http.StatusConflict, "price_too_early", "effectiveFrom")
		require.Contains(t, e.Error.Message, "15/10/2026")
		require.Contains(t, e.Error.Message, "16/10/2026")

		resp, body = admin.do(http.MethodPut, orgPath+"/units/"+kamar1.Id.String(), apigen.UnitInput{Label: "Kamar 1", Terms: monthly(750_000)})
		requireError(t, resp, body, http.StatusConflict, "price_too_early", "terms")

		resp, body = admin.do(http.MethodPost, orgPath+"/units/"+kamar1.Id.String()+"/prices",
			map[string]any{"months": 1, "amount": 750_000, "effectiveFrom": "2026-10-16"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	})

	t.Run("a planned end that cancels an invoice needs correction:create (403, nothing changes)", func(t *testing.T) {
		env.Clock.Set(time.Date(2026, 11, 8, 5, 0, 0, 0, clock.Jakarta))
		generate(t) // the 15 Nov invoice

		karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)
		env.GrantCustomRole(t, o.ID, karyawan.MembershipID, "Pengelola sewa",
			`{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["lease:manage","tenant:read"]}]}`)
		leaseOnly := newClient(t, srv.URL)
		leaseOnly.signIn(env, o.Karyawan.Phone)

		resp, body := leaseOnly.do(http.MethodPut, leasePath+"/planned-end", map[string]any{"plannedEndDate": "2026-11-14"})
		require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
		require.Equal(t, "forbidden", decode[apigen.Error](t, body).Error.Code)

		resp, body = admin.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.NotContains(t, string(body), "plannedEndDate", "nothing changed")

		resp, body = admin.do(http.MethodPut, leasePath+"/planned-end", map[string]any{"plannedEndDate": "2026-11-14"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body), "the admin has correction:create")
	})

	t.Run("move-out settles the lease and answers the ended lease", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, leasePath+"/move-out", map[string]string{"endDate": "2026-11-08"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, apigen.LeaseStatusEnded, decode[apigen.Lease](t, body).Status)
		require.False(t, strings.Contains(string(body), "plannedEndDate"))
	})
}
