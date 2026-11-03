//go:build integration

package billing_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask/invoicetasktest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// setNow moves the fake clock to a moment in Jakarta.
func setNow(env *testenv.Env, y int, m time.Month, dd, hour, minute int) {
	env.Clock.Set(time.Date(y, m, dd, hour, minute, 0, 0, clock.Jakarta))
}

// rental is a tenant renting their own unit.
type rental struct {
	renter
	unitID uuid.UUID
}

// rentOpts are the lease terms of a rental; zero values mean a monthly lease at Rp700.000,
// billed from its start date, with no deposit.
type rentOpts struct {
	term       int
	price      int64
	billedFrom time.Time
	deposit    int64
}

// rent creates a unit priced from today (the fake clock), a tenant, and a lease starting on
// start, saved today, through the services, as the admin.
func (w *world) rent(t *testing.T, name string, start time.Time, o rentOpts) rental {
	t.Helper()
	ctx := context.Background()
	if o.term == 0 {
		o.term = 1
	}
	if o.price == 0 {
		o.price = 700_000
	}
	w.n++
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{
		Label: fmt.Sprintf("Kamar %d", w.n), Terms: []property.TermInput{{Months: o.term, Price: &o.price}},
	})
	require.NoError(t, err)
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	l, err := w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: o.term,
		Terms: tenant.Terms{StartDate: start, BilledFrom: o.billedFrom, DepositAmount: o.deposit},
	})
	require.NoError(t, err)
	return rental{renter: renter{tenantID: tn.ID, leaseID: l.ID}, unitID: unit.ID}
}

// generate runs the organization's invoices:generate task on the fake clock.
func (w *world) generate(t *testing.T) billing.Result {
	t.Helper()
	res, err := w.env.Billing.GenerateOrg(context.Background(), w.org.ID)
	require.NoError(t, err)
	return res
}

// payNow records a cash payment received now (the fake clock) and allocates it, as the payment
// service will (T-052); it returns the allocation rows.
func (w *world) payNow(t *testing.T, r renter, amount int64) []billing.Row {
	t.Helper()
	var rows []billing.Row
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := w.insertPayment(ctx, q, r, amount, w.env.Clock.Now()); err != nil {
			return err
		}
		var err error
		rows, err = billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
		return err
	})
	return rows
}

// correctionRow is a correction as the tests check it.
type correctionRow struct {
	Delta     int64
	Reason    string
	CreatedBy uuid.UUID
}

func (w *world) corrections(t *testing.T, invoiceID uuid.UUID) []correctionRow {
	t.Helper()
	var out []correctionRow
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		rows, err := tx.Query(ctx, `SELECT amount_delta, reason, created_by_staff_user_id FROM correction
			WHERE invoice_id = $1 ORDER BY seq`, invoiceID)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (correctionRow, error) {
			var c correctionRow
			return c, r.Scan(&c.Delta, &c.Reason, &c.CreatedBy)
		})
		return err
	})
	return out
}

func (w *world) lines(t *testing.T, invoiceID uuid.UUID) []dbgen.InvoiceLine {
	t.Helper()
	var out []dbgen.InvoiceLine
	w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
		var err error
		out, err = q.ListInvoiceLines(ctx, dbgen.ListInvoiceLinesParams{OrganizationID: w.org.ID, InvoiceID: invoiceID})
		return err
	})
	return out
}

// invoiceDates is what identifies a generated invoice: its period and due date.
type invoiceDates struct{ From, To, Due time.Time }

func datesOf(rows []dbgen.InvoiceBalance) []invoiceDates {
	out := make([]invoiceDates, len(rows))
	for i, r := range rows {
		out[i] = invoiceDates{From: r.Period, To: r.PeriodEnd, Due: r.DueDate}
	}
	return out
}

func (w *world) lease(t *testing.T, r renter) tenant.Lease {
	t.Helper()
	tn, err := w.env.Tenants.Get(context.Background(), w.admin, r.tenantID)
	require.NoError(t, err)
	for _, l := range tn.Leases {
		if l.ID == r.leaseID {
			return l
		}
	}
	t.Fatalf("lease %s not found", r.leaseID)
	return tenant.Lease{}
}

func TestGenerateInvoices(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	t.Run("the user's example: invoiced at once, a partial payment, the next month, one total", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 10, 0)
		w := newWorld(t, env, "0812801")
		r := w.rent(t, "Rina Wulandari", day(2026, 10, 28), rentOpts{})
		require.Contains(t, env.Queue.Tasks(), invoicetask.Task{
			ID: invoicetask.LeaseTaskID(w.org.ID, r.leaseID), OrganizationID: w.org.ID,
		}, "saving the lease enqueues the organization's generate task, named after the lease")

		// The task runs within seconds: the 28 Oct to 27 Nov invoice, due that day.
		require.Equal(t, 1, w.generate(t).Invoices)
		invs := w.invoices(t, r.renter)
		require.Equal(t, []invoiceDates{{day(2026, 10, 28), day(2026, 11, 27), day(2026, 10, 28)}}, datesOf(invs))
		require.EqualValues(t, 700_000, invs[0].Amount)
		lines := w.lines(t, invs[0].InvoiceID)
		require.Len(t, lines, 1)
		require.Equal(t, "rent", lines[0].Kind)
		require.Equal(t, "Sewa 28 Okt 2026 – 27 Nov 2026", lines[0].Label)

		w.payNow(t, r.renter, 350_000)

		setNow(env, 2026, 11, 20, 5, 0)
		require.Zero(t, w.generate(t).Invoices, "28 Nov is not within the lead time on 20 Nov")
		setNow(env, 2026, 11, 21, 5, 0)
		require.Equal(t, 1, w.generate(t).Invoices)
		invs = w.invoices(t, r.renter)
		require.Equal(t, []invoiceDates{
			{day(2026, 10, 28), day(2026, 11, 27), day(2026, 10, 28)},
			{day(2026, 11, 28), day(2026, 12, 27), day(2026, 11, 28)},
		}, datesOf(invs))
		require.Equal(t, "open", invs[1].Status, "no credit to pay it")

		setNow(env, 2026, 12, 10, 9, 0)
		bal := w.balance(t, r.renter)
		require.EqualValues(t, 1_050_000, bal.Outstanding, "one total: Rp350.000 left from October plus Rp700.000")
		require.Equal(t, day(2026, 10, 28), *bal.OldestUnpaidDueDate)
		require.Len(t, w.invoices(t, r.renter), 2, "still two separate rows")

		rows := w.payNow(t, r.renter, 1_050_000)
		require.Len(t, rows, 2)
		require.Equal(t, invs[0].InvoiceID, rows[0].InvoiceID, "October's rest is paid first")
		require.EqualValues(t, 350_000, rows[0].Amount)
		require.EqualValues(t, 700_000, rows[1].Amount)
		require.Zero(t, w.balance(t, r.renter).Outstanding)
	})

	t.Run("each generated invoice is audited as a system action, and credit pays it at once", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812802")
		r := w.rent(t, "Dewi Lestari", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		w.payNow(t, r.renter, 1_000_000) // Rp300.000 more than October

		setNow(env, 2026, 11, 8, 5, 0)
		require.Equal(t, 1, w.generate(t).Invoices)
		invs := w.invoices(t, r.renter)
		require.EqualValues(t, 300_000, invs[1].Paid, "the credit paid the new invoice in the same transaction")
		require.Zero(t, w.balance(t, r.renter).Credit)

		var created []dbgen.AuditLog
		for _, a := range env.AuditLog(t, w.org.ID) {
			if a.Action == authz.ActionInvoiceCreate {
				created = append(created, a)
			}
		}
		require.Len(t, created, 2)
		for _, a := range created {
			require.False(t, a.ActorStaffUserID.Valid, "no member made it")
			require.Equal(t, "allow", a.Decision)
			require.JSONEq(t, `{"policy":"system","statement":""}`, string(a.Detail))
		}
		require.Contains(t, created[0].Resource, "/invoice/"+invs[1].InvoiceID.String())
	})

	t.Run("generating twice on the same day creates nothing", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 9, 0)
		w := newWorld(t, env, "0812803")
		r := w.rent(t, "Agus Salim", day(2026, 9, 15), rentOpts{})
		require.Equal(t, 2, w.generate(t).Invoices)
		audits := len(env.AuditLog(t, w.org.ID))
		res := w.generate(t)
		require.Equal(t, billing.Result{Leases: 1}, res)
		require.Len(t, w.invoices(t, r.renter), 2)
		require.Len(t, env.AuditLog(t, w.org.ID), audits)
	})

	t.Run("a booking starting 1 Dec, saved 28 Oct, is first invoiced on 24 Nov", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 9, 0)
		w := newWorld(t, env, "0812804")
		r := w.rent(t, "Siti Nurhaliza", day(2026, 12, 1), rentOpts{})
		require.Zero(t, w.generate(t).Invoices)
		setNow(env, 2026, 11, 23, 23, 59)
		require.Zero(t, w.generate(t).Invoices)
		setNow(env, 2026, 11, 24, 5, 0)
		require.Equal(t, 1, w.generate(t).Invoices)
		require.Equal(t, []invoiceDates{{day(2026, 12, 1), day(2026, 12, 31), day(2026, 12, 1)}}, datesOf(w.invoices(t, r.renter)))
	})

	t.Run("a late lease (15 Sep, entered 28 Oct) gets two invoices, or one billed from 15 Oct", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 9, 0)
		w := newWorld(t, env, "0812805")
		all := w.rent(t, "Bambang Pamungkas", day(2026, 9, 15), rentOpts{})
		skip := w.rent(t, "Wulan Guritno", day(2026, 9, 15), rentOpts{billedFrom: day(2026, 10, 15)})
		require.Equal(t, 3, w.generate(t).Invoices)
		require.Equal(t, []invoiceDates{
			{day(2026, 9, 15), day(2026, 10, 14), day(2026, 10, 28)},
			{day(2026, 10, 15), day(2026, 11, 14), day(2026, 11, 4)},
		}, datesOf(w.invoices(t, all.renter)))
		require.Equal(t, []invoiceDates{
			{day(2026, 10, 15), day(2026, 11, 14), day(2026, 11, 4)},
		}, datesOf(w.invoices(t, skip.renter)))

		setNow(env, 2026, 11, 8, 5, 0)
		require.Equal(t, 2, w.generate(t).Invoices, "then an ordinary lease: 15 Nov, due 15 Nov")
		require.Equal(t, invoiceDates{day(2026, 11, 15), day(2026, 12, 14), day(2026, 11, 15)}, datesOf(w.invoices(t, all.renter))[2])
	})

	t.Run("a 3-month lease bills the term's price and three months of each extra", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812806")
		r := w.rent(t, "Hendra Wijaya", day(2026, 10, 15), rentOpts{term: 3, price: 2_000_000})
		_, err := env.Tenants.SetLeaseCharges(ctx, w.admin, r.leaseID, tenant.LeaseChargesInput{Charges: []tenant.ChargeInput{{ChargeTypeID: w.wifi.ID, Amount: 100_000}}})
		require.NoError(t, err)

		require.Equal(t, 1, w.generate(t).Invoices)
		invs := w.invoices(t, r.renter)
		require.Equal(t, []invoiceDates{{day(2026, 10, 15), day(2027, 1, 14), day(2026, 10, 15)}}, datesOf(invs))
		require.EqualValues(t, 2_300_000, invs[0].Amount)
		lines := w.lines(t, invs[0].InvoiceID)
		require.Len(t, lines, 2)
		require.Equal(t, "Sewa 15 Okt 2026 – 14 Jan 2027", lines[0].Label)
		require.EqualValues(t, 2_000_000, lines[0].Amount)
		require.Equal(t, "Wi-Fi (3 bulan)", lines[1].Label)
		require.EqualValues(t, 300_000, lines[1].Amount)
		require.Equal(t, uuid.NullUUID{UUID: w.wifi.ID, Valid: true}, lines[1].ChargeTypeID)

		setNow(env, 2027, 1, 7, 23, 0)
		require.Zero(t, w.generate(t).Invoices)
		setNow(env, 2027, 1, 8, 5, 0)
		require.Equal(t, 1, w.generate(t).Invoices)
		require.Equal(t, invoiceDates{day(2027, 1, 15), day(2027, 4, 14), day(2027, 1, 15)}, datesOf(w.invoices(t, r.renter))[1])
	})

	t.Run("a lease starting 31 Jan is due on the last day of February, then 31 Mar", func(t *testing.T) {
		setNow(env, 2027, 1, 31, 9, 0)
		w := newWorld(t, env, "0812807")
		r := w.rent(t, "Yanti Kusuma", day(2027, 1, 31), rentOpts{})
		w.generate(t)
		setNow(env, 2027, 2, 21, 5, 0)
		w.generate(t)
		setNow(env, 2027, 3, 24, 5, 0)
		w.generate(t)
		require.Equal(t, []invoiceDates{
			{day(2027, 1, 31), day(2027, 2, 27), day(2027, 1, 31)},
			{day(2027, 2, 28), day(2027, 3, 30), day(2027, 2, 28)},
			{day(2027, 3, 31), day(2027, 4, 29), day(2027, 3, 31)},
		}, datesOf(w.invoices(t, r.renter)))
	})

	t.Run("the lead time turns at midnight in Jakarta, not in UTC", func(t *testing.T) {
		setNow(env, 2026, 11, 1, 9, 0)
		w := newWorld(t, env, "0812808")
		r := w.rent(t, "Rudi Hartono", day(2026, 11, 15), rentOpts{})

		setNow(env, 2026, 11, 7, 23, 59) // 16:59 UTC on 7 Nov
		require.Zero(t, w.generate(t).Invoices)
		setNow(env, 2026, 11, 8, 0, 0) // still 7 Nov in UTC (17:00)
		require.Equal(t, "2026-11-07", env.Clock.Now().UTC().Format(time.DateOnly))
		require.Equal(t, 1, w.generate(t).Invoices)

		// The invoice was made on 8 Nov in Jakarta, so a price for that day or earlier would
		// contradict it; 9 Nov is the first day a new price may take effect.
		_, err := env.Properties.CreateUnitPrice(ctx, w.admin, r.unitID, property.PriceInput{Months: 1, Amount: 750_000, EffectiveFrom: day(2026, 11, 8)})
		var early *property.InvoicedPriceError
		require.ErrorAs(t, err, &early)
		require.ErrorIs(t, err, property.ErrPriceTooEarly)
		require.Equal(t, day(2026, 11, 8), early.InvoicedOn)
		require.Equal(t, "effectiveFrom", early.Field)

		_, err = env.Properties.UpdateUnit(ctx, w.admin, r.unitID, property.UnitInput{
			Label: "Kamar 1", Terms: testenv.Monthly(750_000),
		})
		require.ErrorAs(t, err, &early, "updateUnit records a price for today, which was invoiced")
		require.Equal(t, "terms", early.Field)

		_, err = env.Properties.CreateUnitPrice(ctx, w.admin, r.unitID, property.PriceInput{Months: 1, Amount: 750_000, EffectiveFrom: day(2026, 11, 9)})
		require.NoError(t, err)
	})

	t.Run("a month-end lease is generated when its shortened due date enters the lead time", func(t *testing.T) {
		setNow(env, 2026, 10, 31, 9, 0)
		w := newWorld(t, env, "0812809")
		r := w.rent(t, "Lina Marlina", day(2026, 10, 31), rentOpts{})
		w.generate(t)
		setNow(env, 2026, 11, 22, 23, 59)
		require.Zero(t, w.generate(t).Invoices)
		setNow(env, 2026, 11, 23, 0, 0)
		require.Equal(t, 1, w.generate(t).Invoices)
		require.Equal(t, invoiceDates{day(2026, 11, 30), day(2026, 12, 30), day(2026, 11, 30)}, datesOf(w.invoices(t, r.renter))[1])
	})

	t.Run("a lease with no price in force is skipped and logged, the others are invoiced", func(t *testing.T) {
		setNow(env, 2026, 11, 10, 9, 0)
		w := newWorld(t, env, "0812810")
		priced := w.rent(t, "Tono Sudirjo", day(2026, 11, 10), rentOpts{})
		// The unit's only price takes effect on 12 Nov; looking from 10 Nov nothing is in force.
		setNow(env, 2026, 11, 12, 9, 0)
		unpriced := w.rent(t, "Yudi Prasetyo", day(2026, 11, 12), rentOpts{})
		setNow(env, 2026, 11, 10, 12, 0)

		var logs bytes.Buffer
		svc := billing.NewService(env.Store, env.Authz, env.Clock, slog.New(slog.NewJSONHandler(&logs, nil)))
		res, err := svc.GenerateOrg(ctx, w.org.ID)
		require.NoError(t, err)
		require.Equal(t, billing.Result{Leases: 2, Invoices: 1, Skipped: 1}, res)
		require.Len(t, w.invoices(t, priced.renter), 1)
		require.Empty(t, w.invoices(t, unpriced.renter))
		require.Contains(t, logs.String(), `"level":"ERROR"`)
		require.Contains(t, logs.String(), unpriced.leaseID.String())
	})

	t.Run("an organization's task touches only its own leases", func(t *testing.T) {
		setNow(env, 2026, 11, 2, 9, 0)
		a := newWorld(t, env, "0812811")
		b := newWorld(t, env, "0812812")
		ra := a.rent(t, "Ani Yudhoyono", day(2026, 11, 2), rentOpts{})
		rb := b.rent(t, "Bayu Saputra", day(2026, 11, 2), rentOpts{})
		require.Equal(t, billing.Result{Leases: 1, Invoices: 1}, a.generate(t))
		require.Len(t, a.invoices(t, ra.renter), 1)
		require.Empty(t, b.invoices(t, rb.renter))
	})

	t.Run("the daily run enqueues one task per organization, named after the day", func(t *testing.T) {
		ids, err := env.Store.ActiveOrganizationIDs(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, ids)
		fake := &invoicetasktest.Fake{}
		n, err := invoicetask.EnqueueDaily(ctx, env.Store, fake, day(2026, 11, 24))
		require.NoError(t, err)
		require.Equal(t, len(ids), n)
		for i, task := range fake.Tasks() {
			require.Equal(t, invoicetask.Task{ID: invoicetask.DailyTaskID(ids[i], day(2026, 11, 24)), OrganizationID: ids[i]}, task)
		}
	})
}

func TestLeaseRulesOnceInvoiced(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	t.Run("the start date can change until the first invoice, then not", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 9, 0)
		w := newWorld(t, env, "0812821")
		r := w.rent(t, "Rina Wulandari", day(2026, 12, 1), rentOpts{deposit: 500_000})

		_, err := env.Tenants.UpdateLease(ctx, w.admin, r.leaseID, tenant.Terms{StartDate: day(2026, 12, 2), BilledFrom: day(2026, 12, 2)})
		require.NoError(t, err, "no invoice yet")

		setNow(env, 2026, 11, 25, 5, 0)
		require.Equal(t, 1, w.generate(t).Invoices)

		_, err = env.Tenants.UpdateLease(ctx, w.admin, r.leaseID, tenant.Terms{StartDate: day(2026, 12, 3), BilledFrom: day(2026, 12, 3)})
		var invoiced *tenant.InvoicedError
		require.ErrorAs(t, err, &invoiced)
		require.ErrorIs(t, err, tenant.ErrLeaseHasInvoices)
		require.Equal(t, "startDate", invoiced.Field)

		_, err = env.Tenants.UpdateLease(ctx, w.admin, r.leaseID, tenant.Terms{StartDate: day(2026, 12, 2), BilledFrom: day(2027, 1, 2)})
		require.ErrorAs(t, err, &invoiced)
		require.Equal(t, "billedFrom", invoiced.Field)

		l, err := env.Tenants.UpdateLease(ctx, w.admin, r.leaseID, tenant.Terms{StartDate: day(2026, 12, 2), DepositAmount: 600_000})
		require.NoError(t, err, "the other terms stay editable")
		require.EqualValues(t, 600_000, l.DepositAmount)
		require.Equal(t, day(2026, 12, 2), l.StartDate)
	})

	t.Run("moving the start date to today enqueues generation at once, under its own task", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 9, 0)
		w := newWorld(t, env, "0812842")
		r := w.rent(t, "Dewi Lestari", day(2026, 12, 1), rentOpts{})
		require.Zero(t, w.generate(t).Invoices, "booked further ahead than the lead time")

		l, err := env.Tenants.UpdateLease(ctx, w.admin, r.leaseID, tenant.Terms{StartDate: day(2026, 10, 28), BilledFrom: day(2026, 10, 28)})
		require.NoError(t, err)
		datesTask := invoicetask.Task{
			ID:             invoicetask.LeaseDatesTaskID(w.org.ID, r.leaseID, l.StartDate, l.BilledFrom),
			OrganizationID: w.org.ID,
		}
		require.Contains(t, env.Queue.Tasks(), datesTask)
		require.Equal(t, 1, w.generate(t).Invoices, "the task the update enqueued makes the invoice due today")

		before := len(env.Queue.Tasks())
		_, err = env.Tenants.UpdateLease(ctx, w.admin, r.leaseID, tenant.Terms{StartDate: day(2026, 10, 28), DepositAmount: 300_000})
		require.NoError(t, err)
		require.Len(t, env.Queue.Tasks(), before, "an update that keeps the dates enqueues nothing")
	})

	t.Run("planned end, the user's example: the 15 Nov invoice is cancelled, the October debt stays", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812822")
		r := w.rent(t, "Dewi Lestari", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		w.payNow(t, r.renter, 350_000)

		setNow(env, 2026, 11, 8, 5, 0)
		require.Equal(t, 1, w.generate(t).Invoices)

		setNow(env, 2026, 11, 10, 9, 0)
		_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, r.leaseID, ptr(day(2026, 11, 14)))
		require.NoError(t, err)
		invs := w.invoices(t, r.renter)
		require.Equal(t, "cancelled", invs[1].Status)
		require.Zero(t, invs[1].Outstanding, "nothing outstanding, so its reminders stop")
		require.Equal(t, []correctionRow{{Delta: -700_000, Reason: billing.ReasonNotRenewed, CreatedBy: w.org.Admin.UserID}},
			w.corrections(t, invs[1].InvoiceID))
		require.EqualValues(t, 350_000, invs[0].Outstanding, "the October debt stays")

		var audited bool
		for _, a := range env.AuditLog(t, w.org.ID) {
			if a.Action == authz.ActionCorrectionCreate && a.Decision == "allow" &&
				a.ActorStaffUserID.UUID == w.org.Admin.UserID && a.Resource == "property/"+w.kos.ID.String()+"/invoice/"+invs[1].InvoiceID.String() {
				audited = true
			}
		}
		require.True(t, audited, "the correction is audited with the admin as actor")

		rows := w.payNow(t, r.renter, 350_000)
		require.Equal(t, []billing.Row{{PaymentID: rows[0].PaymentID, InvoiceID: invs[0].InvoiceID, Amount: 350_000}}, rows)

		setNow(env, 2026, 12, 8, 5, 0)
		require.Zero(t, w.generate(t).Invoices, "no invoice for 15 Dec")
		require.Len(t, w.invoices(t, r.renter), 2)
	})

	t.Run("planned end on a partly paid invoice turns the payment into credit, and clearing it restores", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812823")
		r := w.rent(t, "Agus Salim", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		w.payNow(t, r.renter, 700_000)
		setNow(env, 2026, 11, 8, 5, 0)
		w.generate(t)
		w.payNow(t, r.renter, 200_000)

		setNow(env, 2026, 11, 10, 9, 0)
		_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, r.leaseID, ptr(day(2026, 11, 14)))
		require.NoError(t, err)
		nov := w.invoices(t, r.renter)[1]
		require.Equal(t, "cancelled", nov.Status)
		require.Zero(t, nov.Paid, "the allocation is reversed")
		bal := w.balance(t, r.renter)
		require.EqualValues(t, 200_000, bal.Credit)
		require.Zero(t, bal.Outstanding)

		// Moving it later restores the period; moving it back cancels it again.
		_, err = env.Tenants.SetPlannedEnd(ctx, w.admin, r.leaseID, ptr(day(2026, 12, 14)))
		require.NoError(t, err)
		nov = w.invoices(t, r.renter)[1]
		require.EqualValues(t, 700_000, nov.Billed)
		require.EqualValues(t, 200_000, nov.Paid, "the credit pays it again")
		_, err = env.Tenants.SetPlannedEnd(ctx, w.admin, r.leaseID, ptr(day(2026, 11, 14)))
		require.NoError(t, err)
		require.Equal(t, "cancelled", w.invoices(t, r.renter)[1].Status)

		// Clearing it restores the invoice with a reversing correction; its reminders resume.
		_, err = env.Tenants.SetPlannedEnd(ctx, w.admin, r.leaseID, nil)
		require.NoError(t, err)
		nov = w.invoices(t, r.renter)[1]
		require.Equal(t, "partial", nov.Status)
		require.EqualValues(t, 500_000, nov.Outstanding)
		require.Equal(t, []correctionRow{
			{Delta: -700_000, Reason: billing.ReasonNotRenewed, CreatedBy: w.org.Admin.UserID},
			{Delta: 700_000, Reason: billing.ReasonPlannedEndChanged, CreatedBy: w.org.Admin.UserID},
			{Delta: -700_000, Reason: billing.ReasonNotRenewed, CreatedBy: w.org.Admin.UserID},
			{Delta: 700_000, Reason: billing.ReasonPlannedEndChanged, CreatedBy: w.org.Admin.UserID},
		}, w.corrections(t, nov.InvoiceID))
		require.Zero(t, w.balance(t, r.renter).Credit)
	})

	t.Run("a member without correction:create cannot set a planned end that cancels an invoice", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812824")
		r := w.rent(t, "Siti Nurhaliza", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		setNow(env, 2026, 11, 8, 5, 0)
		w.generate(t)

		karyawan := env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
		env.GrantCustomRole(t, w.org.ID, karyawan.MembershipID, "Pengelola sewa",
			`{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["lease:manage","tenant:read"]}]}`)
		leaseOnly := env.Principal(t, w.org.ID, w.org.Karyawan.UserID)

		setNow(env, 2026, 11, 10, 9, 0)
		_, err := env.Tenants.SetPlannedEnd(ctx, leaseOnly, r.leaseID, ptr(day(2026, 11, 14)))
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Nil(t, w.lease(t, r.renter).PlannedEndDate, "nothing changed")
		require.Equal(t, "open", w.invoices(t, r.renter)[1].Status)
		require.Empty(t, w.corrections(t, w.invoices(t, r.renter)[1].InvoiceID))
		denial := env.AuditLog(t, w.org.ID)[0]
		require.Equal(t, authz.ActionCorrectionCreate, denial.Action)
		require.Equal(t, "deny", denial.Decision)
		require.Equal(t, w.org.Karyawan.UserID, denial.ActorStaffUserID.UUID)

		_, err = env.Tenants.SetPlannedEnd(ctx, leaseOnly, r.leaseID, ptr(day(2026, 12, 14)))
		require.NoError(t, err, "a date that cancels nothing needs only lease:manage")
	})

	t.Run("move-out without debt cancels the next period and touches no deposit", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812825")
		r := w.rent(t, "Hendra Wijaya", day(2026, 10, 15), rentOpts{deposit: 500_000})
		w.generate(t)
		w.payNow(t, r.renter, 700_000)
		setNow(env, 2026, 11, 8, 5, 0)
		w.generate(t)

		setNow(env, 2026, 11, 10, 9, 0)
		l, err := env.Tenants.MoveOut(ctx, w.admin, r.leaseID, day(2026, 11, 10))
		require.NoError(t, err)
		require.Equal(t, tenant.StatusEnded, l.Status)
		invs := w.invoices(t, r.renter)
		require.Equal(t, "paid", invs[0].Status)
		require.Equal(t, "cancelled", invs[1].Status)
		require.Equal(t, []correctionRow{{Delta: -700_000, Reason: billing.ReasonNotRenewed, CreatedBy: w.org.Admin.UserID}},
			w.corrections(t, invs[1].InvoiceID))
		require.Empty(t, w.corrections(t, invs[0].InvoiceID), "no debt, no deposit taken")
		require.Zero(t, w.balance(t, r.renter).Outstanding)

		setNow(env, 2026, 12, 8, 5, 0)
		require.Zero(t, w.generate(t).Invoices, "an ended lease is never invoiced again")
	})

	t.Run("move-out after the planned end restores the started period it had cancelled", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812840")
		r := w.rent(t, "Rina Marlina", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		w.payNow(t, r.renter, 700_000)
		setNow(env, 2026, 11, 8, 5, 0)
		w.generate(t)
		setNow(env, 2026, 11, 10, 9, 0)
		_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, r.leaseID, ptr(day(2026, 11, 14)))
		require.NoError(t, err)
		require.Equal(t, "cancelled", w.invoices(t, r.renter)[1].Status)

		// The tenant stays until 20 Nov: the 15 Nov period started, so it is billed in full.
		setNow(env, 2026, 11, 20, 9, 0)
		_, err = env.Tenants.MoveOut(ctx, w.admin, r.leaseID, day(2026, 11, 20))
		require.NoError(t, err)
		invs := w.invoices(t, r.renter)
		require.Len(t, invs, 2)
		require.Equal(t, "open", invs[1].Status)
		require.EqualValues(t, 700_000, invs[1].Billed)
		require.Equal(t, []correctionRow{
			{Delta: -700_000, Reason: billing.ReasonNotRenewed, CreatedBy: w.org.Admin.UserID},
			{Delta: 700_000, Reason: billing.ReasonPlannedEndChanged, CreatedBy: w.org.Admin.UserID},
		}, w.corrections(t, invs[1].InvoiceID))
		require.EqualValues(t, 700_000, w.balance(t, r.renter).Outstanding)
	})

	t.Run("move-out after the planned end bills a started period that was never invoiced", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812841")
		r := w.rent(t, "Agus Salim", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		w.payNow(t, r.renter, 700_000)
		setNow(env, 2026, 11, 1, 9, 0)
		_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, r.leaseID, ptr(day(2026, 11, 14)))
		require.NoError(t, err)
		setNow(env, 2026, 11, 8, 5, 0)
		require.Zero(t, w.generate(t).Invoices, "no invoice for a period after the planned end")

		setNow(env, 2026, 11, 20, 9, 0)
		_, err = env.Tenants.MoveOut(ctx, w.admin, r.leaseID, day(2026, 11, 20))
		require.NoError(t, err)
		invs := w.invoices(t, r.renter)
		require.Len(t, invs, 2)
		require.Equal(t, day(2026, 11, 15), invs[1].Period)
		require.Equal(t, "open", invs[1].Status)
		require.EqualValues(t, 700_000, w.balance(t, r.renter).Outstanding)
	})

	t.Run("move-out with a deposit smaller than the debt pays part of it", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812826")
		r := w.rent(t, "Wulan Guritno", day(2026, 10, 15), rentOpts{deposit: 200_000})
		// The generate task has not run: the move-out bills the started period itself.
		setNow(env, 2026, 10, 20, 9, 0)
		_, err := env.Tenants.MoveOut(ctx, w.admin, r.leaseID, day(2026, 10, 20))
		require.NoError(t, err)
		invs := w.invoices(t, r.renter)
		require.Len(t, invs, 1)
		require.Equal(t, day(2026, 10, 15), invs[0].Period)
		require.Equal(t, []correctionRow{{Delta: -200_000, Reason: billing.ReasonDeposit, CreatedBy: w.org.Admin.UserID}},
			w.corrections(t, invs[0].InvoiceID))
		require.EqualValues(t, 500_000, w.balance(t, r.renter).Outstanding, "the rest stays owed")
	})

	t.Run("move-out with a deposit larger than the debt pays all of it, oldest first", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 9, 0)
		w := newWorld(t, env, "0812827")
		r := w.rent(t, "Bambang Pamungkas", day(2026, 9, 15), rentOpts{deposit: 2_000_000})
		w.generate(t)
		w.payNow(t, r.renter, 350_000)

		_, err := env.Tenants.MoveOut(ctx, w.admin, r.leaseID, day(2026, 10, 28))
		require.NoError(t, err)
		invs := w.invoices(t, r.renter)
		require.Equal(t, []correctionRow{{Delta: -350_000, Reason: billing.ReasonDeposit, CreatedBy: w.org.Admin.UserID}},
			w.corrections(t, invs[0].InvoiceID))
		require.Equal(t, []correctionRow{{Delta: -700_000, Reason: billing.ReasonDeposit, CreatedBy: w.org.Admin.UserID}},
			w.corrections(t, invs[1].InvoiceID))
		bal := w.balance(t, r.renter)
		require.Zero(t, bal.Outstanding)
		require.Zero(t, bal.Credit)
	})

	t.Run("a cancelled booking is never invoiced, even when its invoice already exists", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 9, 0)
		w := newWorld(t, env, "0812828")
		soon := w.rent(t, "Rudi Hartono", day(2026, 11, 1), rentOpts{deposit: 300_000})
		later := w.rent(t, "Lina Marlina", day(2026, 12, 1), rentOpts{})
		require.Equal(t, 1, w.generate(t).Invoices, "1 Nov is within the lead time")

		setNow(env, 2026, 10, 30, 9, 0)
		_, err := env.Tenants.MoveOut(ctx, w.admin, soon.leaseID, day(2026, 11, 1))
		require.NoError(t, err)
		_, err = env.Tenants.MoveOut(ctx, w.admin, later.leaseID, day(2026, 12, 1))
		require.NoError(t, err)

		invs := w.invoices(t, soon.renter)
		require.Equal(t, "cancelled", invs[0].Status)
		require.Equal(t, []correctionRow{{Delta: -700_000, Reason: billing.ReasonBookingCancelled, CreatedBy: w.org.Admin.UserID}},
			w.corrections(t, invs[0].InvoiceID), "and the deposit pays nothing")

		setNow(env, 2026, 11, 24, 5, 0)
		require.Zero(t, w.generate(t).Invoices)
		require.Empty(t, w.invoices(t, later.renter))
	})
}
