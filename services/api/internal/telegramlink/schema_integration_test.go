//go:build integration

package telegramlink_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// schemaOrg is an organization with one tenant, a used code and a live link to chatID.
type schemaOrg struct {
	org      testenv.Org
	tenantID uuid.UUID
	codeID   uuid.UUID
	codeHash []byte
	linkID   uuid.UUID
}

func newSchemaOrg(t *testing.T, env *testenv.Env, name, prefix, tenantName string, chatID int64) schemaOrg {
	t.Helper()
	ctx := context.Background()
	so := schemaOrg{org: env.CreateOrg(t, name, prefix), codeID: uuid.New(), linkID: uuid.New()}
	admin := env.Principal(t, so.org.ID, so.org.Admin.UserID)
	tn, err := env.Tenants.Create(ctx, admin, tenant.Input{Name: tenantName, Phone: prefix + "5001"})
	require.NoError(t, err)
	so.tenantID = tn.ID
	sum := sha256.Sum256([]byte("code of " + name))
	so.codeHash = sum[:]
	now := env.Clock.Now()
	require.NoError(t, env.Store.WithOrgTx(ctx, so.org.ID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := q.InsertTelegramLinkCode(ctx, dbgen.InsertTelegramLinkCodeParams{
			ID: so.codeID, OrganizationID: so.org.ID, TenantID: so.tenantID, CodeHash: so.codeHash,
			CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute),
		}); err != nil {
			return err
		}
		if _, err := q.UseTelegramLinkCode(ctx, dbgen.UseTelegramLinkCodeParams{UsedAt: &now, OrganizationID: so.org.ID, ID: so.codeID}); err != nil {
			return err
		}
		return q.InsertTelegramLink(ctx, dbgen.InsertTelegramLinkParams{
			ID: so.linkID, OrganizationID: so.org.ID, TenantID: so.tenantID, LinkCodeID: so.codeID,
			ChatID: chatID, TelegramUserID: chatID, LinkedAt: now,
		})
	}))
	return so
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
}

// TestTelegramLinkSchema checks the database side as the app role: codes and links are isolated
// per organization and cannot be deleted or rewritten, a tenant has one live link, and the two
// lookup functions return ids only, to the app role only.
func TestTelegramLinkSchema(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	const sharedChat = int64(5550007001)
	a := newSchemaOrg(t, env, "Griya Sejahtera", "0812631", "Rina Wulandari", sharedChat)
	b := newSchemaOrg(t, env, "Kos Bulanan Sejahtera", "0812632", "Budi Santoso", sharedChat)
	tx := func(org uuid.UUID, fn func(tx pgx.Tx) error) error { return env.Store.WithOrgTx(ctx, org, fn) }

	t.Run("each organization sees only its own codes and links", func(t *testing.T) {
		for _, w := range []schemaOrg{a, b} {
			for _, table := range []string{"telegram_link_code", "telegram_link"} {
				var all, foreign int
				require.NoError(t, tx(w.org.ID, func(tx pgx.Tx) error {
					return tx.QueryRow(ctx, fmt.Sprintf(
						"SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM %s", table)).Scan(&all, &foreign)
				}))
				require.Equal(t, 1, all, table)
				require.Zero(t, foreign, table)
			}
		}
		var n int
		require.NoError(t, tx(b.org.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM telegram_link WHERE chat_id = $1 AND organization_id = $2", sharedChat, a.org.ID).Scan(&n)
		}))
		require.Zero(t, n, "organization B cannot read A's link to the same chat")

		app, err := pgx.Connect(ctx, env.URLs.App)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, app.Close(ctx)) })
		for _, table := range []string{"telegram_link_code", "telegram_link"} {
			require.NoError(t, app.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n))
			require.Zero(t, n, "no scope, no rows: "+table)
		}
		for _, setting := range []string{"app.telegram_link_code_lookup", "app.telegram_chat_organizations"} {
			_, err = app.Exec(ctx, "SELECT set_config($1, 'on', false)", setting)
			require.NoError(t, err)
		}
		for _, table := range []string{"telegram_link_code", "telegram_link"} {
			require.NoError(t, app.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n))
			require.Zero(t, n, "the lookup policies are for the functions' owner, not the app role: "+table)
		}
	})

	t.Run("the app role cannot delete or rewrite a code or a link", func(t *testing.T) {
		stmts := []struct {
			sql  string
			args []any
		}{
			{"DELETE FROM telegram_link_code WHERE id = $1", []any{a.codeID}},
			{"UPDATE telegram_link_code SET code_hash = $2 WHERE id = $1", []any{a.codeID, make([]byte, 32)}},
			{"UPDATE telegram_link_code SET expires_at = expires_at + interval '1 day' WHERE id = $1", []any{a.codeID}},
			{"UPDATE telegram_link_code SET tenant_id = tenant_id WHERE id = $1", []any{a.codeID}},
			{"DELETE FROM telegram_link WHERE id = $1", []any{a.linkID}},
			{"UPDATE telegram_link SET chat_id = 1 WHERE id = $1", []any{a.linkID}},
			{"UPDATE telegram_link SET tenant_id = tenant_id WHERE id = $1", []any{a.linkID}},
			{"TRUNCATE telegram_link", nil},
		}
		for _, s := range stmts {
			err := tx(a.org.ID, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, s.sql, s.args...)
				return err
			})
			requireCode(t, err, "42501")
		}
	})

	t.Run("a tenant has one live link, a link's tenant is its code's, and a hash is 32 bytes", func(t *testing.T) {
		now := env.Clock.Now()
		insertCode := func(id, tenantID uuid.UUID, hash []byte) error {
			return tx(a.org.ID, func(tx pgx.Tx) error {
				return dbgen.New(tx).InsertTelegramLinkCode(ctx, dbgen.InsertTelegramLinkCodeParams{
					ID: id, OrganizationID: a.org.ID, TenantID: tenantID, CodeHash: hash, CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute),
				})
			})
		}
		insertLink := func(codeID, tenantID uuid.UUID) error {
			return tx(a.org.ID, func(tx pgx.Tx) error {
				return dbgen.New(tx).InsertTelegramLink(ctx, dbgen.InsertTelegramLinkParams{
					ID: uuid.New(), OrganizationID: a.org.ID, TenantID: tenantID, LinkCodeID: codeID,
					ChatID: 5550007099, TelegramUserID: 5550007099, LinkedAt: now,
				})
			})
		}
		requireCode(t, insertCode(uuid.New(), a.tenantID, make([]byte, 31)), "23514")
		requireCode(t, insertCode(uuid.New(), a.tenantID, a.codeHash), "23505")
		requireCode(t, insertCode(uuid.New(), b.tenantID, make([]byte, 32)), "23503")

		second := uuid.New()
		sum := sha256.Sum256([]byte("second code"))
		require.NoError(t, insertCode(second, a.tenantID, sum[:]))
		requireCode(t, insertLink(second, a.tenantID), "23505")   // a live link exists already
		requireCode(t, insertLink(a.codeID, a.tenantID), "23505") // one link per code

		admin := env.Principal(t, a.org.ID, a.org.Admin.UserID)
		other, err := env.Tenants.Create(ctx, admin, tenant.Input{Name: "Dewi Lestari", Phone: "08126315002"})
		require.NoError(t, err)
		requireCode(t, insertLink(second, other.ID), "23503") // not the code's tenant
	})

	t.Run("telegram_link_code_lookup and telegram_chat_organizations return ids, to the app role only", func(t *testing.T) {
		owner, err := pgx.Connect(ctx, env.URLs.Owner)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, owner.Close(ctx)) })
		_, err = owner.Exec(ctx, "CREATE ROLE telegram_stranger NOLOGIN")
		require.NoError(t, err)
		for _, fn := range []struct{ name, signature, result string }{
			{"telegram_link_code_lookup", "telegram_link_code_lookup(bytea)", "TABLE(organization_id uuid, code_id uuid)"},
			{"telegram_chat_organizations", "telegram_chat_organizations(bigint)", "TABLE(organization_id uuid)"},
		} {
			var result string
			var definer bool
			var config []string
			require.NoError(t, owner.QueryRow(ctx, `
				SELECT pg_get_function_result(p.oid), p.prosecdef, coalesce(p.proconfig, '{}')
				FROM pg_proc p WHERE p.proname = $1`, fn.name).Scan(&result, &definer, &config))
			require.Equal(t, fn.result, result, fn.name)
			require.True(t, definer, fn.name)
			require.Contains(t, config, "search_path=pg_catalog, public", fn.name)

			var app, stranger bool
			require.NoError(t, owner.QueryRow(ctx, `
				SELECT has_function_privilege('cashflow_app', $1, 'EXECUTE'),
				       has_function_privilege('telegram_stranger', $1, 'EXECUTE')`, fn.signature).Scan(&app, &stranger))
			require.True(t, app, fn.name)
			require.False(t, stranger, "PUBLIC must not have EXECUTE on "+fn.name)
		}

		org, code, err := env.Store.LookupTelegramLinkCode(ctx, a.codeHash)
		require.NoError(t, err, "the app role runs it with no scope")
		require.Equal(t, a.org.ID, org)
		require.Equal(t, a.codeID, code, "a used code is found too: the organization scope decides")
		_, _, err = env.Store.LookupTelegramLinkCode(ctx, make([]byte, 32))
		require.ErrorIs(t, err, store.ErrTelegramLinkCodeNotFound)

		orgs, err := env.Store.TelegramChatOrganizations(ctx, sharedChat)
		require.NoError(t, err)
		require.ElementsMatch(t, []uuid.UUID{a.org.ID, b.org.ID}, orgs)
		orgs, err = env.Store.TelegramChatOrganizations(ctx, 5550007999)
		require.NoError(t, err)
		require.Empty(t, orgs)

		now := env.Clock.Now()
		require.NoError(t, tx(b.org.ID, func(tx pgx.Tx) error {
			_, err := dbgen.New(tx).UnlinkTelegramChat(ctx, dbgen.UnlinkTelegramChatParams{UnlinkedAt: &now, OrganizationID: b.org.ID, ChatID: sharedChat})
			return err
		}))
		orgs, err = env.Store.TelegramChatOrganizations(ctx, sharedChat)
		require.NoError(t, err)
		require.Equal(t, []uuid.UUID{a.org.ID}, orgs, "an unlinked chat no longer names its organization")
	})

	t.Run("they work for an owner held to row-level security, as in production", func(t *testing.T) {
		owner, err := pgx.Connect(ctx, env.URLs.Owner)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, owner.Close(ctx)) })
		// The test database's owner is a superuser, which skips row-level security, so hand the
		// functions to an ordinary role first. Without the policies they would see nothing.
		for _, stmt := range []string{
			"CREATE ROLE telegram_fn_owner NOLOGIN NOSUPERUSER NOBYPASSRLS",
			"GRANT SELECT ON telegram_link_code, telegram_link TO telegram_fn_owner",
			"ALTER FUNCTION telegram_link_code_lookup(bytea) OWNER TO telegram_fn_owner",
			"ALTER FUNCTION telegram_chat_organizations(bigint) OWNER TO telegram_fn_owner",
		} {
			_, err := owner.Exec(ctx, stmt)
			require.NoError(t, err, stmt)
		}
		_, _, err = env.Store.LookupTelegramLinkCode(ctx, a.codeHash)
		require.ErrorIs(t, err, store.ErrTelegramLinkCodeNotFound, "an owner without the policy is held to the organization scope")
		orgs, err := env.Store.TelegramChatOrganizations(ctx, sharedChat)
		require.NoError(t, err)
		require.Empty(t, orgs, "an owner without the policy is held to the organization scope")

		for _, stmt := range []string{
			"ALTER POLICY telegram_link_code_lookup ON telegram_link_code TO telegram_fn_owner",
			"ALTER POLICY telegram_link_chat_organizations ON telegram_link TO telegram_fn_owner",
		} {
			_, err := owner.Exec(ctx, stmt)
			require.NoError(t, err, stmt)
		}
		org, code, err := env.Store.LookupTelegramLinkCode(ctx, a.codeHash)
		require.NoError(t, err)
		require.Equal(t, a.org.ID, org)
		require.Equal(t, a.codeID, code)
		orgs, err = env.Store.TelegramChatOrganizations(ctx, sharedChat)
		require.NoError(t, err)
		require.Equal(t, []uuid.UUID{a.org.ID}, orgs)

		// Each policy serves only its own function: with its policy taken away again, the chat
		// lookup reads nothing, although the code lookup's policy still belongs to its owner.
		_, err = owner.Exec(ctx, "ALTER POLICY telegram_link_chat_organizations ON telegram_link TO CURRENT_USER")
		require.NoError(t, err)
		orgs, err = env.Store.TelegramChatOrganizations(ctx, sharedChat)
		require.NoError(t, err)
		require.Empty(t, orgs)
	})
}
