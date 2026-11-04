//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// TestSetAutoGoPayKey runs the command as an operator would, with the real configuration loaded
// from the environment: it stores the key, prints only the organization and the webhook path,
// and keeps the path when the key is set again.
func TestSetAutoGoPayKey(t *testing.T) {
	ctx := context.Background()
	te := testenv.New(t)
	o := te.CreateOrg(t, "Griya Sejahtera", "0812401")

	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	secretsKey := "ops-1:" + base64.StdEncoding.EncodeToString(raw)
	t.Setenv("APP_ENV", "production")
	t.Setenv("LOG_LEVEL", "debug") // so anything the command logged would show up on stderr
	t.Setenv("DATABASE_URL", te.URLs.App)
	t.Setenv("SECRETS_KEY", secretsKey)
	t.Setenv("SECRETS_KEY_PREVIOUS", "")

	setKey := func(org, apiKey string) (stdout, stderr string, err error) {
		var out, errOut bytes.Buffer
		err = run(ctx, []string{"set-autogopay-key", "-org", org}, env{
			lookup: os.LookupEnv, connect: connect,
			stdin: strings.NewReader(apiKey + "\n"), stdout: &out, stderr: &errOut,
		})
		return out.String(), errOut.String(), err
	}
	readKey := func() string {
		keys, err := secret.ParseKeyring(secretsKey, "")
		require.NoError(t, err)
		svc := autogopay.NewService(te.Store, keys, te.Authz, te.Clock)
		var v secret.Value
		require.NoError(t, te.Store.WithOrgTx(ctx, o.ID, func(tx pgx.Tx) error {
			v, err = svc.APIKey(ctx, tx, o.ID)
			return err
		}))
		return v.Reveal()
	}
	want := regexp.MustCompile(`^organization ` + o.ID.String() + `\nwebhook path (/webhooks/autogopay/[A-Za-z0-9_-]{43})\n$`)

	const first, second = "agp_live_first_operator_key", "agp_live_second_operator_key"
	stdout, stderr, err := setKey(o.ID.String(), first)
	require.NoError(t, err)
	m := want.FindStringSubmatch(stdout)
	require.NotNil(t, m, "unexpected output %q", stdout)
	path := m[1]
	for _, s := range []string{stdout, stderr} {
		require.NotContains(t, s, first)
		require.NotContains(t, s, "agp_live")
	}
	require.Equal(t, first, readKey())

	stdout, stderr, err = setKey(o.ID.String(), second)
	require.NoError(t, err)
	m = want.FindStringSubmatch(stdout)
	require.NotNil(t, m, "unexpected output %q", stdout)
	require.Equal(t, path, m[1], "setting the key again keeps the webhook path")
	require.NotContains(t, stdout+stderr, "agp_live")
	require.Equal(t, second, readKey())

	t.Run("an organization that does not exist", func(t *testing.T) {
		missing := uuid.New()
		stdout, stderr, err := setKey(missing.String(), first)
		require.ErrorIs(t, err, autogopay.ErrOrganizationNotFound)
		require.ErrorContains(t, err, missing.String())
		require.NotContains(t, err.Error(), "agp_live")
		require.Empty(t, stdout)
		require.NotContains(t, stderr, "agp_live")
	})

	t.Run("a malformed SECRETS_KEY stops the command", func(t *testing.T) {
		t.Setenv("SECRETS_KEY", "ops-1:"+base64.StdEncoding.EncodeToString(raw[:16]))
		_, _, err := setKey(o.ID.String(), first)
		require.ErrorContains(t, err, "SECRETS_KEY has a key of 16 bytes")
		require.Equal(t, second, readKey(), "nothing was stored")
	})
}
