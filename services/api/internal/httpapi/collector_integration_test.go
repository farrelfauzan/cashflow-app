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

// TestDisabledCollectorOverHTTP covers a karyawan who is disabled while leases name them as
// collector: the leases keep them, flagged inactive, and the members list counts them until an
// admin picks another collector.
func TestDisabledCollectorOverHTTP(t *testing.T) {
	env := testenv.New(t) // the clock reads 2 November 2026 in Jakarta
	o := env.CreateOrg(t, "Griya Sejahtera", "0812191")
	other := env.CreateOrg(t, "Kos Lain", "0812192")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	otherPath := "/api/orgs/" + other.ID.String()
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)
	otherAdmin := newClient(t, srv.URL)
	otherAdmin.signIn(env, other.Admin.Phone)

	ok := func(t *testing.T, c *client, method, path string, in any, status int) []byte {
		t.Helper()
		resp, body := c.do(method, path, in)
		require.Equal(t, status, resp.StatusCode, string(body))
		return body
	}
	requireInvalid := func(t *testing.T, resp result, body []byte, field string) {
		t.Helper()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
		e := decode[apigen.Error](t, body).Error
		require.Equal(t, "invalid_input", e.Code)
		require.NotNil(t, e.Field)
		require.Equal(t, field, *e.Field)
	}
	invite := func(t *testing.T, c *client, path, phone, name string, grant apigen.RoleGrant) apigen.Member {
		t.Helper()
		body := ok(t, c, http.MethodPost, path+"/members", apigen.InviteMember{
			Phone: phone, Name: name, Roles: []apigen.RoleGrant{grant},
		}, http.StatusCreated)
		m := decode[apigen.Member](t, body)
		require.Contains(t, string(body), `"collectorLeaseCount":0`)
		return m
	}
	setStatus := func(t *testing.T, m apigen.Member, status apigen.MemberStatus) []byte {
		t.Helper()
		return ok(t, admin, http.MethodPut, orgPath+"/members/"+m.Id.String()+"/status",
			apigen.SetMemberStatus{Status: status}, http.StatusOK)
	}
	counts := func(t *testing.T, c *client, path string) map[uuid.UUID]int64 {
		t.Helper()
		out := map[uuid.UUID]int64{}
		for _, m := range decode[apigen.MemberList](t, ok(t, c, http.MethodGet, path+"/members", nil, http.StatusOK)).Items {
			out[m.Id] = m.CollectorLeaseCount
		}
		return out
	}
	property := func(t *testing.T, c *client, path, name string, kind apigen.PropertyKind, labels ...string) (apigen.Property, map[string]apigen.Unit) {
		t.Helper()
		p := decode[apigen.Property](t, ok(t, c, http.MethodPost, path+"/properties", apigen.PropertyInput{Name: name, Kind: kind}, http.StatusCreated))
		units := map[string]apigen.Unit{}
		for _, label := range labels {
			units[label] = decode[apigen.Unit](t, ok(t, c, http.MethodPost, path+"/properties/"+p.Id.String()+"/units",
				apigen.UnitInput{Label: label, Terms: monthly(850_000)}, http.StatusCreated))
		}
		return p, units
	}
	newTenant := func(t *testing.T, c *client, path, name, phone string) apigen.Tenant {
		t.Helper()
		return decode[apigen.Tenant](t, ok(t, c, http.MethodPost, path+"/tenants", apigen.TenantInput{Name: name, Phone: phone}, http.StatusCreated))
	}
	lease := func(t *testing.T, c *client, path string, tn apigen.Tenant, u apigen.Unit, collector uuid.UUID) apigen.Lease {
		t.Helper()
		return decode[apigen.Lease](t, ok(t, c, http.MethodPost, path+"/leases", map[string]any{
			"tenantId": tn.Id, "unitId": u.Id, "startDate": "2026-10-15", "termMonths": 1, "collectorMemberId": collector,
		}, http.StatusCreated))
	}
	leaseOf := func(t *testing.T, c *client, tn apigen.Tenant, id uuid.UUID) apigen.Lease {
		t.Helper()
		got := decode[apigen.Tenant](t, ok(t, c, http.MethodGet, orgPath+"/tenants/"+tn.Id.String(), nil, http.StatusOK))
		for _, l := range got.Leases {
			if l.Id == id {
				return l
			}
		}
		t.Fatalf("lease %s not found on tenant %s", id, tn.Name)
		return apigen.Lease{}
	}

	// Members of organization A. Dimas is the karyawan CreateOrg made.
	var dimas apigen.Member
	for _, m := range decode[apigen.MemberList](t, ok(t, admin, http.MethodGet, orgPath+"/members", nil, http.StatusOK)).Items {
		if m.UserId == o.Karyawan.UserID {
			dimas = m
		}
	}
	require.NotEqual(t, uuid.Nil, dimas.Id)
	eko := invite(t, admin, orgPath, "0812 1919 4001", "Mas Eko Prasetyo", apigen.RoleGrant{Role: apigen.Karyawan})
	fitri := invite(t, admin, orgPath, "0812 1919 4002", "Mbak Fitri Handayani", apigen.RoleGrant{Role: apigen.Karyawan})

	// Dimas collects two active leases, one on each property, and one that has ended.
	melati, melatiUnits := property(t, admin, orgPath, "Kos Melati", apigen.Kos, "Kamar 1", "Kamar 2")
	_, mawarUnits := property(t, admin, orgPath, "Kontrakan Mawar", apigen.Kontrakan, "Rumah A", "Rumah B")
	rina := newTenant(t, admin, orgPath, "Rina Wulandari", "0812 1919 5001")
	agus := newTenant(t, admin, orgPath, "Agus Santoso", "0812 1919 5002")
	dewi := newTenant(t, admin, orgPath, "Dewi Lestari", "0812 1919 5003")
	rinaLease := lease(t, admin, orgPath, rina, melatiUnits["Kamar 1"], dimas.Id)
	agusLease := lease(t, admin, orgPath, agus, mawarUnits["Rumah A"], dimas.Id)
	dewiLease := lease(t, admin, orgPath, dewi, melatiUnits["Kamar 2"], dimas.Id)
	ok(t, admin, http.MethodPost, orgPath+"/leases/"+dewiLease.Id.String()+"/move-out", map[string]any{"endDate": "2026-11-01"}, http.StatusOK)
	require.True(t, rinaLease.Collector.Active)

	t.Run("disabling the collector keeps them on the leases, flagged inactive", func(t *testing.T) {
		body := setStatus(t, dimas, apigen.MemberStatusDisabled)
		require.Contains(t, string(body), `"status":"disabled"`)
		require.Contains(t, string(body), `"collectorLeaseCount":2`, "the ended lease is not counted")

		resp, body := admin.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"active":false`)
		for _, tn := range decode[apigen.TenantList](t, ok(t, admin, http.MethodGet, orgPath+"/tenants", nil, http.StatusOK)).Items {
			for _, l := range tn.Leases {
				if l.Status == apigen.LeaseStatusActive {
					require.NotNil(t, l.Collector, tn.Name)
					require.Equal(t, dimas.Id, l.Collector.MemberId, tn.Name)
					require.False(t, l.Collector.Active, tn.Name)
				}
			}
		}

		c := counts(t, admin, orgPath)
		require.Equal(t, int64(2), c[dimas.Id])
		require.Equal(t, int64(0), c[eko.Id], "a member who collects nothing")
	})

	t.Run("re-enabling the collector clears the flag", func(t *testing.T) {
		setStatus(t, dimas, apigen.MemberStatusActive)
		require.True(t, leaseOf(t, admin, rina, rinaLease.Id).Collector.Active)
		setStatus(t, dimas, apigen.MemberStatusDisabled)
		require.False(t, leaseOf(t, admin, rina, rinaLease.Id).Collector.Active)
	})

	t.Run("an admin limited to one property sees the flag on its leases; the count stays organization-wide", func(t *testing.T) {
		invite(t, admin, orgPath, "0812 1919 4003", "Mbak Sari Melati", apigen.RoleGrant{Role: apigen.Admin, PropertyId: &melati.Id})
		limited := newClient(t, srv.URL)
		limited.signIn(env, "0812 1919 4003")

		tenants := decode[apigen.TenantList](t, ok(t, limited, http.MethodGet, orgPath+"/tenants", nil, http.StatusOK)).Items
		require.Len(t, tenants, 1, "only the tenant with an active lease on Kos Melati")
		require.Equal(t, rina.Id, tenants[0].Id)
		require.Len(t, tenants[0].Leases, 1)
		require.Equal(t, dimas.Id, tenants[0].Leases[0].Collector.MemberId)
		require.False(t, tenants[0].Leases[0].Collector.Active)

		// The count includes Agus's lease on Kontrakan Mawar, which this admin cannot see.
		require.Equal(t, int64(2), counts(t, admin, orgPath)[dimas.Id])
	})

	t.Run("updateLease keeps the disabled collector it already has", func(t *testing.T) {
		body := ok(t, admin, http.MethodPut, orgPath+"/leases/"+rinaLease.Id.String(), map[string]any{
			"startDate": "2026-10-15", "collectorMemberId": dimas.Id,
		}, http.StatusOK)
		require.Contains(t, string(body), `"active":false`)
		got := decode[apigen.Lease](t, body)
		require.Equal(t, apigen.TermMonths(1), got.TermMonths)
		require.Equal(t, dimas.Id, got.Collector.MemberId)
	})

	t.Run("a different disabled member is refused as collector", func(t *testing.T) {
		setStatus(t, eko, apigen.MemberStatusDisabled)
		resp, body := admin.do(http.MethodPut, orgPath+"/leases/"+rinaLease.Id.String(), map[string]any{
			"startDate": "2026-10-15", "collectorMemberId": eko.Id,
		})
		requireInvalid(t, resp, body, "collectorMemberId")
		require.Equal(t, dimas.Id, leaseOf(t, admin, rina, rinaLease.Id).Collector.MemberId)

		resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": agus.Id, "unitId": mawarUnits["Rumah B"].Id, "startDate": "2026-10-15", "termMonths": 1,
			"collectorMemberId": dimas.Id,
		})
		requireInvalid(t, resp, body, "collectorMemberId")
	})

	t.Run("picking an active member reassigns the lease and lowers the count", func(t *testing.T) {
		got := decode[apigen.Lease](t, ok(t, admin, http.MethodPut, orgPath+"/leases/"+agusLease.Id.String(), map[string]any{
			"startDate": "2026-10-15", "collectorMemberId": fitri.Id,
		}, http.StatusOK))
		require.Equal(t, fitri.Id, got.Collector.MemberId)
		require.True(t, got.Collector.Active)

		c := counts(t, admin, orgPath)
		require.Equal(t, int64(1), c[dimas.Id])
		require.Equal(t, int64(1), c[fitri.Id])
	})

	t.Run("the count is per organization for a member of two", func(t *testing.T) {
		rudiA := invite(t, admin, orgPath, "0812 1919 4004", "Pak Rudi Hartono", apigen.RoleGrant{Role: apigen.Karyawan})
		rudiB := invite(t, otherAdmin, otherPath, "0812 1919 4004", "Pak Rudi Hartono", apigen.RoleGrant{Role: apigen.Karyawan})
		_, units := property(t, otherAdmin, otherPath, "Ruko Kenanga", apigen.Ruko, "Ruko 1")
		otherLease := lease(t, otherAdmin, otherPath, newTenant(t, otherAdmin, otherPath, "Hendra Gunawan", "0812 1919 5004"),
			units["Ruko 1"], rudiB.Id)

		require.Equal(t, int64(1), counts(t, otherAdmin, otherPath)[rudiB.Id])
		require.Equal(t, int64(0), counts(t, admin, orgPath)[rudiA.Id])
		resp, body := admin.do(http.MethodGet, orgPath+"/tenants", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NotContains(t, string(body), otherLease.Id.String())
		require.NotContains(t, string(body), rudiB.Id.String())
	})
}
