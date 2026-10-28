//go:build integration

package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestListPaginationOverHTTP(t *testing.T) {
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812791")
	adminP := env.Principal(t, o.ID, o.Admin.UserID)
	ctx := t.Context()

	melati, err := env.Properties.Create(ctx, adminP, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	for _, n := range []string{"Kontrakan Mawar", "Ruko Godean"} {
		_, err := env.Properties.Create(ctx, adminP, property.PropertyInput{Name: n, Kind: property.KindOther})
		require.NoError(t, err)
	}
	var units []property.Unit
	for _, l := range []string{"Kamar 1", "Kamar 2", "Kamar 3"} {
		u, err := env.Properties.CreateUnit(ctx, adminP, melati.ID, property.UnitInput{Label: l})
		require.NoError(t, err)
		units = append(units, u)
	}
	for i, n := range []string{"Budi Santoso", "Citra Lestari", "Rina Wulandari"} {
		tn, err := env.Tenants.Create(ctx, adminP, tenant.Input{Name: n, Phone: fmt.Sprintf("0857-7910-%04d", i+1)})
		require.NoError(t, err)
		_, err = env.Tenants.CreateLease(ctx, adminP, tenant.NewLease{TenantID: tn.ID, UnitID: units[i].ID, Terms: tenant.Terms{
			StartDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), RentAmount: 850_000,
		}})
		require.NoError(t, err)
	}

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)

	lists := []struct {
		name, path, deprecated string
		total                  int
	}{
		{name: "members", path: orgPath + "/members", deprecated: "members", total: 3},
		{name: "properties", path: orgPath + "/properties", deprecated: "properties", total: 3},
		{name: "units", path: orgPath + "/properties/" + melati.ID.String() + "/units", deprecated: "units", total: 3},
		{name: "tenants", path: orgPath + "/tenants", deprecated: "tenants", total: 3},
	}

	get := func(t *testing.T, path string, v url.Values) (int, []byte) {
		t.Helper()
		if len(v) > 0 {
			path += "?" + v.Encode()
		}
		resp, body := admin.do(http.MethodGet, path, nil)
		return resp.StatusCode, body
	}

	for _, l := range lists {
		t.Run(l.name+": items, page, pageSize and total, with the deprecated array holding the same rows", func(t *testing.T) {
			for _, tc := range []struct {
				name                     string
				query                    url.Values
				wantPage, wantSize, rows int
			}{
				{name: "defaults are page 1 at 20", wantPage: 1, wantSize: 20, rows: 3},
				{name: "page 2 at 2", query: url.Values{"page": {"2"}, "pageSize": {"2"}}, wantPage: 2, wantSize: 2, rows: 1},
				{name: "past the last page", query: url.Values{"page": {"5"}, "pageSize": {"2"}}, wantPage: 5, wantSize: 2, rows: 0},
				{name: "the largest page size", query: url.Values{"pageSize": {"100"}}, wantPage: 1, wantSize: 100, rows: 3},
			} {
				t.Run(tc.name, func(t *testing.T) {
					status, body := get(t, l.path, tc.query)
					require.Equal(t, http.StatusOK, status, string(body))
					var raw map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(body, &raw))
					require.JSONEq(t, string(raw["items"]), string(raw[l.deprecated]))
					var shape struct {
						Items          []json.RawMessage `json:"items"`
						Page, PageSize int
						Total          int
					}
					require.NoError(t, json.Unmarshal(body, &shape))
					require.NotNil(t, shape.Items, "items is an array, never null")
					require.Len(t, shape.Items, tc.rows)
					require.Equal(t, tc.wantPage, shape.Page)
					require.Equal(t, tc.wantSize, shape.PageSize)
					require.Equal(t, l.total, shape.Total)
				})
			}
		})
	}

	t.Run("the typed responses decode and items match the deprecated arrays", func(t *testing.T) {
		_, body := get(t, lists[3].path, url.Values{"q": {"rina"}})
		tl := decode[apigen.TenantList](t, body)
		require.Equal(t, 1, tl.Total)
		require.Equal(t, "Rina Wulandari", tl.Items[0].Name)
		require.Equal(t, tl.Items, tl.Tenants)

		_, body = get(t, lists[2].path, url.Values{"sort": {"-label"}, "pageSize": {"1"}})
		ul := decode[apigen.UnitList](t, body)
		require.Equal(t, "Kamar 3", ul.Items[0].Label)
		require.Equal(t, ul.Items, ul.Units)
	})

	const (
		pageMsg   = "Halaman harus angka dari 1 sampai 1.000.000."
		sizeMsg   = "Jumlah per halaman harus angka dari 1 sampai 100."
		qMsg      = "Pencarian paling panjang 100 huruf."
		sortMsg   = "Urutan tidak dikenal."
		statusMsg = "Status tidak dikenal."
		propMsg   = "Properti tidak dikenal."
		flagMsg   = "Nilai collectorDisabled harus true atau false."
	)
	type badCase struct {
		name       string
		query      url.Values
		field, msg string
		onlyFor    []string // list names; empty means every list
	}
	bad := []badCase{
		{name: "pageSize 101", query: url.Values{"pageSize": {"101"}}, field: "pageSize", msg: sizeMsg},
		{name: "pageSize 0", query: url.Values{"pageSize": {"0"}}, field: "pageSize", msg: sizeMsg},
		{name: "pageSize -1", query: url.Values{"pageSize": {"-1"}}, field: "pageSize", msg: sizeMsg},
		{name: "page 0", query: url.Values{"page": {"0"}}, field: "page", msg: pageMsg},
		{name: "page abc", query: url.Values{"page": {"abc"}}, field: "page", msg: pageMsg},
		{name: "page 1.5", query: url.Values{"page": {"1.5"}}, field: "page", msg: pageMsg},
		{name: "page 1000001", query: url.Values{"page": {"1000001"}}, field: "page", msg: pageMsg},
		{name: "q of 101 characters", query: url.Values{"q": {strings.Repeat("a", 101)}}, field: "q", msg: qMsg},
		{name: "sort bogus", query: url.Values{"sort": {"bogus"}}, field: "sort", msg: sortMsg},
		{name: "status bogus", query: url.Values{"status": {"bogus"}}, field: "status", msg: statusMsg, onlyFor: []string{"members", "tenants"}},
		{name: "propertyId not a uuid", query: url.Values{"propertyId": {"not-a-uuid"}}, field: "propertyId", msg: propMsg, onlyFor: []string{"tenants"}},
		{name: "collectorDisabled yes", query: url.Values{"collectorDisabled": {"yes"}}, field: "collectorDisabled", msg: flagMsg, onlyFor: []string{"members", "tenants"}},
		{name: "collectorDisabled 1", query: url.Values{"collectorDisabled": {"1"}}, field: "collectorDisabled", msg: flagMsg, onlyFor: []string{"members", "tenants"}},
	}
	for _, l := range lists {
		t.Run(l.name+": bad parameters are a 400 naming the field, in Indonesian", func(t *testing.T) {
			for _, tc := range bad {
				if len(tc.onlyFor) > 0 && !slices.Contains(tc.onlyFor, l.name) {
					continue
				}
				t.Run(tc.name, func(t *testing.T) {
					status, body := get(t, l.path, tc.query)
					require.Equal(t, http.StatusBadRequest, status, string(body))
					e := decode[apigen.Error](t, body)
					require.Equal(t, "invalid_input", e.Error.Code)
					require.NotNil(t, e.Error.Field)
					require.Equal(t, tc.field, *e.Error.Field)
					require.Equal(t, tc.msg, e.Error.Message)
				})
			}
		})
	}

	t.Run("edge values that are allowed", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			path  string
			query url.Values
		}{
			{name: "q of 100 characters counted as letters, not bytes", path: lists[3].path, query: url.Values{"q": {strings.Repeat("é", 100)}}},
			{name: "the last page number", path: lists[0].path, query: url.Values{"page": {"1000000"}}},
			{name: "collectorDisabled false", path: lists[3].path, query: url.Values{"collectorDisabled": {"false"}}},
			{name: "a well-formed but unknown propertyId", path: lists[3].path, query: url.Values{"propertyId": {"00000000-0000-4000-8000-000000000000"}}},
			{name: "every tenant status", path: lists[3].path, query: url.Values{"status": {"active"}}},
			{name: "sort createdAt on members", path: lists[0].path, query: url.Values{"sort": {"-createdAt"}}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, body := get(t, tc.path, tc.query)
				require.Equal(t, http.StatusOK, status, string(body))
			})
		}
	})
}
