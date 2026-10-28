//go:build integration

package member_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// memberManager is a custom role that may change any member, including the last Admin, which
// no built-in role can reach: an Admin never changes their own status.
const memberManager = `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["member:manage","role:assign"]}]}`

func allowedBy(rows []dbgen.AuditLog, actor uuid.UUID) []string {
	var out []string
	for _, a := range rows {
		if a.ActorStaffUserID.UUID == actor && a.Decision == "allow" {
			out = append(out, a.Action+" "+a.Resource)
		}
	}
	return out
}

func requireDenied(t *testing.T, env *testenv.Env, org, actor uuid.UUID, action, resource, reason string) {
	t.Helper()
	newest := env.AuditLog(t, org)[0]
	require.Equal(t, "deny", newest.Decision, "the denial is the newest row")
	require.Equal(t, actor, newest.ActorStaffUserID.UUID)
	require.Equal(t, action, newest.Action)
	require.Equal(t, resource, newest.Resource)
	require.Equal(t, reason, newest.Reason)
}

func statusOf(t *testing.T, env *testenv.Env, p authz.Principal, membershipID uuid.UUID) string {
	t.Helper()
	members, err := env.Members.List(context.Background(), p)
	require.NoError(t, err)
	for _, m := range members {
		if m.ID == membershipID {
			return m.Status
		}
	}
	t.Fatalf("member %s not listed", membershipID)
	return ""
}

func TestSetStatus(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812381")
	other := env.CreateOrg(t, "Kos Lain", "0812382")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	supervisor := env.Principal(t, o.ID, o.Supervisor.UserID)
	karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)
	otherAdmin := env.Principal(t, other.ID, other.Admin.UserID)

	secondAdmin, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-3810-0001", Name: "Bu Rina", Roles: grants(authz.RoleAdmin)})
	require.NoError(t, err)
	// The karyawan of o is also an Admin of another organization.
	_, err = env.Members.Invite(ctx, otherAdmin, member.Invite{Phone: o.Karyawan.Phone, Name: o.Karyawan.Name, Roles: grants(authz.RoleAdmin)})
	require.NoError(t, err)

	t.Run("bad status names the field", func(t *testing.T) {
		for _, status := range []string{"", "removed", "Disabled"} {
			_, err := env.Members.SetStatus(ctx, admin, karyawan.MembershipID, status)
			var inputErr *member.InputError
			require.ErrorAs(t, err, &inputErr, status)
			require.Equal(t, "status", inputErr.Field)
		}
	})

	t.Run("a karyawan cannot disable anyone, and the denial is audited", func(t *testing.T) {
		_, err := env.Members.SetStatus(ctx, karyawan, secondAdmin.ID, member.StatusDisabled)
		require.ErrorIs(t, err, authz.ErrForbidden)
		requireDenied(t, env, o.ID, o.Karyawan.UserID, "member:manage", "member/"+secondAdmin.ID.String(), "implicit_deny")
		require.Equal(t, member.StatusActive, statusOf(t, env, admin, secondAdmin.ID))
	})

	t.Run("nobody disables themselves, and the denial is audited", func(t *testing.T) {
		_, err := env.Members.SetStatus(ctx, admin, admin.MembershipID, member.StatusDisabled)
		require.ErrorIs(t, err, authz.ErrForbidden)
		requireDenied(t, env, o.ID, o.Admin.UserID, "member:manage", "member/"+admin.MembershipID.String(), "explicit_deny")
		require.Equal(t, member.StatusActive, statusOf(t, env, admin, admin.MembershipID))
	})

	t.Run("an admin can neither disable nor re-enable a supervisor", func(t *testing.T) {
		for _, status := range []string{member.StatusDisabled, member.StatusActive} {
			_, err := env.Members.SetStatus(ctx, admin, supervisor.MembershipID, status)
			require.ErrorIs(t, err, authz.ErrForbidden, status)
			requireDenied(t, env, o.ID, o.Admin.UserID, "role:assign", "role/supervisor", "implicit_deny")
		}
		require.Equal(t, member.StatusActive, statusOf(t, env, admin, supervisor.MembershipID))
	})

	t.Run("a disabled member keeps their roles but loses this organization only", func(t *testing.T) {
		m, err := env.Members.SetStatus(ctx, admin, karyawan.MembershipID, member.StatusDisabled)
		require.NoError(t, err)
		require.Equal(t, member.StatusDisabled, m.Status)
		require.Equal(t, []string{authz.RoleKaryawan}, roleKeys(m), "roles are kept")

		allowed := allowedBy(env.AuditLog(t, o.ID), o.Admin.UserID)
		require.Contains(t, allowed, "member:manage member/"+karyawan.MembershipID.String())
		require.Contains(t, allowed, "role:assign role/karyawan")

		_, err = env.Authz.LoadPrincipal(ctx, o.ID, o.Karyawan.UserID)
		require.ErrorIs(t, err, authz.ErrNotMember, "every session of theirs is refused in this organization")
		inOther := env.Principal(t, other.ID, o.Karyawan.UserID)
		require.Equal(t, []string{authz.RoleAdmin}, inOther.RoleKeys(), "their other organization is untouched")

		orgs, err := env.Auth.Organizations(ctx, o.Karyawan.UserID)
		require.NoError(t, err)
		require.Len(t, orgs, 1)
		require.Equal(t, other.ID, orgs[0].ID)
	})

	t.Run("setting the same status again changes nothing", func(t *testing.T) {
		m, err := env.Members.SetStatus(ctx, admin, karyawan.MembershipID, member.StatusDisabled)
		require.NoError(t, err)
		require.Equal(t, member.StatusDisabled, m.Status)
		require.Equal(t, []string{authz.RoleKaryawan}, roleKeys(m))
	})

	t.Run("re-enabling restores the same roles and access", func(t *testing.T) {
		m, err := env.Members.SetStatus(ctx, admin, karyawan.MembershipID, member.StatusActive)
		require.NoError(t, err)
		require.Equal(t, member.StatusActive, m.Status)
		require.Equal(t, []string{authz.RoleKaryawan}, roleKeys(m))

		back := env.Principal(t, o.ID, o.Karyawan.UserID)
		require.Equal(t, karyawan.MembershipID, back.MembershipID)
		require.Equal(t, karyawan.RoleKeys(), back.RoleKeys())
		orgs, err := env.Auth.Organizations(ctx, o.Karyawan.UserID)
		require.NoError(t, err)
		require.Len(t, orgs, 2)
	})

	t.Run("an unknown member or one of another organization is not found", func(t *testing.T) {
		_, err := env.Members.SetStatus(ctx, admin, uuid.New(), member.StatusDisabled)
		require.ErrorIs(t, err, member.ErrNotFound)
		_, err = env.Members.SetStatus(ctx, admin, otherAdmin.MembershipID, member.StatusDisabled)
		require.ErrorIs(t, err, member.ErrNotFound)
	})

	t.Run("the last active admin for the whole organization cannot be disabled", func(t *testing.T) {
		melati, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
		require.NoError(t, err)
		// An Admin limited to one property does not count as an Admin of the organization.
		_, err = env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-3810-0002", Name: "Mbak Sari", Roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati.ID}},
		})
		require.NoError(t, err)
		hendra, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-3810-0003", Name: "Pak Hendra", Roles: grants(authz.RoleKaryawan)})
		require.NoError(t, err)
		env.GrantCustomRole(t, o.ID, hendra.ID, "Pengelola Anggota", memberManager)
		manager := env.Principal(t, o.ID, hendra.UserID)

		m, err := env.Members.SetStatus(ctx, admin, secondAdmin.ID, member.StatusDisabled)
		require.NoError(t, err, "disabling one of two admins works")
		require.Equal(t, member.StatusDisabled, m.Status)

		allowsBefore := len(allowedBy(env.AuditLog(t, o.ID), hendra.UserID))
		_, err = env.Members.SetStatus(ctx, manager, admin.MembershipID, member.StatusDisabled)
		require.ErrorIs(t, err, member.ErrLastAdmin)
		require.Equal(t, member.StatusActive, statusOf(t, env, admin, admin.MembershipID))
		require.Len(t, allowedBy(env.AuditLog(t, o.ID), hendra.UserID), allowsBefore,
			"no allowed change is audited for a change that did not happen")

		_, err = env.Members.SetStatus(ctx, manager, secondAdmin.ID, member.StatusActive)
		require.NoError(t, err)
		_, err = env.Members.SetStatus(ctx, manager, admin.MembershipID, member.StatusDisabled)
		require.NoError(t, err, "with a second admin back, the first can be disabled")
	})
}

// TestTwoAdminsDisablingEachOther makes the race deterministic: the test disables one admin
// in a transaction it keeps open, as the first of two concurrent calls would, starts the
// other admin's call to disable the first, waits until Postgres reports that call blocked on
// the organization lock, and only then commits. The waiting call must then see that it would
// remove the last admin.
func TestTwoAdminsDisablingEachOther(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812383")
	first := env.Principal(t, o.ID, o.Admin.UserID)
	invited, err := env.Members.Invite(ctx, first, member.Invite{Phone: "0812-3830-0001", Name: "Bu Rina", Roles: grants(authz.RoleAdmin)})
	require.NoError(t, err)
	second := env.Principal(t, o.ID, invited.UserID)

	done := make(chan error, 1)
	err = env.Store.WithOrgTx(ctx, o.ID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := q.LockOrganizationMembers(ctx, o.ID); err != nil {
			return err
		}
		n, err := q.SetMembershipStatus(ctx, dbgen.SetMembershipStatusParams{
			OrganizationID: o.ID, ID: second.MembershipID, Status: member.StatusDisabled,
		})
		if err != nil {
			return err
		}
		require.Equal(t, int64(1), n)

		go func() {
			_, err := env.Members.SetStatus(ctx, second, first.MembershipID, member.StatusDisabled)
			done <- err
		}()
		testenv.WaitUntilBlockedOn(ctx, t, tx)
		return nil // commit: the waiting call now sees the second admin disabled
	})
	require.NoError(t, err)

	select {
	case err = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the waiting call never returned")
	}
	require.ErrorIs(t, err, member.ErrLastAdmin)
	require.Equal(t, member.StatusActive, statusOf(t, env, first, first.MembershipID))
	require.Equal(t, member.StatusDisabled, statusOf(t, env, first, second.MembershipID))
}

func TestSetRolesKeepsAnAdmin(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812384")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	melati, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	hendra, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-3840-0001", Name: "Pak Hendra", Roles: grants(authz.RoleKaryawan)})
	require.NoError(t, err)
	env.GrantCustomRole(t, o.ID, hendra.ID, "Pengelola Anggota", memberManager)
	manager := env.Principal(t, o.ID, hendra.UserID)

	refused := []struct {
		name  string
		roles []member.RoleGrant
	}{
		{name: "to karyawan", roles: grants(authz.RoleKaryawan)},
		{name: "to admin limited to one property", roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati.ID}}},
		{
			name:  "to admin limited to one property and karyawan",
			roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati.ID}, {Role: authz.RoleKaryawan}},
		},
	}
	for _, tc := range refused {
		t.Run("the last admin cannot be demoted "+tc.name, func(t *testing.T) {
			allowsBefore := len(allowedBy(env.AuditLog(t, o.ID), hendra.UserID))
			_, err := env.Members.SetRoles(ctx, manager, admin.MembershipID, tc.roles)
			require.ErrorIs(t, err, member.ErrLastAdmin)
			require.Equal(t, []string{authz.RoleAdmin}, env.Principal(t, o.ID, o.Admin.UserID).RoleKeys(), "nothing changes")
			require.Len(t, allowedBy(env.AuditLog(t, o.ID), hendra.UserID), allowsBefore,
				"no allowed change is audited for a change that did not happen")
		})
	}

	t.Run("the last admin can gain roles while keeping admin", func(t *testing.T) {
		m, err := env.Members.SetRoles(ctx, manager, admin.MembershipID, grants(authz.RoleAdmin, authz.RoleKaryawan))
		require.NoError(t, err)
		require.Equal(t, []string{authz.RoleAdmin, authz.RoleKaryawan}, roleKeys(m))
	})

	t.Run("an unauthorized caller is refused before the rule", func(t *testing.T) {
		karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)
		_, err := env.Members.SetRoles(ctx, karyawan, admin.MembershipID, grants(authz.RoleKaryawan))
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("demoting one of two admins works", func(t *testing.T) {
		rina, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-3840-0002", Name: "Bu Rina", Roles: grants(authz.RoleAdmin)})
		require.NoError(t, err)
		m, err := env.Members.SetRoles(ctx, admin, rina.ID, []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati.ID}})
		require.NoError(t, err)
		require.Equal(t, []member.RoleRef{{Key: authz.RoleAdmin, Name: "Admin", Builtin: true, PropertyID: melati.ID, PropertyName: melati.Name}}, m.Roles)
	})
}

// TestAdminRemovedTwiceAtOnce makes the race between a disable and a demotion deterministic,
// in both orders: the test applies the first change in a transaction it keeps open (holding
// the organization lock, as the service would), starts the second admin's change against the
// first admin, waits until Postgres reports it blocked, and only then commits.
func TestAdminRemovedTwiceAtOnce(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)

	cases := []struct {
		name   string
		prefix string
		// first removes the second admin inside the held transaction.
		first func(q *dbgen.Queries, org, membershipID uuid.UUID) error
		// then is the second admin's call against the first admin.
		then func(by authz.Principal, membershipID uuid.UUID) error
	}{
		{
			name: "a disable holds the lock, a demotion waits", prefix: "0812385",
			first: func(q *dbgen.Queries, org, membershipID uuid.UUID) error {
				_, err := q.SetMembershipStatus(ctx, dbgen.SetMembershipStatusParams{
					OrganizationID: org, ID: membershipID, Status: member.StatusDisabled,
				})
				return err
			},
			then: func(by authz.Principal, membershipID uuid.UUID) error {
				_, err := env.Members.SetRoles(ctx, by, membershipID, grants(authz.RoleKaryawan))
				return err
			},
		},
		{
			name: "a demotion holds the lock, a disable waits", prefix: "0812386",
			first: func(q *dbgen.Queries, org, membershipID uuid.UUID) error {
				err := q.DeleteBuiltinRoleAssignments(ctx, dbgen.DeleteBuiltinRoleAssignmentsParams{
					OrganizationID: org, MembershipID: membershipID,
				})
				if err != nil {
					return err
				}
				role := authz.RoleKaryawan
				return q.CreateBuiltinRoleAssignment(ctx, dbgen.CreateBuiltinRoleAssignmentParams{
					ID: uuid.New(), OrganizationID: org, MembershipID: membershipID, BuiltinRole: &role,
				})
			},
			then: func(by authz.Principal, membershipID uuid.UUID) error {
				_, err := env.Members.SetStatus(ctx, by, membershipID, member.StatusDisabled)
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := env.CreateOrg(t, "Griya "+tc.prefix, tc.prefix)
			first := env.Principal(t, o.ID, o.Admin.UserID)
			invited, err := env.Members.Invite(ctx, first, member.Invite{Phone: tc.prefix + "901", Name: "Bu Rina", Roles: grants(authz.RoleAdmin)})
			require.NoError(t, err)
			second := env.Principal(t, o.ID, invited.UserID)

			done := make(chan error, 1)
			err = env.Store.WithOrgTx(ctx, o.ID, func(tx pgx.Tx) error {
				q := dbgen.New(tx)
				if err := q.LockOrganizationMembers(ctx, o.ID); err != nil {
					return err
				}
				if err := tc.first(q, o.ID, second.MembershipID); err != nil {
					return err
				}
				go func() { done <- tc.then(second, first.MembershipID) }()
				testenv.WaitUntilBlockedOn(ctx, t, tx)
				return nil // commit: the waiting call now sees the second admin removed
			})
			require.NoError(t, err)

			select {
			case err = <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("the waiting call never returned")
			}
			require.ErrorIs(t, err, member.ErrLastAdmin)
			require.Equal(t, member.StatusActive, statusOf(t, env, first, first.MembershipID))
			require.Equal(t, []string{authz.RoleAdmin}, env.Principal(t, o.ID, o.Admin.UserID).RoleKeys())
		})
	}
}
