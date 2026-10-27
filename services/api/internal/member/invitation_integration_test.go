//go:build integration

package member_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// memberByID finds a member in the organization's list, as the members screen sees it.
func memberByID(t *testing.T, env *testenv.Env, p authz.Principal, id uuid.UUID) member.Member {
	t.Helper()
	members, err := env.Members.List(context.Background(), p)
	require.NoError(t, err)
	for _, m := range members {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("member %s is not in the list", id)
	return member.Member{}
}

func requireSentAt(t *testing.T, m member.Member, at time.Time) {
	t.Helper()
	require.NotNil(t, m.Invitation)
	require.Equal(t, member.InvitationSent, m.Invitation.State)
	require.NotNil(t, m.Invitation.LastSentAt)
	require.True(t, at.Equal(*m.Invitation.LastSentAt), "last sent at %s, want %s", m.Invitation.LastSentAt, at)
}

func requireNotSent(t *testing.T, m member.Member, lastSentAt *time.Time) {
	t.Helper()
	require.NotNil(t, m.Invitation)
	require.Equal(t, member.InvitationNotSent, m.Invitation.State)
	if lastSentAt == nil {
		require.Nil(t, m.Invitation.LastSentAt)
		return
	}
	require.NotNil(t, m.Invitation.LastSentAt)
	require.True(t, lastSentAt.Equal(*m.Invitation.LastSentAt))
}

func TestInviteSendsTheInvitation(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812381")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)

	t.Run("founders were never invited", func(t *testing.T) {
		members, err := env.Members.List(ctx, admin)
		require.NoError(t, err)
		require.Len(t, members, 3)
		for _, m := range members {
			require.Nil(t, m.Invitation, m.Name)
		}
		require.Empty(t, env.Messenger.Sent(), "creating the organization sends nothing")
	})

	var sari member.Member
	t.Run("an invite sends one message to the invitee", func(t *testing.T) {
		var err error
		sari, err = env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-3810-0001", Name: "Mbak Sari", Roles: grants(authz.RoleKaryawan),
		})
		require.NoError(t, err)
		requireSentAt(t, sari, env.Clock.Now())

		sent := env.Messenger.Sent()
		require.Len(t, sent, 1)
		require.Equal(t, messaging.Message{
			OrganizationID: o.ID,
			To:             "+6281238100001",
			Template:       messaging.TemplateMemberInvite,
			Params: map[string]string{
				"name": "Mbak Sari", "role": "Karyawan", "organization": "Griya Sejahtera", "inviter": o.Admin.Name,
			},
			Buttons: []messaging.Button{{Title: "Masuk", Payload: messaging.TemplateMemberInvite, URL: testenv.LoginURL}},
		}, sent[0])
		_, err = messaging.Render(sent[0])
		require.NoError(t, err, "the message renders")

		listed := memberByID(t, env, admin, sari.ID)
		requireSentAt(t, listed, env.Clock.Now())
		require.Nil(t, listed.FirstSignInAt, "not signed in yet")
	})

	t.Run("roles limited to a property name the property", func(t *testing.T) {
		melati, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
		require.NoError(t, err)
		mawar, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Mawar", Kind: property.KindKos})
		require.NoError(t, err)

		_, err = env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-3810-0002", Name: "Pak Joko",
			Roles: []member.RoleGrant{
				{Role: authz.RoleAdmin, PropertyID: melati.ID},
				{Role: authz.RoleAdmin, PropertyID: mawar.ID},
				{Role: authz.RoleKaryawan},
			},
		})
		require.NoError(t, err)
		sent := env.Messenger.Sent()
		role := sent[len(sent)-1].Params["role"]
		require.Contains(t, []string{"Admin (Kos Melati, Kos Mawar) dan Karyawan", "Admin (Kos Mawar, Kos Melati) dan Karyawan"}, role)
	})

	t.Run("re-inviting a member sends nothing", func(t *testing.T) {
		before := len(env.Messenger.Sent())
		_, err := env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-3810-0001", Name: "Mbak Sari", Roles: grants(authz.RoleKaryawan),
		})
		require.ErrorIs(t, err, member.ErrAlreadyMember)
		require.Len(t, env.Messenger.Sent(), before)
	})

	t.Run("a denied invite sends nothing", func(t *testing.T) {
		before := len(env.Messenger.Sent())
		_, err := env.Members.Invite(ctx, karyawan, member.Invite{
			Phone: "0812-3810-0003", Name: "Mas Eko", Roles: grants(authz.RoleKaryawan),
		})
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Len(t, env.Messenger.Sent(), before)
	})
}

func TestInviteWhenWhatsAppFails(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "the WhatsApp session is logged out", err: messaging.ErrNotConnected},
		{name: "the number cannot receive WhatsApp", err: messaging.ErrUndeliverable},
		{name: "WhatsApp is throttling the number", err: messaging.ErrRateLimited},
		{name: "the provider fails unexpectedly", err: fmt.Errorf("gowa: calling server: %w", context.DeadlineExceeded)},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			env := testenv.New(t)
			o := env.CreateOrg(t, "Griya Sejahtera", "0812382")
			admin := env.Principal(t, o.ID, o.Admin.UserID)
			phone := fmt.Sprintf("0812-3820-%04d", i+1)

			env.Messenger.FailWith(tc.err)
			m, err := env.Members.Invite(ctx, admin, member.Invite{Phone: phone, Name: "Mbak Sari", Roles: grants(authz.RoleKaryawan)})
			require.NoError(t, err, "a failed send never undoes the invite")
			requireNotSent(t, m, nil)
			requireNotSent(t, memberByID(t, env, admin, m.ID), nil)
			require.Equal(t, authz.RoleKaryawan, env.Principal(t, o.ID, m.UserID).Roles[0].Key, "the member exists with the role")

			env.Messenger.FailWith(nil)
			m, err = env.Members.ResendInvitation(ctx, admin, m.ID)
			require.NoError(t, err)
			requireSentAt(t, m, env.Clock.Now())
			firstSent := *m.Invitation.LastSentAt

			env.Clock.Advance(time.Minute)
			env.Messenger.FailWith(tc.err)
			m, err = env.Members.ResendInvitation(ctx, admin, m.ID)
			require.ErrorIs(t, err, member.ErrSendFailed)
			require.ErrorIs(t, err, tc.err)
			requireNotSent(t, m, &firstSent)
			requireNotSent(t, memberByID(t, env, admin, m.ID), &firstSent)
		})
	}
}

func TestResendInvitation(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812383")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)

	sari, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-3830-0001", Name: "Mbak Sari", Roles: grants(authz.RoleKaryawan)})
	require.NoError(t, err)

	t.Run("a resend sends again and moves lastSentAt", func(t *testing.T) {
		env.Clock.Advance(10 * time.Minute)
		before := len(env.Messenger.Sent())
		m, err := env.Members.ResendInvitation(ctx, admin, sari.ID)
		require.NoError(t, err)
		requireSentAt(t, m, env.Clock.Now())

		sent := env.Messenger.Sent()
		require.Len(t, sent, before+1)
		require.Equal(t, "+6281238300001", sent[len(sent)-1].To)
		require.Equal(t, messaging.TemplateMemberInvite, sent[len(sent)-1].Template)

		audit := env.AuditLog(t, o.ID)
		require.Equal(t, "allow", audit[0].Decision)
		require.Equal(t, "member:invite", audit[0].Action)
		require.Equal(t, "member/"+sari.ID.String(), audit[0].Resource)
	})

	tests := []struct {
		name    string
		p       authz.Principal
		id      uuid.UUID
		wantErr error
	}{
		{name: "a founder was never invited", p: admin, id: karyawan.MembershipID, wantErr: member.ErrNotInvited},
		{name: "an unknown member", p: admin, id: uuid.New(), wantErr: member.ErrNotFound},
		{name: "a karyawan cannot resend", p: karyawan, id: sari.ID, wantErr: authz.ErrForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := len(env.Messenger.Sent())
			_, err := env.Members.ResendInvitation(ctx, tc.p, tc.id)
			require.ErrorIs(t, err, tc.wantErr)
			require.Len(t, env.Messenger.Sent(), before, "nothing is sent")
		})
	}

	t.Run("the denied resend is audited", func(t *testing.T) {
		audit := env.AuditLog(t, o.ID)
		require.Equal(t, "deny", audit[0].Decision)
		require.Equal(t, "member:invite", audit[0].Action)
		require.Equal(t, o.Karyawan.UserID, audit[0].ActorStaffUserID.UUID)
	})

	t.Run("a disabled member gets no invitation", func(t *testing.T) {
		_, err := env.Members.SetStatus(ctx, admin, sari.ID, member.StatusDisabled)
		require.NoError(t, err)
		before := len(env.Messenger.Sent())
		_, err = env.Members.ResendInvitation(ctx, admin, sari.ID)
		require.ErrorIs(t, err, member.ErrMemberDisabled)
		require.Len(t, env.Messenger.Sent(), before)
	})
}

func TestInvitationLimitPerPhone(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812384")
	other := env.CreateOrg(t, "Kontrakan Lain", "0812385")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	otherAdmin := env.Principal(t, other.ID, other.Admin.UserID)
	invite := member.Invite{Phone: "0812-3840-0001", Name: "Mbak Sari", Roles: grants(authz.RoleKaryawan)}

	sari, err := env.Members.Invite(ctx, admin, invite)
	require.NoError(t, err)
	for range member.InvitesPerPhone - 1 {
		_, err := env.Members.ResendInvitation(ctx, admin, sari.ID)
		require.NoError(t, err)
	}
	require.Len(t, env.Messenger.Sent(), member.InvitesPerPhone)
	lastSent := env.Clock.Now()

	t.Run("one more resend is refused and changes nothing", func(t *testing.T) {
		env.Clock.Advance(time.Minute)
		_, err := env.Members.ResendInvitation(ctx, admin, sari.ID)
		require.ErrorIs(t, err, member.ErrInviteRateLimited)
		require.Len(t, env.Messenger.Sent(), member.InvitesPerPhone)
		requireSentAt(t, memberByID(t, env, admin, sari.ID), lastSent)
	})

	var elsewhere member.Member
	t.Run("another organization's invite still succeeds, not sent", func(t *testing.T) {
		var err error
		elsewhere, err = env.Members.Invite(ctx, otherAdmin, invite)
		require.NoError(t, err)
		requireNotSent(t, elsewhere, nil)
		require.Len(t, env.Messenger.Sent(), member.InvitesPerPhone)
	})

	t.Run("the limit lifts after a day", func(t *testing.T) {
		env.Clock.Advance(member.InvitePhoneSpan)
		m, err := env.Members.ResendInvitation(ctx, otherAdmin, elsewhere.ID)
		require.NoError(t, err)
		requireSentAt(t, m, env.Clock.Now())
	})
}

func TestInvitationLimitPerOrganization(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812386")
	admin := env.Principal(t, o.ID, o.Admin.UserID)

	var first member.Member
	for i := range member.InvitesPerOrganization {
		m, err := env.Members.Invite(ctx, admin, member.Invite{
			Phone: fmt.Sprintf("0812-3860-%04d", i+1), Name: fmt.Sprintf("Karyawan %d", i+1), Roles: grants(authz.RoleKaryawan),
		})
		require.NoError(t, err)
		requireSentAt(t, m, env.Clock.Now())
		if i == 0 {
			first = m
		}
	}
	require.Len(t, env.Messenger.Sent(), member.InvitesPerOrganization)

	var late member.Member
	t.Run("the next invite is created but not sent", func(t *testing.T) {
		var err error
		late, err = env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-3869-0001", Name: "Mas Telat", Roles: grants(authz.RoleKaryawan),
		})
		require.NoError(t, err)
		requireNotSent(t, late, nil)
		require.Len(t, env.Messenger.Sent(), member.InvitesPerOrganization)
	})

	t.Run("a resend is refused", func(t *testing.T) {
		_, err := env.Members.ResendInvitation(ctx, admin, first.ID)
		require.ErrorIs(t, err, member.ErrInviteRateLimited)
		require.Len(t, env.Messenger.Sent(), member.InvitesPerOrganization)
	})

	t.Run("the limit lifts after an hour", func(t *testing.T) {
		env.Clock.Advance(member.InviteOrganizationSpan)
		m, err := env.Members.ResendInvitation(ctx, admin, late.ID)
		require.NoError(t, err)
		requireSentAt(t, m, env.Clock.Now())
	})
}

func TestFirstSignInAt(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812387")
	other := env.CreateOrg(t, "Kontrakan Lain", "0812388")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	otherAdmin := env.Principal(t, other.ID, other.Admin.UserID)

	sari, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-3870-0001", Name: "Mbak Sari", Roles: grants(authz.RoleKaryawan)})
	require.NoError(t, err)
	sariElsewhere, err := env.Members.Invite(ctx, otherAdmin, member.Invite{Phone: "0812-3870-0001", Name: "Mbak Sari", Roles: grants(authz.RoleAdmin)})
	require.NoError(t, err)
	require.Nil(t, sari.FirstSignInAt)

	signIn := func(t *testing.T) {
		t.Helper()
		require.NoError(t, env.Auth.RequestCode(ctx, "0812-3870-0001"))
		_, _, err := env.Auth.VerifyCode(ctx, "0812-3870-0001", env.LastCode(t))
		require.NoError(t, err)
	}

	t.Run("a wrong code does not count", func(t *testing.T) {
		env.Clock.Advance(time.Minute)
		require.NoError(t, env.Auth.RequestCode(ctx, "0812-3870-0001"))
		wrong := "000000"
		if env.LastCode(t) == wrong {
			wrong = "111111"
		}
		_, _, err := env.Auth.VerifyCode(ctx, "0812-3870-0001", wrong)
		require.Error(t, err)
		require.Nil(t, memberByID(t, env, admin, sari.ID).FirstSignInAt)
	})

	firstAt := env.Clock.Now().Add(time.Hour)
	t.Run("the first sign-in is recorded for every organization", func(t *testing.T) {
		env.Clock.Advance(time.Hour)
		signIn(t)
		got := memberByID(t, env, admin, sari.ID).FirstSignInAt
		require.NotNil(t, got)
		require.True(t, firstAt.Equal(*got))
		got = memberByID(t, env, otherAdmin, sariElsewhere.ID).FirstSignInAt
		require.NotNil(t, got, "per person, not per organization")
		require.True(t, firstAt.Equal(*got))
	})

	t.Run("later sign-ins keep the first", func(t *testing.T) {
		env.Clock.Advance(48 * time.Hour)
		signIn(t)
		got := memberByID(t, env, admin, sari.ID).FirstSignInAt
		require.NotNil(t, got)
		require.True(t, firstAt.Equal(*got))
	})
}
