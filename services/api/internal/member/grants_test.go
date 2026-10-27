package member

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
)

func TestValidGrants(t *testing.T) {
	t.Parallel()
	melati, mawar := uuid.New(), uuid.New()

	t.Run("orders roles and drops repeats", func(t *testing.T) {
		t.Parallel()
		got, err := validGrants([]RoleGrant{
			{Role: authz.RoleKaryawan},
			{Role: authz.RoleAdmin, PropertyID: melati},
			{Role: authz.RoleAdmin, PropertyID: melati},
			{Role: authz.RoleKaryawan},
		})
		require.NoError(t, err)
		require.Equal(t, []RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati}, {Role: authz.RoleKaryawan}}, got)
	})

	t.Run("one role in two properties", func(t *testing.T) {
		t.Parallel()
		got, err := validGrants([]RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati}, {Role: authz.RoleAdmin, PropertyID: mawar}})
		require.NoError(t, err)
		require.Len(t, got, 2)
	})

	tests := map[string][]RoleGrant{
		"no roles":                      nil,
		"unknown role":                  {{Role: "owner"}},
		"organization-wide and limited": {{Role: authz.RoleAdmin}, {Role: authz.RoleAdmin, PropertyID: melati}},
		"limited and organization-wide": {{Role: authz.RoleAdmin, PropertyID: melati}, {Role: authz.RoleAdmin}},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := validGrants(in)
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, "roles", inputErr.Field)
		})
	}
}

func TestChangedBuiltinRoles(t *testing.T) {
	t.Parallel()
	melati, mawar := uuid.New(), uuid.New()
	current := []RoleRef{
		{Key: authz.RoleAdmin, Builtin: true, PropertyID: melati},
		{Key: authz.RoleKaryawan, Builtin: true},
		{Key: "custom:x", Name: "Kasir"},
	}
	tests := []struct {
		name string
		next []RoleGrant
		want []string
	}{
		{"nothing changes", []RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati}, {Role: authz.RoleKaryawan}}, nil},
		{"a property limit moves", []RoleGrant{{Role: authz.RoleAdmin, PropertyID: mawar}, {Role: authz.RoleKaryawan}}, []string{authz.RoleAdmin}},
		{"a property limit is lifted", []RoleGrant{{Role: authz.RoleAdmin}, {Role: authz.RoleKaryawan}}, []string{authz.RoleAdmin}},
		{"a second property is added", []RoleGrant{
			{Role: authz.RoleAdmin, PropertyID: melati}, {Role: authz.RoleAdmin, PropertyID: mawar}, {Role: authz.RoleKaryawan},
		}, []string{authz.RoleAdmin}},
		{"a role is removed and another added", []RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati}, {Role: authz.RoleSupervisor}},
			[]string{authz.RoleSupervisor, authz.RoleKaryawan}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, changedBuiltinRoles(current, tc.next))
		})
	}
}
