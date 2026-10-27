//go:build integration

package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestMemberInvitationOverHTTP(t *testing.T) {
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812191")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)
	karyawan := newClient(t, srv.URL)
	karyawan.signIn(env, o.Karyawan.Phone)
	karyawanP := env.Principal(t, o.ID, o.Karyawan.UserID)

	resend := func(c *client, id uuid.UUID) (result, []byte) {
		t.Helper()
		return c.do(http.MethodPost, orgPath+"/members/"+id.String()+"/invitation", nil)
	}
	invite := func(phone, name string) (result, []byte) {
		t.Helper()
		return admin.do(http.MethodPost, orgPath+"/members", apigen.InviteMember{
			Phone: phone, Name: name, Roles: []apigen.RoleGrant{{Role: apigen.Karyawan}},
		})
	}
	requireError := func(t *testing.T, resp result, data []byte, status int, code string) {
		t.Helper()
		require.Equal(t, status, resp.StatusCode, string(data))
		e := decode[apigen.Error](t, data)
		require.Equal(t, code, e.Error.Code)
		require.LessOrEqual(t, utf8.RuneCountInString(e.Error.Message), 40, "short enough for the members column: %q", e.Error.Message)
	}

	t.Run("founders have no invitation and have signed in or not", func(t *testing.T) {
		resp, data := admin.do(http.MethodGet, orgPath+"/members", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Contains(t, string(data), `"invitation":null`)
		for _, m := range decode[apigen.MemberList](t, data).Members {
			require.Nil(t, m.Invitation, m.Name)
			if m.Phone == "+62812191002" || m.Phone == "+62812191003" {
				require.NotNil(t, m.FirstSignInAt, "%s signed in above", m.Name)
			} else {
				require.Nil(t, m.FirstSignInAt, m.Name)
			}
		}
	})

	var sari apigen.Member
	t.Run("an invite answers 201 with the invitation sent", func(t *testing.T) {
		resp, data := invite("0812 1919 0001", "Mbak Sari")
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))
		require.Contains(t, string(data), `"firstSignInAt":null`)
		sari = decode[apigen.Member](t, data)
		require.NotNil(t, sari.Invitation)
		require.Equal(t, apigen.Sent, sari.Invitation.State)
		require.NotNil(t, sari.Invitation.LastSentAt)
		require.True(t, env.Clock.Now().Equal(*sari.Invitation.LastSentAt))

		sent := env.Messenger.Sent()
		last := sent[len(sent)-1]
		require.Equal(t, messaging.TemplateMemberInvite, last.Template)
		require.Equal(t, "+6281219190001", last.To)
		require.Equal(t, testenv.LoginURL, last.Buttons[0].URL)
	})

	t.Run("inviting the same number again is a conflict and sends nothing", func(t *testing.T) {
		before := len(env.Messenger.Sent())
		resp, data := invite("0812 1919 0001", "Mbak Sari")
		require.Equal(t, http.StatusConflict, resp.StatusCode, string(data))
		require.Equal(t, "already_member", decode[apigen.Error](t, data).Error.Code)
		require.Len(t, env.Messenger.Sent(), before)
	})

	t.Run("a resend answers the member with a new lastSentAt", func(t *testing.T) {
		env.Clock.Advance(5 * time.Minute)
		resp, data := resend(admin, sari.Id)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
		m := decode[apigen.Member](t, data)
		require.Equal(t, apigen.Sent, m.Invitation.State)
		require.True(t, env.Clock.Now().Equal(*m.Invitation.LastSentAt))
	})

	t.Run("a resend for a founder is not_invited", func(t *testing.T) {
		resp, data := resend(admin, karyawanP.MembershipID)
		requireError(t, resp, data, http.StatusConflict, "not_invited")
	})

	t.Run("an unknown member is not found", func(t *testing.T) {
		resp, _ := resend(admin, uuid.New())
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		resp, _ = admin.do(http.MethodPost, orgPath+"/members/not-a-uuid/invitation", nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("a karyawan cannot resend, and nothing is sent", func(t *testing.T) {
		before := len(env.Messenger.Sent())
		resp, data := resend(karyawan, sari.Id)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, string(data))
		require.Equal(t, "forbidden", decode[apigen.Error](t, data).Error.Code)
		require.Len(t, env.Messenger.Sent(), before)
	})

	t.Run("when WhatsApp is down the invite succeeds and the resend is 503", func(t *testing.T) {
		env.Messenger.FailWith(messaging.ErrNotConnected)
		t.Cleanup(func() { env.Messenger.FailWith(nil) })

		resp, data := invite("0812 1919 0002", "Pak Joko")
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))
		joko := decode[apigen.Member](t, data)
		require.Equal(t, apigen.NotSent, joko.Invitation.State)
		require.Nil(t, joko.Invitation.LastSentAt)
		require.NotContains(t, string(data), "lastSentAt")

		resp, data = resend(admin, joko.Id)
		requireError(t, resp, data, http.StatusServiceUnavailable, "whatsapp_unavailable")
		require.NotContains(t, string(data), "gowa")
		require.NotContains(t, strings.ToLower(string(data)), "connected")

		resp, data = admin.do(http.MethodGet, orgPath+"/members", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		for _, m := range decode[apigen.MemberList](t, data).Members {
			if m.Id == joko.Id {
				require.Equal(t, apigen.NotSent, m.Invitation.State)
			}
		}
	})

	t.Run("too many sends to one number are rate_limited", func(t *testing.T) {
		// Sari has had the invite and one resend.
		for range member.InvitesPerPhone - 2 {
			resp, data := resend(admin, sari.Id)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
		}
		resp, data := resend(admin, sari.Id)
		requireError(t, resp, data, http.StatusTooManyRequests, "rate_limited")
	})

	t.Run("a disabled member is member_disabled", func(t *testing.T) {
		resp, data := admin.do(http.MethodPut, orgPath+"/members/"+sari.Id.String()+"/status",
			apigen.SetMemberStatus{Status: apigen.MemberStatusDisabled})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
		resp, data = resend(admin, sari.Id)
		requireError(t, resp, data, http.StatusConflict, "member_disabled")
	})

	t.Run("the invitee's first sign-in shows on the member", func(t *testing.T) {
		resp, data := invite("0812 1919 0003", "Mas Eko")
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(data))
		eko := decode[apigen.Member](t, data)
		require.Nil(t, eko.FirstSignInAt)

		newClient(t, srv.URL).signIn(env, "0812 1919 0003")
		resp, data = admin.do(http.MethodGet, orgPath+"/members", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		for _, m := range decode[apigen.MemberList](t, data).Members {
			if m.Id == eko.Id {
				require.NotNil(t, m.FirstSignInAt)
				require.True(t, env.Clock.Now().Equal(*m.FirstSignInAt))
			}
		}
	})
}
