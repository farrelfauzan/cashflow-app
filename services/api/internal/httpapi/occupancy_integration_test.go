//go:build integration

package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestOccupancyOverHTTP(t *testing.T) {
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812181")
	adminP := env.Principal(t, o.ID, o.Admin.UserID)
	ctx := t.Context()

	melati, err := env.Properties.Create(ctx, adminP, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	kamar1, err := env.Properties.CreateUnit(ctx, adminP, melati.ID, property.UnitInput{Label: "Kamar 1"})
	require.NoError(t, err)
	_, err = env.Properties.CreateUnit(ctx, adminP, melati.ID, property.UnitInput{Label: "Kamar 2"})
	require.NoError(t, err)
	rina, err := env.Tenants.Create(ctx, adminP, tenant.Input{Name: "Rina Wulandari", Phone: "0812-1818-5001"})
	require.NoError(t, err)
	_, err = env.Tenants.CreateLease(ctx, adminP, tenant.NewLease{TenantID: rina.ID, UnitID: kamar1.ID, Terms: tenant.Terms{
		StartDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), RentAmount: 850_000,
	}})
	require.NoError(t, err)

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)

	t.Run("the unit list says who rents and who does not", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgPath+"/properties/"+melati.ID.String()+"/units", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"occupied":true`)
		require.Contains(t, string(body), `"currentTenant":{"id":"`+rina.ID.String()+`","name":"Rina Wulandari"}`)
		require.Contains(t, string(body), `"occupied":false`)

		units := decode[apigen.UnitList](t, body).Items
		require.Len(t, units, 2)
		require.True(t, units[0].Occupied)
		require.NotNil(t, units[0].CurrentTenant)
		require.False(t, units[1].Occupied)
		require.Nil(t, units[1].CurrentTenant, "a free unit has no currentTenant key")
	})

	t.Run("the property list counts occupied units", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgPath+"/properties", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"occupiedUnitCount":1`)
		require.Contains(t, string(body), `"unitCount":2`)
	})

	t.Run("a new unit and a renamed property fill the new fields", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/properties/"+melati.ID.String()+"/units", apigen.UnitInput{Label: "Kamar 3"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"occupied":false`)
		require.NotContains(t, string(body), "currentTenant")

		resp, body = admin.do(http.MethodPut, orgPath+"/units/"+kamar1.ID.String(), apigen.UnitInput{Label: "Kamar 1"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"occupied":true`)
		require.Contains(t, string(body), `"currentTenant"`)

		resp, body = admin.do(http.MethodPut, orgPath+"/properties/"+melati.ID.String(), apigen.PropertyInput{Name: "Kos Melati", Kind: apigen.Kos})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"occupiedUnitCount":1`)
	})
}
