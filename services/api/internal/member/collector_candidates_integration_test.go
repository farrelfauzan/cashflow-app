//go:build integration

package member_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func candidateNames(cs []member.CollectorCandidate) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func TestCollectorCandidates(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812391")
	other := env.CreateOrg(t, "Kontrakan Lain", "0812392")
	admin := env.Principal(t, o.ID, o.Admin.UserID)

	melati, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	sari, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-3910-0001", Name: "Mbak Sari",
		Roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati.ID}}})
	require.NoError(t, err)
	limited := env.Principal(t, o.ID, sari.UserID)
	_, err = env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-3910-0002", Name: "Andi Pratama", Roles: grants(authz.RoleKaryawan)})
	require.NoError(t, err)
	rudi, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-3910-0003", Name: "Rudi Hartono", Roles: grants(authz.RoleKaryawan)})
	require.NoError(t, err)
	_, err = env.Members.SetStatus(ctx, admin, rudi.ID, member.StatusDisabled)
	require.NoError(t, err)

	want := []string{
		"Andi Pratama", o.Karyawan.Name, // karyawan first, by name
		o.Supervisor.Name, "Mbak Sari", o.Admin.Name, // then everyone else, by name
	}

	t.Run("an admin lists the active members, karyawan first", func(t *testing.T) {
		got, err := env.Members.CollectorCandidates(ctx, admin)
		require.NoError(t, err)
		require.Equal(t, want, candidateNames(got), "the disabled karyawan is not offered")
		require.True(t, got[0].Karyawan)
		require.True(t, got[1].Karyawan)
		require.False(t, got[2].Karyawan)
	})

	t.Run("an admin limited to one property lists them too, unaudited", func(t *testing.T) {
		before := len(env.AuditLog(t, o.ID))
		got, err := env.Members.CollectorCandidates(ctx, limited)
		require.NoError(t, err)
		require.Equal(t, want, candidateNames(got))
		require.Len(t, env.AuditLog(t, o.ID), before, "listing writes no allowed audit row")

		_, err = env.Members.List(ctx, limited)
		require.ErrorIs(t, err, authz.ErrForbidden, "the full member list stays organization-wide")
	})

	t.Run("a karyawan and the supervisor are refused, and each refusal is audited", func(t *testing.T) {
		for name, person := range map[string]testenv.Person{"karyawan": o.Karyawan, "supervisor": o.Supervisor} {
			p := env.Principal(t, o.ID, person.UserID)
			_, err := env.Members.CollectorCandidates(ctx, p)
			require.ErrorIs(t, err, authz.ErrForbidden, name)
			denied := false
			for _, a := range env.AuditLog(t, o.ID) {
				if a.ActorStaffUserID.UUID == p.UserID && a.Decision == "deny" && a.Action == authz.ActionLeaseManage {
					denied = true
				}
			}
			require.True(t, denied, "%s: the refusal is audited", name)
		}
	})

	t.Run("another organization's members are never offered", func(t *testing.T) {
		got, err := env.Members.CollectorCandidates(ctx, env.Principal(t, other.ID, other.Admin.UserID))
		require.NoError(t, err)
		require.ElementsMatch(t, []string{other.Supervisor.Name, other.Admin.Name, other.Karyawan.Name}, candidateNames(got))
	})
}
