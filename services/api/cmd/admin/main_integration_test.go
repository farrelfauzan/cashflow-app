//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/devlog"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
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

// TestSetBrokenPromiseThreshold runs set-broken-promise-threshold as an operator would: it stores
// the threshold, prints it, audits it as a platform action, and with 1 a tenant's first broken
// promise puts them on the visit list.
func TestSetBrokenPromiseThreshold(t *testing.T) {
	ctx := context.Background()
	te := testenv.New(t)
	te.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	o := te.CreateOrg(t, "Griya Sejahtera", "0812403")

	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", te.URLs.App)
	t.Setenv("SECRETS_KEY", "ops-1:"+base64.StdEncoding.EncodeToString(raw))
	t.Setenv("SECRETS_KEY_PREVIOUS", "")

	set := func(args ...string) (string, error) {
		var out, errOut bytes.Buffer
		err := run(ctx, append([]string{"set-broken-promise-threshold"}, args...), env{
			lookup: os.LookupEnv, connect: connect, stdout: &out, stderr: &errOut,
		})
		return out.String(), err
	}

	got, err := te.Orgs.BrokenPromiseThreshold(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, org.DefaultBrokenPromiseThreshold, got)

	out, err := set("-org", o.ID.String(), "-n", "1")
	require.NoError(t, err)
	require.Equal(t, "organization "+o.ID.String()+"\nbroken-promise threshold 1\n", out)
	got, err = te.Orgs.BrokenPromiseThreshold(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, 1, got)

	var audited int
	for _, a := range te.AuditLog(t, o.ID) {
		if a.Action == "settings:update" && a.Resource == "organization/"+o.ID.String() {
			require.Equal(t, "allow", a.Decision)
			require.False(t, a.ActorStaffUserID.Valid, "the operator tool has no acting member")
			audited++
		}
	}
	require.Equal(t, 1, audited)

	t.Run("with 1, the first broken promise opens a visit task", func(t *testing.T) {
		admin := te.Principal(t, o.ID, o.Admin.UserID)
		kos, err := te.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
		require.NoError(t, err)
		unit, err := te.Properties.CreateUnit(ctx, admin, kos.ID, property.UnitInput{Label: "Kamar 1", Terms: testenv.Monthly(700_000)})
		require.NoError(t, err)
		rina, err := te.Tenants.Create(ctx, admin, tenant.Input{Name: "Rina Wulandari", Phone: "081240305001"})
		require.NoError(t, err)
		_, err = te.Tenants.CreateLease(ctx, admin, tenant.NewLease{
			TenantID: rina.ID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: tenant.Today(te.Clock)},
		})
		require.NoError(t, err)
		_, err = te.Billing.GenerateOrg(ctx, o.ID)
		require.NoError(t, err)
		promises := promise.NewService(promise.Deps{Store: te.Store, Tenants: te.Billing, Clock: te.Clock})
		date := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
		_, err = promises.MakeForTenant(ctx, admin, rina.ID, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: &date})
		require.NoError(t, err)

		te.Clock.Set(time.Date(2026, 11, 4, 0, 30, 0, 0, clock.Jakarta))
		res, err := te.Promises.CloseDue(ctx, o.ID)
		require.NoError(t, err)
		require.Equal(t, promise.CloseResult{Due: 1, Broken: 1, VisitTasks: 1}, res)
	})

	t.Run("a threshold out of range is refused and nothing changes", func(t *testing.T) {
		_, err := set("-org", o.ID.String(), "-n", "11")
		require.ErrorContains(t, err, "-n must be 1 to 10")
		got, err := te.Orgs.BrokenPromiseThreshold(ctx, o.ID)
		require.NoError(t, err)
		require.Equal(t, 1, got)
	})

	t.Run("an organization that does not exist", func(t *testing.T) {
		missing := uuid.New()
		out, err := set("-org", missing.String(), "-n", "3")
		require.ErrorIs(t, err, org.ErrOrganizationNotFound)
		require.ErrorContains(t, err, missing.String())
		require.Empty(t, out)
	})
}

// TestRunDay runs run-day as a developer would, with the real configuration from the environment.
// Two tenants owe a monthly invoice due 2 November; Sari promised to pay on 3 November and did
// not. Running 4 November without devlog closes her promise broken and records reminders without
// sending them; running 5 November (H+3) with devlog records Rina's H+3 reminder and writes it to
// the devlog file; a rerun records and sends nothing; and 25 November (7 days before the next due
// date) bills the next periods.
func TestRunDay(t *testing.T) {
	ctx := context.Background()
	te := testenv.New(t) // its clock reads 2 November 2026, 09:00 in Jakarta
	o := te.CreateOrg(t, "Griya Sejahtera", "0812404")
	te.CreateOrg(t, "Kos Lain", "0812405") // another active organization

	devlogFile := filepath.Join(t.TempDir(), "devlog.jsonl")
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", te.URLs.App)
	t.Setenv("SECRETS_KEY", testenv.SecretsKey)
	t.Setenv("SECRETS_KEY_PREVIOUS", "")
	t.Setenv("WHATSAPP_PROVIDER", "")
	t.Setenv("DEVLOG_FILE", devlogFile)
	t.Setenv("PUBLIC_WEB_ORIGIN", "http://localhost:3000")

	admin := te.Principal(t, o.ID, o.Admin.UserID)
	kos, err := te.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	lease := func(room, name, phone string) uuid.UUID {
		unit, err := te.Properties.CreateUnit(ctx, admin, kos.ID, property.UnitInput{Label: room, Terms: testenv.Monthly(700_000)})
		require.NoError(t, err)
		tn, err := te.Tenants.Create(ctx, admin, tenant.Input{Name: name, Phone: phone})
		require.NoError(t, err)
		_, err = te.Tenants.CreateLease(ctx, admin, tenant.NewLease{
			TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: tenant.Today(te.Clock)},
		})
		require.NoError(t, err)
		return tn.ID
	}
	rina := lease("Kamar 1", "Rina Wulandari", "081240405001")
	sari := lease("Kamar 2", "Sari Puspita", "081240405002")
	res, err := te.Billing.GenerateOrg(ctx, o.ID)
	require.NoError(t, err)
	require.Equal(t, 2, res.Invoices, "each lease's first invoice, due 2 November")
	promises := promise.NewService(promise.Deps{Store: te.Store, Tenants: te.Billing, Clock: te.Clock})
	promised := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	_, err = promises.MakeForTenant(ctx, admin, sari, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: &promised})
	require.NoError(t, err)

	runDay := func(args ...string) (string, string) {
		t.Helper()
		var out, errOut bytes.Buffer
		err := run(ctx, append([]string{"run-day"}, args...), env{
			lookup: os.LookupEnv, connect: connect, stdout: &out, stderr: &errOut,
			// The real clock: every day run here is in the past.
			clock: clock.NewFake(time.Date(2026, 11, 30, 12, 0, 0, 0, clock.Jakarta)),
		})
		require.NoError(t, err, "stderr: %s", errOut.String())
		require.NotContains(t, errOut.String(), "in the future")
		return out.String(), errOut.String()
	}
	stages := func(tenantID uuid.UUID) map[string]bool {
		t.Helper()
		got := map[string]bool{}
		require.NoError(t, te.Store.WithOrgTx(ctx, o.ID, func(tx pgx.Tx) error {
			q := dbgen.New(tx)
			owing, err := q.ListTenantOwingInvoices(ctx, dbgen.ListTenantOwingInvoicesParams{OrganizationID: o.ID, TenantID: tenantID})
			if err != nil {
				return err
			}
			ids := make([]uuid.UUID, len(owing))
			for i, inv := range owing {
				ids[i] = inv.InvoiceID
			}
			rows, err := q.ListRecordedReminderStages(ctx, dbgen.ListRecordedReminderStagesParams{OrganizationID: o.ID, InvoiceIds: ids})
			for _, r := range rows {
				got[r.Stage] = true
			}
			return err
		}))
		return got
	}
	devlogLines := func() []devlog.Line {
		t.Helper()
		b, err := os.ReadFile(devlogFile)
		if os.IsNotExist(err) {
			return nil
		}
		require.NoError(t, err)
		var lines []devlog.Line
		for _, raw := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var l devlog.Line
			require.NoError(t, json.Unmarshal([]byte(raw), &l))
			lines = append(lines, l)
		}
		return lines
	}
	orgOnly := []string{"-org", o.ID.String()}

	t.Run("without devlog, the day after a promise closes it broken and sends nothing", func(t *testing.T) {
		out, errOut := runDay(append([]string{"-day", "2026-11-04"}, orgOnly...)...)
		require.Regexp(t, `^Griya Sejahtera 2026-11-04: 1 promise closed \(0 kept, 0 partly kept, 1 broken\), 0 invoices created, [1-9]\d* reminders? recorded \(\d+ visit tasks?\), 0 sent\n$`, out)
		require.Contains(t, errOut, "reminders recorded, not sent: run-day sends only with WHATSAPP_PROVIDER=devlog")
		require.False(t, stages(rina)[reminder.StageHPlus3], "H+2 is not yet H+3")
		require.Empty(t, devlogLines())
	})

	t.Setenv("WHATSAPP_PROVIDER", "devlog")

	t.Run("H+3 with devlog records the H+3 reminder and writes it to the devlog", func(t *testing.T) {
		out, errOut := runDay(append([]string{"-day", "2026-11-05"}, orgOnly...)...)
		require.Regexp(t, `^Griya Sejahtera 2026-11-05: 0 promises closed .*, 0 invoices created, ([1-9]\d*) reminders? recorded \(\d+ visit tasks?\), [1-9]\d* sent\n$`, out)
		require.NotContains(t, errOut, notSentNote)
		require.True(t, stages(rina)[reminder.StageHPlus3])

		var toRina []devlog.Line
		for _, l := range devlogLines() {
			if strings.HasSuffix(l.To, "81240405001") {
				toRina = append(toRina, l)
			}
		}
		require.Len(t, toRina, 1, "only the H+3 reminder of 5 November; the 4 November ones stay unsent")
		require.Equal(t, messaging.TemplateInvoiceOverdue, toRina[0].Template)
		require.Contains(t, toRina[0].Text, "http://localhost:3000/t/", "the message carries a payment link on PUBLIC_WEB_ORIGIN")
	})

	t.Run("running the same day again records and sends nothing new", func(t *testing.T) {
		before := len(devlogLines())
		out, _ := runDay(append([]string{"-day", "2026-11-05"}, orgOnly...)...)
		require.Equal(t, "Griya Sejahtera 2026-11-05: 0 promises closed (0 kept, 0 partly kept, 0 broken), 0 invoices created, 0 reminders recorded (0 visit tasks), 0 sent\n", out)
		require.Len(t, devlogLines(), before)
	})

	t.Run("the next period is billed 7 days before it is due", func(t *testing.T) {
		out, _ := runDay(append([]string{"-day", "2026-11-24"}, orgOnly...)...)
		require.Contains(t, out, ", 0 invoices created,", "8 days before 2 December")
		out, _ = runDay(append([]string{"-day", "2026-11-25"}, orgOnly...)...)
		require.Contains(t, out, ", 2 invoices created,")
	})

	t.Run("without -org every active organization runs", func(t *testing.T) {
		out, _ := runDay("-day", "2026-11-25")
		require.Contains(t, out, "Griya Sejahtera 2026-11-25: 0 promises closed (0 kept, 0 partly kept, 0 broken), 0 invoices created")
		require.Contains(t, out, "Kos Lain 2026-11-25: 0 promises closed")
		require.Equal(t, 2, strings.Count(out, "\n"))
	})

	t.Run("an organization that does not exist", func(t *testing.T) {
		missing := uuid.New()
		var out bytes.Buffer
		err := run(ctx, []string{"run-day", "-day", "2026-11-05", "-org", missing.String()}, env{
			lookup: os.LookupEnv, connect: connect, stdout: &out, stderr: &bytes.Buffer{},
			clock: clock.NewFake(time.Date(2026, 11, 30, 12, 0, 0, 0, clock.Jakarta)),
		})
		require.ErrorContains(t, err, missing.String()+" is not an active organization")
		require.Empty(t, out.String())
	})
}
