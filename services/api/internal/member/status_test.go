package member

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
)

func TestStatusRequestsAndOrgAdmin(t *testing.T) {
	t.Parallel()
	melati, mawar := uuid.New(), uuid.New()
	admin := RoleRef{Key: authz.RoleAdmin, Builtin: true}
	karyawan := RoleRef{Key: authz.RoleKaryawan, Builtin: true}
	limited := func(r RoleRef, property uuid.UUID) RoleRef { r.PropertyID = property; return r }
	custom := RoleRef{Key: "custom:" + uuid.NewString(), Name: "Pengelola Anggota"}

	tests := []struct {
		name      string
		roles     []RoleRef
		wantRoles []string // role:assign requests after member:manage
		wantAdmin bool
	}{
		{name: "no roles", wantRoles: nil},
		{name: "admin for the whole organization", roles: []RoleRef{admin}, wantRoles: []string{"role/admin"}, wantAdmin: true},
		{
			name:      "one role limited to two properties is checked once",
			roles:     []RoleRef{limited(karyawan, melati), limited(karyawan, mawar)},
			wantRoles: []string{"role/karyawan"},
		},
		{
			name:      "an admin limited to a property is not an organization admin",
			roles:     []RoleRef{limited(admin, melati), karyawan},
			wantRoles: []string{"role/admin", "role/karyawan"},
		},
		{name: "custom roles need no role:assign here", roles: []RoleRef{custom}, wantRoles: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := Member{ID: uuid.New(), UserID: uuid.New(), Roles: tc.roles}
			reqs := statusRequests(m)
			require.Equal(t, authz.ActionMemberManage, reqs[0].Action)
			require.Equal(t, m.UserID, reqs[0].Resource.OwnerID, "the self guardrail needs the owner")
			require.Equal(t, "member/"+m.ID.String(), reqs[0].Resource.Path())
			var got []string
			for _, r := range reqs[1:] {
				require.Equal(t, authz.ActionRoleAssign, r.Action)
				got = append(got, r.Resource.Path())
			}
			require.Equal(t, tc.wantRoles, got)
			require.Equal(t, tc.wantAdmin, holdsOrgAdmin(m))
		})
	}
}
