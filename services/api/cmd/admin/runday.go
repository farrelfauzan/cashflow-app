package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/provider"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
)

// When in the day the worker's daily jobs run, in Asia/Jakarta: the times of
// promisetask.DailyCron, invoicetask.DailyCron and remindertask.DailyCron (a unit test keeps them
// in step).
const (
	promisesAt  = 30 * time.Minute
	invoicesAt  = 5 * time.Hour
	remindersAt = 8 * time.Hour
)

// runDayRequest is what run-day was asked to do.
type runDayRequest struct {
	day time.Time // midnight in Asia/Jakarta
	org uuid.UUID // uuid.Nil means every active organization
}

// parseRunDay reads run-day's flags. Nothing here touches the database.
func parseRunDay(args []string, stderr io.Writer) (runDayRequest, error) {
	fs := flag.NewFlagSet("run-day", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dayFlag := fs.String("day", "", "the day to run, YYYY-MM-DD in Asia/Jakarta")
	orgFlag := fs.String("org", "", "one organization id (default: every active organization)")
	if err := fs.Parse(args); err != nil {
		return runDayRequest{}, err
	}
	if fs.NArg() > 0 {
		return runDayRequest{}, fmt.Errorf("run-day takes no arguments besides its flags\n%s", usage)
	}
	if *dayFlag == "" {
		return runDayRequest{}, fmt.Errorf("run-day needs -day YYYY-MM-DD\n%s", usage)
	}
	day, err := time.ParseInLocation(time.DateOnly, *dayFlag, clock.Jakarta)
	if err != nil {
		return runDayRequest{}, fmt.Errorf("run-day: -day must be a date as YYYY-MM-DD, got %q", *dayFlag)
	}
	req := runDayRequest{day: day}
	if *orgFlag != "" {
		req.org, err = uuid.Parse(*orgFlag)
		if err != nil || req.org == uuid.Nil {
			return runDayRequest{}, errors.New("run-day: -org must be an organization id (a UUID)")
		}
	}
	return req, nil
}

// notSentNote is printed when run-day records reminders without sending them.
const notSentNote = "reminders recorded, not sent: run-day sends only with WHATSAPP_PROVIDER=devlog"

// runDay runs the worker's three daily jobs for one day, in development only: for each
// organization it closes the due promises at 00:30, generates invoices at 05:00 and decides
// reminders at 08:00 of that day in Asia/Jakarta, each through the service the worker's handler
// calls, on a clock that reads that moment. With WHATSAPP_PROVIDER=devlog it then sends the
// reminders it decided, on the 08:00 clock, to the devlog; otherwise it leaves them unsent.
func runDay(ctx context.Context, args []string, e env) error {
	if err := requireDevelopment(e.lookup); err != nil {
		return err
	}
	req, err := parseRunDay(args, e.stderr)
	if err != nil {
		return err
	}
	sys := e.clock
	if sys == nil {
		sys = clock.System()
	}
	if req.day.After(clock.StartOfDay(sys.Now())) {
		fmt.Fprintf(e.stderr, "admin: warning: %s is in the future; records are dated in the future; the real daily runs will treat them as done\n",
			req.day.Format(time.DateOnly))
	}

	cn, err := e.connect(ctx, e.stderr)
	if err != nil {
		return err
	}
	defer cn.store.Close()

	d := dayRunner{store: cn.store, log: cn.log, sys: sys, day: req.day}
	if d.provider, d.webOrigin, err = runDaySender(cn.admin, cn.log, e.stderr); err != nil {
		return err
	}

	ids, err := runDayOrgs(ctx, cn.store, req.org)
	if err != nil {
		return err
	}
	orgs := org.NewService(cn.store, authz.NewService(cn.store, sys, cn.log))
	var errs []error
	for _, id := range ids {
		name, err := orgs.Name(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("organization %s: %w", id, err))
			continue
		}
		sum, err := d.run(ctx, id)
		if _, werr := fmt.Fprintln(e.stdout, sum.line(name, req.day)); werr != nil {
			return werr
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// runDaySender returns the provider run-day sends reminders with and the payment links' web
// origin: the devlog provider with WHATSAPP_PROVIDER=devlog in development, never Telegram or a
// real WhatsApp. Otherwise it returns no provider and prints notSentNote to stderr.
func runDaySender(cfg config.Admin, log *slog.Logger, stderr io.Writer) (messaging.Provider, string, error) {
	sending, sends, err := cfg.RunDaySending()
	if err != nil {
		return nil, "", fmt.Errorf("run-day: %w", err)
	}
	if !sends {
		fmt.Fprintln(stderr, "admin:", notSentNote)
		return nil, "", nil
	}
	p, err := provider.New(sending.WhatsApp, log)
	if err != nil {
		return nil, "", err
	}
	return p, sending.WebOrigin, nil
}

// runDayOrgs returns the organizations to run: every active one, or only one when it is active.
func runDayOrgs(ctx context.Context, st *store.Store, only uuid.UUID) ([]uuid.UUID, error) {
	ids, err := st.ActiveOrganizationIDs(ctx)
	if err != nil {
		return nil, err
	}
	if only == uuid.Nil {
		return ids, nil
	}
	for _, id := range ids {
		if id == only {
			return []uuid.UUID{id}, nil
		}
	}
	return nil, fmt.Errorf("run-day: %s is not an active organization", only)
}

// dayRunner runs the daily jobs of one day.
type dayRunner struct {
	store *store.Store
	log   *slog.Logger
	sys   clock.Clock
	day   time.Time
	// provider sends the decided reminders (the devlog); nil leaves them unsent.
	provider  messaging.Provider
	webOrigin string // the base of the payment links in the reminders sent
}

// at returns a clock that reads the moment offset into the day and moves on from there.
func (d dayRunner) at(offset time.Duration) clock.Clock {
	moment := d.day.Add(offset)
	return clock.Offset(d.sys, moment.Sub(d.sys.Now()))
}

// decided collects the ids of the reminders a Decider records, in place of the worker's queue,
// so run-day sends them itself.
type decided struct{ ids []uuid.UUID }

// EnqueueSend implements reminder.SendQueue.
func (q *decided) EnqueueSend(_ context.Context, _, reminderID uuid.UUID) error {
	q.ids = append(q.ids, reminderID)
	return nil
}

// daySummary is what one organization's day did.
type daySummary struct {
	promises  promise.CloseResult
	invoices  billing.Result
	reminders reminder.Result
	sent      int // reminders the provider accepted
}

// run runs the jobs in the worker's order, then sends what the reminder run decided. A failing
// job does not stop the later ones, as the worker's separate tasks would not; the failures come
// back joined.
func (d dayRunner) run(ctx context.Context, id uuid.UUID) (daySummary, error) {
	var sum daySummary
	var errs []error

	c := d.at(promisesAt)
	res, err := promise.NewCloser(d.store, c, d.log).CloseDue(ctx, id)
	sum.promises = res
	if err != nil {
		errs = append(errs, fmt.Errorf("closing promises: %w", err))
	}

	c = d.at(invoicesAt)
	gen := billing.NewService(d.store, authz.NewService(d.store, c, d.log), c, d.log, promise.NewCloser(d.store, c, d.log))
	sum.invoices, err = gen.GenerateOrg(ctx, id)
	if err != nil {
		errs = append(errs, fmt.Errorf("generating invoices: %w", err))
	}

	c = d.at(remindersAt)
	var q decided
	var sends reminder.SendQueue
	if d.provider != nil {
		sends = &q
	}
	sum.reminders, err = reminder.NewDecider(d.store, c, d.log, sends).DecideOrg(ctx, id)
	if err != nil {
		errs = append(errs, fmt.Errorf("deciding reminders: %w", err))
	}
	if d.provider != nil {
		sum.sent, err = d.send(ctx, c, id, q.ids)
		if err != nil {
			errs = append(errs, fmt.Errorf("sending reminders: %w", err))
		}
	}
	return sum, errors.Join(errs...)
}

// send sends the reminders through the worker's reminder.Sender on c, and returns how many the
// provider accepted.
func (d dayRunner) send(ctx context.Context, c clock.Clock, org uuid.UUID, ids []uuid.UUID) (int, error) {
	sender := reminder.NewSender(reminder.SenderDeps{
		Store: d.store, Links: paylink.NewService(d.store, authz.NewService(d.store, c, d.log), c, d.webOrigin),
		Provider: d.provider, Clock: c, Log: d.log,
	})
	sent := 0
	var errs []error
	for _, id := range ids {
		outcome, err := sender.Send(ctx, org, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("reminder %s: %w", id, err))
			continue
		}
		if outcome == reminder.OutcomeSent {
			sent++
		}
	}
	return sent, errors.Join(errs...)
}

// line is the one line run-day prints for an organization.
func (s daySummary) line(name string, day time.Time) string {
	p, i, r := s.promises, s.invoices, s.reminders
	out := fmt.Sprintf("%s %s: %s closed (%d kept, %d partly kept, %d broken), %s created, %s recorded (%s), %d sent",
		name, day.Format(time.DateOnly), count(p.Kept+p.PartiallyKept+p.Broken, "promise"), p.Kept, p.PartiallyKept, p.Broken,
		count(i.Invoices, "invoice"), count(r.Reminders, "reminder"), count(p.VisitTasks+r.VisitTasks, "visit task"), s.sent)
	if i.Skipped > 0 {
		out += ", " + count(i.Skipped, "lease") + " without a price"
	}
	if failed := p.Failed + r.Failed; failed > 0 {
		out += ", " + count(failed, "tenant") + " failed"
	}
	return out
}

// count writes n with the noun, adding an s unless n is 1: "1 invoice", "0 invoices".
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
