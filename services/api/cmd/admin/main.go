// Command admin is the platform operator's tool. Until the onboarding screens exist it creates
// organizations and their first members:
//
//	admin create-org -name "Griya Sejahtera" -display-name "Griya Sejahtera" \
//	    -member "+6281234567890|Bu Ani|supervisor" \
//	    -member "0813-1111-2222|Pak Budi|admin"
//
// Each -member is "phone|name|role,role". Roles are supervisor, admin and karyawan. At least
// one member must be an admin.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/logging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] != "create-org" {
		return errors.New(`usage: admin create-org -name NAME -display-name NAME -member "phone|name|roles" [-member ...]`)
	}

	fs := flag.NewFlagSet("create-org", flag.ContinueOnError)
	name := fs.String("name", "", "organization name")
	displayName := fs.String("display-name", "", "name shown in WhatsApp messages")
	var members memberFlags
	fs.Var(&members, "member", `a first member as "phone|name|role,role" (repeatable)`)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadAdmin(ctx)
	if err != nil {
		return err
	}
	log, err := logging.New(os.Stderr, cfg.LogLevel)
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer st.Close()

	svc := org.NewService(st, authz.NewService(st, clock.System(), log))
	id, err := svc.Create(ctx, org.New{Name: *name, DisplayName: *displayName, Members: members})
	if err != nil {
		return err
	}
	fmt.Println(id)
	return nil
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
