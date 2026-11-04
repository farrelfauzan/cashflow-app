//go:build integration

package billing_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// kinds returns the kind of each invoice of r, oldest period first, as invoices orders them.
func (w *world) kinds(t *testing.T, r renter) []string {
	t.Helper()
	var out []string
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		rows, err := tx.Query(ctx, "SELECT kind FROM invoice WHERE tenant_id = $1 ORDER BY period, seq", r.tenantID)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	return out
}

// labels returns the labels of an invoice's lines, in the order they were written.
func (w *world) labels(t *testing.T, invoiceID uuid.UUID) []string {
	t.Helper()
	var out []string
	for _, ln := range w.lines(t, invoiceID) {
		out = append(out, ln.Label)
	}
	return out
}

// moveOut ends r's lease on end as the admin, today being the fake clock's day.
func (w *world) moveOut(t *testing.T, r renter, end time.Time) {
	t.Helper()
	_, err := w.env.Tenants.MoveOut(context.Background(), w.admin, r.leaseID, end)
	require.NoError(t, err)
}

// movedOutAgain runs the move-out's money settlement once more on r's ended lease, as a retried
// transaction would.
func (w *world) movedOutAgain(t *testing.T, r renter) {
	t.Helper()
	l := w.lease(t, r)
	require.NotNil(t, l.EndDate)
	err := w.env.Store.WithOrgTx(context.Background(), w.org.ID, func(tx pgx.Tx) error {
		return w.env.Billing.MovedOut(context.Background(), tx, w.admin, l)
	})
	require.NoError(t, err)
}

func TestFinalInvoiceAtMoveOut(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	t.Run("none: a move-out with no unbilled reading makes no final invoice", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newMeterWorld(t, env, "0812971")
		r := w.rent(t, "Hendra Wijaya", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 120_000)
		setNow(env, 2026, 11, 8, 5, 0)
		w.generate(t) // the reading goes on the 15 Nov invoice

		setNow(env, 2026, 11, 20, 9, 0)
		w.moveOut(t, r.renter, day(2026, 11, 20))
		require.Equal(t, []string{billing.InvoiceRegular, billing.InvoiceRegular}, w.kinds(t, r.renter))
	})

	t.Run("one: the reading recorded after the last regular invoice goes on one final invoice", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newMeterWorld(t, env, "0812972")
		r := w.rent(t, "Rina Wulandari", day(2026, 10, 15), rentOpts{deposit: 800_000})
		w.generate(t)
		w.payNow(t, r.renter, 700_000)
		setNow(env, 2026, 11, 8, 5, 0)
		require.Equal(t, 1, w.generate(t).Invoices, "the 15 Nov invoice, with no reading yet")

		setNow(env, 2026, 11, 10, 9, 0)
		listrik := w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 150_000)
		air := w.meter(t, w.admin, r.leaseID, w.air, day(2026, 10, 1), 40_000)

		// The tenant leaves on 15 Nov: that period started, so it is billed in full, and the
		// final invoice's period is the same day as the start of a regular one.
		setNow(env, 2026, 11, 15, 10, 0)
		w.moveOut(t, r.renter, day(2026, 11, 15))
		require.Equal(t, []string{billing.InvoiceRegular, billing.InvoiceRegular, billing.InvoiceFinal}, w.kinds(t, r.renter))
		invs := w.invoices(t, r.renter)
		nov, final := invs[1], invs[2]
		require.Equal(t, invoiceDates{From: day(2026, 11, 15), To: day(2026, 11, 15), Due: day(2026, 11, 15)}, datesOf(invs)[2],
			"the lease's last day, due on the move-out day")
		require.EqualValues(t, 190_000, final.Amount)
		require.Equal(t, []string{"Air Oktober 2026", "Listrik Oktober 2026"}, w.labels(t, final.InvoiceID))
		require.Equal(t, []string{"Sewa 15 Nov 2026 – 14 Des 2026"}, w.labels(t, nov.InvoiceID))

		// The readings are billed now, by the final invoice, and locked.
		list := w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{})
		for _, it := range list.Items {
			require.Equal(t, &billing.BilledRef{InvoiceID: final.InvoiceID, Period: day(2026, 11, 15)}, it.Billed, it.ChargeTypeName)
		}
		require.Nil(t, list.NextInvoice)
		err := w.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			_, err := tx.Exec(ctx, "UPDATE metered_charge SET amount = 1 WHERE id = $1", listrik.ID)
			return err
		})
		requireCode(t, err, "42501")
		require.EqualValues(t, 40_000, w.storedAmount(t, air.ID))

		// The deposit pays the open invoices oldest period first, the final one like any other.
		require.Equal(t, []correctionRow{{Delta: -700_000, Reason: billing.ReasonDeposit, CreatedBy: w.org.Admin.UserID}},
			w.corrections(t, nov.InvoiceID))
		require.Equal(t, []correctionRow{{Delta: -100_000, Reason: billing.ReasonDeposit, CreatedBy: w.org.Admin.UserID}},
			w.corrections(t, final.InvoiceID))
		require.EqualValues(t, 90_000, w.balance(t, r.renter).Outstanding)

		// It is audited as a generated invoice, a system action.
		var audited bool
		for _, a := range env.AuditLog(t, w.org.ID) {
			if a.Action == authz.ActionInvoiceCreate && a.Decision == "allow" && !a.ActorStaffUserID.Valid &&
				strings.HasSuffix(a.Resource, "/invoice/"+final.InvoiceID.String()) {
				audited = true
			}
		}
		require.True(t, audited)

		t.Run("the account shows it as final", func(t *testing.T) {
			acct, err := env.Billing.Account(ctx, w.admin, r.tenantID)
			require.NoError(t, err)
			require.Len(t, acct.Invoices, 3)
			got := acct.Invoices[2]
			require.Equal(t, final.InvoiceID, got.ID)
			require.Equal(t, billing.InvoiceFinal, got.Kind)
			require.Equal(t, billing.InvoiceRegular, acct.Invoices[1].Kind)
			require.Equal(t, "open", got.Status, "a deposit is a correction, not a payment")
			require.EqualValues(t, 90_000, got.Outstanding)
			require.Len(t, got.Lines, 2)
			require.Equal(t, listrik.ID, got.Lines[1].MeteredChargeID)
		})

		t.Run("Penagihan lists it in the month of the end date, and as debt from the next day", func(t *testing.T) {
			items := allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: "2026-11"})
			var kinds []string
			for _, it := range items {
				kinds = append(kinds, it.Kind)
			}
			slices.Sort(kinds)
			require.Equal(t, []string{billing.InvoiceFinal, billing.InvoiceRegular}, kinds)
			require.Empty(t, allInvoices(t, env, w.admin, billing.InvoiceQuery{Status: billing.InvoiceStatusOverdue}),
				"due today, not debt yet")

			setNow(env, 2026, 11, 16, 9, 0)
			overdue := allInvoices(t, env, w.admin, billing.InvoiceQuery{Status: billing.InvoiceStatusOverdue})
			require.Len(t, overdue, 1)
			require.Equal(t, final.InvoiceID, overdue[0].ID)
			require.Equal(t, billing.InvoiceFinal, overdue[0].Kind)
			require.Equal(t, 1, overdue[0].OverdueDays)
			d, err := env.Billing.Dashboard(ctx, w.admin, "2026-11")
			require.NoError(t, err)
			require.Len(t, d.Arrears, 1)
			require.Equal(t, "Rina Wulandari", d.Arrears[0].TenantName)
			require.EqualValues(t, 90_000, d.Arrears[0].Overdue)
			require.Equal(t, day(2026, 11, 15), d.Arrears[0].OldestUnpaidDueDate)
		})

		t.Run("it never stands for a regular period", func(t *testing.T) {
			w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
				periods, err := q.ListLeaseInvoicePeriods(ctx, dbgen.ListLeaseInvoicePeriodsParams{OrganizationID: w.org.ID, LeaseID: r.leaseID})
				require.NoError(t, err)
				require.Equal(t, []time.Time{day(2026, 10, 15), day(2026, 11, 15)}, periods)
				rows, err := q.ListInvoicePeriodsOfLeases(ctx, dbgen.ListInvoicePeriodsOfLeasesParams{OrganizationID: w.org.ID, LeaseIds: []uuid.UUID{r.leaseID}})
				require.NoError(t, err)
				require.Len(t, rows, 2)
				return nil
			})
			setNow(env, 2026, 12, 8, 5, 0)
			require.Zero(t, w.generate(t).Invoices)
		})
	})

	t.Run("repeated: a retried move-out makes no second final invoice", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newMeterWorld(t, env, "0812973")
		r := w.rent(t, "Agus Salim", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		setNow(env, 2026, 11, 8, 5, 0)
		w.generate(t)
		setNow(env, 2026, 11, 10, 9, 0)
		w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 150_000)
		setNow(env, 2026, 11, 20, 9, 0)
		w.moveOut(t, r.renter, day(2026, 11, 20))
		require.Equal(t, []string{billing.InvoiceRegular, billing.InvoiceRegular, billing.InvoiceFinal}, w.kinds(t, r.renter))
		final := w.invoices(t, r.renter)[2]

		w.movedOutAgain(t, r.renter)
		require.Len(t, w.invoices(t, r.renter), 3, "nothing unbilled, nothing made")

		// Even with an amount left unbilled (written past the service, which refuses an ended
		// lease), the unique key keeps the lease at one final invoice.
		stray := uuid.New()
		w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			return q.CreateMeteredCharge(ctx, dbgen.CreateMeteredChargeParams{
				ID: stray, OrganizationID: w.org.ID, LeaseID: r.leaseID, ChargeTypeID: w.air.ID,
				UsageMonth: day(2026, 11, 1), Amount: 30_000, RecordedByStaffUserID: w.org.Admin.UserID, Now: env.Clock.Now(),
			})
		})
		w.movedOutAgain(t, r.renter)
		require.Equal(t, []string{billing.InvoiceRegular, billing.InvoiceRegular, billing.InvoiceFinal}, w.kinds(t, r.renter))
		require.Equal(t, []string{"Listrik Oktober 2026"}, w.labels(t, final.InvoiceID))
		for _, ln := range w.meteredLines(t, r.renter) {
			require.NotEqual(t, stray, ln.MeteredChargeID.UUID)
		}

		err := w.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			_, err := tx.Exec(ctx, `INSERT INTO invoice (id, organization_id, lease_id, tenant_id, kind, period, period_end, due_date, amount)
				VALUES ($1, $2, $3, $4, 'final', '2026-11-20', '2026-11-20', '2026-11-20', 30000)`,
				uuid.New(), w.org.ID, r.leaseID, r.tenantID)
			return err
		})
		pgErr := requireCode(t, err, "23505")
		require.Equal(t, "invoice_one_final", pgErr.ConstraintName)

		err = w.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			_, err := tx.Exec(ctx, `INSERT INTO invoice (id, organization_id, lease_id, tenant_id, kind, period, period_end, due_date, amount)
				VALUES ($1, $2, $3, $4, 'final', '2026-11-15', '2026-12-14', '2026-11-20', 30000)`,
				uuid.New(), w.org.ID, r.leaseID, r.tenantID)
			return err
		})
		pgErr = requireCode(t, err, "23514")
		require.Equal(t, "invoice_final_one_day", pgErr.ConstraintName, "a final invoice covers the last day only")
	})

	t.Run("readings that went on a regular invoice made at the move-out leave no final invoice", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newMeterWorld(t, env, "0812974")
		r := w.rent(t, "Dewi Lestari", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		setNow(env, 2026, 11, 3, 9, 0)
		mc := w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 150_000)

		// The generate task never ran for 15 Nov: the move-out makes that invoice, reading included.
		setNow(env, 2026, 11, 20, 9, 0)
		w.moveOut(t, r.renter, day(2026, 11, 20))
		require.Equal(t, []string{billing.InvoiceRegular, billing.InvoiceRegular}, w.kinds(t, r.renter))
		nov := w.invoices(t, r.renter)[1]
		require.Equal(t, []string{"Sewa 15 Nov 2026 – 14 Des 2026", "Listrik Oktober 2026"}, w.labels(t, nov.InvoiceID))
		lines := w.meteredLines(t, r.renter)
		require.Len(t, lines, 1)
		require.Equal(t, mc.ID, lines[0].MeteredChargeID.UUID)
	})

	t.Run("usage billed on an invoice the end cancels stays owed", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newMeterWorld(t, env, "0812975")
		planned := w.rent(t, "Siti Nurhaliza", day(2026, 10, 15), rentOpts{})
		moved := w.rent(t, "Bambang Wijaya", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		w.payNow(t, planned.renter, 700_000)
		w.payNow(t, moved.renter, 700_000)
		setNow(env, 2026, 11, 3, 9, 0)
		for _, r := range []rental{planned, moved} {
			w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 150_000)
		}
		setNow(env, 2026, 11, 8, 5, 0)
		require.Equal(t, 2, w.generate(t).Invoices, "each 15 Nov invoice bills Listrik Oktober")

		setNow(env, 2026, 11, 10, 9, 0)
		setEnd := func(end *time.Time) {
			t.Helper()
			_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, planned.leaseID, end)
			require.NoError(t, err)
		}
		nov := func(r rental) dbgen.InvoiceBalance { return w.invoices(t, r.renter)[1] }

		setEnd(ptr(day(2026, 11, 14)))
		require.EqualValues(t, 150_000, nov(planned).Billed, "only the rent is cancelled")
		require.Equal(t, "open", nov(planned).Status)
		setEnd(ptr(day(2026, 11, 12)))
		require.EqualValues(t, 150_000, nov(planned).Billed, "a second change cancels nothing twice")
		setEnd(nil)
		require.EqualValues(t, 850_000, nov(planned).Billed, "clearing it restores the invoice in full")
		setEnd(ptr(day(2026, 11, 14)))
		w.moveOut(t, planned.renter, day(2026, 11, 10))
		require.Equal(t, []correctionRow{
			{Delta: -700_000, Reason: billing.ReasonNotRenewed, CreatedBy: w.org.Admin.UserID},
			{Delta: 700_000, Reason: billing.ReasonPlannedEndChanged, CreatedBy: w.org.Admin.UserID},
			{Delta: -700_000, Reason: billing.ReasonNotRenewed, CreatedBy: w.org.Admin.UserID},
		}, w.corrections(t, nov(planned).InvoiceID), "the move-out cancels nothing more")
		require.EqualValues(t, 150_000, w.balance(t, planned.renter).Outstanding)

		w.moveOut(t, moved.renter, day(2026, 11, 10))
		require.Equal(t, []correctionRow{{Delta: -700_000, Reason: billing.ReasonNotRenewed, CreatedBy: w.org.Admin.UserID}},
			w.corrections(t, nov(moved).InvoiceID))
		require.EqualValues(t, 150_000, nov(moved).Outstanding)
		require.EqualValues(t, 150_000, w.balance(t, moved.renter).Outstanding)
		for _, r := range []rental{planned, moved} {
			require.Equal(t, []string{billing.InvoiceRegular, billing.InvoiceRegular}, w.kinds(t, r.renter), "nothing left unbilled")
		}
	})

	t.Run("a cancelled booking makes no final invoice; its reading stays unbilled", func(t *testing.T) {
		setNow(env, 2026, 11, 10, 9, 0)
		w := newMeterWorld(t, env, "0812976")
		r := w.rent(t, "Rudi Hartono", day(2026, 11, 20), rentOpts{deposit: 300_000})
		w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 11, 1), 50_000)
		w.moveOut(t, r.renter, day(2026, 11, 20))
		require.Empty(t, w.invoices(t, r.renter))
		list := w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{})
		require.Len(t, list.Items, 1)
		require.Nil(t, list.Items[0].Billed)
	})

	t.Run("a final invoice never blocks or shifts a regular period", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newMeterWorld(t, env, "0812977")
		r := w.rent(t, "Citra Ayu", day(2026, 10, 15), rentOpts{})
		w.generate(t)

		// A final invoice on an active lease, for the start of its next period, written by hand:
		// the service only makes one at move-out.
		setNow(env, 2026, 11, 3, 9, 0)
		w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			id, err := q.InsertFinalInvoiceIfMissing(ctx, dbgen.InsertFinalInvoiceIfMissingParams{
				ID: uuid.New(), OrganizationID: w.org.ID, LeaseID: r.leaseID, TenantID: r.tenantID,
				EndDate: day(2026, 11, 15), DueDate: day(2026, 11, 3), Amount: 50_000, CreatedAt: env.Clock.Now(),
			})
			if err != nil {
				return err
			}
			_, err = q.CreateInvoiceLine(ctx, dbgen.CreateInvoiceLineParams{
				ID: uuid.New(), OrganizationID: w.org.ID, InvoiceID: id, Kind: billing.LineCharge,
				ChargeTypeID: uuid.NullUUID{UUID: w.wifi.ID, Valid: true}, Label: "Wi-Fi", Amount: 50_000,
			})
			return err
		})
		require.Equal(t, &billing.NextInvoice{Period: day(2026, 11, 15), IssuedOn: day(2026, 11, 8)},
			w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{}).NextInvoice)

		setNow(env, 2026, 11, 8, 5, 0)
		require.Equal(t, 1, w.generate(t).Invoices)
		require.Equal(t, []string{billing.InvoiceRegular, billing.InvoiceFinal, billing.InvoiceRegular}, w.kinds(t, r.renter),
			"the final invoice was written first for the same day")
		invs := w.invoices(t, r.renter)
		require.Equal(t, day(2026, 11, 15), invs[2].Period)
		require.Equal(t, []string{"Sewa 15 Nov 2026 – 14 Des 2026"}, w.labels(t, invs[2].InvoiceID))
		require.Zero(t, w.generate(t).Invoices, "a rerun creates nothing")
	})
}

func TestFinalInvoiceIsolation(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 10, 15, 9, 0)
	a := newMeterWorld(t, env, "0812978")
	b := newMeterWorld(t, env, "0812979")
	ra := a.rent(t, "Rina Wulandari", day(2026, 10, 15), rentOpts{})
	rb := b.rent(t, "Budi Santoso", day(2026, 10, 15), rentOpts{})
	a.generate(t)
	b.generate(t)
	setNow(env, 2026, 11, 3, 9, 0)
	a.meter(t, a.admin, ra.leaseID, a.listrik, day(2026, 10, 1), 150_000)
	a.moveOut(t, ra.renter, day(2026, 11, 3))
	require.Equal(t, []string{billing.InvoiceRegular, billing.InvoiceFinal}, a.kinds(t, ra.renter))
	final := a.invoices(t, ra.renter)[1]

	t.Run("only its organization sees it", func(t *testing.T) {
		for _, x := range []struct {
			w    meterWorld
			want int
		}{{a, 1}, {b, 0}} {
			var n int
			x.w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				return tx.QueryRow(ctx, "SELECT count(*) FROM invoice WHERE kind = 'final'").Scan(&n)
			})
			require.Equal(t, x.want, n)
		}
		_, err := env.Billing.Account(ctx, b.admin, ra.tenantID)
		require.ErrorIs(t, err, billing.ErrNotFound)
		for _, it := range allInvoices(t, env, b.admin, billing.InvoiceQuery{Period: "2026-11"}) {
			require.NotEqual(t, final.InvoiceID, it.ID)
		}
	})

	t.Run("no final invoice can be written for another organization", func(t *testing.T) {
		insert := func(org, leaseID, tenantID uuid.UUID) error {
			return b.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
				_, err := q.InsertFinalInvoiceIfMissing(ctx, dbgen.InsertFinalInvoiceIfMissingParams{
					ID: uuid.New(), OrganizationID: org, LeaseID: leaseID, TenantID: tenantID,
					EndDate: day(2026, 11, 3), DueDate: day(2026, 11, 3), Amount: 1, CreatedAt: env.Clock.Now(),
				})
				return err
			})
		}
		pgErr := requireCode(t, insert(a.org.ID, ra.leaseID, ra.tenantID), "42501")
		require.Contains(t, pgErr.Message, "row-level security")
		other := a.rent(t, "Dodi Saputra", day(2026, 11, 1), rentOpts{})
		requireCode(t, insert(b.org.ID, other.leaseID, other.tenantID), "23503")
		require.Equal(t, []string{billing.InvoiceRegular}, b.kinds(t, rb.renter))
	})
}
