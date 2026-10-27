//go:build integration

package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/pgtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// tablesWithoutRLS lists the public tables that are allowed to skip row-level security.
// Adding to this list needs a reason in review.
var tablesWithoutRLS = map[string]bool{
	"goose_db_version": true, // goose's own bookkeeping, holds no organization data
}

// newStore starts a throwaway PostgreSQL, applies the migrations as the owner, and returns a
// Store that connects as the app role, exactly like production.
func newStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	urls := pgtest.Start(t)
	require.NoError(t, Migrate(ctx, urls.Owner, log))

	s, err := Open(ctx, urls.App, log)
	require.NoError(t, err)
	t.Cleanup(s.Close)
	return s
}

func TestOrganizationIsolation(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	orgA, orgB := uuid.New(), uuid.New()
	create := func(id uuid.UUID, name string) {
		t.Helper()
		err := s.WithOrgTx(ctx, id, func(tx pgx.Tx) error {
			_, err := dbgen.New(tx).CreateOrganization(ctx, dbgen.CreateOrganizationParams{
				ID: id, Name: name, DisplayName: name,
			})
			return err
		})
		require.NoError(t, err)
	}
	create(orgA, "Kos Melati")
	create(orgB, "Kontrakan Mawar")

	t.Run("the app role cannot bypass row-level security", func(t *testing.T) {
		var super, bypass bool
		err := s.WithOrgTx(ctx, orgA, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx,
				"SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user",
			).Scan(&super, &bypass)
		})
		require.NoError(t, err)
		require.False(t, super, "app role must not be a superuser")
		require.False(t, bypass, "app role must not have BYPASSRLS")
	})

	t.Run("each organization sees only itself", func(t *testing.T) {
		err := s.WithOrgTx(ctx, orgA, func(tx pgx.Tx) error {
			var n int
			require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM organization").Scan(&n))
			require.Equal(t, 1, n)

			got, err := dbgen.New(tx).GetOrganization(ctx, orgA)
			require.NoError(t, err)
			require.Equal(t, "Kos Melati", got.Name)

			_, err = dbgen.New(tx).GetOrganization(ctx, orgB)
			require.ErrorIs(t, err, pgx.ErrNoRows)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("an organization cannot write another organization's row", func(t *testing.T) {
		err := s.WithOrgTx(ctx, orgA, func(tx pgx.Tx) error {
			_, err := dbgen.New(tx).CreateOrganization(ctx, dbgen.CreateOrganizationParams{
				ID: uuid.New(), Name: "Intruder", DisplayName: "Intruder",
			})
			return err
		})
		var pgErr *pgconn.PgError
		require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
		require.Equal(t, "42501", pgErr.Code, "expected a row-level security violation")
	})

	t.Run("a missing organization id fails before any query", func(t *testing.T) {
		called := false
		err := s.WithOrgTx(ctx, uuid.Nil, func(pgx.Tx) error {
			called = true
			return nil
		})
		require.ErrorIs(t, err, ErrNoOrganization)
		require.False(t, called)
	})

	t.Run("a query without an organization sees nothing", func(t *testing.T) {
		var n int
		require.NoError(t, s.pool.QueryRow(ctx, "SELECT count(*) FROM organization").Scan(&n))
		require.Zero(t, n)
	})

	t.Run("the auth scope without a user sees no organization", func(t *testing.T) {
		err := s.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
			var n int
			require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM organization").Scan(&n))
			require.Zero(t, n)
			return nil
		})
		require.NoError(t, err)
	})
}

// TestEveryTableHasForcedRLS fails when a migration adds a table without enabling and forcing
// row-level security, so a forgotten policy cannot reach production unnoticed.
func TestEveryTableHasForcedRLS(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	var offenders []string
	err := s.WithOrgTx(ctx, uuid.New(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT c.relname
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public'
			  AND c.relkind = 'r'
			  AND NOT (c.relrowsecurity AND c.relforcerowsecurity)
			ORDER BY c.relname`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			if !tablesWithoutRLS[name] {
				offenders = append(offenders, name)
			}
		}
		return rows.Err()
	})
	require.NoError(t, err)
	require.Empty(t, offenders, "tables without enabled and forced row-level security")
}
