//go:build integration

package reminder_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// world is one organization with a kos, its admin, the promise service and the reminder decider.
type world struct {
	env      *testenv.Env
	org      testenv.Org
	admin    authz.Principal
	kos      property.Property
	promises *promise.Service
	decider  *reminder.Decider
	prefix   string
	n        int
}

func newWorld(t *testing.T, env *testenv.Env, name, prefix string) *world {
	t.Helper()
	w := &world{env: env, org: env.CreateOrg(t, name, prefix), prefix: prefix}
	links := paylink.NewService(env.Store, env.Authz, env.Clock, "https://app.test")
	w.promises = promise.NewService(promise.Deps{Store: env.Store, Links: links, Tenants: env.Billing, Clock: env.Clock})
	w.decider = reminder.NewDecider(env.Store, env.Clock, env.Log, nil)
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	var err error
	w.kos, err = env.Properties.Create(context.Background(), w.admin, property.PropertyInput{Name: "Kos Melati " + prefix, Kind: property.KindKos})
	require.NoError(t, err)
	return w
}

// renter makes a tenant with one monthly lease of Rp700.000 per start date given.
func (w *world) renter(t *testing.T, name string, starts ...time.Time) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	w.n++
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	for i, start := range starts {
		unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{
			Label: fmt.Sprintf("Kamar %d-%d", w.n, i+1), Terms: testenv.Monthly(700_000),
		})
		require.NoError(t, err)
		_, err = w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
			TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: start},
		})
		require.NoError(t, err)
	}
	return tn.ID
}

func (w *world) generate(t *testing.T) {
	t.Helper()
	_, err := w.env.Billing.GenerateOrg(context.Background(), w.org.ID)
	require.NoError(t, err)
}

// invoices are the tenant's invoice ids, oldest period first.
func (w *world) invoices(t *testing.T, tenantID uuid.UUID) []uuid.UUID {
	t.Helper()
	acct, err := w.env.Billing.Account(context.Background(), w.admin, tenantID)
	require.NoError(t, err)
	ids := make([]uuid.UUID, len(acct.Invoices))
	for i, inv := range acct.Invoices {
		ids[i] = inv.ID
	}
	return ids
}

func (w *world) pay(t *testing.T, tenantID uuid.UUID, amount int64) {
	t.Helper()
	_, err := w.env.Billing.RecordPayment(context.Background(), w.admin, billing.PaymentInput{
		RequestID: uuid.New(), TenantID: tenantID, Amount: amount, Method: billing.MethodCash, ReceivedOn: tenant.Today(w.env.Clock),
	})
	require.NoError(t, err)
}

func (w *world) promise(t *testing.T, tenantID uuid.UUID, d time.Time) promise.Promise {
	t.Helper()
	p, err := w.promises.MakeForTenant(context.Background(), w.admin, tenantID, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: &d})
	require.NoError(t, err)
	return p
}

// decide runs the organization's reminders:decide task at now.
func (w *world) decide(t *testing.T, now time.Time) reminder.Result {
	t.Helper()
	w.env.Clock.Set(now)
	res, err := w.decider.DecideOrg(context.Background(), w.org.ID)
	require.NoError(t, err)
	return res
}

func (w *world) inTx(fn func(ctx context.Context, tx pgx.Tx) error) error {
	ctx := context.Background()
	return w.env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error { return fn(ctx, tx) })
}

// recordedReminder is one reminder row with its stages.
type recordedReminder struct {
	ID             uuid.UUID
	RunDay         time.Time
	PromiseVersion uuid.NullUUID
	BrokenPromise  uuid.NullUUID
	Status         string
	Stages         []reminder.InvoiceStage
}

// reminders are the tenant's reminder rows in the order they were recorded.
func (w *world) reminders(t *testing.T, tenantID uuid.UUID) []recordedReminder {
	t.Helper()
	var out []recordedReminder
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, run_day, promise_version_id, broken_promise_id, status FROM reminder
			WHERE tenant_id = $1 ORDER BY run_day, created_at, id`, tenantID)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (recordedReminder, error) {
			var r recordedReminder
			err := row.Scan(&r.ID, &r.RunDay, &r.PromiseVersion, &r.BrokenPromise, &r.Status)
			return r, err
		})
		if err != nil {
			return err
		}
		for i := range out {
			rows, err := tx.Query(ctx, "SELECT invoice_id, stage FROM reminder_invoice WHERE reminder_id = $1", out[i].ID)
			if err != nil {
				return err
			}
			out[i].Stages, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (reminder.InvoiceStage, error) {
				var s reminder.InvoiceStage
				err := row.Scan(&s.InvoiceID, &s.Stage)
				return s, err
			})
			if err != nil {
				return err
			}
		}
		return nil
	}))
	return out
}

// openVisits are the tenant's open visit tasks.
func (w *world) openVisits(t *testing.T, tenantID uuid.UUID) []dbgen.ListOpenVisitTasksRow {
	t.Helper()
	var out []dbgen.ListOpenVisitTasksRow
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListOpenVisitTasks(ctx, w.org.ID)
		for _, row := range rows {
			if row.TenantID == tenantID {
				out = append(out, row)
			}
		}
		return err
	}))
	return out
}

// requireReminder checks one reminder row: its day, its stages (in any order), its H-1 and its
// broken notice (uuid.Nil for none). It is pending.
func requireReminder(t *testing.T, got recordedReminder, runDay time.Time, h1, broken uuid.UUID, stages ...reminder.InvoiceStage) {
	t.Helper()
	require.Equal(t, runDay, got.RunDay)
	require.ElementsMatch(t, stages, got.Stages)
	require.Equal(t, uuid.NullUUID{UUID: h1, Valid: h1 != uuid.Nil}, got.PromiseVersion)
	require.Equal(t, uuid.NullUUID{UUID: broken, Valid: broken != uuid.Nil}, got.BrokenPromise)
	require.Equal(t, "pending", got.Status)
}

func requireCode(t *testing.T, err error, code string) *pgconn.PgError {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
	return pgErr
}

// wib is a time on a day in Asia/Jakarta.
func wib(m time.Month, d, hour, minute int) time.Time {
	return time.Date(2026, m, d, hour, minute, 0, 0, clock.Jakarta)
}

// TestDecideThroughTheMonth walks invoices due 10 Nov from H-4 to H+8: each stage is recorded
// once, on the first run of its window, the Jakarta day turning at midnight; a rerun adds
// nothing; a payment stops the rest; H+7 opens one visit task.
func TestDecideThroughTheMonth(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812971")
	due := day(2026, 11, 10)
	rina := w.renter(t, "Rina Wulandari", due)
	siti := w.renter(t, "Siti Rahayu", due, due) // two rooms, two invoices
	env.Clock.Set(wib(time.November, 3, 9, 0))
	w.generate(t)
	rinaInv := w.invoices(t, rina)
	sitiInv := w.invoices(t, siti)
	require.Len(t, rinaInv, 1)
	require.Len(t, sitiInv, 2)

	require.Equal(t, reminder.Result{}, w.decide(t, wib(time.November, 6, 8, 0)), "H-4: nobody yet")
	require.Equal(t, reminder.Result{}, w.decide(t, wib(time.November, 6, 23, 59)), "23:59 WIB is still H-4")

	t.Run("00:01 WIB on H-3: one reminder per tenant, both of Siti's invoices on hers", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 2, Reminders: 2, Stages: 3}, w.decide(t, wib(time.November, 7, 0, 1)))
		got := w.reminders(t, rina)
		require.Len(t, got, 1)
		requireReminder(t, got[0], day(2026, 11, 7), uuid.Nil, uuid.Nil, stage(rinaInv[0], reminder.StageHMinus3))
		got = w.reminders(t, siti)
		require.Len(t, got, 1)
		requireReminder(t, got[0], day(2026, 11, 7), uuid.Nil, uuid.Nil,
			stage(sitiInv[0], reminder.StageHMinus3), stage(sitiInv[1], reminder.StageHMinus3))
	})

	t.Run("a rerun the same day records nothing", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 2}, w.decide(t, wib(time.November, 7, 8, 0)))
		require.Len(t, w.reminders(t, rina), 1)
		require.Len(t, w.reminders(t, siti), 1)
	})

	t.Run("the rest of H-3's window records nothing", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 2}, w.decide(t, wib(time.November, 9, 8, 0)))
	})

	t.Run("the due date: H0 only", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 2, Reminders: 2, Stages: 3}, w.decide(t, wib(time.November, 10, 8, 0)))
		got := w.reminders(t, rina)
		require.Len(t, got, 2)
		requireReminder(t, got[1], due, uuid.Nil, uuid.Nil, stage(rinaInv[0], reminder.StageH0))
	})

	env.Clock.Set(wib(time.November, 11, 10, 0))
	w.pay(t, siti, 1_400_000) // Siti pays both
	w.pay(t, rina, 100_000)   // Rina pays part

	t.Run("H+3: a paid invoice gets nothing, a partly paid one its H+3", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 1, Reminders: 1, Stages: 1}, w.decide(t, wib(time.November, 13, 8, 0)))
		got := w.reminders(t, rina)
		require.Len(t, got, 3)
		requireReminder(t, got[2], day(2026, 11, 13), uuid.Nil, uuid.Nil, stage(rinaInv[0], reminder.StageHPlus3))
		require.Len(t, w.reminders(t, siti), 2, "Siti's reminders stopped when she paid")
	})

	t.Run("H+7: no message, one visit task", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 1, VisitTasks: 1}, w.decide(t, wib(time.November, 17, 8, 0)))
		tasks := w.openVisits(t, rina)
		require.Len(t, tasks, 1)
		require.Equal(t, reminder.VisitReasonH7, tasks[0].Reason)
		require.True(t, wib(time.November, 17, 8, 0).Equal(tasks[0].CreatedAt))
		require.Len(t, w.reminders(t, rina), 3)
		require.Empty(t, w.openVisits(t, siti))
	})

	t.Run("the next day opens no second task", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 1}, w.decide(t, wib(time.November, 18, 8, 0)))
		require.Len(t, w.openVisits(t, rina), 1)
	})

	t.Run("a resolved visit does not reopen for the same debt; a debt reaching H+7 later opens a new one", func(t *testing.T) {
		first := w.openVisits(t, rina)
		require.Len(t, first, 1)
		env.Clock.Set(wib(time.November, 18, 10, 0))
		require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE visit_task SET resolved_at = $1 WHERE tenant_id = $2 AND resolved_at IS NULL",
				env.Clock.Now(), rina)
			return err
		}))

		require.Equal(t, reminder.Result{Tenants: 1}, w.decide(t, wib(time.November, 19, 8, 0)),
			"the 10 Nov invoice is still unpaid, but the karyawan already visited for it")
		require.Empty(t, w.openVisits(t, rina))

		env.Clock.Set(wib(time.December, 3, 9, 0))
		w.generate(t) // the 10 Dec invoices, due 10 Dec
		require.Len(t, w.invoices(t, rina), 2)
		// 17 Dec is H+7 of the 10 Dec invoices: Rina's reached it after her visit; Siti, who paid
		// November, never had one.
		require.Equal(t, reminder.Result{Tenants: 2, VisitTasks: 2}, w.decide(t, wib(time.December, 17, 8, 0)))
		again := w.openVisits(t, rina)
		require.Len(t, again, 1)
		require.NotEqual(t, first[0].ID, again[0].ID)
		require.True(t, wib(time.December, 17, 8, 0).Equal(again[0].CreatedAt))
		require.Len(t, w.openVisits(t, siti), 1)
	})
}

// TestDecideWithPromises: an active promise silences H+3 and H+7 for what it covers, H-1 is
// recorded once per version, and a broken promise is notified once, with H+3 and H+7 resuming.
func TestDecideWithPromises(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812972")
	due := day(2026, 11, 10)
	rina := w.renter(t, "Rina Wulandari", due)
	dewi := w.renter(t, "Dewi Lestari", due)
	env.Clock.Set(wib(time.November, 3, 9, 0))
	w.generate(t)
	rinaInv, dewiInv := w.invoices(t, rina)[0], w.invoices(t, dewi)[0]

	require.Equal(t, reminder.Result{Tenants: 2, Reminders: 2, Stages: 2}, w.decide(t, wib(time.November, 10, 8, 0)), "H0")

	env.Clock.Set(wib(time.November, 11, 9, 0))
	dewiPromise := w.promise(t, dewi, day(2026, 11, 12))
	rinaPromise := w.promise(t, rina, day(2026, 11, 20))

	t.Run("H-1 of Dewi's promise on its day (made too late for the day before)", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 2, Reminders: 1, PromiseH1: 1}, w.decide(t, wib(time.November, 12, 8, 0)))
		got := w.reminders(t, dewi)
		require.Len(t, got, 2)
		requireReminder(t, got[1], day(2026, 11, 12), dewiPromise.Latest().ID, uuid.Nil)
	})

	env.Clock.Set(wib(time.November, 13, 0, 30))
	closed, err := env.Promises.CloseDue(context.Background(), w.org.ID)
	require.NoError(t, err)
	require.Equal(t, 1, closed.Broken)

	t.Run("H+3: Rina's covered invoice is skipped; Dewi's broken promise and resumed H+3 are one reminder", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 2, Reminders: 1, Stages: 1, BrokenNotices: 1}, w.decide(t, wib(time.November, 13, 8, 0)))
		got := w.reminders(t, dewi)
		require.Len(t, got, 3)
		requireReminder(t, got[2], day(2026, 11, 13), uuid.Nil, dewiPromise.ID, stage(dewiInv, reminder.StageHPlus3))
		require.Len(t, w.reminders(t, rina), 1, "only H0")
	})

	t.Run("the broken notice is recorded once", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 2}, w.decide(t, wib(time.November, 14, 8, 0)))
	})

	t.Run("H+7: Dewi's promise is broken, so her visit; Rina's is active, so none", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 2, VisitTasks: 1}, w.decide(t, wib(time.November, 17, 8, 0)))
		require.Len(t, w.openVisits(t, dewi), 1)
		require.Empty(t, w.openVisits(t, rina))
	})

	t.Run("H-1 per version: an edit to a new date gets its own", func(t *testing.T) {
		require.Equal(t, reminder.Result{Tenants: 2, Reminders: 1, PromiseH1: 1}, w.decide(t, wib(time.November, 19, 8, 0)))
		env.Clock.Set(wib(time.November, 19, 10, 0))
		edited := w.promise(t, rina, day(2026, 11, 22))
		require.Equal(t, 2, edited.Latest().Number)
		require.Equal(t, reminder.Result{Tenants: 2}, w.decide(t, wib(time.November, 20, 8, 0)), "v2's H-1 is 21 Nov")
		require.Equal(t, reminder.Result{Tenants: 2, Reminders: 1, PromiseH1: 1}, w.decide(t, wib(time.November, 21, 8, 0)))
		require.Equal(t, reminder.Result{Tenants: 2}, w.decide(t, wib(time.November, 21, 9, 0)), "a rerun")
		got := w.reminders(t, rina)
		require.Len(t, got, 3)
		requireReminder(t, got[1], day(2026, 11, 19), rinaPromise.Latest().ID, uuid.Nil)
		requireReminder(t, got[2], day(2026, 11, 21), edited.Latest().ID, uuid.Nil)
	})

	t.Run("once Rina's promise breaks: her notice and her visit; H+3's window has closed", func(t *testing.T) {
		env.Clock.Set(wib(time.November, 23, 0, 30))
		_, err := env.Promises.CloseDue(context.Background(), w.org.ID)
		require.NoError(t, err)
		require.Equal(t, reminder.Result{Tenants: 2, Reminders: 1, BrokenNotices: 1, VisitTasks: 1}, w.decide(t, wib(time.November, 23, 8, 0)))
		got := w.reminders(t, rina)
		require.Len(t, got, 4)
		requireReminder(t, got[3], day(2026, 11, 23), uuid.Nil, rinaPromise.ID)
		require.Len(t, w.openVisits(t, rina), 1)
		require.Equal(t, []reminder.InvoiceStage{stage(rinaInv, reminder.StageH0)}, got[0].Stages, "Rina's only invoice stage stays H0")
	})
}

// TestDecideBrokenNoticeWindow: a broken promise is notified only by a run within 3 days of its
// close; a run after a longer pause sends no stale notice.
func TestDecideBrokenNoticeWindow(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812973")
	rina := w.renter(t, "Rina Wulandari", day(2026, 11, 2))
	w.generate(t)
	env.Clock.Set(wib(time.November, 3, 9, 0))
	w.promise(t, rina, day(2026, 11, 4))
	env.Clock.Set(wib(time.November, 5, 0, 30))
	_, err := env.Promises.CloseDue(context.Background(), w.org.ID)
	require.NoError(t, err)

	// 5 Nov + 4 days: the notice's window has passed; the invoice (due 2 Nov) is at H+7.
	require.Equal(t, reminder.Result{Tenants: 1, VisitTasks: 1}, w.decide(t, wib(time.November, 9, 8, 0)))
	require.Empty(t, w.reminders(t, rina))
}

// TestDecideIsolation: deciding for one organization never touches or shows another's tenants.
func TestDecideIsolation(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	a := newWorld(t, env, "Griya Sejahtera", "0812974")
	b := newWorld(t, env, "Kos Bulanan Sejahtera", "0812975")
	due := day(2026, 11, 10)
	rina, dewi := a.renter(t, "Rina Wulandari", due), b.renter(t, "Dewi Lestari", due)
	env.Clock.Set(wib(time.November, 3, 9, 0))
	a.generate(t)
	b.generate(t)

	require.Equal(t, reminder.Result{Tenants: 1, Reminders: 1, Stages: 1}, a.decide(t, wib(time.November, 10, 8, 0)))
	require.Empty(t, b.reminders(t, dewi), "b's tenant was not decided through a")
	require.Equal(t, reminder.Result{Tenants: 1, Reminders: 1, Stages: 1}, b.decide(t, wib(time.November, 10, 8, 0)))

	t.Run("each organization sees only its own rows", func(t *testing.T) {
		for _, w := range []*world{a, b} {
			for _, table := range []string{"reminder", "reminder_invoice"} {
				var all, foreign int
				require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
					return tx.QueryRow(ctx, fmt.Sprintf(
						"SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM %s", table)).Scan(&all, &foreign)
				}))
				require.Equal(t, 1, all, table)
				require.Zero(t, foreign, table)
			}
		}
		require.Empty(t, a.reminders(t, dewi))
		require.Empty(t, b.reminders(t, rina))
	})

	t.Run("no reminder can be written for another organization", func(t *testing.T) {
		err := a.inTx(func(ctx context.Context, tx pgx.Tx) error {
			return dbgen.New(tx).InsertReminder(ctx, dbgen.InsertReminderParams{
				ID: uuid.New(), OrganizationID: b.org.ID, TenantID: dewi, RunDay: due, CreatedAt: env.Clock.Now(),
			})
		})
		require.Contains(t, requireCode(t, err, "42501").Message, "row-level security")
	})
}

// TestReminderTableRules: the database holds the reminder rules on its own: a reminder carries
// something, a stage, H-1 or notice is recorded once and belongs to the reminder's tenant, stages
// are append-only for every role, and the app role may change only a reminder's status and
// sent_at.
func TestReminderTableRules(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812976")
	due := day(2026, 11, 10)
	rina := w.renter(t, "Rina Wulandari", due)
	budi := w.renter(t, "Budi Santoso", due)
	env.Clock.Set(wib(time.November, 3, 9, 0))
	w.generate(t)
	rinaInv, budiInv := w.invoices(t, rina)[0], w.invoices(t, budi)[0]
	budiPromise := w.promise(t, budi, day(2026, 11, 20))
	require.Equal(t, reminder.Result{Tenants: 2, Reminders: 2, Stages: 2}, w.decide(t, wib(time.November, 7, 8, 0)))
	rinaReminder := w.reminders(t, rina)[0].ID
	// An edit of Budi's promise whose H-1 nothing records, for the cross-tenant case.
	budiEdit := w.promise(t, budi, day(2026, 11, 21)).Latest().ID

	exec := func(sql string, args ...any) error {
		return w.inTx(func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)
			return err
		})
	}
	insert := func(r dbgen.InsertReminderParams, stages ...reminder.InvoiceStage) error {
		return w.inTx(func(ctx context.Context, tx pgx.Tx) error {
			q := dbgen.New(tx)
			if err := q.InsertReminder(ctx, r); err != nil {
				return err
			}
			for _, s := range stages {
				if err := q.InsertReminderInvoices(ctx, dbgen.InsertReminderInvoicesParams{
					OrganizationID: w.org.ID, ReminderID: r.ID, InvoiceIds: []uuid.UUID{s.InvoiceID}, TenantID: r.TenantID, Stages: []string{s.Stage},
				}); err != nil {
					return err
				}
			}
			return nil
		})
	}
	newReminder := func(tenantID uuid.UUID) dbgen.InsertReminderParams {
		return dbgen.InsertReminderParams{ID: uuid.New(), OrganizationID: w.org.ID, TenantID: tenantID, RunDay: due, CreatedAt: env.Clock.Now()}
	}
	withH1 := func(r dbgen.InsertReminderParams, v uuid.UUID) dbgen.InsertReminderParams {
		r.PromiseVersionID = uuid.NullUUID{UUID: v, Valid: true}
		return r
	}

	t.Run("an H-1 can be recorded once per version", func(t *testing.T) {
		require.NoError(t, insert(withH1(newReminder(budi), budiPromise.Latest().ID)))
		err := insert(withH1(newReminder(budi), budiPromise.Latest().ID))
		require.Equal(t, "reminder_promise_version_once", requireCode(t, err, "23505").ConstraintName)
	})

	tests := []struct {
		name       string
		insert     func() error
		code       string
		constraint string
	}{
		{
			name:   "a reminder that carries nothing fails at commit",
			insert: func() error { return insert(newReminder(rina)) },
			code:   "23514", constraint: "reminder_carries_something",
		},
		{
			name:   "a stage recorded twice",
			insert: func() error { return insert(newReminder(rina), stage(rinaInv, reminder.StageHMinus3)) },
			code:   "23505", constraint: "reminder_invoice_pkey",
		},
		{
			name:   "an unknown stage",
			insert: func() error { return insert(newReminder(rina), stage(rinaInv, "h7")) },
			code:   "23514", constraint: "reminder_invoice_stage",
		},
		{
			name:   "another tenant's invoice",
			insert: func() error { return insert(newReminder(rina), stage(budiInv, reminder.StageH0)) },
			code:   "23503", constraint: "reminder_invoice_same_tenant",
		},
		{
			name:   "another tenant's promise version",
			insert: func() error { return insert(withH1(newReminder(rina), budiEdit)) },
			code:   "23503", constraint: "reminder_promise_version_same_tenant",
		},
		{
			name: "another tenant's broken promise",
			insert: func() error {
				r := newReminder(rina)
				r.BrokenPromiseID = uuid.NullUUID{UUID: budiPromise.ID, Valid: true}
				return insert(r)
			},
			code: "23503", constraint: "reminder_broken_promise_same_tenant",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.constraint, requireCode(t, tc.insert(), tc.code).ConstraintName)
		})
	}

	t.Run("the app role may not change what was decided", func(t *testing.T) {
		for name, stmt := range map[string]string{
			"delete a reminder":          "DELETE FROM reminder",
			"truncate reminders":         "TRUNCATE reminder",
			"change a reminder's day":    "UPDATE reminder SET run_day = run_day",
			"change a reminder's H-1":    "UPDATE reminder SET promise_version_id = promise_version_id",
			"change a reminder's tenant": "UPDATE reminder SET tenant_id = tenant_id",
			"update a stage":             "UPDATE reminder_invoice SET stage = stage",
			"delete a stage":             "DELETE FROM reminder_invoice",
			"truncate stages":            "TRUNCATE reminder_invoice",
		} {
			t.Run(name, func(t *testing.T) {
				require.Contains(t, requireCode(t, exec(stmt), "42501").Message, "permission denied")
			})
		}
	})

	t.Run("sending may set status and sent_at", func(t *testing.T) {
		require.NoError(t, exec("UPDATE reminder SET status = 'sent', sent_at = $1 WHERE id = $2", env.Clock.Now(), rinaReminder))
		require.Equal(t, "sent", w.reminders(t, rina)[0].Status)
		err := exec("UPDATE reminder SET status = 'dibaca' WHERE id = $1", rinaReminder)
		require.Equal(t, "reminder_status", requireCode(t, err, "23514").ConstraintName)
	})

	t.Run("the owner is stopped by the trigger on stages", func(t *testing.T) {
		conn, err := pgx.Connect(ctx, env.URLs.Owner)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, conn.Close(ctx)) })
		for name, stmt := range map[string]string{
			"update":   "UPDATE reminder_invoice SET stage = stage",
			"delete":   "DELETE FROM reminder_invoice",
			"truncate": "TRUNCATE reminder_invoice",
		} {
			t.Run(name, func(t *testing.T) {
				_, err := conn.Exec(ctx, stmt)
				require.Contains(t, requireCode(t, err, "42501").Message, "is append-only")
			})
		}
	})
}
