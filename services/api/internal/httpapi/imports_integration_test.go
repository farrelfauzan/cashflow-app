//go:build integration

package httpapi_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestImportOverHTTP(t *testing.T) {
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812171")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Imports: env.Imports,
		CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	path := "/api/orgs/" + o.ID.String() + "/imports/tenants"
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)
	csv := []byte("properti;kamar;nama_penyewa;no_wa;sewa;tanggal_mulai\n" +
		"Kos Melati;Kamar 1;Rina Wulandari;0812-1717-5001;850.000;01/10/2026\n")

	t.Run("without dryRun nothing is saved", func(t *testing.T) {
		resp, body := admin.send(http.MethodPost, path, "text/csv", csv)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		res := decode[apigen.ImportResult](t, body)
		require.True(t, res.DryRun)
		require.False(t, res.Committed)
		require.Equal(t, 1, res.Summary.LeasesCreated)
		require.Equal(t, int64(850_000), res.Rows[0].Values.RentAmount)
		require.Contains(t, string(body), `"startDate":"2026-10-01"`, "01/10/2026 is read day first and shown as a plain date")

		resp, body = admin.do(http.MethodGet, "/api/orgs/"+o.ID.String()+"/tenants", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Empty(t, decode[apigen.TenantList](t, body).Tenants)
	})

	t.Run("dryRun=false saves the rows", func(t *testing.T) {
		resp, body := admin.send(http.MethodPost, path+"?dryRun=false", "text/csv", csv)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		res := decode[apigen.ImportResult](t, body)
		require.True(t, res.Committed)
		require.Equal(t, apigen.ImportRowStatusOk, res.Rows[0].Status)
		require.Equal(t, apigen.ImportAction("created"), *res.Rows[0].Lease)
	})

	t.Run("a bad row comes back with its column, and nothing is saved", func(t *testing.T) {
		bad := []byte("property,unit,tenant_name,phone,rent,start_date\n" +
			"Kos Melati,Kamar 2,Budi Santoso,12345,900.000,2026-10-01\n")
		resp, body := admin.send(http.MethodPost, path+"?dryRun=false", "text/csv", bad)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		res := decode[apigen.ImportResult](t, body)
		require.False(t, res.Committed)
		require.Equal(t, apigen.ImportRowStatusError, res.Rows[0].Status)
		require.Equal(t, "phone", res.Rows[0].Errors[0].Column)
	})

	t.Run("a file the API cannot read is refused with field file", func(t *testing.T) {
		resp, body := admin.send(http.MethodPost, path, "text/csv", []byte("nama,kamar\nRina,1\n"))
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
		e := decode[apigen.Error](t, body)
		require.Equal(t, "invalid_input", e.Error.Code)
		require.Equal(t, "file", *e.Error.Field)
	})

	t.Run("dryRun must be true or false", func(t *testing.T) {
		resp, body := admin.send(http.MethodPost, path+"?dryRun=ya", "text/csv", csv)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
		require.Equal(t, "dryRun", *decode[apigen.Error](t, body).Error.Field)
	})

	t.Run("a file over 1 MiB is refused", func(t *testing.T) {
		big := append(bytes.Clone(csv), bytes.Repeat([]byte("#"), 1<<20)...)
		resp, body := admin.send(http.MethodPost, path, "text/csv", big)
		require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode, string(body))
		require.Equal(t, "too_large", decode[apigen.Error](t, body).Error.Code)
	})

	t.Run("the template downloads as a CSV attachment", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, path+"/template", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, "text/csv; charset=utf-8", resp.Header.Get("Content-Type"))
		require.Equal(t, `attachment; filename="template-impor-penyewa.csv"`, resp.Header.Get("Content-Disposition"))
		require.True(t, bytes.HasPrefix(body, []byte("\xef\xbb\xbfproperti;")), "byte order mark, then the Indonesian header")

		karyawan := newClient(t, srv.URL)
		karyawan.signIn(env, o.Karyawan.Phone)
		resp, _ = karyawan.do(http.MethodGet, path+"/template", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("a karyawan is refused, even for a dry run", func(t *testing.T) {
		karyawan := newClient(t, srv.URL)
		karyawan.signIn(env, o.Karyawan.Phone)
		resp, body := karyawan.send(http.MethodPost, path, "text/csv", csv)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
	})
}
