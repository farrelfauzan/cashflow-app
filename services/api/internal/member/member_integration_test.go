//go:build integration

package member_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func roleKeys(m member.Member) []string {
	var out []string
	for _, r := range m.Roles {
		out = append(out, r.Key)
	}
	return out
}

// grants gives each role for the whole organization.
func grants(roles ...string) []member.RoleGrant {
	out := make([]member.RoleGrant, len(roles))
	for i, r := range roles {
		out[i] = member.RoleGrant{Role: r}
	}
	return out
}

func TestMembers(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812333")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	supervisor := env.Principal(t, o.ID, o.Supervisor.UserID)
	karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)

	t.Run("admins and supervisors can list members, karyawan cannot", func(t *testing.T) {
		for _, p := range []authz.Principal{admin, supervisor} {
			members, err := env.Members.List(ctx, p)
			require.NoError(t, err)
			require.Len(t, members, 3)
		}
		_, err := env.Members.List(ctx, karyawan)
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	var invited member.Member
	t.Run("an admin adds a karyawan by phone", func(t *testing.T) {
		var err error
		invited, err = env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-3334-4455", Name: "  Mbak Sari  ", Roles: []string{authz.RoleKaryawan},
		})
		require.NoError(t, err)
		require.Equal(t, "Mbak Sari", invited.Name)
		require.Equal(t, "+6281233344455", invited.Phone)
		require.Equal(t, []string{authz.RoleKaryawan}, roleKeys(invited))

		audit := env.AuditLog(t, o.ID)
		var actions []string
		for _, a := range audit {
			if a.ActorStaffUserID.UUID == o.Admin.UserID && a.Decision == "allow" {
				actions = append(actions, a.Action+" "+a.Resource)
			}
		}
		require.Contains(t, actions, "member:invite member")
		require.Contains(t, actions, "role:assign role/karyawan")
	})

	t.Run("the same number cannot be added twice", func(t *testing.T) {
		_, err := env.Members.Invite(ctx, admin, member.Invite{
			Phone: "+6281233344455", Name: "Sari", Roles: []string{authz.RoleKaryawan},
		})
		require.ErrorIs(t, err, member.ErrAlreadyMember)
	})

	t.Run("an admin cannot hand out the supervisor role, and the denial is audited", func(t *testing.T) {
		_, err := env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-9999-0001", Name: "Calon Supervisor", Roles: []string{authz.RoleSupervisor},
		})
		require.ErrorIs(t, err, authz.ErrForbidden)

		audit := env.AuditLog(t, o.ID)
		require.Equal(t, "deny", audit[0].Decision, "the denial is the newest row")
		require.Equal(t, "role:assign", audit[0].Action)
		require.Equal(t, "role/supervisor", audit[0].Resource)
		require.Equal(t, "implicit_deny", audit[0].Reason)
		require.Equal(t, o.Admin.UserID, audit[0].ActorStaffUserID.UUID)
	})

	t.Run("bad input names the field", func(t *testing.T) {
		tests := map[string]member.Invite{
			"phone": {Phone: "12", Name: "X", Roles: []string{authz.RoleKaryawan}},
			"name":  {Phone: "081234500001", Name: "   ", Roles: []string{authz.RoleKaryawan}},
			"roles": {Phone: "081234500001", Name: "X", Roles: []string{"owner"}},
		}
		for field, in := range tests {
			_, err := env.Members.Invite(ctx, admin, in)
			var inputErr *member.InputError
			require.ErrorAs(t, err, &inputErr, field)
			require.Equal(t, field, inputErr.Field)
		}
		_, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "081234500001", Name: "X"})
		var inputErr *member.InputError
		require.ErrorAs(t, err, &inputErr)
		require.Equal(t, "roles", inputErr.Field, "at least one role is required")
	})

	t.Run("an admin promotes the karyawan to admin", func(t *testing.T) {
		updated, err := env.Members.SetRoles(ctx, admin, invited.ID, grants(authz.RoleAdmin, authz.RoleKaryawan))
		require.NoError(t, err)
		require.Equal(t, []string{authz.RoleAdmin, authz.RoleKaryawan}, roleKeys(updated))
	})

	t.Run("nobody changes their own roles", func(t *testing.T) {
		_, err := env.Members.SetRoles(ctx, admin, admin.MembershipID, grants(authz.RoleAdmin, authz.RoleKaryawan))
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("an admin cannot demote the supervisor", func(t *testing.T) {
		_, err := env.Members.SetRoles(ctx, admin, supervisor.MembershipID, grants(authz.RoleKaryawan))
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("the supervisor never writes", func(t *testing.T) {
		_, err := env.Members.SetRoles(ctx, supervisor, invited.ID, grants(authz.RoleKaryawan))
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("an unknown member is not found", func(t *testing.T) {
		_, err := env.Members.SetRoles(ctx, admin, uuid.New(), grants(authz.RoleKaryawan))
		require.ErrorIs(t, err, member.ErrNotFound)
	})
}

func TestMembersStayInsideTheirOrganization(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	a := env.CreateOrg(t, "Kos Melati", "0812444")
	b := env.CreateOrg(t, "Kontrakan Mawar", "0812555")
	adminA := env.Principal(t, a.ID, a.Admin.UserID)
	adminB := env.Principal(t, b.ID, b.Admin.UserID)

	t.Run("a member of A is not a member of B", func(t *testing.T) {
		_, err := env.Authz.LoadPrincipal(ctx, b.ID, a.Admin.UserID)
		require.ErrorIs(t, err, authz.ErrNotMember)
	})

	t.Run("A's admin sees only A's members", func(t *testing.T) {
		members, err := env.Members.List(ctx, adminA)
		require.NoError(t, err)
		for _, m := range members {
			require.True(t, slices.Contains([]uuid.UUID{a.Supervisor.UserID, a.Admin.UserID, a.Karyawan.UserID}, m.UserID))
		}
	})

	t.Run("A's admin cannot change a member of B, even with the right id", func(t *testing.T) {
		bKaryawan := env.Principal(t, b.ID, b.Karyawan.UserID)
		_, err := env.Members.SetRoles(ctx, adminA, bKaryawan.MembershipID, grants(authz.RoleAdmin))
		require.ErrorIs(t, err, member.ErrNotFound)
	})

	t.Run("one person can belong to two organizations with different roles", func(t *testing.T) {
		_, err := env.Members.Invite(ctx, adminB, member.Invite{
			Phone: a.Karyawan.Phone, Name: "dipakai nama lama", Roles: []string{authz.RoleAdmin},
		})
		require.NoError(t, err)

		inA := env.Principal(t, a.ID, a.Karyawan.UserID)
		inB := env.Principal(t, b.ID, a.Karyawan.UserID)
		require.Equal(t, []string{authz.RoleKaryawan}, inA.RoleKeys())
		require.Equal(t, []string{authz.RoleAdmin}, inB.RoleKeys())

		orgs, err := env.Auth.Organizations(ctx, a.Karyawan.UserID)
		require.NoError(t, err)
		require.Len(t, orgs, 2)
	})
}
