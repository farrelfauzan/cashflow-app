//go:build integration

package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestCollectorCandidatesOverHTTP(t *testing.T) {
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812393")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)
	path := "/api/orgs/" + o.ID.String() + "/collector-candidates"

	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)
	resp, body := admin.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	got := decode[apigen.CollectorCandidateList](t, body).Candidates
	require.Len(t, got, 3)
	require.Equal(t, o.Karyawan.Name, got[0].Name)
	require.True(t, got[0].Karyawan)
	require.NotContains(t, string(body), "phone", "names only, for the picker")
	require.NotContains(t, string(body), "roles")

	karyawan := newClient(t, srv.URL)
	karyawan.signIn(env, o.Karyawan.Phone)
	resp, body = karyawan.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
}
