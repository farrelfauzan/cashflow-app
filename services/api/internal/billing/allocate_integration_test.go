//go:build integration

package billing_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// at is a moment on a day in Jakarta, when a payment is received.
func at(y int, m time.Month, d, hour int) time.Time {
	return time.Date(y, m, d, hour, 0, 0, 0, clock.Jakarta)
}

// world is one organization with an admin, a kos and a Wi-Fi charge type.
type world struct {
	env    *testenv.Env
	org    testenv.Org
	admin  authz.Principal
	kos    property.Property
	wifi   tenant.ChargeType
	prefix string
	n      int
}

// renter is a tenant with one monthly lease on its own room, started 28 Oct 2026.
type renter struct {
	tenantID uuid.UUID
	leaseID  uuid.UUID
}

func newWorld(t *testing.T, env *testenv.Env, prefix string) *world {
	t.Helper()
	ctx := context.Background()
	w := &world{env: env, org: env.CreateOrg(t, "Griya Sejahtera "+prefix, prefix), prefix: prefix}
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	var err error
	w.kos, err = env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	w.wifi, err = env.Tenants.CreateChargeType(ctx, w.admin, tenant.ChargeTypeInput{Name: "Wi-Fi"})
	require.NoError(t, err)
	return w
}

func (w *world) newRenter(t *testing.T, name string) renter {
	t.Helper()
	ctx := context.Background()
	w.n++
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{
		Label: fmt.Sprintf("Kamar %d", w.n), Terms: testenv.Monthly(700_000),
	})
	require.NoError(t, err)
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	l, err := w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: day(2026, 10, 28)},
	})
	require.NoError(t, err)
	return renter{tenantID: tn.ID, leaseID: l.ID}
}

// inTx runs fn in the world's organization scope.
func (w *world) inTx(fn func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error) error {
	ctx := context.Background()
	return w.env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error { return fn(ctx, tx, dbgen.New(tx)) })
}

func (w *world) mustTx(t *testing.T, fn func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error) {
	t.Helper()
	require.NoError(t, w.inTx(fn))
}

// insertInvoice inserts a monthly invoice for the period starting on from, due that day, with
// a rent line and one Wi-Fi line per extra amount; its amount is the sum.
func (w *world) insertInvoice(ctx context.Context, q *dbgen.Queries, r renter, from time.Time, rent int64, extras ...int64) (uuid.UUID, error) {
	total := rent
	for _, e := range extras {
		total += e
	}
	inv, err := w.insertBareInvoice(ctx, q, r, from, total)
	if err != nil {
		return uuid.Nil, err
	}
	if err := w.insertLine(ctx, q, inv, "rent", "Sewa", rent); err != nil {
		return uuid.Nil, err
	}
	for _, e := range extras {
		if err := w.insertLine(ctx, q, inv, "charge", "Wi-Fi", e); err != nil {
			return uuid.Nil, err
		}
	}
	return inv, nil
}

func (w *world) insertBareInvoice(ctx context.Context, q *dbgen.Queries, r renter, from time.Time, amount int64) (uuid.UUID, error) {
	inv, err := q.CreateInvoice(ctx, dbgen.CreateInvoiceParams{
		ID: uuid.New(), OrganizationID: w.org.ID, LeaseID: r.leaseID, TenantID: r.tenantID,
		Period: from, PeriodEnd: from.AddDate(0, 1, -1), DueDate: from, Amount: amount,
	})
	return inv.ID, err
}

func (w *world) insertLine(ctx context.Context, q *dbgen.Queries, invoiceID uuid.UUID, kind, label string, amount int64) error {
	var ct uuid.NullUUID
	if kind == "charge" {
		ct = uuid.NullUUID{UUID: w.wifi.ID, Valid: true}
	}
	_, err := q.CreateInvoiceLine(ctx, dbgen.CreateInvoiceLineParams{
		ID: uuid.New(), OrganizationID: w.org.ID, InvoiceID: invoiceID, Kind: kind, ChargeTypeID: ct, Label: label, Amount: amount,
	})
	return err
}

// insertPayment records a cash payment taken by the admin.
func (w *world) insertPayment(ctx context.Context, q *dbgen.Queries, r renter, amount int64, received time.Time) (uuid.UUID, error) {
	p, err := q.CreatePayment(ctx, dbgen.CreatePaymentParams{
		ID: uuid.New(), OrganizationID: w.org.ID, TenantID: r.tenantID, Amount: amount, Method: "cash",
		ReceivedAt: received, Source: "manual",
		RecordedByStaffUserID: uuid.NullUUID{UUID: w.org.Admin.UserID, Valid: true},
	})
	return p.ID, err
}

func (w *world) insertCorrection(ctx context.Context, q *dbgen.Queries, invoiceID, paymentID uuid.UUID, delta int64, reason string) error {
	_, err := q.CreateCorrection(ctx, dbgen.CreateCorrectionParams{
		ID: uuid.New(), OrganizationID: w.org.ID,
		InvoiceID:   uuid.NullUUID{UUID: invoiceID, Valid: invoiceID != uuid.Nil},
		PaymentID:   uuid.NullUUID{UUID: paymentID, Valid: paymentID != uuid.Nil},
		AmountDelta: delta, Reason: reason, CreatedByStaffUserID: w.org.Admin.UserID,
	})
	return err
}

func (w *world) allocate(t *testing.T, r renter) []billing.Row {
	t.Helper()
	var rows []billing.Row
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		rows, err = billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
		return err
	})
	return rows
}

func (w *world) invoices(t *testing.T, r renter) []dbgen.InvoiceBalance {
	t.Helper()
	var out []dbgen.InvoiceBalance
	w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
		var err error
		out, err = q.ListInvoiceBalancesByTenant(ctx, dbgen.ListInvoiceBalancesByTenantParams{OrganizationID: w.org.ID, TenantID: r.tenantID})
		return err
	})
	return out
}

func (w *world) balance(t *testing.T, r renter) dbgen.TenantBalance {
	t.Helper()
	var out dbgen.TenantBalance
	w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
		var err error
		out, err = q.GetTenantBalance(ctx, dbgen.GetTenantBalanceParams{OrganizationID: w.org.ID, TenantID: r.tenantID})
		return err
	})
	return out
}

// requireCode asserts err is a Postgres error with the given SQLSTATE.
func requireCode(t *testing.T, err error, code string) *pgconn.PgError {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error %s, got %v", code, err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
	return pgErr
}

// summary is what one invoice looks like after allocation, for comparing two runs.
type summary struct {
	Period         time.Time
	Billed, Paid   int64
	Status         string
	PaidOverBilled bool
}

func summarize(rows []dbgen.InvoiceBalance) []summary {
	out := make([]summary, len(rows))
	for i, r := range rows {
		out[i] = summary{Period: r.Period, Billed: r.Billed, Paid: r.Paid, Status: r.Status, PaidOverBilled: r.Paid > r.Billed}
	}
	return out
}

func TestAllocate(t *testing.T) {
	env := testenv.New(t)
	w := newWorld(t, env, "0812771")

	t.Run("the user's example: a partial payment, the next month, then one payment for both", func(t *testing.T) {
		r := w.newRenter(t, "Rina Wulandari")
		var oct, nov uuid.UUID

		// 28 Oct: the lease is saved and the 28 Oct to 27 Nov invoice is due that day. The
		// tenant pays half.
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			var err error
			if oct, err = w.insertInvoice(ctx, q, r, day(2026, 10, 28), 700_000); err != nil {
				return err
			}
			_, err = billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
			return err
		})
		var first uuid.UUID
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			var err error
			if first, err = w.insertPayment(ctx, q, r, 350_000, at(2026, 10, 28, 14)); err != nil {
				return err
			}
			rows, err := billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
			require.Equal(t, []billing.Row{{PaymentID: first, InvoiceID: oct, Amount: 350_000}}, rows)
			return err
		})

		// 21 Nov: the 28 Nov to 27 Dec invoice is generated, due 28 Nov; no credit pays it.
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			var err error
			if nov, err = w.insertInvoice(ctx, q, r, day(2026, 11, 28), 700_000); err != nil {
				return err
			}
			rows, err := billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
			require.Empty(t, rows)
			return err
		})

		// 10 Dec: one total of Rp1.050.000, oldest unpaid due 28 Oct, two separate invoices.
		b := w.balance(t, r)
		require.Equal(t, int64(1_050_000), b.Outstanding)
		require.NotNil(t, b.OldestUnpaidDueDate)
		require.Equal(t, day(2026, 10, 28), *b.OldestUnpaidDueDate)
		require.Zero(t, b.Credit)
		require.Equal(t, []summary{
			{Period: day(2026, 10, 28), Billed: 700_000, Paid: 350_000, Status: "partial"},
			{Period: day(2026, 11, 28), Billed: 700_000, Paid: 0, Status: "open"},
		}, summarize(w.invoices(t, r)))

		// A payment of Rp1.050.000 pays the October rest first, then November.
		var second uuid.UUID
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			var err error
			if second, err = w.insertPayment(ctx, q, r, 1_050_000, at(2026, 12, 10, 9)); err != nil {
				return err
			}
			rows, err := billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
			require.Equal(t, []billing.Row{
				{PaymentID: second, InvoiceID: oct, Amount: 350_000},
				{PaymentID: second, InvoiceID: nov, Amount: 700_000},
			}, rows)
			return err
		})
		inv := w.invoices(t, r)
		require.Equal(t, "paid", inv[0].Status)
		require.Equal(t, "paid", inv[1].Status)
		b = w.balance(t, r)
		require.Zero(t, b.Outstanding)
		require.Nil(t, b.OldestUnpaidDueDate)
		require.Zero(t, b.Credit)

		t.Run("rerunning allocation changes nothing", func(t *testing.T) {
			require.Empty(t, w.allocate(t, r))
			require.Empty(t, w.allocate(t, r))
			require.Equal(t, summarize(inv), summarize(w.invoices(t, r)))
		})
	})

	t.Run("an overpayment becomes credit and pays the next invoice the moment it exists", func(t *testing.T) {
		r := w.newRenter(t, "Budi Santoso")
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			if _, err := w.insertInvoice(ctx, q, r, day(2026, 10, 28), 650_000, 50_000); err != nil {
				return err
			}
			if _, err := w.insertPayment(ctx, q, r, 1_000_000, at(2026, 10, 28, 10)); err != nil {
				return err
			}
			_, err := billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
			return err
		})
		b := w.balance(t, r)
		require.Zero(t, b.Outstanding)
		require.Equal(t, int64(300_000), b.Credit)

		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			if _, err := w.insertInvoice(ctx, q, r, day(2026, 11, 28), 650_000, 50_000); err != nil {
				return err
			}
			rows, err := billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
			require.Len(t, rows, 1)
			require.Equal(t, int64(300_000), rows[0].Amount)
			return err
		})
		b = w.balance(t, r)
		require.Equal(t, int64(400_000), b.Outstanding)
		require.Equal(t, day(2026, 11, 28), *b.OldestUnpaidDueDate)
		require.Zero(t, b.Credit)
		require.Equal(t, "partial", w.invoices(t, r)[1].Status)
	})

	t.Run("an invoice cancelled after it was partly paid gives its money back as credit", func(t *testing.T) {
		// The planned-end case: Rp200.000 paid toward the 15 Nov invoice, then it is cancelled.
		r := w.newRenter(t, "Dewi Lestari")
		var nov uuid.UUID
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			var err error
			if nov, err = w.insertInvoice(ctx, q, r, day(2026, 11, 15), 700_000); err != nil {
				return err
			}
			if _, err := w.insertPayment(ctx, q, r, 200_000, at(2026, 11, 9, 10)); err != nil {
				return err
			}
			_, err = billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
			return err
		})
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			if err := w.insertCorrection(ctx, q, nov, uuid.Nil, -700_000, "sewa tidak diperpanjang"); err != nil {
				return err
			}
			rows, err := billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
			require.Len(t, rows, 1)
			require.Equal(t, int64(-200_000), rows[0].Amount)
			return err
		})
		inv := w.invoices(t, r)
		require.Equal(t, "cancelled", inv[0].Status)
		require.Zero(t, inv[0].Billed)
		require.Zero(t, inv[0].Paid)
		b := w.balance(t, r)
		require.Zero(t, b.Outstanding)
		require.Nil(t, b.OldestUnpaidDueDate)
		require.Equal(t, int64(200_000), b.Credit)
		require.Empty(t, w.allocate(t, r))
	})

	t.Run("a payment corrected down takes back from the newest invoice first", func(t *testing.T) {
		r := w.newRenter(t, "Agus Setiawan")
		var pay uuid.UUID
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			if _, err := w.insertInvoice(ctx, q, r, day(2026, 10, 28), 700_000); err != nil {
				return err
			}
			if _, err := w.insertInvoice(ctx, q, r, day(2026, 11, 28), 700_000); err != nil {
				return err
			}
			var err error
			if pay, err = w.insertPayment(ctx, q, r, 1_400_000, at(2026, 11, 28, 10)); err != nil {
				return err
			}
			_, err = billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
			return err
		})
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			if err := w.insertCorrection(ctx, q, uuid.Nil, pay, -1_000_000, "salah ketik nominal"); err != nil {
				return err
			}
			_, err := billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
			return err
		})
		require.Equal(t, []summary{
			{Period: day(2026, 10, 28), Billed: 700_000, Paid: 400_000, Status: "partial"},
			{Period: day(2026, 11, 28), Billed: 700_000, Paid: 0, Status: "open"},
		}, summarize(w.invoices(t, r)))
		b := w.balance(t, r)
		require.Equal(t, int64(1_000_000), b.Outstanding)
		require.Zero(t, b.Credit)
	})

	t.Run("a tenant that is not in the organization is not found", func(t *testing.T) {
		other := newWorld(t, env, "0812772")
		stranger := other.newRenter(t, "Sari Indah")
		for name, id := range map[string]uuid.UUID{"unknown": uuid.New(), "another organization's": stranger.tenantID} {
			t.Run(name, func(t *testing.T) {
				err := w.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
					_, err := billing.Allocate(ctx, tx, w.org.ID, id)
					return err
				})
				require.ErrorIs(t, err, billing.ErrTenantNotFound)
			})
		}
	})

	t.Run("two payments allocated at the same time never over-pay", func(t *testing.T) {
		// Two invoices of Rp700.000 and two payments of Rp1.000.000: whatever the order, both
		// invoices are paid exactly once and Rp600.000 is credit.
		setup := func(t *testing.T, name string) renter {
			t.Helper()
			r := w.newRenter(t, name)
			w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
				if _, err := w.insertInvoice(ctx, q, r, day(2026, 10, 28), 700_000); err != nil {
					return err
				}
				_, err := w.insertInvoice(ctx, q, r, day(2026, 11, 28), 700_000)
				return err
			})
			return r
		}
		payAndAllocate := func(r renter, hour int) func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			return func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
				if _, err := w.insertPayment(ctx, q, r, 1_000_000, at(2026, 11, 28, hour)); err != nil {
					return err
				}
				_, err := billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
				return err
			}
		}

		sequential := setup(t, "Hendra Wijaya")
		w.mustTx(t, payAndAllocate(sequential, 9))
		w.mustTx(t, payAndAllocate(sequential, 10))
		want := summarize(w.invoices(t, sequential))
		wantBalance := w.balance(t, sequential)
		require.Equal(t, int64(600_000), wantBalance.Credit)

		t.Run("the second waits for the first's lock", func(t *testing.T) {
			r := setup(t, "Joko Susilo")
			done := make(chan error, 1)
			w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
				if err := payAndAllocate(r, 9)(ctx, tx, q); err != nil {
					return err
				}
				go func() { done <- w.inTx(payAndAllocate(r, 10)) }()
				// The second transaction inserts its payment, then waits on the tenant lock.
				testenv.WaitUntilBlockedOn(ctx, t, tx)
				return nil
			})
			require.NoError(t, <-done)
			require.Equal(t, want, summarize(w.invoices(t, r)))
			require.Equal(t, wantBalance.Credit, w.balance(t, r).Credit)
		})

		t.Run("both start together", func(t *testing.T) {
			for i := range 5 {
				r := setup(t, fmt.Sprintf("Kurnia Putri %d", i))
				start := make(chan struct{})
				errs := make([]error, 2)
				var wg sync.WaitGroup
				for j := range 2 {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						errs[j] = w.inTx(payAndAllocate(r, 9+j))
					}()
				}
				close(start)
				wg.Wait()
				require.NoError(t, errors.Join(errs...), "round %d", i)
				require.Equal(t, want, summarize(w.invoices(t, r)), "round %d", i)
				require.Equal(t, wantBalance.Credit, w.balance(t, r).Credit, "round %d", i)
				require.Empty(t, w.allocate(t, r))
			}
		})
	})
}
