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

func TestLeaseChargesOverHTTP(t *testing.T) {
	env := testenv.New(t) // the clock reads 2 November 2026 in Jakarta
	o := env.CreateOrg(t, "Griya Sejahtera", "0812171")

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
	resp, body = admin.do(http.MethodPost, orgPath+"/properties/"+melati.Id.String()+"/units", apigen.UnitInput{Label: "Kamar 1"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	kamar1 := decode[apigen.Unit](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: "Rina Wulandari", Phone: "0812 1717 5001"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	rina := decode[apigen.Tenant](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
		"tenantId": rina.Id, "unitId": kamar1.Id, "startDate": "2026-10-01", "rentAmount": 850000,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	require.Contains(t, string(body), `"charges":[]`, "an empty list, never null")
	lease := decode[apigen.Lease](t, body)
	chargesPath := orgPath + "/leases/" + lease.Id.String() + "/charges"

	var wifi, listrik apigen.ChargeType
	t.Run("the admin adds charge types and lists them by name", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/charge-types", map[string]any{"name": " Wi-Fi "})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		wifi = decode[apigen.ChargeType](t, body)
		require.Equal(t, "Wi-Fi", wifi.Name)
		require.False(t, wifi.Metered)

		resp, body = admin.do(http.MethodPost, orgPath+"/charge-types", map[string]any{"name": "Listrik", "metered": true})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		listrik = decode[apigen.ChargeType](t, body)

		resp, body = admin.do(http.MethodGet, orgPath+"/charge-types", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		types := decode[apigen.ChargeTypeList](t, body).ChargeTypes
		require.Len(t, types, 2)
		require.Equal(t, "Listrik", types[0].Name)
		require.Equal(t, "Wi-Fi", types[1].Name)
	})

	t.Run("a taken name or bad input names the field", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/charge-types", map[string]any{"name": "WI-FI"})
		require.Equal(t, http.StatusConflict, resp.StatusCode)
		e := decode[apigen.Error](t, body)
		require.Equal(t, "charge_type_taken", e.Error.Code)
		require.Equal(t, "name", *e.Error.Field)

		resp, body = admin.do(http.MethodPost, orgPath+"/charge-types", map[string]any{"name": ""})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Equal(t, "name", *decode[apigen.Error](t, body).Error.Field)

		resp, body = admin.do(http.MethodPost, orgPath+"/charge-types", map[string]any{"name": "Air", "price": 1})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Equal(t, "invalid_json", decode[apigen.Error](t, body).Error.Code)
	})

	t.Run("the admin sets, replaces and clears the lease's extras", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, chargesPath, map[string]any{
			"charges": []map[string]any{{"chargeTypeId": wifi.Id, "amount": 50000}},
		})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		got := decode[apigen.Lease](t, body)
		require.Equal(t, []apigen.LeaseCharge{{ChargeTypeId: wifi.Id, Name: "Wi-Fi", Amount: 50_000}}, got.Charges)

		resp, body = admin.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, got.Charges, decode[apigen.Tenant](t, body).Leases[0].Charges)
		resp, body = admin.do(http.MethodGet, orgPath+"/tenants", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, got.Charges, decode[apigen.TenantList](t, body).Tenants[0].Leases[0].Charges)

		resp, body = admin.do(http.MethodPut, chargesPath, map[string]any{"charges": []any{}})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"charges":[]`)
	})

	t.Run("bad extras name the field", func(t *testing.T) {
		cases := []struct {
			name  string
			body  any
			field string
		}{
			{"no list", map[string]any{}, "charges"},
			{"a null list", map[string]any{"charges": nil}, "charges"},
			{"a metered type", map[string]any{"charges": []map[string]any{{"chargeTypeId": listrik.Id, "amount": 100000}}}, "charges[0].chargeTypeId"},
			{"the same type twice", map[string]any{"charges": []map[string]any{
				{"chargeTypeId": wifi.Id, "amount": 50000}, {"chargeTypeId": wifi.Id, "amount": 60000},
			}}, "charges[1].chargeTypeId"},
			{"a free charge", map[string]any{"charges": []map[string]any{{"chargeTypeId": wifi.Id, "amount": 0}}}, "charges[0].amount"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := admin.do(http.MethodPut, chargesPath, tc.body)
				require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
				e := decode[apigen.Error](t, body)
				require.Equal(t, "invalid_input", e.Error.Code)
				require.Equal(t, tc.field, *e.Error.Field)
			})
		}
		resp, body := admin.do(http.MethodPut, chargesPath, map[string]any{"charges": []map[string]any{{"chargeTypeId": wifi.Id, "amount": 1.5}}})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Equal(t, "invalid_json", decode[apigen.Error](t, body).Error.Code, "rupiah are whole numbers")
	})

	t.Run("a karyawan is refused everywhere", func(t *testing.T) {
		karyawan := newClient(t, srv.URL)
		karyawan.signIn(env, o.Karyawan.Phone)
		resp, _ := karyawan.do(http.MethodGet, orgPath+"/charge-types", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		resp, _ = karyawan.do(http.MethodPost, orgPath+"/charge-types", map[string]any{"name": "Air"})
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		resp, _ = karyawan.do(http.MethodPut, chargesPath, map[string]any{"charges": []any{}})
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("an unknown or ended lease", func(t *testing.T) {
		resp, _ := admin.do(http.MethodPut, orgPath+"/leases/"+uuid.NewString()+"/charges", map[string]any{"charges": []any{}})
		require.Equal(t, http.StatusNotFound, resp.StatusCode)

		resp, body := admin.do(http.MethodPost, orgPath+"/leases/"+lease.Id.String()+"/move-out", map[string]string{"endDate": "2026-11-02"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		resp, body = admin.do(http.MethodPut, chargesPath, map[string]any{"charges": []any{}})
		require.Equal(t, http.StatusConflict, resp.StatusCode)
		require.Equal(t, "lease_ended", decode[apigen.Error](t, body).Error.Code)
	})
}
