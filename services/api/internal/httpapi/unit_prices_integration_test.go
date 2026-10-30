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

func TestUnitPricesAndTermsOverHTTP(t *testing.T) {
	env := testenv.New(t) // the clock reads 2 November 2026 in Jakarta
	o := env.CreateOrg(t, "Griya Sejahtera", "0812191")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)
	supervisor := newClient(t, srv.URL)
	supervisor.signIn(env, o.Supervisor.Phone)

	resp, body := admin.do(http.MethodPost, orgPath+"/properties", apigen.PropertyInput{Name: "Kos Melati", Kind: apigen.Kos})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	melati := decode[apigen.Property](t, body)
	createUnit := func(in apigen.UnitInput) apigen.Unit {
		t.Helper()
		resp, body := admin.do(http.MethodPost, orgPath+"/properties/"+melati.Id.String()+"/units", in)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		return decode[apigen.Unit](t, body)
	}
	price := func(v int64) *int64 { return &v }
	kamar1 := createUnit(apigen.UnitInput{Label: "Kamar 1", Terms: &[]apigen.UnitTermInput{
		{Months: 12, Price: price(9_000_000)}, {Months: 1, Price: price(850_000)},
	}})
	kamar2 := createUnit(apigen.UnitInput{Label: "Kamar 2"})
	pricesPath := orgPath + "/units/" + kamar1.Id.String() + "/prices"

	requireError := func(t *testing.T, resp result, body []byte, status int, code, field string) {
		t.Helper()
		require.Equal(t, status, resp.StatusCode, string(body))
		e := decode[apigen.Error](t, body).Error
		require.Equal(t, code, e.Code)
		require.Equal(t, field, *e.Field)
		require.NotEmpty(t, e.Message)
	}

	t.Run("createUnitPrice", func(t *testing.T) {
		tests := []struct {
			name   string
			body   any
			status int
			code   string
			field  string
		}{
			{"a later price is recorded", map[string]any{"months": 1, "amount": 900_000, "effectiveFrom": "2027-01-01"}, http.StatusCreated, "", ""},
			{"a term the unit does not offer", map[string]any{"months": 3, "amount": 2_400_000, "effectiveFrom": "2027-01-01"},
				http.StatusConflict, "term_not_offered", "months"},
			{"a date before today", map[string]any{"months": 1, "amount": 900_000, "effectiveFrom": "2026-11-01"},
				http.StatusConflict, "price_too_early", "effectiveFrom"},
			{"a custom term the unit does not offer", map[string]any{"months": 2, "amount": 1_700_000, "effectiveFrom": "2027-01-01"},
				http.StatusConflict, "term_not_offered", "months"},
			{"a term over 60 months", map[string]any{"months": 61, "amount": 900_000, "effectiveFrom": "2027-01-01"},
				http.StatusBadRequest, "invalid_input", "months"},
			{"no amount", map[string]any{"months": 1, "effectiveFrom": "2027-01-01"}, http.StatusBadRequest, "invalid_input", "amount"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := admin.do(http.MethodPost, pricesPath, tc.body)
				if tc.code != "" {
					requireError(t, resp, body, tc.status, tc.code, tc.field)
					return
				}
				require.Equal(t, tc.status, resp.StatusCode, string(body))
				got := decode[apigen.UnitPrice](t, body)
				require.Equal(t, apigen.TermMonths(1), got.Months)
				require.Equal(t, int64(900_000), got.Amount)
				require.Contains(t, string(body), `"effectiveFrom":"2027-01-01"`)
				require.Equal(t, o.Admin.Name, *got.RecordedByName)
			})
		}

		resp, body := admin.do(http.MethodPost, pricesPath, map[string]any{"months": 3, "amount": 1, "effectiveFrom": "2027-01-01"})
		require.Contains(t, decode[apigen.Error](t, body).Error.Message, "3 bulan", "the message names the term")
		require.Equal(t, http.StatusConflict, resp.StatusCode)

		resp, _ = supervisor.do(http.MethodPost, pricesPath, map[string]any{"months": 1, "amount": 1, "effectiveFrom": "2027-01-01"})
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		resp, _ = admin.do(http.MethodPost, orgPath+"/units/"+uuid.NewString()+"/prices", map[string]any{"months": 1, "amount": 1, "effectiveFrom": "2027-01-01"})
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("listUnitPrices answers the history, newest first", func(t *testing.T) {
		resp, body := supervisor.do(http.MethodGet, pricesPath, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		history := decode[apigen.UnitPriceHistory](t, body)
		require.Len(t, history, 3)
		require.Equal(t, int64(900_000), history[0].Amount)
		require.Contains(t, string(body), `"effectiveFrom":"2026-11-02"`)

		resp, _ = admin.do(http.MethodGet, orgPath+"/units/"+uuid.NewString()+"/prices", nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("a unit answers its terms with today's price and the next one", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgPath+"/properties/"+melati.Id.String()+"/units", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body),
			`"terms":[{"months":1,"nextPrice":{"amount":900000,"effectiveFrom":"2027-01-01"},"price":850000},{"months":12,"price":9000000}]`)
		require.Contains(t, string(body), `"terms":[{"months":1}]`, "Kamar 2 offers the kos default with no price")
	})

	resp, body = admin.do(http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: "Rina Wulandari", Phone: "0812 1919 5001"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	rina := decode[apigen.Tenant](t, body)

	t.Run("createLease refuses a term that is not offered or not priced", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": rina.Id, "unitId": kamar1.Id, "startDate": "2026-11-01", "termMonths": 6,
		})
		requireError(t, resp, body, http.StatusConflict, "term_not_offered", "termMonths")
		resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": rina.Id, "unitId": kamar2.Id, "startDate": "2026-11-01", "termMonths": 1,
		})
		requireError(t, resp, body, http.StatusConflict, "no_price", "termMonths")
		resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": rina.Id, "unitId": kamar1.Id, "startDate": "2026-11-01", "termMonths": 61,
		})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_input", "termMonths")
		resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": rina.Id, "unitId": kamar1.Id, "startDate": "2026-11-01", "termMonths": 1, "rentAmount": 850000,
		})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "the old rentAmount field is unknown")
		require.Equal(t, "invalid_json", decode[apigen.Error](t, body).Error.Code)
	})

	t.Run("a lease answers its term and the period rent in force today", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": rina.Id, "unitId": kamar1.Id, "startDate": "2026-11-01", "termMonths": 12,
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"termMonths":12`)
		require.Contains(t, string(body), `"periodRent":9000000`)
		lease := decode[apigen.Lease](t, body)

		resp, body = admin.do(http.MethodPut, orgPath+"/leases/"+lease.Id.String(), map[string]any{"startDate": "2026-11-01", "depositAmount": 500000})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"termMonths":12`, "an update keeps the term")
	})

	t.Run("a unit offers one custom term and a lease bills by it", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/properties/"+melati.Id.String()+"/units", map[string]any{
			"label": "Kamar 3", "terms": []map[string]any{{"months": 1, "price": 850_000}, {"months": 2, "price": 1_650_000}, {"months": 24}},
		})
		requireError(t, resp, body, http.StatusBadRequest, "invalid_input", "terms")

		kamar3 := createUnit(apigen.UnitInput{Label: "Kamar 3", Terms: &[]apigen.UnitTermInput{
			{Months: 2, Price: price(1_650_000)}, {Months: 1, Price: price(850_000)},
		}})
		require.Equal(t, []apigen.UnitTerm{{Months: 1, Price: price(850_000)}, {Months: 2, Price: price(1_650_000)}}, kamar3.Terms)

		resp, body = admin.do(http.MethodPost, orgPath+"/tenants", map[string]any{"name": "Wulan Sari", "phone": "0812-1910-5009"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		wulan := decode[apigen.Tenant](t, body)
		resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": wulan.Id, "unitId": kamar3.Id, "startDate": "2026-11-01", "termMonths": 2,
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"termMonths":2`)
		require.Contains(t, string(body), `"periodRent":1650000`)
	})
}
