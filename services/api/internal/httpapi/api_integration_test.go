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
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(c.t, err)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(c.t.Context(), method, c.base+path, r)
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
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
			Phone: "0812 1313 9999", Name: "Mbak Sari", Roles: []apigen.InviteMemberRoles{apigen.InviteMemberRolesKaryawan},
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		newMember = decode[apigen.Member](t, body)
		require.Equal(t, "+6281213139999", newMember.Phone)
	})

	t.Run("input errors name the field", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/members", apigen.InviteMember{
			Phone: "12", Name: "X", Roles: []apigen.InviteMemberRoles{apigen.InviteMemberRolesKaryawan},
		})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		e := decode[apigen.Error](t, body)
		require.Equal(t, "invalid_input", e.Error.Code)
		require.Equal(t, "phone", *e.Error.Field)
	})

	t.Run("the admin cannot make anyone a supervisor", func(t *testing.T) {
		resp, body := admin.do(http.MethodPut, orgPath+"/members/"+newMember.Id.String()+"/roles",
			apigen.SetRoles{Roles: []apigen.RoleGrant{{Role: apigen.RoleGrantRoleSupervisor}}})
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
	})

	t.Run("an admin limited to one property sees only that property", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/members", apigen.InviteMember{
			Phone: "0812 1515 9999", Name: "Mbak Sari", Roles: []apigen.InviteMemberRoles{apigen.InviteMemberRolesKaryawan},
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		sari := decode[apigen.Member](t, body)

		resp, body = admin.do(http.MethodPut, orgPath+"/members/"+sari.Id.String()+"/roles", apigen.SetRoles{Roles: []apigen.RoleGrant{
			{Role: apigen.RoleGrantRoleAdmin, PropertyId: &mawar.Id},
		}})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		updated := decode[apigen.Member](t, body)
		require.Len(t, updated.Roles, 1)
		require.Equal(t, mawar.Id, *updated.Roles[0].PropertyId)

		limited := newClient(t, srv.URL)
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
		access := map[string]apigen.CapabilityAccess{}
		for _, c := range me.Capabilities {
			access[c.Action] = c.Access
		}
		require.Equal(t, apigen.Conditional, access["unit:manage"])

		resp, _ = limited.do(http.MethodGet, orgPath+"/properties/"+melati.Id.String()+"/units", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		resp, _ = limited.do(http.MethodPut, orgPath+"/units/"+unit.Id.String(), apigen.UnitInput{Label: "Kamar 13"})
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		resp, _ = limited.do(http.MethodPost, orgPath+"/properties/"+mawar.Id.String()+"/units", apigen.UnitInput{Label: "Rumah A"})
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("a karyawan cannot list properties", func(t *testing.T) {
		karyawan := newClient(t, srv.URL)
		karyawan.signIn(env, o.Karyawan.Phone)
		resp, body := karyawan.do(http.MethodGet, orgPath+"/properties", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		require.Contains(t, string(body), `"code":"forbidden"`)
	})
}
