//go:build integration

package autogopay_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// newKey returns a SECRETS_KEY value with a fresh random key.
func newKey(t *testing.T, id string) string {
	t.Helper()
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	return id + ":" + base64.StdEncoding.EncodeToString(raw)
}

func keyring(t *testing.T, current, previous string) *secret.Keyring {
	t.Helper()
	k, err := secret.ParseKeyring(current, previous)
	require.NoError(t, err)
	return k
}

type world struct {
	env *testenv.Env
	svc *autogopay.Service
}

func newWorld(t *testing.T) world {
	t.Helper()
	env := testenv.New(t)
	return world{env: env, svc: autogopay.NewService(env.Store, keyring(t, newKey(t, "k1"), ""), env.Authz, env.Clock)}
}

// inOrg runs fn in org's scope and fails the test on an error.
func (w world) inOrg(t *testing.T, org uuid.UUID, fn func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error) {
	t.Helper()
	require.NoError(t, w.tryOrg(org, fn))
}

func (w world) tryOrg(org uuid.UUID, fn func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error) error {
	ctx := context.Background()
	return w.env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error { return fn(ctx, tx, dbgen.New(tx)) })
}

func (w world) account(t *testing.T, org uuid.UUID) dbgen.AutogopayAccount {
	t.Helper()
	var acc dbgen.AutogopayAccount
	w.inOrg(t, org, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
		var err error
		acc, err = q.GetAutogopayAccount(ctx, org)
		return err
	})
	return acc
}

func (w world) apiKey(org uuid.UUID, svc *autogopay.Service) (secret.Value, error) {
	var v secret.Value
	err := w.tryOrg(org, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		v, err = svc.APIKey(ctx, tx, org)
		return err
	})
	return v, err
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
}

func TestSetAndReadAPIKey(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	org := w.env.CreateOrg(t, "Griya Sejahtera", "0812301")
	const first, second = "agp_live_first_9c1f2e", "agp_live_second_5b7d0a"

	webhookID, err := w.svc.SetAPIKey(ctx, org.ID, "  "+first+"\n")
	require.NoError(t, err)
	require.Regexp(t, `^[A-Za-z0-9_-]{43}$`, webhookID)

	stored := w.account(t, org.ID)
	require.Equal(t, webhookID, stored.WebhookID)
	require.Equal(t, "k1", stored.KeyID)
	require.False(t, bytes.Contains(stored.ApiKeyCiphertext, []byte(first)), "the database holds ciphertext, not the key")
	require.False(t, stored.UpdatedByStaffUserID.Valid, "the operator tool has no acting member")
	require.Equal(t, w.env.Clock.Now().UTC(), stored.UpdatedAt.UTC())

	got, err := w.apiKey(org.ID, w.svc)
	require.NoError(t, err)
	require.Equal(t, first, got.Reveal(), "surrounding whitespace is trimmed")

	t.Run("setting it again keeps the webhook id and replaces the key", func(t *testing.T) {
		w.env.Clock.Advance(time.Hour)
		again, err := w.svc.SetAPIKey(ctx, org.ID, second)
		require.NoError(t, err)
		require.Equal(t, webhookID, again)

		replaced := w.account(t, org.ID)
		require.Equal(t, webhookID, replaced.WebhookID)
		require.NotEqual(t, stored.ApiKeyCiphertext, replaced.ApiKeyCiphertext)
		require.Equal(t, stored.CreatedAt, replaced.CreatedAt)
		require.True(t, replaced.UpdatedAt.After(stored.UpdatedAt))

		got, err := w.apiKey(org.ID, w.svc)
		require.NoError(t, err)
		require.Equal(t, second, got.Reveal())
	})

	t.Run("each setting is in the audit log as a platform action", func(t *testing.T) {
		var n int
		for _, row := range w.env.AuditLog(t, org.ID) {
			if row.Action == "settings:update" && row.Resource == "autogopay_account/"+org.ID.String() {
				require.Equal(t, "allow", row.Decision)
				require.False(t, row.ActorStaffUserID.Valid)
				n++
			}
		}
		require.Equal(t, 2, n)
	})
}

func TestSetAPIKeyRefusals(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	org := w.env.CreateOrg(t, "Kost Putri Anggrek", "0812302")

	t.Run("an invalid key is refused before anything is stored", func(t *testing.T) {
		_, err := w.svc.SetAPIKey(ctx, org.ID, "agp\nlive")
		require.ErrorIs(t, err, autogopay.ErrInvalidAPIKey)
		_, err = w.apiKey(org.ID, w.svc)
		require.ErrorIs(t, err, autogopay.ErrNoAccount)
	})

	t.Run("an organization that does not exist", func(t *testing.T) {
		missing := uuid.New()
		_, err := w.svc.SetAPIKey(ctx, missing, "agp_live_x")
		require.ErrorIs(t, err, autogopay.ErrOrganizationNotFound)
		_, err = w.svc.SetAPIKey(ctx, uuid.Nil, "agp_live_x")
		require.ErrorIs(t, err, autogopay.ErrOrganizationNotFound)
	})
}

func TestAPIKeyRotation(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	org := env.CreateOrg(t, "Griya Sejahtera", "0812303")
	k1, k2 := newKey(t, "k1"), newKey(t, "k2")
	w := world{env: env, svc: autogopay.NewService(env.Store, keyring(t, k1, ""), env.Authz, env.Clock)}
	_, err := w.svc.SetAPIKey(ctx, org.ID, "agp_live_before_rotation")
	require.NoError(t, err)

	rotated := autogopay.NewService(env.Store, keyring(t, k2, k1), env.Authz, env.Clock)
	got, err := w.apiKey(org.ID, rotated)
	require.NoError(t, err, "the previous key still decrypts the stored row")
	require.Equal(t, "agp_live_before_rotation", got.Reveal())

	_, err = rotated.SetAPIKey(ctx, org.ID, "agp_live_after_rotation")
	require.NoError(t, err)
	require.Equal(t, "k2", w.account(t, org.ID).KeyID)

	withoutK2 := autogopay.NewService(env.Store, keyring(t, k1, ""), env.Authz, env.Clock)
	_, err = w.apiKey(org.ID, withoutK2)
	require.ErrorIs(t, err, secret.ErrUnknownKey)
}

// TestAccountIsolation: one organization can neither read nor write another's account, a
// ciphertext copied onto another organization's row does not decrypt there, and the app role
// cannot delete an account.
func TestAccountIsolation(t *testing.T) {
	ctx := context.Background()
	w := newWorld(t)
	a := w.env.CreateOrg(t, "Griya Sejahtera", "0812304")
	b := w.env.CreateOrg(t, "Kos Bulanan Sejahtera", "0812305")
	_, err := w.svc.SetAPIKey(ctx, a.ID, "agp_live_org_a")
	require.NoError(t, err)
	_, err = w.svc.SetAPIKey(ctx, b.ID, "agp_live_org_b")
	require.NoError(t, err)
	accA, accB := w.account(t, a.ID), w.account(t, b.ID)
	require.NotEqual(t, accA.WebhookID, accB.WebhookID)

	t.Run("each organization sees only its own row", func(t *testing.T) {
		for _, org := range []uuid.UUID{a.ID, b.ID} {
			var all, foreign int
			w.inOrg(t, org, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				return tx.QueryRow(ctx,
					"SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM autogopay_account",
				).Scan(&all, &foreign)
			})
			require.Equal(t, 1, all)
			require.Zero(t, foreign)
		}
	})

	t.Run("another organization's account reads as none", func(t *testing.T) {
		err := w.tryOrg(a.ID, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			_, err := q.GetAutogopayAccount(ctx, b.ID)
			require.ErrorIs(t, err, pgx.ErrNoRows)
			_, err = w.svc.APIKey(ctx, tx, b.ID)
			require.ErrorIs(t, err, autogopay.ErrNoAccount)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("an account cannot be written for another organization", func(t *testing.T) {
		err := w.tryOrg(a.ID, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			_, err := q.UpsertAutogopayAccount(ctx, dbgen.UpsertAutogopayAccountParams{
				OrganizationID: b.ID, ApiKeyCiphertext: accA.ApiKeyCiphertext, KeyID: accA.KeyID,
				WebhookID: "a" + accA.WebhookID[1:], Now: w.env.Clock.Now(),
			})
			return err
		})
		requireCode(t, err, "42501")
	})

	t.Run("another organization's account cannot be updated", func(t *testing.T) {
		var tag pgconn.CommandTag
		w.inOrg(t, a.ID, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			var err error
			tag, err = tx.Exec(ctx, "UPDATE autogopay_account SET key_id = 'x' WHERE organization_id = $1", b.ID)
			return err
		})
		require.Zero(t, tag.RowsAffected())
		require.Equal(t, accB, w.account(t, b.ID))
	})

	t.Run("a ciphertext copied onto another organization does not decrypt", func(t *testing.T) {
		// Copy A's ciphertext onto B's row, in B's own scope: the row is B's, the bytes are A's.
		w.inOrg(t, b.ID, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			_, err := tx.Exec(ctx, "UPDATE autogopay_account SET api_key_ciphertext = $2, key_id = $3 WHERE organization_id = $1",
				b.ID, accA.ApiKeyCiphertext, accA.KeyID)
			return err
		})
		_, err := w.apiKey(b.ID, w.svc)
		require.ErrorIs(t, err, secret.ErrDecrypt)
		require.NotContains(t, err.Error(), "agp_live")

		got, err := w.apiKey(a.ID, w.svc)
		require.NoError(t, err)
		require.Equal(t, "agp_live_org_a", got.Reveal(), "A's own row is unchanged")
	})

	t.Run("the app role cannot delete an account", func(t *testing.T) {
		err := w.tryOrg(a.ID, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			_, err := tx.Exec(ctx, "DELETE FROM autogopay_account WHERE organization_id = $1", a.ID)
			return err
		})
		requireCode(t, err, "42501")
		w.account(t, a.ID)
	})

	t.Run("the app role has select, insert and update only", func(t *testing.T) {
		w.inOrg(t, a.ID, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			for priv, want := range map[string]bool{"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": false, "TRUNCATE": false} {
				var has bool
				if err := tx.QueryRow(ctx, "SELECT has_table_privilege(current_user, 'autogopay_account', $1)", priv).Scan(&has); err != nil {
					return err
				}
				require.Equal(t, want, has, priv)
			}
			return nil
		})
	})
}
