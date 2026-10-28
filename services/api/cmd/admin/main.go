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
// seed-demo refuses to run unless APP_ENV is explicitly "development", and it checks that
// before it connects to the database.
package main

import (
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

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/demoseed"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/logging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

const usage = `usage:
  admin create-org -name NAME -display-name NAME -member "phone|name|roles" [-member ...]
  admin seed-demo   (development only: APP_ENV=development)`

// errNotDevelopment stops seed-demo anywhere but an explicit development environment.
var errNotDevelopment = errors.New("seed-demo only runs with APP_ENV=development")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], env{lookup: os.LookupEnv, connect: connect, stdout: os.Stdout, stderr: os.Stderr})
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

// env is what the command needs from the process, so tests can check that seed-demo refuses
// before anything connects to the database.
type env struct {
	lookup  func(key string) (string, bool)
	connect func(ctx context.Context) (*store.Store, *slog.Logger, error)
	stdout  io.Writer
	stderr  io.Writer
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
	}
	return errors.New(usage)
}

// connect loads the admin configuration and opens the database as the app role.
func connect(ctx context.Context) (*store.Store, *slog.Logger, error) {
	cfg, err := config.LoadAdmin(ctx)
	if err != nil {
		return nil, nil, err
	}
	log, err := logging.New(os.Stderr, cfg.LogLevel)
	if err != nil {
		return nil, nil, err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return nil, nil, err
	}
	return st, log, nil
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

	st, log, err := e.connect(ctx)
	if err != nil {
		return err
	}
	defer st.Close()

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

	st, log, err := e.connect(ctx)
	if err != nil {
		return err
	}
	defer st.Close()

	c := clock.System()
	az := authz.NewService(st, c, log)
	// The demo seed sends no WhatsApp: its invited members show the invitation not_sent.
	members := member.NewService(st, az, member.Invitations{Clock: c, Log: log})
	res, err := demoseed.Run(ctx, demoseed.Deps{
		Store:      st,
		Authz:      az,
		Orgs:       org.NewService(st, az),
		Members:    members,
		Properties: property.NewService(st, az),
		Tenants:    tenant.NewService(st, az, c),
	}, c)
	if err != nil {
		return err
	}
	for _, n := range res.Notes {
		fmt.Fprintln(e.stderr, "admin: note:", n)
	}
	return writeLogins(e.stdout, res.Logins)
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
