//go:build integration

package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestMemberStatusOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812171")
	other := env.CreateOrg(t, "Kos Lain", "0812172")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	otherPath := "/api/orgs/" + other.ID.String()
	adminP := env.Principal(t, o.ID, o.Admin.UserID)
	karyawanP := env.Principal(t, o.ID, o.Karyawan.UserID)

	// The karyawan of o is also an Admin of the other organization.
	_, err := env.Members.Invite(ctx, env.Principal(t, other.ID, other.Admin.UserID), member.Invite{
		Phone: o.Karyawan.Phone, Name: o.Karyawan.Name, Roles: []member.RoleGrant{{Role: authz.RoleAdmin}},
	})
	require.NoError(t, err)

	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)
	karyawan := newClient(t, srv.URL)
	karyawan.signIn(env, o.Karyawan.Phone)

	statusPath := func(membershipID uuid.UUID) string {
		return orgPath + "/members/" + membershipID.String() + "/status"
	}
	setStatus := func(c *client, membershipID uuid.UUID, body any) (result, []byte) {
		t.Helper()
		return c.do(http.MethodPut, statusPath(membershipID), body)
	}

	t.Run("a bad status names the field", func(t *testing.T) {
		for _, body := range []any{
			map[string]string{"status": "removed"},
			map[string]string{},
		} {
			resp, data := setStatus(admin, karyawanP.MembershipID, body)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(data))
			e := decode[apigen.Error](t, data)
			require.Equal(t, "invalid_input", e.Error.Code)
			require.Equal(t, "status", *e.Error.Field)
		}
	})

	t.Run("nobody disables themselves", func(t *testing.T) {
		resp, data := setStatus(admin, adminP.MembershipID, apigen.SetMemberStatus{Status: apigen.MemberStatusDisabled})
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		require.Contains(t, string(data), `"code":"forbidden"`)
	})

	t.Run("an unknown member is not found", func(t *testing.T) {
		resp, _ := setStatus(admin, uuid.New(), apigen.SetMemberStatus{Status: apigen.MemberStatusDisabled})
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("a disabled member's session is refused in that organization only", func(t *testing.T) {
		resp, _ := karyawan.do(http.MethodGet, orgPath+"/me", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		resp, data := setStatus(admin, karyawanP.MembershipID, apigen.SetMemberStatus{Status: apigen.MemberStatusDisabled})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
		m := decode[apigen.Member](t, data)
		require.Equal(t, apigen.MemberStatusDisabled, m.Status)
		require.Len(t, m.Roles, 1)
		require.Equal(t, authz.RoleKaryawan, m.Roles[0].Key)

		resp, data = karyawan.do(http.MethodGet, orgPath+"/me", nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "the same answer as for a non-member")
		require.Contains(t, string(data), `"code":"not_found"`)

		resp, _ = karyawan.do(http.MethodGet, otherPath+"/me", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, "the same session still works in the other organization")

		resp, data = karyawan.do(http.MethodGet, "/api/me", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		me := decode[apigen.MeResponse](t, data)
		require.Len(t, me.Organizations, 1)
		require.Equal(t, other.ID, me.Organizations[0].Id)
	})

	t.Run("re-enabling restores access for the same session", func(t *testing.T) {
		resp, data := setStatus(admin, karyawanP.MembershipID, apigen.SetMemberStatus{Status: apigen.MemberStatusActive})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
		require.Equal(t, apigen.MemberStatusActive, decode[apigen.Member](t, data).Status)

		resp, data = karyawan.do(http.MethodGet, orgPath+"/me", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, authz.RoleKaryawan, decode[apigen.OrgMeResponse](t, data).Roles[0].Key)
	})

	t.Run("the last active admin cannot be disabled or demoted", func(t *testing.T) {
		hendra, err := env.Members.Invite(ctx, adminP, member.Invite{
			Phone: "0812-1710-0001", Name: "Pak Hendra", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}},
		})
		require.NoError(t, err)
		env.GrantCustomRole(t, o.ID, hendra.ID, "Pengelola Anggota",
			`{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["member:manage","role:assign"]}]}`)
		manager := newClient(t, srv.URL)
		manager.signIn(env, "0812-1710-0001")

		resp, data := setStatus(manager, adminP.MembershipID, apigen.SetMemberStatus{Status: apigen.MemberStatusDisabled})
		require.Equal(t, http.StatusConflict, resp.StatusCode, string(data))
		e := decode[apigen.Error](t, data)
		require.Equal(t, "last_admin", e.Error.Code)
		require.Equal(t, "Anggota ini admin terakhir yang aktif. Tambahkan admin lain dulu.", e.Error.Message)

		resp, _ = admin.do(http.MethodGet, orgPath+"/me", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, "the admin is still active")

		resp, data = manager.do(http.MethodPut, orgPath+"/members/"+adminP.MembershipID.String()+"/roles",
			apigen.SetRoles{Roles: []apigen.RoleGrant{{Role: apigen.Karyawan}}})
		require.Equal(t, http.StatusConflict, resp.StatusCode, string(data))
		require.Equal(t, "last_admin", decode[apigen.Error](t, data).Error.Code)

		resp, data = admin.do(http.MethodGet, orgPath+"/me", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, authz.RoleAdmin, decode[apigen.OrgMeResponse](t, data).Roles[0].Key, "the admin keeps the role")
	})
}
