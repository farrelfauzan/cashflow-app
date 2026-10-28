//go:build integration

package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

const origin = "http://app.test"

// client is one browser: it keeps its own cookies and sends the app's Origin.
type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

// result is what a test needs from a response. The body is already read and closed.
type result struct {
	StatusCode int
	Header     http.Header
}

func (c *client) do(method, path string, body any) (result, []byte) {
	c.t.Helper()
	var raw []byte
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(c.t, err)
		raw = b
	}
	return c.send(method, path, "application/json", raw)
}

// send makes a request with a body that is already encoded, such as a CSV file.
func (c *client) send(method, path, contentType string, body []byte) (result, []byte) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(c.t.Context(), method, c.base+path, r)
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Origin", origin)
	resp, err := c.http.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(c.t, err)
	return result{StatusCode: resp.StatusCode, Header: resp.Header}, data
}

func (c *client) signIn(env *testenv.Env, phone string) result {
	c.t.Helper()
	resp, _ := c.do(http.MethodPost, "/api/auth/otp/request", apigen.LoginCodeRequest{Phone: phone})
	require.Equal(c.t, http.StatusAccepted, resp.StatusCode)
	resp, body := c.do(http.MethodPost, "/api/auth/otp/verify", apigen.LoginCodeVerify{Phone: phone, Code: env.LastCode(c.t)})
	require.Equal(c.t, http.StatusOK, resp.StatusCode, string(body))
	return resp
}

func decode[T any](t *testing.T, data []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(data, &v), string(data))
	return v
}

func TestStaffLoginAndMembersOverHTTP(t *testing.T) {
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812131")
	other := env.CreateOrg(t, "Kos Lain", "0812141")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	admin := newClient(t, srv.URL)

	t.Run("a wrong code is refused", func(t *testing.T) {
		resp, _ := admin.do(http.MethodPost, "/api/auth/otp/request", apigen.LoginCodeRequest{Phone: o.Admin.Phone})
		require.Equal(t, http.StatusAccepted, resp.StatusCode)
		code := env.LastCode(t)
		wrong := "000000"
		if code == wrong {
			wrong = "111111"
		}
		resp, body := admin.do(http.MethodPost, "/api/auth/otp/verify", apigen.LoginCodeVerify{Phone: o.Admin.Phone, Code: wrong})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Contains(t, string(body), `"code":"invalid_code"`)
	})

	t.Run("the admin signs in and gets a safe session cookie", func(t *testing.T) {
		resp := admin.signIn(env, o.Admin.Phone)
		cookie := resp.Header.Get("Set-Cookie")
		require.Contains(t, cookie, "cf_session=")
		require.Contains(t, cookie, "HttpOnly")
		require.Contains(t, cookie, "SameSite=Lax")

		resp, body := admin.do(http.MethodGet, "/api/me", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		me := decode[apigen.MeResponse](t, body)
		require.Equal(t, o.Admin.UserID, me.User.Id)
		require.Len(t, me.Organizations, 1)
		require.Equal(t, o.ID, me.Organizations[0].Id)
	})

	t.Run("the admin's roles and capabilities", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgPath+"/me", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		me := decode[apigen.OrgMeResponse](t, body)
		require.Equal(t, "admin", me.Roles[0].Key)
		require.Nil(t, me.Roles[0].PropertyName, "an organization-wide role names no property")
		access := map[string]apigen.CapabilityAccess{}
		for _, c := range me.Capabilities {
			access[c.Action] = c.Access
		}
		require.Equal(t, apigen.Allow, access["member:invite"])
		require.Equal(t, apigen.Conditional, access["role:assign"])
		require.Equal(t, apigen.Deny, access["payroll:approve"])
	})

	var newMember apigen.Member
	t.Run("the admin lists and adds members", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgPath+"/members", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Len(t, decode[apigen.MemberList](t, body).Members, 3)

		resp, body = admin.do(http.MethodPost, orgPath+"/members", apigen.InviteMember{
			Phone: "0812 1313 9999", Name: "Mbak Sari", Roles: []apigen.RoleGrant{{Role: apigen.Karyawan}},
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		newMember = decode[apigen.Member](t, body)
		require.Equal(t, "+6281213139999", newMember.Phone)
	})

	t.Run("input errors name the field", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/members", apigen.InviteMember{
			Phone: "12", Name: "X", Roles: []apigen.RoleGrant{{Role: apigen.Karyawan}},
		})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		e := decode[apigen.Error](t, body)
		require.Equal(t, "invalid_input", e.Error.Code)
		require.Equal(t, "phone", *e.Error.Field)
	})

	t.Run("the admin cannot make anyone a supervisor", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, orgPath+"/members/"+newMember.Id.String()+"/roles",
			apigen.SetRoles{Roles: []apigen.RoleGrant{{Role: apigen.Supervisor}}})
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		require.Contains(t, string(body), `"code":"forbidden"`)
		require.NotContains(t, string(body), "role/", "the response does not leak policy detail")
	})

	t.Run("another organization looks like it does not exist", func(t *testing.T) {
		resp, _ := admin.do(http.MethodGet, "/api/orgs/"+other.ID.String()+"/members", nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		resp, _ = admin.do(http.MethodGet, "/api/orgs/"+uuid.NewString()+"/members", nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("a karyawan cannot see the member list, and the attempt is audited", func(t *testing.T) {
		karyawan := newClient(t, srv.URL)
		karyawan.signIn(env, o.Karyawan.Phone)
		resp, _ := karyawan.do(http.MethodGet, orgPath+"/members", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)

		audit := env.AuditLog(t, o.ID)
		require.Equal(t, "deny", audit[0].Decision)
		require.Equal(t, "member:read", audit[0].Action)
		require.Equal(t, o.Karyawan.UserID, audit[0].ActorStaffUserID.UUID)
	})

	t.Run("a request from another site is refused", func(t *testing.T) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/auth/logout", strings.NewReader(""))
		require.NoError(t, err)
		req.Header.Set("Origin", "https://evil.example.com")
		resp, err := admin.http.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("logout ends the session", func(t *testing.T) {
		resp, _ := admin.do(http.MethodPost, "/api/auth/logout", nil)
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		resp, _ = admin.do(http.MethodGet, "/api/me", nil)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestPropertiesOverHTTP(t *testing.T) {
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812151")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)

	create := func(name string, kind apigen.PropertyKind) apigen.Property {
		t.Helper()
		resp, body := admin.do(http.MethodPost, orgPath+"/properties", apigen.PropertyInput{Name: name, Kind: kind})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		return decode[apigen.Property](t, body)
	}
	melati := create("Kos Melati", apigen.Kos)
	mawar := create("Kontrakan Mawar", apigen.Kontrakan)

	var unit apigen.Unit
	t.Run("the admin adds and changes a unit", func(t *testing.T) {
		rentAmount := int64(1_250_000)
		resp, body := admin.do(http.MethodPost, orgPath+"/properties/"+melati.Id.String()+"/units",
			apigen.UnitInput{Label: "Kamar 11", DefaultRent: &rentAmount})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		unit = decode[apigen.Unit](t, body)
		require.Equal(t, melati.Id, unit.PropertyId)
		require.Equal(t, rentAmount, *unit.DefaultRent)

		resp, body = admin.do(http.MethodPut, orgPath+"/units/"+unit.Id.String(), apigen.UnitInput{Label: "Kamar 12"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Nil(t, decode[apigen.Unit](t, body).DefaultRent)

		resp, body = admin.do(http.MethodGet, orgPath+"/properties/"+melati.Id.String()+"/units", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Len(t, decode[apigen.UnitList](t, body).Units, 1)

		resp, body = admin.do(http.MethodGet, orgPath+"/properties/"+melati.Id.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		got := decode[apigen.Property](t, body)
		require.Equal(t, melati.Id, got.Id)
		require.Equal(t, int64(1), got.UnitCount)
	})

	t.Run("conflicts and bad input name the field", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/properties", apigen.PropertyInput{Name: "kos melati", Kind: apigen.Kos})
		require.Equal(t, http.StatusConflict, resp.StatusCode)
		e := decode[apigen.Error](t, body)
		require.Equal(t, "name_taken", e.Error.Code)
		require.Equal(t, "name", *e.Error.Field)

		resp, body = admin.do(http.MethodPost, orgPath+"/properties/"+melati.Id.String()+"/units", apigen.UnitInput{Label: "KAMAR 12"})
		require.Equal(t, http.StatusConflict, resp.StatusCode)
		require.Equal(t, "label_taken", decode[apigen.Error](t, body).Error.Code)

		resp, body = admin.do(http.MethodPut, orgPath+"/properties/"+melati.Id.String(), apigen.PropertyInput{Name: "Kos Melati", Kind: "apartemen"})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		e = decode[apigen.Error](t, body)
		require.Equal(t, "invalid_input", e.Error.Code)
		require.Equal(t, "kind", *e.Error.Field)

		resp, _ = admin.do(http.MethodPut, orgPath+"/units/"+uuid.NewString(), apigen.UnitInput{Label: "Kamar 1"})
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		resp, _ = admin.do(http.MethodGet, orgPath+"/properties/not-a-uuid/units", nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		resp, body = admin.do(http.MethodGet, orgPath+"/properties/"+uuid.NewString(), nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		require.Equal(t, "not_found", decode[apigen.Error](t, body).Error.Code)
		resp, _ = admin.do(http.MethodGet, orgPath+"/properties/not-a-uuid", nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("an invite limited to an unknown property is refused", func(t *testing.T) {
		unknown := uuid.New()
		resp, body := admin.do(http.MethodPost, orgPath+"/members", apigen.InviteMember{
			Phone: "0812 1515 8888", Name: "Pak Joko", Roles: []apigen.RoleGrant{{Role: apigen.Admin, PropertyId: &unknown}},
		})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
		e := decode[apigen.Error](t, body)
		require.Equal(t, "invalid_input", e.Error.Code)
		require.Equal(t, "roles", *e.Error.Field)
	})

	limited := newClient(t, srv.URL)
	t.Run("an admin limited to one property sees only that property", func(t *testing.T) {
		// One call: the member never holds the Admin role for the whole organization.
		resp, body := admin.do(http.MethodPost, orgPath+"/members", apigen.InviteMember{
			Phone: "0812 1515 9999", Name: "Mbak Sari", Roles: []apigen.RoleGrant{{Role: apigen.Admin, PropertyId: &mawar.Id}},
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		sari := decode[apigen.Member](t, body)
		require.Len(t, sari.Roles, 1)
		require.Equal(t, "admin", sari.Roles[0].Key)
		require.Equal(t, mawar.Id, *sari.Roles[0].PropertyId)
		require.Equal(t, mawar.Name, *sari.Roles[0].PropertyName, "the role names its property")

		limited.signIn(env, "0812 1515 9999")

		resp, body = limited.do(http.MethodGet, orgPath+"/properties", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		props := decode[apigen.PropertyList](t, body).Properties
		require.Len(t, props, 1)
		require.Equal(t, mawar.Id, props[0].Id)

		resp, body = limited.do(http.MethodGet, orgPath+"/me", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		me := decode[apigen.OrgMeResponse](t, body)
		require.Equal(t, mawar.Id, *me.Roles[0].PropertyId)
		require.Equal(t, mawar.Name, *me.Roles[0].PropertyName, "/me names the property too")
		access := map[string]apigen.CapabilityAccess{}
		for _, c := range me.Capabilities {
			access[c.Action] = c.Access
		}
		require.Equal(t, apigen.Conditional, access["unit:manage"])
		for _, action := range []string{"member:read", "member:invite", "role:assign", "settings:update"} {
			require.Equal(t, apigen.Deny, access[action], "%s: records with no property, as the server answers", action)
		}
		resp, _ = limited.do(http.MethodGet, orgPath+"/members", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)

		resp, _ = limited.do(http.MethodGet, orgPath+"/properties/"+mawar.Id.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		resp, body = limited.do(http.MethodGet, orgPath+"/properties/"+melati.Id.String(), nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		require.Contains(t, string(body), `"code":"forbidden"`)
		resp, _ = limited.do(http.MethodGet, orgPath+"/properties/"+melati.Id.String()+"/units", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		resp, _ = limited.do(http.MethodPut, orgPath+"/units/"+unit.Id.String(), apigen.UnitInput{Label: "Kamar 13"})
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		resp, _ = limited.do(http.MethodPost, orgPath+"/properties/"+mawar.Id.String()+"/units", apigen.UnitInput{Label: "Rumah A"})
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("an admin limited to one property cannot invite, even into that property", func(t *testing.T) {
		resp, body := limited.do(http.MethodPost, orgPath+"/members", apigen.InviteMember{
			Phone: "0812 1515 7777", Name: "Mas Budi", Roles: []apigen.RoleGrant{{Role: apigen.Karyawan, PropertyId: &mawar.Id}},
		})
		require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"code":"forbidden"`)
	})

	t.Run("a karyawan cannot list properties", func(t *testing.T) {
		karyawan := newClient(t, srv.URL)
		karyawan.signIn(env, o.Karyawan.Phone)
		resp, body := karyawan.do(http.MethodGet, orgPath+"/properties", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		require.Contains(t, string(body), `"code":"forbidden"`)
	})
}

func TestTenantsAndLeasesOverHTTP(t *testing.T) {
	env := testenv.New(t) // the clock reads 2 November 2026 in Jakarta
	o := env.CreateOrg(t, "Griya Sejahtera", "0812161")

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

	resp, body = admin.do(http.MethodGet, orgPath+"/members", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var karyawanMember uuid.UUID
	for _, m := range decode[apigen.MemberList](t, body).Members {
		if m.UserId == o.Karyawan.UserID {
			karyawanMember = m.Id
		}
	}

	var rina apigen.Tenant
	t.Run("the admin adds a tenant", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: "Rina Wulandari", Phone: "0812 1616 5001"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		rina = decode[apigen.Tenant](t, body)
		require.Equal(t, "+6281216165001", rina.Phone)
		require.Contains(t, string(body), `"leases":[]`, "an empty list, never null")

		resp, body = admin.do(http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: "Rina", Phone: "+62 812-1616-5001"})
		require.Equal(t, http.StatusConflict, resp.StatusCode)
		e := decode[apigen.Error](t, body)
		require.Equal(t, "phone_taken", e.Error.Code)
		require.Equal(t, "phone", *e.Error.Field)
	})

	var lease apigen.Lease
	t.Run("the admin rents a unit with dates as plain days", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": rina.Id, "unitId": kamar1.Id, "startDate": "2026-10-01",
			"rentAmount": 850000, "depositAmount": 850000, "collectorMemberId": karyawanMember,
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"startDate":"2026-10-01"`)
		lease = decode[apigen.Lease](t, body)
		require.Equal(t, apigen.LeaseStatusActive, lease.Status)
		require.Equal(t, o.Karyawan.Name, lease.Collector.Name)

		resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": rina.Id, "unitId": kamar1.Id, "startDate": "2026-11-01", "rentAmount": 1,
		})
		require.Equal(t, http.StatusConflict, resp.StatusCode)
		require.Equal(t, "unit_occupied", decode[apigen.Error](t, body).Error.Code)

		resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
			"tenantId": rina.Id, "unitId": kamar1.Id, "startDate": "1 Oktober 2026", "rentAmount": 1,
		})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Equal(t, "invalid_json", decode[apigen.Error](t, body).Error.Code)
	})

	t.Run("the karyawan sees the tenant they collect from", func(t *testing.T) {
		karyawan := newClient(t, srv.URL)
		karyawan.signIn(env, o.Karyawan.Phone)
		resp, body := karyawan.do(http.MethodGet, orgPath+"/tenants", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		tenants := decode[apigen.TenantList](t, body).Tenants
		require.Len(t, tenants, 1)
		require.Equal(t, rina.Id, tenants[0].Id)

		resp, _ = karyawan.do(http.MethodPost, orgPath+"/leases/"+lease.Id.String()+"/move-out", apigen.MoveOut{})
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("the admin records the move-out", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/leases/"+lease.Id.String()+"/move-out", map[string]string{"endDate": "2026-12-01"})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.Equal(t, "endDate", *decode[apigen.Error](t, body).Error.Field)

		resp, body = admin.do(http.MethodPost, orgPath+"/leases/"+lease.Id.String()+"/move-out", map[string]string{"endDate": "2026-11-02"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"endDate":"2026-11-02"`)
		require.Contains(t, string(body), `"status":"ended"`)

		resp, body = admin.do(http.MethodPut, orgPath+"/leases/"+lease.Id.String(), map[string]any{
			"startDate": "2026-10-01", "rentAmount": 1,
		})
		require.Equal(t, http.StatusConflict, resp.StatusCode)
		require.Equal(t, "lease_ended", decode[apigen.Error](t, body).Error.Code)

		resp, body = admin.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Len(t, decode[apigen.Tenant](t, body).Leases, 1)
		resp, _ = admin.do(http.MethodGet, orgPath+"/tenants/"+uuid.NewString(), nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
