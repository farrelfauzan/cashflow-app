//go:build integration

package org_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestCreateValidatesTheFirstMembers(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	tests := map[string]org.New{
		"no name":      {DisplayName: "X", Members: []org.NewMember{{Phone: "081200000001", Name: "A", Roles: []string{"admin"}}}},
		"no members":   {Name: "X", DisplayName: "X"},
		"no admin":     {Name: "X", DisplayName: "X", Members: []org.NewMember{{Phone: "081200000001", Name: "A", Roles: []string{"supervisor"}}}},
		"unknown role": {Name: "X", DisplayName: "X", Members: []org.NewMember{{Phone: "081200000001", Name: "A", Roles: []string{"owner"}}}},
		"bad phone":    {Name: "X", DisplayName: "X", Members: []org.NewMember{{Phone: "12", Name: "A", Roles: []string{"admin"}}}},
		"phone twice":  {Name: "X", DisplayName: "X", Members: []org.NewMember{{Phone: "081200000001", Name: "A", Roles: []string{"admin"}}, {Phone: "+6281200000001", Name: "B", Roles: []string{"karyawan"}}}},
		"missing name": {Name: "X", DisplayName: "X", Members: []org.NewMember{{Phone: "081200000001", Roles: []string{"admin"}}}},
		"no roles":     {Name: "X", DisplayName: "X", Members: []org.NewMember{{Phone: "081200000001", Name: "A"}}},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := env.Orgs.Create(ctx, in)
			require.Error(t, err)
		})
	}
}

// TestScopesSeeOnlyWhatTheyShould checks the row-level security policies added for login and
// roles, from every scope the store offers.
func TestScopesSeeOnlyWhatTheyShould(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	a := env.CreateOrg(t, "Kos Melati", "0812101")
	b := env.CreateOrg(t, "Kontrakan Mawar", "0812202")
	require.NoError(t, env.Auth.RequestCode(ctx, a.Admin.Phone)) // creates an OTP challenge

	count := func(t *testing.T, tx pgx.Tx, table string) int {
		t.Helper()
		var n int
		require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n))
		return n
	}

	t.Run("an organization scope sees its own members and nothing from login", func(t *testing.T) {
		err := env.Store.WithOrgTx(ctx, a.ID, func(tx pgx.Tx) error {
			require.Equal(t, 3, count(t, tx, "membership"))
			require.Equal(t, 3, count(t, tx, "staff_user"), "only A's people, not B's")
			require.Equal(t, 3, count(t, tx, "role_assignment"))
			require.Zero(t, count(t, tx, "otp_challenge"))
			require.Zero(t, count(t, tx, "staff_session"))
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("the auth scope sees people and login data but no organization data", func(t *testing.T) {
		err := env.Store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
			require.Equal(t, 6, count(t, tx, "staff_user"))
			require.Equal(t, 1, count(t, tx, "otp_challenge"))
			require.Zero(t, count(t, tx, "membership"))
			require.Zero(t, count(t, tx, "organization"))
			require.Zero(t, count(t, tx, "role_assignment"))
			require.Zero(t, count(t, tx, "audit_log"))
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("a signed-in user sees only their own memberships and organizations", func(t *testing.T) {
		err := env.Store.WithAuthTx(ctx, a.Admin.UserID, func(tx pgx.Tx) error {
			require.Equal(t, 1, count(t, tx, "membership"))
			require.Equal(t, 1, count(t, tx, "organization"))
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("an organization scope cannot create a staff user", func(t *testing.T) {
		err := env.Store.WithOrgTx(ctx, b.ID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "INSERT INTO staff_user (id, phone, name) VALUES ($1, '+6281299999999', 'X')", uuid.New())
			return err
		})
		require.ErrorContains(t, err, "row-level security")
	})

	t.Run("an assignment cannot point at another organization's membership", func(t *testing.T) {
		bAdmin := env.Principal(t, b.ID, b.Admin.UserID)
		err := env.Store.WithOrgTx(ctx, a.ID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				"INSERT INTO role_assignment (id, organization_id, membership_id, builtin_role) VALUES ($1, $2, $3, $4)",
				uuid.New(), a.ID, bAdmin.MembershipID, authz.RoleSupervisor)
			return err
		})
		require.ErrorContains(t, err, "foreign key")
	})
}
