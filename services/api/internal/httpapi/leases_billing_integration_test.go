//go:build integration

package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestLeaseBillingFieldsOverHTTP(t *testing.T) {
	env := testenv.New(t) // the clock reads 2 November 2026 in Jakarta
	o := env.CreateOrg(t, "Griya Sejahtera", "0812181")

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
	units := map[string]apigen.Unit{}
	for _, label := range []string{"Kamar 1", "Kamar 2", "Kamar 3", "Kamar 4"} {
		resp, body = admin.do(http.MethodPost, orgPath+"/properties/"+melati.Id.String()+"/units", apigen.UnitInput{Label: label})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		units[label] = decode[apigen.Unit](t, body)
	}
	resp, body = admin.do(http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: "Rina Wulandari", Phone: "0812 1818 5001"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	rina := decode[apigen.Tenant](t, body)

	createLease := func(unit string, extra map[string]any) (result, []byte) {
		in := map[string]any{"tenantId": rina.Id, "unitId": units[unit].Id, "rentAmount": 850000}
		for k, v := range extra {
			in[k] = v
		}
		return admin.do(http.MethodPost, orgPath+"/leases", in)
	}
	leasePath := func(l apigen.Lease, suffix string) string {
		return orgPath + "/leases/" + l.Id.String() + suffix
	}
	requireError := func(t *testing.T, resp result, body []byte, status int, code, field string) {
		t.Helper()
		require.Equal(t, status, resp.StatusCode, string(body))
		e := decode[apigen.Error](t, body).Error
		require.Equal(t, code, e.Code)
		if field != "" {
			require.NotNil(t, e.Field)
			require.Equal(t, field, *e.Field)
		}
	}

	var lease apigen.Lease
	t.Run("a lease answers with enteredOn and billedFrom, and no plannedEndDate until one is set", func(t *testing.T) {
		resp, body := createLease("Kamar 1", map[string]any{"startDate": "2026-09-15"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"enteredOn":"2026-11-02"`)
		require.Contains(t, string(body), `"billedFrom":"2026-09-15"`, "defaults to the start date")
		require.NotContains(t, string(body), "plannedEndDate")
		lease = decode[apigen.Lease](t, body)
	})

	t.Run("createLease takes billedFrom", func(t *testing.T) {
		resp, body := createLease("Kamar 2", map[string]any{"startDate": "2026-09-15", "billedFrom": "2026-10-15"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"startDate":"2026-09-15"`)
		require.Contains(t, string(body), `"billedFrom":"2026-10-15"`)
	})

	t.Run("a bad billedFrom answers 400 with the field", func(t *testing.T) {
		for name, billedFrom := range map[string]string{
			"not an anniversary": "2026-10-20",
			"before the start":   "2026-08-15",
			"past the bound":     "2026-12-15",
		} {
			t.Run(name, func(t *testing.T) {
				resp, body := createLease("Kamar 3", map[string]any{"startDate": "2026-09-15", "billedFrom": billedFrom})
				requireError(t, resp, body, http.StatusBadRequest, "invalid_input", "billedFrom")
			})
		}
		resp, body := createLease("Kamar 3", map[string]any{"startDate": "2026-09-15", "billedFrom": "15 Oktober 2026"})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_json", "")
	})

	t.Run("updateLease takes billedFrom, and omitting it bills from the start date again", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, leasePath(lease, ""), map[string]any{
			"startDate": "2026-09-15", "rentAmount": 900000, "billedFrom": "2026-11-15",
		})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"billedFrom":"2026-11-15"`)

		resp, body = admin.do(http.MethodPut, leasePath(lease, ""), map[string]any{"startDate": "2026-09-15", "rentAmount": 900000})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"billedFrom":"2026-09-15"`)
		require.Contains(t, string(body), `"enteredOn":"2026-11-02"`)

		resp, body = admin.do(http.MethodPut, leasePath(lease, ""), map[string]any{
			"startDate": "2026-09-15", "rentAmount": 900000, "billedFrom": "2026-12-15",
		})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_input", "billedFrom")
	})

	t.Run("setLeasePlannedEnd sets, changes and clears the date", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": "2026-11-14"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"plannedEndDate":"2026-11-14"`)
		require.Equal(t, apigen.LeaseStatusActive, decode[apigen.Lease](t, body).Status)

		resp, body = admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": "2026-11-30"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"plannedEndDate":"2026-11-30"`)

		resp, body = admin.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"plannedEndDate":"2026-11-30"`, "the tenant's leases carry it too")

		resp, body = admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": nil})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.NotContains(t, string(body), "plannedEndDate", "a cleared date is absent, not null")
		require.Nil(t, decode[apigen.Lease](t, body).PlannedEndDate)
	})

	t.Run("a bad planned end answers 400 with the field", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": "2026-11-01"})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_input", "plannedEndDate") // before today

		resp, body = admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": "2026-09-01"})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_input", "plannedEndDate") // before today and the start

		resp, body = admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": "14 November 2026"})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_json", "")
	})

	t.Run("a start date after the planned end answers 400 with the field", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": "2026-11-14"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		resp, body = admin.do(http.MethodPut, leasePath(lease, ""), map[string]any{"startDate": "2026-11-20", "rentAmount": 900000})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_input", "startDate")
		resp, body = admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": nil})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	})

	t.Run("a karyawan is denied and an unknown lease is not found", func(t *testing.T) {
		karyawan := newClient(t, srv.URL)
		karyawan.signIn(env, o.Karyawan.Phone)
		resp, _ := karyawan.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": "2026-11-14"})
		require.Equal(t, http.StatusForbidden, resp.StatusCode)

		resp, _ = admin.do(http.MethodPut, orgPath+"/leases/"+uuid.NewString()+"/planned-end", map[string]any{"plannedEndDate": nil})
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("move-out clears the date, and an ended lease answers 409 lease_ended", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": "2026-11-14"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		resp, body = admin.do(http.MethodPost, leasePath(lease, "/move-out"), map[string]string{"endDate": "2026-11-02"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"status":"ended"`)
		require.NotContains(t, string(body), "plannedEndDate")

		resp, body = admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": "2026-11-30"})
		requireError(t, resp, body, http.StatusConflict, "lease_ended", "")
		resp, body = admin.do(http.MethodPut, leasePath(lease, "/planned-end"), map[string]any{"plannedEndDate": nil})
		requireError(t, resp, body, http.StatusConflict, "lease_ended", "")
	})
}
