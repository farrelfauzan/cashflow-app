//go:build integration

package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// TestMeteredChargesOverHTTP covers setLeaseCharges' metered types, recordMeteredCharge,
// listLeaseMeteredCharges and listMissingMeterReadings end to end.
func TestMeteredChargesOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 3, 9, 0, 0, 0, clock.Jakarta))
	o := env.CreateOrg(t, "Griya Sejahtera", "0812181")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing,
		CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)
	supervisor := newClient(t, srv.URL)
	supervisor.signIn(env, o.Supervisor.Phone)
	karyawan := newClient(t, srv.URL)
	karyawan.signIn(env, o.Karyawan.Phone)

	requireError := func(t *testing.T, resp result, body []byte, status int, code, field string) {
		t.Helper()
		require.Equal(t, status, resp.StatusCode, string(body))
		e := decode[apigen.Error](t, body)
		require.Equal(t, code, e.Error.Code)
		require.NotEmpty(t, e.Error.Message)
		if field == "" {
			require.Nil(t, e.Error.Field)
			return
		}
		require.NotNil(t, e.Error.Field)
		require.Equal(t, field, *e.Error.Field)
	}

	resp, body := admin.do(http.MethodPost, orgPath+"/properties", apigen.PropertyInput{Name: "Kos Melati", Kind: apigen.Kos})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	melati := decode[apigen.Property](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/properties/"+melati.Id.String()+"/units", apigen.UnitInput{Label: "Kamar 1", Terms: monthly(700_000)})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	kamar1 := decode[apigen.Unit](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: "Rina Wulandari", Phone: "0812 1818 5001"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	rina := decode[apigen.Tenant](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
		"tenantId": rina.Id, "unitId": kamar1.Id, "termMonths": 1, "startDate": "2026-10-01",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	require.Contains(t, string(body), `"meteredChargeTypes":[]`, "an empty list, never null")
	lease := decode[apigen.Lease](t, body)
	_, err := env.Billing.GenerateOrg(context.Background(), o.ID)
	require.NoError(t, err)

	resp, body = admin.do(http.MethodPost, orgPath+"/charge-types", map[string]any{"name": "Wi-Fi"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	wifi := decode[apigen.ChargeType](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/charge-types", map[string]any{"name": "Listrik", "metered": true})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	listrik := decode[apigen.ChargeType](t, body)

	leasePath := orgPath + "/leases/" + lease.Id.String()
	meterPath := func(ct uuid.UUID, month string) string {
		return leasePath + "/metered-charges/" + ct.String() + "/" + month
	}

	t.Run("setLeaseCharges sets the expected metered types, and keeps them when they are left out", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, leasePath+"/charges", map[string]any{
			"charges": []map[string]any{{"chargeTypeId": wifi.Id, "amount": 50000}}, "meteredChargeTypeIds": []uuid.UUID{listrik.Id},
		})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		got := decode[apigen.Lease](t, body)
		require.Equal(t, []apigen.LeaseMeteredType{{ChargeTypeId: listrik.Id, Name: "Listrik"}}, got.MeteredChargeTypes)

		for _, keep := range []map[string]any{{"charges": []any{}}, {"charges": []any{}, "meteredChargeTypeIds": nil}} {
			resp, body = admin.do(http.MethodPut, leasePath+"/charges", keep)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			require.Len(t, decode[apigen.Lease](t, body).MeteredChargeTypes, 1)
		}

		resp, body = admin.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, got.MeteredChargeTypes, decode[apigen.Tenant](t, body).Leases[0].MeteredChargeTypes)

		resp, body = admin.do(http.MethodPut, leasePath+"/charges", map[string]any{"charges": []any{}, "meteredChargeTypeIds": []uuid.UUID{wifi.Id}})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_input", "meteredChargeTypeIds[0]")
		resp, body = admin.do(http.MethodPut, leasePath+"/charges", map[string]any{"charges": []any{}, "meteredChargeTypeIds": []uuid.UUID{listrik.Id, listrik.Id}})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_input", "meteredChargeTypeIds[1]")
	})

	t.Run("the missing-readings list flags last month's reading", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgPath+"/metered-charges/missing", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		list := decode[apigen.MissingMeterReadingList](t, body)
		require.Equal(t, 1, list.Total)
		require.Equal(t, 20, list.PageSize)
		item := list.Items[0]
		require.Equal(t, lease.Id, item.LeaseId)
		require.Equal(t, "Rina Wulandari", item.TenantName)
		require.Equal(t, "Kos Melati", item.PropertyName)
		require.Equal(t, "Kamar 1", item.UnitLabel)
		require.Equal(t, []apigen.LeaseMeteredType{{ChargeTypeId: listrik.Id, Name: "Listrik"}}, item.Missing)
		require.NotNil(t, item.NextInvoice)
		require.Equal(t, "2026-12-01", item.NextInvoice.Period.String())
		require.Equal(t, "2026-11-24", item.NextInvoice.IssuedOn.String())

		resp, body = admin.do(http.MethodGet, orgPath+"/metered-charges/missing?month=2026-09", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Zero(t, decode[apigen.MissingMeterReadingList](t, body).Total, "the lease started in October")
	})

	var recorded apigen.MeteredCharge
	t.Run("the admin records and edits October's listrik", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, meterPath(listrik.Id, "2026-10"), map[string]any{"amount": 123000, "meterStart": 1200, "meterEnd": 1350})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		recorded = decode[apigen.MeteredCharge](t, body)
		require.Equal(t, "2026-10", recorded.UsageMonth)
		require.Equal(t, "Listrik", recorded.ChargeTypeName)
		require.EqualValues(t, 123_000, recorded.Amount)
		require.Equal(t, o.Admin.Name, recorded.RecordedBy.Name)
		require.Nil(t, recorded.Billed)
		require.NotContains(t, string(body), `"billed"`)

		resp, body = admin.do(http.MethodPut, meterPath(listrik.Id, "2026-10"), map[string]any{"amount": 150000})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		edited := decode[apigen.MeteredCharge](t, body)
		require.Equal(t, recorded.Id, edited.Id)
		require.EqualValues(t, 150_000, edited.Amount)
		require.Nil(t, edited.MeterStart)

		resp, body = admin.do(http.MethodGet, orgPath+"/metered-charges/missing", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Zero(t, decode[apigen.MissingMeterReadingList](t, body).Total)
	})

	t.Run("the lease's list shows it with the next invoice", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, leasePath+"/metered-charges", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		list := decode[apigen.MeteredChargeList](t, body)
		require.Equal(t, 1, list.Total)
		require.Equal(t, recorded.Id, list.Items[0].Id)
		require.Equal(t, "2026-12-01", list.NextInvoice.Period.String())
		require.Equal(t, "2026-11-24", list.NextInvoice.IssuedOn.String())

		resp, body = admin.do(http.MethodGet, leasePath+"/metered-charges?usageMonth=2026-09&chargeTypeId="+listrik.Id.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Zero(t, decode[apigen.MeteredChargeList](t, body).Total)

		resp, body = supervisor.do(http.MethodGet, leasePath+"/metered-charges", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, "the Supervisor reads")
	})

	t.Run("bad input names the field", func(t *testing.T) {
		cases := []struct {
			name  string
			path  string
			body  any
			field string
		}{
			{"a usage month that is not YYYY-MM", meterPath(listrik.Id, "2026-13"), map[string]any{"amount": 1}, "usageMonth"},
			{"a usage month of a day", meterPath(listrik.Id, "2026-10-01"), map[string]any{"amount": 1}, "usageMonth"},
			{"a month before the lease", meterPath(listrik.Id, "2026-09"), map[string]any{"amount": 1}, "usageMonth"},
			{"a month after this one", meterPath(listrik.Id, "2026-12"), map[string]any{"amount": 1}, "usageMonth"},
			{"a malformed charge type", leasePath + "/metered-charges/listrik/2026-10", map[string]any{"amount": 1}, "chargeTypeId"},
			{"an unknown charge type", meterPath(uuid.New(), "2026-10"), map[string]any{"amount": 1}, "chargeTypeId"},
			{"a type that is not metered", meterPath(wifi.Id, "2026-10"), map[string]any{"amount": 1}, "chargeTypeId"},
			{"no amount", meterPath(listrik.Id, "2026-10"), map[string]any{}, "amount"},
			{"a negative meter", meterPath(listrik.Id, "2026-10"), map[string]any{"amount": 1, "meterStart": -1}, "meterStart"},
			{"the meter going backwards", meterPath(listrik.Id, "2026-10"), map[string]any{"amount": 1, "meterStart": 10, "meterEnd": 9}, "meterEnd"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := admin.do(http.MethodPut, tc.path, tc.body)
				requireError(t, resp, body, http.StatusBadRequest, "invalid_input", tc.field)
			})
		}
		resp, body := admin.do(http.MethodPut, meterPath(listrik.Id, "2026-10"), map[string]any{"amount": 1, "kwh": 5})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_json", "")

		for param, field := range map[string]string{
			"usageMonth=Okt": "usageMonth", "chargeTypeId=listrik": "chargeTypeId", "page=0": "page", "pageSize=101": "pageSize",
		} {
			resp, body := admin.do(http.MethodGet, leasePath+"/metered-charges?"+param, nil)
			requireError(t, resp, body, http.StatusBadRequest, "invalid_input", field)
		}
		for param, field := range map[string]string{
			"month=2026-1": "month", "month=2026-12": "month", "propertyId=kos": "propertyId", "pageSize=0": "pageSize",
		} {
			resp, body := admin.do(http.MethodGet, orgPath+"/metered-charges/missing?"+param, nil)
			requireError(t, resp, body, http.StatusBadRequest, "invalid_input", field)
		}
		resp, body = admin.do(http.MethodGet, orgPath+"/leases/"+uuid.NewString()+"/metered-charges", nil)
		requireError(t, resp, body, http.StatusNotFound, "not_found", "")
		resp, body = admin.do(http.MethodPut, orgPath+"/leases/"+uuid.NewString()+"/metered-charges/"+listrik.Id.String()+"/2026-10", map[string]any{"amount": 1})
		requireError(t, resp, body, http.StatusNotFound, "not_found", "")
	})

	t.Run("the Supervisor and another collector's karyawan are refused", func(t *testing.T) {
		resp, body := supervisor.do(http.MethodPut, meterPath(listrik.Id, "2026-10"), map[string]any{"amount": 1})
		requireError(t, resp, body, http.StatusForbidden, "forbidden", "")
		resp, body = karyawan.do(http.MethodPut, meterPath(listrik.Id, "2026-10"), map[string]any{"amount": 1})
		requireError(t, resp, body, http.StatusForbidden, "forbidden", "")
		resp, body = karyawan.do(http.MethodGet, leasePath+"/metered-charges", nil)
		requireError(t, resp, body, http.StatusForbidden, "forbidden", "")
		resp, body = karyawan.do(http.MethodGet, orgPath+"/metered-charges/missing", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Zero(t, decode[apigen.MissingMeterReadingList](t, body).Total, "no tenant is assigned to them")
	})

	t.Run("/me reports the new actions", func(t *testing.T) {
		access := func(c *client) map[string]apigen.CapabilityAccess {
			resp, body := c.do(http.MethodGet, orgPath+"/me", nil)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			out := map[string]apigen.CapabilityAccess{}
			for _, cp := range decode[apigen.OrgMeResponse](t, body).Capabilities {
				out[cp.Action] = cp.Access
			}
			return out
		}
		a, s, k := access(admin), access(supervisor), access(karyawan)
		require.Equal(t, apigen.Allow, a["charge:record"])
		require.Equal(t, apigen.Allow, s["charge:read"])
		require.Equal(t, apigen.Deny, s["charge:record"])
		require.Equal(t, apigen.Conditional, k["charge:record"])
	})

	t.Run("once billed the amount is locked and the line names it", func(t *testing.T) {
		env.Clock.Set(time.Date(2026, 11, 24, 6, 0, 0, 0, clock.Jakarta))
		_, err := env.Billing.GenerateOrg(context.Background(), o.ID)
		require.NoError(t, err)

		resp, body := admin.do(http.MethodPut, meterPath(listrik.Id, "2026-10"), map[string]any{"amount": 1})
		requireError(t, resp, body, http.StatusConflict, "metered_charge_billed", "")

		resp, body = admin.do(http.MethodGet, leasePath+"/metered-charges", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		item := decode[apigen.MeteredChargeList](t, body).Items[0]
		require.NotNil(t, item.Billed)
		require.Equal(t, "2026-12-01", item.Billed.Period.String())
		require.Equal(t, apigen.Regular, item.Billed.Kind)

		resp, body = admin.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String()+"/account", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		acct := decode[apigen.TenantAccount](t, body)
		dec := acct.Invoices[len(acct.Invoices)-1]
		require.Equal(t, item.Billed.InvoiceId, dec.Id)
		var metered []apigen.InvoiceLine
		for _, l := range dec.Lines {
			if l.MeteredChargeId != nil {
				metered = append(metered, l)
			}
		}
		require.Len(t, metered, 1)
		require.Equal(t, recorded.Id, *metered[0].MeteredChargeId)
		require.Equal(t, "Listrik Oktober 2026", metered[0].Label)
		require.Nil(t, dec.Lines[0].MeteredChargeId, "the rent line names none")

		resp, body = admin.do(http.MethodGet, orgPath+"/invoices?period=2026-12", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"meteredChargeId":"`+recorded.Id.String()+`"`)
	})

	t.Run("move-out bills the rest on a final invoice; an ended lease answers lease_ended", func(t *testing.T) {
		// Recorded after the 1 Dec invoice was made, so no regular invoice bills it.
		resp, body := admin.do(http.MethodPut, meterPath(listrik.Id, "2026-11"), map[string]any{"amount": 98000})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		november := decode[apigen.MeteredCharge](t, body)

		resp, body = admin.do(http.MethodPost, leasePath+"/move-out", map[string]string{"endDate": "2026-11-24"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		resp, body = admin.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String()+"/account", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		acct := decode[apigen.TenantAccount](t, body)
		kinds := map[string]apigen.InvoiceKind{}
		for _, inv := range acct.Invoices {
			kinds[inv.Period.String()] = inv.Kind
		}
		require.Equal(t, map[string]apigen.InvoiceKind{
			"2026-10-01": apigen.Regular, "2026-11-01": apigen.Regular, "2026-11-24": apigen.Final, "2026-12-01": apigen.Regular,
		}, kinds)
		final := acct.Invoices[2]
		require.Equal(t, "2026-11-24", final.DueDate.String())
		require.EqualValues(t, 98_000, final.Amount)
		require.Len(t, final.Lines, 1)
		require.Equal(t, "Listrik November 2026", final.Lines[0].Label)
		require.Equal(t, november.Id, *final.Lines[0].MeteredChargeId)
		dec := acct.Invoices[3]
		require.EqualValues(t, 150_000, dec.Billed, "the 1 Dec rent is cancelled, Listrik Oktober billed on it stays owed")

		resp, body = admin.do(http.MethodGet, orgPath+"/invoices?period=2026-11", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		list := decode[apigen.InvoiceList](t, body)
		var listed []apigen.InvoiceKind
		for _, it := range list.Items {
			if it.Id == final.Id {
				listed = append(listed, it.Kind)
			}
		}
		require.Equal(t, []apigen.InvoiceKind{apigen.Final}, listed)
		require.Contains(t, string(body), `"kind":"final"`)
		require.Contains(t, string(body), `"kind":"regular"`)

		resp, body = admin.do(http.MethodPut, meterPath(listrik.Id, "2026-11"), map[string]any{"amount": 1})
		requireError(t, resp, body, http.StatusConflict, "lease_ended", "")
		resp, body = admin.do(http.MethodGet, leasePath+"/metered-charges", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NotContains(t, string(body), `"nextInvoice"`)
	})
}
