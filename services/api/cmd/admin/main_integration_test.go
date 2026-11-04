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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
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

// TestSetBankAccount runs set-bank-account as an operator would: it stores the trimmed values,
// prints them, replaces them when run again, and audits each run as a platform action.
func TestSetBankAccount(t *testing.T) {
	ctx := context.Background()
	te := testenv.New(t)
	o := te.CreateOrg(t, "Griya Sejahtera", "0812402")

	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", te.URLs.App)
	t.Setenv("SECRETS_KEY", "ops-1:"+base64.StdEncoding.EncodeToString(raw))
	t.Setenv("SECRETS_KEY_PREVIOUS", "")

	set := func(args ...string) (string, error) {
		var out, errOut bytes.Buffer
		err := run(ctx, append([]string{"set-bank-account"}, args...), env{
			lookup: os.LookupEnv, connect: connect, stdout: &out, stderr: &errOut,
		})
		return out.String(), err
	}
	stored := func() dbgen.OrganizationBankAccount {
		var row dbgen.OrganizationBankAccount
		require.NoError(t, te.Store.WithOrgTx(ctx, o.ID, func(tx pgx.Tx) error {
			row, err = dbgen.New(tx).GetBankAccount(ctx, o.ID)
			return err
		}))
		return row
	}

	out, err := set("-org", o.ID.String(), "-bank", " BCA ", "-number", "123 456 7890", "-holder", "Suhwendi")
	require.NoError(t, err)
	require.Equal(t, "organization "+o.ID.String()+"\nbank BCA\naccount number 123 456 7890\naccount holder Suhwendi\n", out)
	row := stored()
	require.Equal(t, "BCA", row.BankName)
	require.Equal(t, "123 456 7890", row.AccountNumber)
	require.Equal(t, "Suhwendi", row.AccountHolder)
	require.False(t, row.UpdatedByStaffUserID.Valid, "the operator tool has no acting member")

	out, err = set("-org", o.ID.String(), "-bank", "Bank Mandiri", "-number", "137-00-1234567-8", "-holder", "PT Griya Sejahtera")
	require.NoError(t, err)
	require.Contains(t, out, "bank Bank Mandiri\n")
	row = stored()
	require.Equal(t, "Bank Mandiri", row.BankName)
	require.Equal(t, "137-00-1234567-8", row.AccountNumber)

	var audited int
	for _, a := range te.AuditLog(t, o.ID) {
		if a.Action == "settings:update" && a.Resource == "organization_bank_account/"+o.ID.String() {
			require.Equal(t, "allow", a.Decision)
			require.False(t, a.ActorStaffUserID.Valid)
			audited++
		}
	}
	require.Equal(t, 2, audited)

	t.Run("an organization that does not exist", func(t *testing.T) {
		missing := uuid.New()
		out, err := set("-org", missing.String(), "-bank", "BCA", "-number", "1234567890", "-holder", "Suhwendi")
		require.ErrorIs(t, err, paylink.ErrOrganizationNotFound)
		require.ErrorContains(t, err, missing.String())
		require.Empty(t, out)
	})
}
