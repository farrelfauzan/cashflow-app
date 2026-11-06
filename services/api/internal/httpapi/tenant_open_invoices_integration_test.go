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

// TestTenantOpenInvoicesOverHTTP covers listTenantOpenInvoices: the invoices a tenant still owes,
// of any month, filtered by invoice:read like listInvoices, without needing payment:read.
func TestTenantOpenInvoicesOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 3, 9, 0, 0, 0, clock.Jakarta))
	o := env.CreateOrg(t, "Griya Sejahtera", "0812981")
	other := env.CreateOrg(t, "Kos Lain", "0812982")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing,
		CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	signedIn := func(phone string) *client {
		c := newClient(t, srv.URL)
		c.signIn(env, phone)
		return c
	}
	ok := func(t *testing.T, c *client, method, path string, in any, status int) []byte {
		t.Helper()
		resp, body := c.do(method, path, in)
		require.Equal(t, status, resp.StatusCode, string(body))
		return body
	}
	requireError := func(t *testing.T, c *client, path string, status int, code string) {
		t.Helper()
		resp, body := c.do(http.MethodGet, path, nil)
		require.Equal(t, status, resp.StatusCode, string(body))
		e := decode[apigen.Error](t, body)
		require.Equal(t, code, e.Error.Code)
		require.NotEmpty(t, e.Error.Message)
	}
	admin := signedIn(o.Admin.Phone)
	supervisor := signedIn(o.Supervisor.Phone)
	dimasClient := signedIn(o.Karyawan.Phone)

	invite := func(t *testing.T, phone, name string, grant apigen.RoleGrant) apigen.Member {
		t.Helper()
		return decode[apigen.Member](t, ok(t, admin, http.MethodPost, orgPath+"/members", apigen.InviteMember{
			Phone: phone, Name: name, Roles: []apigen.RoleGrant{grant},
		}, http.StatusCreated))
	}
	property := func(t *testing.T, name string, kind apigen.PropertyKind, label string, price int64) (apigen.Property, apigen.Unit) {
		t.Helper()
		p := decode[apigen.Property](t, ok(t, admin, http.MethodPost, orgPath+"/properties", apigen.PropertyInput{Name: name, Kind: kind}, http.StatusCreated))
		u := decode[apigen.Unit](t, ok(t, admin, http.MethodPost, orgPath+"/properties/"+p.Id.String()+"/units",
			apigen.UnitInput{Label: label, Terms: monthly(price)}, http.StatusCreated))
		return p, u
	}
	newTenant := func(t *testing.T, name, phone string) apigen.Tenant {
		t.Helper()
		return decode[apigen.Tenant](t, ok(t, admin, http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: name, Phone: phone}, http.StatusCreated))
	}
	lease := func(t *testing.T, tn apigen.Tenant, u apigen.Unit, start string, collector uuid.UUID) {
		t.Helper()
		ok(t, admin, http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": tn.Id, "unitId": u.Id, "startDate": start, "termMonths": 1, "collectorMemberId": collector,
		}, http.StatusCreated)
	}

	var dimas apigen.Member
	for _, m := range decode[apigen.MemberList](t, ok(t, admin, http.MethodGet, orgPath+"/members", nil, http.StatusOK)).Items {
		if m.UserId == o.Karyawan.UserID {
			dimas = m
		}
	}
	require.NotEqual(t, uuid.Nil, dimas.Id)
	eko := invite(t, "0812 9819 4001", "Mas Eko Prasetyo", apigen.RoleGrant{Role: apigen.Karyawan})
	invite(t, "0812 9819 4002", "Mbak Fitri Handayani", apigen.RoleGrant{Role: apigen.Karyawan})

	// Wulan rents two units: Kamar 1 in the kos since 3 Aug (entered late today), collected by
	// Dimas, and Rumah A in the kontrakan since 1 Nov, collected by Eko.
	melati, kamar := property(t, "Kos Melati", apigen.Kos, "Kamar 1", 700_000)
	_, rumah := property(t, "Kontrakan Mawar", apigen.Kontrakan, "Rumah A", 1_500_000)
	wulan := newTenant(t, "Wulan Sari", "0812 9819 5001")
	agus := newTenant(t, "Agus Santoso", "0812 9819 5002")
	lease(t, wulan, kamar, "2026-08-03", dimas.Id)
	lease(t, wulan, rumah, "2026-11-01", eko.Id)
	_, err := env.Billing.GenerateOrg(context.Background(), o.ID)
	require.NoError(t, err)

	// August is cancelled, then 900.000 pays September in full and 200.000 of October.
	account := decode[apigen.TenantAccount](t, ok(t, admin, http.MethodGet, orgPath+"/tenants/"+wulan.Id.String()+"/account", nil, http.StatusOK))
	require.Len(t, account.Invoices, 5)
	byPeriod := map[string]apigen.AccountInvoice{}
	for _, inv := range account.Invoices {
		byPeriod[inv.Period.String()] = inv
	}
	ok(t, admin, http.MethodPost, orgPath+"/corrections", map[string]any{
		"requestId": uuid.New(), "invoiceId": byPeriod["2026-08-03"].Id, "amountDelta": -700_000, "reason": "sudah dibayar tunai ke pemilik",
	}, http.StatusCreated)
	ok(t, admin, http.MethodPost, orgPath+"/payments", map[string]any{
		"requestId": uuid.New(), "tenantId": wulan.Id, "amount": 900_000, "method": "cash", "receivedOn": "2026-11-03",
	}, http.StatusCreated)

	october := apigen.TenantOpenInvoice{
		Id: byPeriod["2026-10-03"].Id, Kind: apigen.Regular, Period: date(2026, 10, 3), PeriodEnd: date(2026, 11, 2),
		DueDate: byPeriod["2026-10-03"].DueDate, UnitLabel: "Kamar 1", PropertyName: "Kos Melati", Amount: 700_000, Balance: 500_000,
	}
	rumahNov := apigen.TenantOpenInvoice{
		Id: byPeriod["2026-11-01"].Id, Kind: apigen.Regular, Period: date(2026, 11, 1), PeriodEnd: date(2026, 11, 30),
		DueDate: byPeriod["2026-11-01"].DueDate, UnitLabel: "Rumah A", PropertyName: "Kontrakan Mawar", Amount: 1_500_000, Balance: 1_500_000,
	}
	kamarNov := apigen.TenantOpenInvoice{
		Id: byPeriod["2026-11-03"].Id, Kind: apigen.Regular, Period: date(2026, 11, 3), PeriodEnd: date(2026, 12, 2),
		DueDate: byPeriod["2026-11-03"].DueDate, UnitLabel: "Kamar 1", PropertyName: "Kos Melati", Amount: 700_000, Balance: 700_000,
	}
	path := func(tenantID uuid.UUID) string { return orgPath + "/tenants/" + tenantID.String() + "/open-invoices" }
	list := func(t *testing.T, c *client, tenantID uuid.UUID) ([]apigen.TenantOpenInvoice, string) {
		t.Helper()
		body := ok(t, c, http.MethodGet, path(tenantID), nil, http.StatusOK)
		return decode[[]apigen.TenantOpenInvoice](t, body), string(body)
	}

	invite(t, "0812 9819 4003", "Mbak Sari Melati", apigen.RoleGrant{Role: apigen.Admin, PropertyId: &melati.Id})
	nina := invite(t, "0812 9819 4004", "Nina Tamu", apigen.RoleGrant{Role: apigen.Karyawan})
	env.GrantCustomRole(t, o.ID, nina.Id, "Tanpa tagihan", `{"Version":"2026-10-25","Statement":[`+
		`{"Effect":"Deny","Action":["invoice:read"],"Resource":["*"]}]}`)

	t.Run("who sees which open invoices", func(t *testing.T) {
		tests := []struct {
			name   string
			client *client
			tenant uuid.UUID
			want   []apigen.TenantOpenInvoice
		}{
			{"an admin sees every open invoice of any month, oldest period first", admin, wulan.Id, []apigen.TenantOpenInvoice{october, rumahNov, kamarNov}},
			{"a Supervisor reads everything", supervisor, wulan.Id, []apigen.TenantOpenInvoice{october, rumahNov, kamarNov}},
			{"a karyawan sees the invoices of the lease they collect", dimasClient, wulan.Id, []apigen.TenantOpenInvoice{october, kamarNov}},
			{"another karyawan sees only their lease", signedIn("0812 9819 4001"), wulan.Id, []apigen.TenantOpenInvoice{rumahNov}},
			{"a karyawan who collects nothing of the tenant gets []", signedIn("0812 9819 4002"), wulan.Id, []apigen.TenantOpenInvoice{}},
			{"an admin limited to the kos sees only the kos", signedIn("0812 9819 4003"), wulan.Id, []apigen.TenantOpenInvoice{october, kamarNov}},
			{"a tenant with nothing billed answers []", admin, agus.Id, []apigen.TenantOpenInvoice{}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				got, body := list(t, tc.client, tc.tenant)
				require.Equal(t, tc.want, got)
				if len(tc.want) == 0 {
					require.JSONEq(t, `[]`, body, "an empty array, never null")
				}
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		otherAdmin := signedIn(other.Admin.Phone)
		otherTenant := decode[apigen.Tenant](t, ok(t, otherAdmin, http.MethodPost, "/api/orgs/"+other.ID.String()+"/tenants",
			apigen.TenantInput{Name: "Lina Marlina", Phone: "0812 9829 5001"}, http.StatusCreated))
		tests := []struct {
			name   string
			client *client
			path   string
			status int
			code   string
		}{
			{"an unknown tenant", admin, path(uuid.New()), http.StatusNotFound, "not_found"},
			{"a tenant of another organization", admin, path(otherTenant.Id), http.StatusNotFound, "not_found"},
			{"a malformed tenant id", admin, orgPath + "/tenants/kamar-1/open-invoices", http.StatusNotFound, "not_found"},
			{"a member whose invoice:read is denied", signedIn("0812 9819 4004"), path(wulan.Id), http.StatusForbidden, "forbidden"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				requireError(t, tc.client, tc.path, tc.status, tc.code)
			})
		}
	})

	t.Run("a karyawan without payment:read reads open invoices while the account stays 403", func(t *testing.T) {
		got, _ := list(t, dimasClient, wulan.Id)
		require.Len(t, got, 2)
		requireError(t, dimasClient, orgPath+"/tenants/"+wulan.Id.String()+"/account", http.StatusForbidden, "forbidden")
	})
}
