//go:build integration

package member_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
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
			Phone: "0812-3334-4455", Name: "  Mbak Sari  ", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}},
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
			Phone: "+6281233344455", Name: "Sari", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}},
		})
		require.ErrorIs(t, err, member.ErrAlreadyMember)
	})

	t.Run("an admin cannot hand out the supervisor role, and the denial is audited", func(t *testing.T) {
		_, err := env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-9999-0001", Name: "Calon Supervisor", Roles: []member.RoleGrant{{Role: authz.RoleSupervisor}},
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
			"phone": {Phone: "12", Name: "X", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}},
			"name":  {Phone: "081234500001", Name: "   ", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}},
			"roles": {Phone: "081234500001", Name: "X", Roles: []member.RoleGrant{{Role: "owner"}}},
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
			Phone: a.Karyawan.Phone, Name: "dipakai nama lama", Roles: []member.RoleGrant{{Role: authz.RoleAdmin}},
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

// footprint is what an invite can write: the staff user (found by phone, in any organization),
// and the organization's memberships and role assignments.
type footprint struct {
	staffUser       bool
	memberships     int
	roleAssignments int
}

func footprintOf(t *testing.T, env *testenv.Env, org uuid.UUID, e164 string) footprint {
	t.Helper()
	ctx := context.Background()
	var f footprint
	err := env.Store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		_, err := dbgen.New(tx).GetStaffUserByPhone(ctx, e164)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		f.staffUser = err == nil
		return err
	})
	require.NoError(t, err)
	err = env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		members, err := q.ListMembers(ctx, org)
		if err != nil {
			return err
		}
		roles, err := q.ListOrgRoleAssignments(ctx, org)
		f.memberships, f.roleAssignments = len(members), len(roles)
		return err
	})
	require.NoError(t, err)
	return f
}

func TestInviteWithRoleGrants(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812371")
	other := env.CreateOrg(t, "Kontrakan Lain", "0812372")
	admin := env.Principal(t, o.ID, o.Admin.UserID)

	newProperty := func(p authz.Principal, name string) property.Property {
		t.Helper()
		prop, err := env.Properties.Create(ctx, p, property.PropertyInput{Name: name, Kind: property.KindKos})
		require.NoError(t, err)
		return prop
	}
	melati := newProperty(admin, "Kos Melati")
	mawar := newProperty(admin, "Kos Mawar")
	elsewhere := newProperty(env.Principal(t, other.ID, other.Admin.UserID), "Kos Seberang")

	var limited authz.Principal
	t.Run("an admin invites an admin limited to one property in one step", func(t *testing.T) {
		m, err := env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-3710-0001", Name: "Mbak Sari",
			Roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati.ID}},
		})
		require.NoError(t, err)
		require.Equal(t, []member.RoleRef{{Key: authz.RoleAdmin, Name: "Admin", Builtin: true, PropertyID: melati.ID, PropertyName: melati.Name}}, m.Roles,
			"exactly the limited grant, never the role for the whole organization")

		limited = env.Principal(t, o.ID, m.UserID)
		require.Len(t, limited.Roles, 1)
		require.Equal(t, melati.ID, limited.Roles[0].Property)

		var actions []string
		for _, a := range env.AuditLog(t, o.ID) {
			if a.ActorStaffUserID.UUID == o.Admin.UserID && a.Decision == "allow" {
				actions = append(actions, a.Action+" "+a.Resource)
			}
		}
		require.Contains(t, actions, "member:invite member")
		require.Contains(t, actions, "role:assign role/admin")
	})

	t.Run("one role in two properties and a second role arrive together", func(t *testing.T) {
		m, err := env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-3710-0002", Name: "Mas Budi",
			Roles: []member.RoleGrant{
				{Role: authz.RoleKaryawan, PropertyID: melati.ID},
				{Role: authz.RoleKaryawan, PropertyID: mawar.ID},
				{Role: authz.RoleKaryawan, PropertyID: mawar.ID}, // repeated grants count once
				{Role: authz.RoleAdmin},
			},
		})
		require.NoError(t, err)
		require.Len(t, m.Roles, 3)
	})

	refused := []struct {
		name    string
		by      func() authz.Principal
		roles   []member.RoleGrant
		wantErr error // nil means an input error on the roles field
	}{
		{
			name:    "an admin cannot invite a supervisor",
			by:      func() authz.Principal { return admin },
			roles:   []member.RoleGrant{{Role: authz.RoleSupervisor}},
			wantErr: authz.ErrForbidden,
		},
		{
			name:    "an admin cannot invite a supervisor limited to a property",
			by:      func() authz.Principal { return admin },
			roles:   []member.RoleGrant{{Role: authz.RoleKaryawan}, {Role: authz.RoleSupervisor, PropertyID: melati.ID}},
			wantErr: authz.ErrForbidden,
		},
		{
			name:    "an admin limited to a property cannot invite into that property",
			by:      func() authz.Principal { return limited },
			roles:   []member.RoleGrant{{Role: authz.RoleKaryawan, PropertyID: melati.ID}},
			wantErr: authz.ErrForbidden,
		},
		{
			name:    "an admin limited to a property cannot invite into another property",
			by:      func() authz.Principal { return limited },
			roles:   []member.RoleGrant{{Role: authz.RoleKaryawan, PropertyID: mawar.ID}},
			wantErr: authz.ErrForbidden,
		},
		{
			name:  "an unknown property",
			by:    func() authz.Principal { return admin },
			roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: uuid.New()}},
		},
		{
			name:  "a property of another organization",
			by:    func() authz.Principal { return admin },
			roles: []member.RoleGrant{{Role: authz.RoleKaryawan}, {Role: authz.RoleAdmin, PropertyID: elsewhere.ID}},
		},
		{
			name:  "the same role for the whole organization and for a property",
			by:    func() authz.Principal { return admin },
			roles: []member.RoleGrant{{Role: authz.RoleAdmin}, {Role: authz.RoleAdmin, PropertyID: melati.ID}},
		},
		{
			name:  "an unknown role",
			by:    func() authz.Principal { return admin },
			roles: []member.RoleGrant{{Role: authz.RoleKaryawan}, {Role: "owner"}},
		},
		{
			name: "no roles",
			by:   func() authz.Principal { return admin },
		},
	}
	const phone = "+6281237109999"
	for _, tc := range refused {
		t.Run(tc.name+" and nothing is left behind", func(t *testing.T) {
			before := footprintOf(t, env, o.ID, phone)
			require.False(t, before.staffUser)

			_, err := env.Members.Invite(ctx, tc.by(), member.Invite{Phone: phone, Name: "Calon Anggota", Roles: tc.roles})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				var inputErr *member.InputError
				require.ErrorAs(t, err, &inputErr)
				require.Equal(t, "roles", inputErr.Field)
			}
			require.Equal(t, before, footprintOf(t, env, o.ID, phone), "no staff user, membership or role assignment")
		})
	}
}
