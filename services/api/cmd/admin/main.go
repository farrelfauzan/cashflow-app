// Command admin is the platform operator's tool. Until the onboarding screens exist it creates
// organizations and their first members:
//
//	admin create-org -name "Griya Sejahtera" -display-name "Griya Sejahtera" \
//	    -member "+6281234567890|Bu Ani|supervisor" \
//	    -member "0813-1111-2222|Pak Budi|admin"
//
// Each -member is "phone|name|role,role". Roles are supervisor, admin and karyawan. At least
// one member must be an admin.
//
// In development it also fills the database with demo organizations and prints who can sign in:
//
//	APP_ENV=development admin seed-demo
//
// and bills the seeded leases and records a few demo payments (the invoices and payments of the
// demo organizations only, never of any other organization):
//
//	APP_ENV=development admin generate-invoices
//
// and runs the worker's daily jobs as if it were another day, for a demo:
//
//	APP_ENV=development admin run-day -day 2026-11-10 [-org 6f1c...]
//
// For each active organization (or only -org) it closes the due payment promises at 00:30,
// generates invoices at 05:00 and decides reminders at 08:00 of that day in Asia/Jakarta, in
// that order, through the same services as the worker, each on a clock that reads that moment.
// It prints one line per organization with what each job did. With WHATSAPP_PROVIDER=devlog it
// then sends the reminders it recorded itself, on the 08:00 clock, to the devlog (DEVLOG_FILE
// too when set), with payment links on PUBLIC_WEB_ORIGIN; never through Telegram or any other
// provider. Otherwise it records them unsent, and the worker, which sends on the real clock only
// what was decided today, marks them skipped. A day in the future is allowed with a warning: the
// real daily runs will then treat what it recorded as done.
//
// These three refuse to run unless APP_ENV is explicitly "development", and check that before
// they connect to the database.
//
// It stores an organization's AutoGoPay API key, encrypted under SECRETS_KEY, and prints the
// webhook path to register at AutoGoPay for that organization:
//
//	admin set-autogopay-key -org 6f1c... < key.txt
//	printf '%s\n' "$AUTOGOPAY_KEY" | admin set-autogopay-key -org 6f1c...
//
// The key is read as one line from stdin, never from a flag or an environment variable, so it
// stays out of the shell history and the process list, and it is never printed. Setting it again
// replaces the key and keeps the webhook path. It runs in every environment.
//
// It sets the bank account an organization's tenants transfer to by hand, shown on the tenant
// payment page (decision 38), and prints what it stored:
//
//	admin set-bank-account -org 6f1c... -bank BCA -number "123 456 7890" -holder "Suhwendi"
//
// The account number is 5 to 34 digits, spaces or '-'. Setting it again replaces it. These
// values are not secret. It runs in every environment.
//
// It sets how many broken payment promises put one of an organization's tenants on the karyawan
// visit list (1 to 10, default 2; decision 41.4) until the settings screen exists:
//
//	admin set-broken-promise-threshold -org 6f1c... -n 3
//
// It applies from the next promise that closes broken. It runs in every environment.
//
// Every command needs DATABASE_URL (the app role) and SECRETS_KEY.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/demoseed"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/logging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

const usage = `usage:
  admin create-org -name NAME -display-name NAME -member "phone|name|roles" [-member ...]
  admin seed-demo   (development only: APP_ENV=development)
  admin generate-invoices   (development only: invoices and payments of the demo organizations)
  admin run-day -day YYYY-MM-DD [-org ORGANIZATION_ID]   (development only: the daily jobs of that day)
  admin set-autogopay-key -org ORGANIZATION_ID   (reads the API key as one line from stdin)
  admin set-bank-account -org ORGANIZATION_ID -bank NAME -number NUMBER -holder NAME
  admin set-broken-promise-threshold -org ORGANIZATION_ID -n 1..10`

// errNotDevelopment stops the demo commands anywhere but an explicit development environment.
var errNotDevelopment = errors.New("demo commands only run with APP_ENV=development")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], env{
		lookup: os.LookupEnv, connect: connect, clock: clock.System(), stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr,
	})
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

// env is what the command needs from the process, so tests can check that seed-demo refuses
// before anything connects to the database.
type env struct {
	lookup func(key string) (string, bool)
	// connect loads the configuration and opens the database, logging to logTo.
	connect func(ctx context.Context, logTo io.Writer) (conn, error)
	// clock is the real clock run-day shifts from; nil means clock.System().
	clock  clock.Clock
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// conn is what a command gets once connected.
type conn struct {
	store *store.Store
	log   *slog.Logger
	keys  *secret.Keyring // from SECRETS_KEY and SECRETS_KEY_PREVIOUS
	// admin is the loaded configuration; run-day reads its reminder sending settings.
	admin config.Admin
}

func run(ctx context.Context, args []string, e env) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "create-org":
		return createOrg(ctx, args[1:], e)
	case "seed-demo":
		return seedDemo(ctx, args[1:], e)
	case "generate-invoices":
		return generateInvoices(ctx, args[1:], e)
	case "run-day":
		return runDay(ctx, args[1:], e)
	case "set-autogopay-key":
		return setAutoGoPayKey(ctx, args[1:], e)
	case "set-bank-account":
		return setBankAccount(ctx, args[1:], e)
	case "set-broken-promise-threshold":
		return setBrokenPromiseThreshold(ctx, args[1:], e)
	}
	return errors.New(usage)
}

// connect loads the admin configuration and opens the database as the app role.
func connect(ctx context.Context, logTo io.Writer) (conn, error) {
	cfg, err := config.LoadAdmin(ctx)
	if err != nil {
		return conn{}, err
	}
	keys, err := cfg.Secrets.Keyring()
	if err != nil {
		return conn{}, err
	}
	log, err := logging.New(logTo, cfg.LogLevel)
	if err != nil {
		return conn{}, err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return conn{}, err
	}
	return conn{store: st, log: log, keys: keys, admin: cfg}, nil
}

func createOrg(ctx context.Context, args []string, e env) error {
	fs := flag.NewFlagSet("create-org", flag.ContinueOnError)
	name := fs.String("name", "", "organization name")
	displayName := fs.String("display-name", "", "name shown in WhatsApp messages")
	var members memberFlags
	fs.Var(&members, "member", `a first member as "phone|name|role,role" (repeatable)`)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cn, err := e.connect(ctx, e.stderr)
	if err != nil {
		return err
	}
	defer cn.store.Close()
	st, log := cn.store, cn.log

	svc := org.NewService(st, authz.NewService(st, clock.System(), log))
	id, err := svc.Create(ctx, org.New{Name: *name, DisplayName: *displayName, Members: members})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(e.stdout, id)
	return err
}

// requireDevelopment passes only when APP_ENV is set, and set to exactly "development". The
// configuration's own default of development when APP_ENV is unset does not count.
func requireDevelopment(lookup func(string) (string, bool)) error {
	v, ok := lookup("APP_ENV")
	if !ok {
		return fmt.Errorf("%w; APP_ENV is not set", errNotDevelopment)
	}
	if v != "development" {
		return fmt.Errorf("%w; APP_ENV is %q", errNotDevelopment, v)
	}
	return nil
}

func seedDemo(ctx context.Context, args []string, e env) error {
	if len(args) > 0 {
		return fmt.Errorf("seed-demo takes no arguments\n%s", usage)
	}
	if err := requireDevelopment(e.lookup); err != nil {
		return err
	}

	cn, err := e.connect(ctx, e.stderr)
	if err != nil {
		return err
	}
	defer cn.store.Close()
	st, log := cn.store, cn.log

	c := clock.System()
	az := authz.NewService(st, c, log)
	// The demo seed sends no WhatsApp: its invited members show the invitation not_sent.
	members := member.NewService(st, az, member.Invitations{Clock: c, Log: log})
	// The services that create units and leases read a clock 40 days back, so the leases are
	// entered then; the rest read the real one.
	past := demoseed.Behind(c, demoseed.EntryDays)
	money := billing.NewService(st, az, c, log, promise.NewCloser(st, c, log))
	res, err := demoseed.Run(ctx, demoseed.Deps{
		Store:          st,
		Authz:          az,
		Orgs:           org.NewService(st, az),
		Members:        members,
		Properties:     property.NewService(st, az, c),
		PastProperties: property.NewService(st, az, past),
		// No queue: the demo seed's leases are invoiced by generate-invoices, or the worker's
		// daily run.
		Tenants:     tenant.NewService(st, az, c, money, nil),
		PastTenants: tenant.NewService(st, az, past, money, nil),
		// Only sets the bank account; it mints no links, so it needs no web origin.
		PayLinks: paylink.NewService(st, az, c, ""),
		// Stores the demo AutoGoPay key, which works only against cmd/fakeautogopay.
		AutoGoPay: autogopay.NewService(st, cn.keys, az, c),
	}, c)
	if err != nil {
		return err
	}
	for _, n := range res.Notes {
		fmt.Fprintln(e.stderr, "admin: note:", n)
	}
	return writeLogins(e.stdout, res.Logins)
}

// generateInvoices bills the demo organizations and records their demo payments, in
// development only. It runs the generation synchronously on the system clock.
func generateInvoices(ctx context.Context, args []string, e env) error {
	if len(args) > 0 {
		return fmt.Errorf("generate-invoices takes no arguments\n%s", usage)
	}
	if err := requireDevelopment(e.lookup); err != nil {
		return err
	}

	cn, err := e.connect(ctx, e.stderr)
	if err != nil {
		return err
	}
	defer cn.store.Close()
	st, log := cn.store, cn.log

	c := clock.System()
	az := authz.NewService(st, c, log)
	sums, err := demoseed.GenerateInvoices(ctx, demoseed.GenerateDeps{
		Store: st, Authz: az, Orgs: org.NewService(st, az), Billing: billing.NewService(st, az, c, log, promise.NewCloser(st, c, log)),
	}, c)
	if err != nil {
		return err
	}
	for _, s := range sums {
		fmt.Fprintf(e.stdout, "%s: %d invoices created, %d payments recorded (%d leases", s.Organization, s.Invoices, s.Payments, s.Leases)
		if s.Skipped > 0 {
			fmt.Fprintf(e.stdout, ", %d without a price", s.Skipped)
		}
		fmt.Fprintln(e.stdout, ")")
	}
	return nil
}

// maxKeyLine bounds what set-autogopay-key reads from stdin: a little more than the longest key
// autogopay accepts, so an oversized line is refused rather than truncated.
const maxKeyLine = autogopay.MaxAPIKeyBytes + 64

// setAutoGoPayKey stores an organization's AutoGoPay API key, read as one line from stdin, and
// prints the organization id and the webhook path. Nothing it prints or returns contains the key.
func setAutoGoPayKey(ctx context.Context, args []string, e env) error {
	fs := flag.NewFlagSet("set-autogopay-key", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	orgFlag := fs.String("org", "", "the organization id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		// Not echoed: a key typed as an argument must not end up in the error.
		return fmt.Errorf("set-autogopay-key takes no arguments; the API key is read from stdin\n%s", usage)
	}
	if *orgFlag == "" {
		return fmt.Errorf("set-autogopay-key needs -org ORGANIZATION_ID\n%s", usage)
	}
	orgID, err := uuid.Parse(*orgFlag)
	if err != nil || orgID == uuid.Nil {
		return errors.New("set-autogopay-key: -org must be an organization id (a UUID)")
	}
	key, err := readKeyLine(e.stdin)
	if err != nil {
		return err
	}

	cn, err := e.connect(ctx, e.stderr)
	if err != nil {
		return err
	}
	defer cn.store.Close()

	clk := clock.System()
	svc := autogopay.NewService(cn.store, cn.keys, authz.NewService(cn.store, clk, cn.log), clk)
	webhookID, err := svc.SetAPIKey(ctx, orgID, key)
	if err != nil {
		return fmt.Errorf("setting the AutoGoPay API key of organization %s: %w", orgID, err)
	}
	_, err = fmt.Fprintf(e.stdout, "organization %s\nwebhook path %s\n", orgID, autogopay.WebhookPath(webhookID))
	return err
}

// readKeyLine reads the first line of r and checks it as an API key. Only one line is read, so a
// key typed at a terminal ends with Enter.
func readKeyLine(r io.Reader) (string, error) {
	if r == nil {
		return "", errors.New("set-autogopay-key: no API key on stdin")
	}
	line, err := bufio.NewReaderSize(io.LimitReader(r, maxKeyLine+1), maxKeyLine+1).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("set-autogopay-key: reading the API key from stdin: %w", err)
	}
	if len(line) > maxKeyLine {
		return "", fmt.Errorf("set-autogopay-key: the API key on stdin is longer than %d bytes", autogopay.MaxAPIKeyBytes)
	}
	if strings.TrimSpace(line) == "" {
		return "", errors.New("set-autogopay-key: no API key on stdin; pipe it in, for example < key.txt")
	}
	key, err := autogopay.NormalizeAPIKey(line)
	if err != nil {
		return "", fmt.Errorf("set-autogopay-key: %w", err)
	}
	return key, nil
}

// setBankAccount stores an organization's bank account for manual transfers and prints what was
// stored. Every value is checked before it connects to the database.
func setBankAccount(ctx context.Context, args []string, e env) error {
	fs := flag.NewFlagSet("set-bank-account", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	orgFlag := fs.String("org", "", "the organization id")
	bank := fs.String("bank", "", `the bank's name, for example "BCA"`)
	number := fs.String("number", "", "the account number: 5 to 34 digits, spaces or '-'")
	holder := fs.String("holder", "", "the account holder's name, as the bank shows it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("set-bank-account takes no arguments besides its flags\n%s", usage)
	}
	if *orgFlag == "" || *bank == "" || *number == "" || *holder == "" {
		return fmt.Errorf("set-bank-account needs -org, -bank, -number and -holder\n%s", usage)
	}
	orgID, err := uuid.Parse(*orgFlag)
	if err != nil || orgID == uuid.Nil {
		return errors.New("set-bank-account: -org must be an organization id (a UUID)")
	}
	account, err := paylink.NormalizeBankAccount(paylink.BankAccount{BankName: *bank, AccountNumber: *number, AccountHolder: *holder})
	if err != nil {
		return fmt.Errorf("set-bank-account: %w", err)
	}

	cn, err := e.connect(ctx, e.stderr)
	if err != nil {
		return err
	}
	defer cn.store.Close()

	clk := clock.System()
	// The bank account needs no link origin: no link is minted here.
	svc := paylink.NewService(cn.store, authz.NewService(cn.store, clk, cn.log), clk, "")
	stored, err := svc.SetBankAccount(ctx, orgID, account)
	if err != nil {
		return fmt.Errorf("setting the bank account of organization %s: %w", orgID, err)
	}
	_, err = fmt.Fprintf(e.stdout, "organization %s\nbank %s\naccount number %s\naccount holder %s\n",
		orgID, stored.BankName, stored.AccountNumber, stored.AccountHolder)
	return err
}

// setBrokenPromiseThreshold sets an organization's broken-promise threshold and prints it. The
// flags are checked before it connects to the database.
func setBrokenPromiseThreshold(ctx context.Context, args []string, e env) error {
	fs := flag.NewFlagSet("set-broken-promise-threshold", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	orgFlag := fs.String("org", "", "the organization id")
	n := fs.Int("n", 0, "how many broken promises put a tenant on the visit list, 1 to 10")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("set-broken-promise-threshold takes no arguments besides its flags\n%s", usage)
	}
	if *orgFlag == "" {
		return fmt.Errorf("set-broken-promise-threshold needs -org and -n\n%s", usage)
	}
	orgID, err := uuid.Parse(*orgFlag)
	if err != nil || orgID == uuid.Nil {
		return errors.New("set-broken-promise-threshold: -org must be an organization id (a UUID)")
	}
	if *n < org.MinBrokenPromiseThreshold || *n > org.MaxBrokenPromiseThreshold {
		return fmt.Errorf("set-broken-promise-threshold: -n must be %d to %d", org.MinBrokenPromiseThreshold, org.MaxBrokenPromiseThreshold)
	}

	cn, err := e.connect(ctx, e.stderr)
	if err != nil {
		return err
	}
	defer cn.store.Close()

	clk := clock.System()
	if err := org.NewService(cn.store, authz.NewService(cn.store, clk, cn.log)).SetBrokenPromiseThreshold(ctx, orgID, *n); err != nil {
		return fmt.Errorf("setting the broken-promise threshold of organization %s: %w", orgID, err)
	}
	_, err = fmt.Fprintf(e.stdout, "organization %s\nbroken-promise threshold %d\n", orgID, *n)
	return err
}

// writeLogins prints the demo logins as a plain table.
func writeLogins(w io.Writer, logins []demoseed.Login) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PHONE\tNAME\tORGANIZATION\tROLES\tPROPERTY LIMIT\tSTATUS")
	for _, l := range logins {
		limit := "-"
		if len(l.PropertyLimits) > 0 {
			limit = strings.Join(l.PropertyLimits, ", ")
		}
		status := "active"
		if l.Disabled {
			status = "disabled (cannot sign in)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", l.Phone, l.Name, l.Organization, strings.Join(l.Roles, ", "), limit, status)
	}
	return tw.Flush()
}

type memberFlags []org.NewMember

func (m *memberFlags) String() string { return fmt.Sprint(len(*m), " members") }

func (m *memberFlags) Set(v string) error {
	parts := strings.Split(v, "|")
	if len(parts) != 3 {
		return fmt.Errorf(`member %q must look like "phone|name|role,role"`, v)
	}
	*m = append(*m, org.NewMember{
		Phone: strings.TrimSpace(parts[0]),
		Name:  strings.TrimSpace(parts[1]),
		Roles: strings.Split(strings.ReplaceAll(parts[2], " ", ""), ","),
	})
	return nil
}
