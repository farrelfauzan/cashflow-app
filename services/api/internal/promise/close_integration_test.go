//go:build integration

package promise_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// wib is a time on a day in Asia/Jakarta.
func wib(m time.Month, d, hour, minute int) time.Time {
	return time.Date(2026, m, d, hour, minute, 0, 0, clock.Jakarta)
}

// latest is the tenant's newest promise.
func (w *world) latest(t *testing.T, r renter) promise.Promise {
	t.Helper()
	got := w.list(t, r)
	require.NotEmpty(t, got)
	return got[0]
}

// requireStatus checks the tenant's newest promise's status and, once closed, when it closed.
func (w *world) requireStatus(t *testing.T, r renter, status string, closedAt time.Time) {
	t.Helper()
	p := w.latest(t, r)
	require.Equal(t, status, p.Status)
	if status == promise.StatusActive {
		require.Nil(t, p.ClosedAt)
		return
	}
	require.NotNil(t, p.ClosedAt)
	require.True(t, closedAt.Equal(*p.ClosedAt), "closed at %s, want %s", p.ClosedAt, closedAt)
}

// forTenant makes a promise for r as the organization's admin, so no payment link is involved.
func (w *world) forTenant(t *testing.T, r renter, in promise.Input) promise.Promise {
	t.Helper()
	p, err := w.promises.MakeForTenant(context.Background(), w.admin, r.tenantID, in)
	require.NoError(t, err)
	return p
}

// closeDue runs the organization's promises:close task.
func (w *world) closeDue(t *testing.T) promise.CloseResult {
	t.Helper()
	res, err := w.env.Promises.CloseDue(context.Background(), w.org.ID)
	require.NoError(t, err)
	return res
}

// openVisits are the organization's open visit tasks of r.
func (w *world) openVisits(t *testing.T, r renter) []dbgen.ListOpenVisitTasksRow {
	t.Helper()
	var out []dbgen.ListOpenVisitTasksRow
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListOpenVisitTasks(ctx, w.org.ID)
		for _, row := range rows {
			if row.TenantID == r.tenantID {
				out = append(out, row)
			}
		}
		return err
	}))
	return out
}

func amountOn(d time.Time, amount, payingNow int64) promise.Input {
	in := onDay(d)
	in.PromisedAmount, in.PayingNow = &amount, payingNow
	return in
}

// TestPaymentClosesAKeptPromise: a manual payment that reaches what the promise needs closes it as
// kept in the payment's own transaction; one that does not leaves it active, whatever the date.
func TestPaymentClosesAKeptPromise(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812961")

	t.Run("an amount: kept once the payments since it reach the amount", func(t *testing.T) {
		rina := w.rent(t, "Rina Wulandari")
		w.forTenant(t, rina, amountOn(day(2026, 11, 10), 300_000, 0))
		w.pay(t, rina, 100_000)
		w.requireStatus(t, rina, promise.StatusActive, time.Time{})
		env.Clock.Set(wib(time.November, 3, 14, 15))
		w.pay(t, rina, 200_000)
		w.requireStatus(t, rina, promise.StatusKept, wib(time.November, 3, 14, 15))
	})

	t.Run("an amount with paying now: the target is both", func(t *testing.T) {
		dewi := w.rent(t, "Dewi Lestari")
		w.forTenant(t, dewi, amountOn(day(2026, 11, 10), 300_000, 200_000))
		w.pay(t, dewi, 200_000)
		w.requireStatus(t, dewi, promise.StatusActive, time.Time{})
		w.pay(t, dewi, 299_999)
		w.requireStatus(t, dewi, promise.StatusActive, time.Time{})
		w.pay(t, dewi, 1)
		w.requireStatus(t, dewi, promise.StatusKept, env.Clock.Now())
	})

	t.Run("no amount: kept once every covered invoice is paid", func(t *testing.T) {
		budi := w.rent(t, "Budi Santoso")
		w.forTenant(t, budi, onDay(day(2026, 11, 10)))
		w.pay(t, budi, 699_999)
		w.requireStatus(t, budi, promise.StatusActive, time.Time{})
		w.pay(t, budi, 1)
		w.requireStatus(t, budi, promise.StatusKept, env.Clock.Now())
	})

	t.Run("a payment made before the promise does not count toward it", func(t *testing.T) {
		siti := w.rent(t, "Siti Rahayu")
		w.pay(t, siti, 100_000)
		w.forTenant(t, siti, amountOn(day(2026, 11, 10), 300_000, 0))
		w.pay(t, siti, 250_000)
		w.requireStatus(t, siti, promise.StatusActive, time.Time{})
		w.pay(t, siti, 50_000)
		w.requireStatus(t, siti, promise.StatusKept, env.Clock.Now())
	})

	t.Run("an edit counts only the payments after the newest version", func(t *testing.T) {
		agus := w.rent(t, "Agus Pratama")
		w.forTenant(t, agus, amountOn(day(2026, 11, 10), 300_000, 0))
		w.pay(t, agus, 200_000)
		edited := w.forTenant(t, agus, amountOn(day(2026, 11, 12), 200_000, 0))
		require.Equal(t, 2, edited.Latest().Number)
		w.pay(t, agus, 150_000)
		w.requireStatus(t, agus, promise.StatusActive, time.Time{})
		w.pay(t, agus, 50_000)
		w.requireStatus(t, agus, promise.StatusKept, env.Clock.Now())
	})

	t.Run("a repeated payment request counts once", func(t *testing.T) {
		yanti := w.rent(t, "Yanti Kusuma")
		w.forTenant(t, yanti, amountOn(day(2026, 11, 10), 300_000, 0))
		in := billing.PaymentInput{
			RequestID: uuid.New(), TenantID: yanti.tenantID, Amount: 200_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 3),
		}
		for range 2 {
			_, err := env.Billing.RecordPayment(context.Background(), w.admin, in)
			require.NoError(t, err)
		}
		w.requireStatus(t, yanti, promise.StatusActive, time.Time{})
	})
}

// TestCloseDueAroundMidnight: a promise for day D is left alone until D ends in Asia/Jakarta and
// closed by the first run after; running again changes nothing.
func TestCloseDueAroundMidnight(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812962")
	ctx := context.Background()
	d := day(2026, 11, 10)

	// Kept only at the end: no payment, but its one covered invoice is cancelled ("sewa tidak
	// diperpanjang"), so nothing it covers is owed any more.
	kept := w.rent(t, "Rina Wulandari")
	w.forTenant(t, kept, onDay(d))
	zero := int64(0)
	_, err := env.Billing.CreateCorrection(ctx, w.admin, billing.CorrectionInput{
		RequestID: uuid.New(), InvoiceID: kept.invoiceID, NewAmount: &zero, Reason: "sewa tidak diperpanjang",
	})
	require.NoError(t, err)
	w.requireStatus(t, kept, promise.StatusActive, time.Time{})

	partial := w.rent(t, "Dewi Lestari")
	w.forTenant(t, partial, amountOn(d, 500_000, 0))
	w.pay(t, partial, 200_000)

	broken := w.rent(t, "Budi Santoso")
	w.forTenant(t, broken, amountOn(d, 500_000, 0))

	tomorrow := w.rent(t, "Siti Rahayu")
	w.forTenant(t, tomorrow, onDay(day(2026, 11, 11)))

	env.Clock.Set(wib(time.November, 10, 23, 59))
	require.Equal(t, promise.CloseResult{}, w.closeDue(t), "D 23:59 WIB: the promised day has not ended")
	for _, r := range []renter{kept, partial, broken, tomorrow} {
		w.requireStatus(t, r, promise.StatusActive, time.Time{})
	}

	closedAt := wib(time.November, 11, 0, 30)
	env.Clock.Set(closedAt)
	require.Equal(t, promise.CloseResult{Due: 3, Kept: 1, PartiallyKept: 1, Broken: 1}, w.closeDue(t))
	w.requireStatus(t, kept, promise.StatusKept, closedAt)
	w.requireStatus(t, partial, promise.StatusPartiallyKept, closedAt)
	w.requireStatus(t, broken, promise.StatusBroken, closedAt)
	w.requireStatus(t, tomorrow, promise.StatusActive, time.Time{})
	require.Empty(t, w.openVisits(t, broken), "one broken promise is below the default threshold of 2")

	t.Run("running again changes nothing", func(t *testing.T) {
		env.Clock.Set(wib(time.November, 11, 1, 0))
		require.Equal(t, promise.CloseResult{}, w.closeDue(t))
		w.requireStatus(t, kept, promise.StatusKept, closedAt)
		w.requireStatus(t, partial, promise.StatusPartiallyKept, closedAt)
		w.requireStatus(t, broken, promise.StatusBroken, closedAt)
		w.requireStatus(t, tomorrow, promise.StatusActive, time.Time{})
	})

	t.Run("the next day's run closes the next day's promise", func(t *testing.T) {
		env.Clock.Set(wib(time.November, 12, 0, 30))
		require.Equal(t, promise.CloseResult{Due: 1, Broken: 1}, w.closeDue(t))
		w.requireStatus(t, tomorrow, promise.StatusBroken, wib(time.November, 12, 0, 30))
	})

	t.Run("a closed promise is never closed early by a later payment", func(t *testing.T) {
		w.pay(t, partial, 300_000)
		w.requireStatus(t, partial, promise.StatusPartiallyKept, closedAt)
	})
}

// TestBrokenPromisesOpenOneVisitTask: with the default threshold of 2, the second broken promise
// opens one broken_promises visit task, and a third adds no second open one.
func TestBrokenPromisesOpenOneVisitTask(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812963")
	rina := w.rent(t, "Rina Wulandari")
	budi := w.rent(t, "Budi Santoso")

	threshold, err := env.Orgs.BrokenPromiseThreshold(context.Background(), w.org.ID)
	require.NoError(t, err)
	require.Equal(t, 2, threshold, "the default")

	// breakOne makes a promise for the next day and runs the close the day after it.
	breakOne := func(t *testing.T, r renter, madeOn int) promise.CloseResult {
		t.Helper()
		env.Clock.Set(wib(time.November, madeOn, 9, 0))
		w.forTenant(t, r, onDay(day(2026, 11, madeOn+1)))
		env.Clock.Set(wib(time.November, madeOn+2, 0, 30))
		return w.closeDue(t)
	}

	require.Equal(t, promise.CloseResult{Due: 1, Broken: 1}, breakOne(t, rina, 2))
	require.Empty(t, w.openVisits(t, rina))

	require.Equal(t, promise.CloseResult{Due: 1, Broken: 1, VisitTasks: 1}, breakOne(t, rina, 4))
	tasks := w.openVisits(t, rina)
	require.Len(t, tasks, 1)
	require.Equal(t, promise.VisitReasonBrokenPromises, tasks[0].Reason)
	require.True(t, wib(time.November, 6, 0, 30).Equal(tasks[0].CreatedAt))

	require.Equal(t, promise.CloseResult{Due: 1, Broken: 1}, breakOne(t, rina, 6), "a third broken promise opens no second task")
	require.Equal(t, tasks, w.openVisits(t, rina))

	t.Run("a partially kept promise does not count", func(t *testing.T) {
		env.Clock.Set(wib(time.November, 8, 9, 0))
		w.forTenant(t, budi, onDay(day(2026, 11, 9)))
		w.pay(t, budi, 100_000)
		env.Clock.Set(wib(time.November, 10, 0, 30))
		require.Equal(t, promise.CloseResult{Due: 1, PartiallyKept: 1}, w.closeDue(t))
		require.Equal(t, promise.CloseResult{Due: 1, Broken: 1}, breakOne(t, budi, 10))
		require.Empty(t, w.openVisits(t, budi), "one broken and one partially kept")
	})
}

// TestCloseIsolation: closing one organization's promises never touches or shows another's.
func TestCloseIsolation(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	a := newWorld(t, env, "Griya Sejahtera", "0812964")
	b := newWorld(t, env, "Kos Bulanan Sejahtera", "0812965")
	require.NoError(t, env.Orgs.SetBrokenPromiseThreshold(context.Background(), a.org.ID, 1))
	require.NoError(t, env.Orgs.SetBrokenPromiseThreshold(context.Background(), b.org.ID, 1))
	rina, dewi := a.rent(t, "Rina Wulandari"), b.rent(t, "Dewi Lestari")
	a.forTenant(t, rina, onDay(day(2026, 11, 3)))
	b.forTenant(t, dewi, onDay(day(2026, 11, 3)))

	env.Clock.Set(wib(time.November, 4, 0, 30))
	require.Equal(t, promise.CloseResult{Due: 1, Broken: 1, VisitTasks: 1}, a.closeDue(t))
	b.requireStatus(t, dewi, promise.StatusActive, time.Time{})

	t.Run("the other organization's tenants are not closed through this one", func(t *testing.T) {
		res, err := env.Promises.CloseDue(context.Background(), a.org.ID)
		require.NoError(t, err)
		require.Zero(t, res.Due)
		b.requireStatus(t, dewi, promise.StatusActive, time.Time{})
	})

	require.Equal(t, promise.CloseResult{Due: 1, Broken: 1, VisitTasks: 1}, b.closeDue(t))

	t.Run("each organization sees only its own visit tasks", func(t *testing.T) {
		for _, w := range []*world{a, b} {
			var all, foreign int
			require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
				return tx.QueryRow(ctx,
					"SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM visit_task").Scan(&all, &foreign)
			}))
			require.Equal(t, 1, all)
			require.Zero(t, foreign)
		}
	})

	t.Run("no visit task can be written for another organization", func(t *testing.T) {
		err := a.inTx(func(ctx context.Context, tx pgx.Tx) error {
			_, err := dbgen.New(tx).OpenVisitTask(ctx, dbgen.OpenVisitTaskParams{
				ID: uuid.New(), OrganizationID: b.org.ID, TenantID: dewi.tenantID, Reason: promise.VisitReasonBrokenPromises, CreatedAt: env.Clock.Now(),
			})
			return err
		})
		require.Contains(t, requireCode(t, err, "42501").Message, "row-level security")
	})

	t.Run("no setting of another organization can be changed from this one's scope", func(t *testing.T) {
		var n int64
		require.NoError(t, a.inTx(func(ctx context.Context, tx pgx.Tx) error {
			var err error
			n, err = dbgen.New(tx).SetBrokenPromiseThreshold(ctx, dbgen.SetBrokenPromiseThresholdParams{ID: b.org.ID, Threshold: 5})
			return err
		}))
		require.Zero(t, n)
		got, err := env.Orgs.BrokenPromiseThreshold(context.Background(), b.org.ID)
		require.NoError(t, err)
		require.Equal(t, 1, got)
	})
}

// TestVisitTaskRules: a tenant has one open task per reason; a task is never deleted, only its
// resolved_at changes, once.
func TestVisitTaskRules(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812966")
	rina := w.rent(t, "Rina Wulandari")

	open := func(reason string) (int64, error) {
		var n int64
		err := w.inTx(func(ctx context.Context, tx pgx.Tx) error {
			var err error
			n, err = dbgen.New(tx).OpenVisitTask(ctx, dbgen.OpenVisitTaskParams{
				ID: uuid.New(), OrganizationID: w.org.ID, TenantID: rina.tenantID, Reason: reason, CreatedAt: env.Clock.Now(),
			})
			return err
		})
		return n, err
	}
	exec := func(sql string, args ...any) error {
		return w.inTx(func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)
			return err
		})
	}

	n, err := open(promise.VisitReasonBrokenPromises)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
	n, err = open(promise.VisitReasonBrokenPromises)
	require.NoError(t, err)
	require.Zero(t, n, "one open task per tenant and reason")
	n, err = open("h7")
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "another reason is another task")

	_, err = open("lupa")
	require.Equal(t, "visit_task_reason", requireCode(t, err, "23514").ConstraintName)

	for name, stmt := range map[string]string{
		"delete":          "DELETE FROM visit_task",
		"truncate":        "TRUNCATE visit_task",
		"change reason":   "UPDATE visit_task SET reason = 'undeliverable'",
		"change tenant":   "UPDATE visit_task SET tenant_id = tenant_id",
		"change created":  "UPDATE visit_task SET created_at = now()",
		"change the org":  "UPDATE visit_task SET organization_id = organization_id",
		"change the id":   "UPDATE visit_task SET id = id",
		"update two cols": "UPDATE visit_task SET resolved_at = now(), reason = reason",
	} {
		t.Run("the app role may not "+name, func(t *testing.T) {
			require.Contains(t, requireCode(t, exec(stmt), "42501").Message, "permission denied")
		})
	}

	t.Run("resolving sets resolved_at once, and a new task can open after it", func(t *testing.T) {
		resolve := func() error {
			return exec("UPDATE visit_task SET resolved_at = $1 WHERE reason = 'broken_promises'", env.Clock.Now())
		}
		require.NoError(t, resolve())
		require.Contains(t, requireCode(t, resolve(), "42501").Message, "is resolved")

		n, err := open(promise.VisitReasonBrokenPromises)
		require.NoError(t, err)
		require.Equal(t, int64(1), n)
		var total int
		require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM visit_task WHERE reason = 'broken_promises'").Scan(&total)
		}))
		require.Equal(t, 2, total, "the resolved task is kept")
	})

	t.Run("the threshold stays within 1 to 10 in the database", func(t *testing.T) {
		for _, bad := range []int{0, 11} {
			err := exec("UPDATE organization SET broken_promise_threshold = $1", bad)
			require.Equal(t, "organization_broken_promise_threshold", requireCode(t, err, "23514").ConstraintName, fmt.Sprint(bad))
		}
	})
}
