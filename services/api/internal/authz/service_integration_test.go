//go:build integration

package authz_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestCustomRolesComeFromTheDatabase(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812666")
	karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)

	addCustomRole := func(name, policy string) {
		t.Helper()
		err := env.Store.WithOrgTx(ctx, o.ID, func(tx pgx.Tx) error {
			q := dbgen.New(tx)
			role, err := q.CreateRole(ctx, dbgen.CreateRoleParams{
				ID: uuid.New(), OrganizationID: o.ID, Name: name, Policy: []byte(policy),
			})
			if err != nil {
				return err
			}
			return q.CreateCustomRoleAssignment(ctx, dbgen.CreateCustomRoleAssignmentParams{
				ID: uuid.New(), OrganizationID: o.ID, MembershipID: karyawan.MembershipID,
				RoleID: uuid.NullUUID{UUID: role.ID, Valid: true},
			})
		})
		require.NoError(t, err)
	}

	addCustomRole("Kasir", `{"Version":"2026-10-25","Statement":[
		{"Sid":"ReadLedger","Effect":"Allow","Action":["ledger:read"]}]}`)
	// A broken policy must grant nothing rather than fail the whole principal.
	addCustomRole("Rusak", `{"Version":"2026-10-25","Statement":[
		{"Effect":"Allow","Action":["ledger:typo"]}]}`)

	p := env.Principal(t, o.ID, o.Karyawan.UserID)
	require.ElementsMatch(t, []string{"karyawan", p.Roles[1].Key}, p.RoleKeys())
	require.Equal(t, "Kasir", p.Roles[1].Name)

	d := authz.Evaluate(p, authz.Request{Action: "ledger:read", Resource: authz.Resource{Type: "ledger"}})
	require.True(t, d.Allowed)
	require.Equal(t, "ReadLedger", d.Statement)
}

func TestDenialsAreAuditedEvenWhenTheChangeRollsBack(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Kos Melati", "0812777")
	karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)

	err := env.Store.WithOrgTx(ctx, o.ID, func(tx pgx.Tx) error {
		return env.Authz.Authorize(ctx, tx, karyawan, authz.Request{
			Action: "payroll:approve", Resource: authz.Resource{Type: "payroll_run", ID: "2026-10", ActorsKnown: true},
		})
	})
	require.ErrorIs(t, err, authz.ErrForbidden)

	audit := env.AuditLog(t, o.ID)
	require.Equal(t, "deny", audit[0].Decision)
	require.Equal(t, "payroll:approve", audit[0].Action)
	require.Equal(t, "payroll_run/2026-10", audit[0].Resource)
	require.Equal(t, o.Karyawan.UserID, audit[0].ActorStaffUserID.UUID)
}

func TestAuditLogIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Kos Kenanga", "0812888")
	require.NotEmpty(t, env.AuditLog(t, o.ID), "creating the organization is audited")

	for name, stmt := range map[string]string{
		"update": "UPDATE audit_log SET reason = 'edited'",
		"delete": "DELETE FROM audit_log",
	} {
		t.Run(name, func(t *testing.T) {
			err := env.Store.WithOrgTx(ctx, o.ID, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, stmt)
				return err
			})
			var pgErr *pgconn.PgError
			require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
			require.Equal(t, "42501", pgErr.Code)
		})
	}

	t.Run("not even the owner role can change it", func(t *testing.T) {
		conn, err := pgx.Connect(ctx, env.URLs.Owner)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close(ctx) })
		_, err = conn.Exec(ctx, "UPDATE audit_log SET reason = 'edited'")
		require.ErrorContains(t, err, "append-only")
	})
}
