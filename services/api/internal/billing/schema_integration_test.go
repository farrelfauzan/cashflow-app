//go:build integration

package billing_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// moneyTables are the append-only tables of slice 2.
var moneyTables = []string{"invoice", "invoice_line", "payment", "payment_allocation", "correction"}

// moneyViews are the derived balances.
var moneyViews = []string{"invoice_balance", "tenant_balance"}

// ledgerRows are the ids fillLedger created.
type ledgerRows struct {
	invoice, payment uuid.UUID
}

// fillLedger gives a renter one of every money row: an invoice with a rent and a Wi-Fi line,
// a payment allocated to it, and a correction of each.
func (w *world) fillLedger(t *testing.T, r renter) ledgerRows {
	t.Helper()
	var out ledgerRows
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
		var err error
		if out.invoice, err = w.insertInvoice(ctx, q, r, day(2026, 10, 28), 650_000, 50_000); err != nil {
			return err
		}
		if out.payment, err = w.insertPayment(ctx, q, r, 500_000, at(2026, 10, 28, 10)); err != nil {
			return err
		}
		if _, err := billing.Allocate(ctx, tx, w.org.ID, r.tenantID); err != nil {
			return err
		}
		if err := w.insertCorrection(ctx, q, out.invoice, uuid.Nil, -50_000, "Wi-Fi tidak aktif bulan ini"); err != nil {
			return err
		}
		if err := w.insertCorrection(ctx, q, uuid.Nil, out.payment, -100_000, "uang palsu dikembalikan"); err != nil {
			return err
		}
		_, err = billing.Allocate(ctx, tx, w.org.ID, r.tenantID)
		return err
	})
	return out
}

// monthly is the n-th monthly period start of the test leases, from 28 Oct 2026.
func monthly(n int) time.Time { return day(2026, 10, 28).AddDate(0, n, 0) }

func TestMoneyRowsAreAppendOnly(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "0812773")
	w.fillLedger(t, w.newRenter(t, "Rina Wulandari"))

	statements := map[string]string{
		"update":   "UPDATE %s SET created_at = created_at",
		"delete":   "DELETE FROM %s",
		"truncate": "TRUNCATE %s CASCADE",
	}

	t.Run("the app role has no privilege to change them", func(t *testing.T) {
		for _, table := range moneyTables {
			for verb, stmt := range statements {
				t.Run(table+" "+verb, func(t *testing.T) {
					err := w.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
						_, err := tx.Exec(ctx, fmt.Sprintf(stmt, table))
						return err
					})
					pgErr := requireCode(t, err, "42501")
					require.Contains(t, pgErr.Message, "permission denied")
				})
			}
		}
	})

	t.Run("the owner is stopped by the trigger", func(t *testing.T) {
		conn, err := pgx.Connect(ctx, env.URLs.Owner)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, conn.Close(ctx)) })
		for _, table := range moneyTables {
			var n int
			require.NoError(t, conn.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&n))
			require.Positive(t, n, "%s needs rows for the row trigger to fire", table)
			for verb, stmt := range statements {
				t.Run(table+" "+verb, func(t *testing.T) {
					_, err := conn.Exec(ctx, fmt.Sprintf(stmt, table))
					pgErr := requireCode(t, err, "42501")
					require.Contains(t, pgErr.Message, "is append-only")
				})
			}
		}
	})
}

func TestMoneyConstraints(t *testing.T) {
	env := testenv.New(t)
	w := newWorld(t, env, "0812774")
	r := w.newRenter(t, "Rina Wulandari")
	other := w.newRenter(t, "Budi Santoso")

	// Rows the correction and allocation cases build on: a Rp700.000 invoice and a Rp500.000
	// payment of r, and a payment and an invoice of other.
	var inv, pay, otherInv, otherPay uuid.UUID
	w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
		var err error
		if inv, err = w.insertInvoice(ctx, q, r, monthly(100), 700_000); err != nil {
			return err
		}
		if pay, err = w.insertPayment(ctx, q, r, 500_000, at(2026, 10, 28, 9)); err != nil {
			return err
		}
		if otherInv, err = w.insertInvoice(ctx, q, other, monthly(100), 700_000); err != nil {
			return err
		}
		otherPay, err = w.insertPayment(ctx, q, other, 500_000, at(2026, 10, 28, 9))
		return err
	})

	allocation := func(paymentID, invoiceID, tenantID uuid.UUID) func(ctx context.Context, q *dbgen.Queries, n int) error {
		return func(ctx context.Context, q *dbgen.Queries, _ int) error {
			return q.CreatePaymentAllocation(ctx, dbgen.CreatePaymentAllocationParams{
				ID: uuid.New(), OrganizationID: w.org.ID, PaymentID: paymentID, InvoiceID: invoiceID, TenantID: tenantID, Amount: 1,
			})
		}
	}
	correction := func(invoiceID, paymentID uuid.UUID, delta int64, reason string) func(ctx context.Context, q *dbgen.Queries, n int) error {
		return func(ctx context.Context, q *dbgen.Queries, _ int) error {
			return w.insertCorrection(ctx, q, invoiceID, paymentID, delta, reason)
		}
	}

	tests := []struct {
		name string
		// build inserts rows; n is unique per case, for a period of its own.
		build      func(ctx context.Context, q *dbgen.Queries, n int) error
		code       string // "" when the transaction commits
		constraint string
		atCommit   bool // the error comes from the deferred check at commit
	}{
		{
			name: "a rent and a charge line that add up commit",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				_, err := w.insertInvoice(ctx, q, r, monthly(n), 650_000, 50_000)
				return err
			},
		},
		{
			name: "lines that add up to less than the amount fail at commit",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				id, err := w.insertBareInvoice(ctx, q, r, monthly(n), 700_000)
				if err != nil {
					return err
				}
				return w.insertLine(ctx, q, id, "rent", "Sewa", 600_000)
			},
			code: "23514", constraint: "invoice_lines_add_up", atCommit: true,
		},
		{
			name: "lines that add up to more than the amount fail at commit",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				id, err := w.insertBareInvoice(ctx, q, r, monthly(n), 700_000)
				if err != nil {
					return err
				}
				if err := w.insertLine(ctx, q, id, "rent", "Sewa", 700_000); err != nil {
					return err
				}
				return w.insertLine(ctx, q, id, "charge", "Wi-Fi", 50_000)
			},
			code: "23514", constraint: "invoice_lines_add_up", atCommit: true,
		},
		{
			name: "an invoice without lines fails at commit",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				_, err := w.insertBareInvoice(ctx, q, r, monthly(n), 700_000)
				return err
			},
			code: "23514", constraint: "invoice_lines_add_up", atCommit: true,
		},
		{
			name: "a charge line without a charge type is refused",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				id, err := w.insertBareInvoice(ctx, q, r, monthly(n), 700_000)
				if err != nil {
					return err
				}
				_, err = q.CreateInvoiceLine(ctx, dbgen.CreateInvoiceLineParams{
					ID: uuid.New(), OrganizationID: w.org.ID, InvoiceID: id, Kind: "charge", Label: "Listrik", Amount: 700_000,
				})
				return err
			},
			code: "23514", constraint: "invoice_line_charge_type",
		},
		{
			name: "a rent line with a charge type is refused",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				id, err := w.insertBareInvoice(ctx, q, r, monthly(n), 700_000)
				if err != nil {
					return err
				}
				_, err = q.CreateInvoiceLine(ctx, dbgen.CreateInvoiceLineParams{
					ID: uuid.New(), OrganizationID: w.org.ID, InvoiceID: id, Kind: "rent", Label: "Sewa", Amount: 700_000,
					ChargeTypeID: uuid.NullUUID{UUID: w.wifi.ID, Valid: true},
				})
				return err
			},
			code: "23514", constraint: "invoice_line_charge_type",
		},
		{
			name: "a blank line label is refused",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				id, err := w.insertBareInvoice(ctx, q, r, monthly(n), 700_000)
				if err != nil {
					return err
				}
				return w.insertLine(ctx, q, id, "rent", "  ", 700_000)
			},
			code: "23514", constraint: "invoice_line_label_check",
		},
		{
			name: "a second rent line is refused",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				id, err := w.insertBareInvoice(ctx, q, r, monthly(n), 700_000)
				if err != nil {
					return err
				}
				if err := w.insertLine(ctx, q, id, "rent", "Sewa", 350_000); err != nil {
					return err
				}
				return w.insertLine(ctx, q, id, "rent", "Sewa", 350_000)
			},
			code: "23505", constraint: "invoice_line_one_rent",
		},
		{
			name: "a second invoice for the same lease and period is refused",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				if _, err := w.insertInvoice(ctx, q, r, monthly(n), 700_000); err != nil {
					return err
				}
				_, err := w.insertInvoice(ctx, q, r, monthly(n), 700_000)
				return err
			},
			code: "23505", constraint: "invoice_regular_period",
		},
		{
			name: "an invoice naming a tenant other than its lease's is refused",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				_, err := w.insertInvoice(ctx, q, renter{tenantID: other.tenantID, leaseID: r.leaseID}, monthly(n), 700_000)
				return err
			},
			code: "23503", constraint: "invoice_lease_id_tenant_id_organization_id_fkey",
		},
		{
			name: "a period that ends before it starts is refused",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				_, err := q.CreateInvoice(ctx, dbgen.CreateInvoiceParams{
					ID: uuid.New(), OrganizationID: w.org.ID, LeaseID: r.leaseID, TenantID: r.tenantID,
					Period: monthly(n), PeriodEnd: monthly(n).AddDate(0, 0, -1), DueDate: monthly(n), Amount: 1,
				})
				return err
			},
			code: "23514", constraint: "invoice_period_in_order",
		},
		{
			name: "a gateway reference is recorded once per source",
			build: func(ctx context.Context, q *dbgen.Queries, _ int) error {
				for range 2 {
					ref := "TRX-20261028-0001"
					if _, err := q.CreatePayment(ctx, dbgen.CreatePaymentParams{
						ID: uuid.New(), OrganizationID: w.org.ID, TenantID: r.tenantID, Amount: 700_000, Method: "qris",
						ReceivedAt: at(2026, 10, 28, 11), Source: "qris", SourceRef: &ref,
					}); err != nil {
						return err
					}
				}
				return nil
			},
			code: "23505", constraint: "payment_source_ref_unique",
		},
		{
			name: "a manual payment that names no recorder is refused",
			build: func(ctx context.Context, q *dbgen.Queries, _ int) error {
				_, err := q.CreatePayment(ctx, dbgen.CreatePaymentParams{
					ID: uuid.New(), OrganizationID: w.org.ID, TenantID: r.tenantID, Amount: 700_000, Method: "cash",
					ReceivedAt: at(2026, 10, 28, 11), Source: "manual",
				})
				return err
			},
			code: "23514", constraint: "payment_manual_recorded_by",
		},
		{
			name: "a manual payment cannot claim to be QRIS",
			build: func(ctx context.Context, q *dbgen.Queries, _ int) error {
				_, err := q.CreatePayment(ctx, dbgen.CreatePaymentParams{
					ID: uuid.New(), OrganizationID: w.org.ID, TenantID: r.tenantID, Amount: 700_000, Method: "qris",
					ReceivedAt: at(2026, 10, 28, 11), Source: "manual",
					RecordedByStaffUserID: uuid.NullUUID{UUID: w.org.Admin.UserID, Valid: true},
				})
				return err
			},
			code: "23514", constraint: "payment_method_matches_source",
		},
		{
			name: "a QRIS payment cannot be cash",
			build: func(ctx context.Context, q *dbgen.Queries, _ int) error {
				ref := "TRX-20261028-0002"
				_, err := q.CreatePayment(ctx, dbgen.CreatePaymentParams{
					ID: uuid.New(), OrganizationID: w.org.ID, TenantID: r.tenantID, Amount: 700_000, Method: "cash",
					ReceivedAt: at(2026, 10, 28, 11), Source: "qris", SourceRef: &ref,
				})
				return err
			},
			code: "23514", constraint: "payment_method_matches_source",
		},
		{
			name:  "an allocation of a payment to another tenant's invoice is refused",
			build: allocation(pay, otherInv, r.tenantID),
			code:  "23503", constraint: "payment_allocation_invoice_id_tenant_id_organization_id_fkey",
		},
		{
			name:  "the same allocation under the other tenant's name is refused too",
			build: allocation(pay, otherInv, other.tenantID),
			code:  "23503", constraint: "payment_allocation_payment_id_tenant_id_organization_id_fkey",
		},
		{
			name:  "an allocation of another tenant's payment to this invoice is refused",
			build: allocation(otherPay, inv, r.tenantID),
			code:  "23503", constraint: "payment_allocation_payment_id_tenant_id_organization_id_fkey",
		},
		{
			name:  "a correction that would make an invoice negative is refused",
			build: correction(inv, uuid.Nil, -700_001, "salah hitung"),
			code:  "23514", constraint: "correction_not_negative",
		},
		{
			name:  "a correction that would make a payment negative is refused",
			build: correction(uuid.Nil, pay, -500_001, "salah hitung"),
			code:  "23514", constraint: "correction_not_negative",
		},
		{
			name:  "a blank reason is refused",
			build: correction(inv, uuid.Nil, -100, "   "),
			code:  "23514", constraint: "correction_reason_check",
		},
		{
			name:  "a correction naming both an invoice and a payment is refused",
			build: correction(inv, pay, -100, "salah hitung"),
			code:  "23514", constraint: "correction_one_target",
		},
		{
			name:  "a correction naming neither is refused",
			build: correction(uuid.Nil, uuid.Nil, -100, "salah hitung"),
			code:  "23514", constraint: "correction_one_target",
		},
		{
			name:  "a zero correction is refused",
			build: correction(inv, uuid.Nil, 0, "salah hitung"),
			code:  "23514", constraint: "correction_amount_delta_check",
		},
		{
			name: "corrections add up: down to zero cancels, one rupiah more is refused",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				id, err := w.insertInvoice(ctx, q, r, monthly(n), 700_000)
				if err != nil {
					return err
				}
				if err := w.insertCorrection(ctx, q, id, uuid.Nil, -400_000, "diskon"); err != nil {
					return err
				}
				if err := w.insertCorrection(ctx, q, id, uuid.Nil, -300_000, "sewa tidak diperpanjang"); err != nil {
					return err
				}
				return w.insertCorrection(ctx, q, id, uuid.Nil, -1, "salah hitung")
			},
			code: "23514", constraint: "correction_not_negative",
		},
		{
			name: "an invoice and a payment may be corrected to exactly zero",
			build: func(ctx context.Context, q *dbgen.Queries, n int) error {
				id, err := w.insertInvoice(ctx, q, r, monthly(n), 700_000)
				if err != nil {
					return err
				}
				if err := w.insertCorrection(ctx, q, id, uuid.Nil, -700_000, "sewa tidak diperpanjang"); err != nil {
					return err
				}
				p, err := w.insertPayment(ctx, q, r, 100_000, at(2026, 10, 29, 9))
				if err != nil {
					return err
				}
				return w.insertCorrection(ctx, q, uuid.Nil, p, -100_000, "dicatat dua kali")
			},
		},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reachedCommit := false
			err := w.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
				if err := tc.build(ctx, q, i); err != nil {
					return err
				}
				reachedCommit = true
				return nil
			})
			if tc.code == "" {
				require.NoError(t, err)
				return
			}
			pgErr := requireCode(t, err, tc.code)
			require.Equal(t, tc.constraint, pgErr.ConstraintName)
			require.Equal(t, tc.atCommit, reachedCommit, "whether the error came only at commit")
		})
	}

	t.Run("a cancelled invoice shows as cancelled", func(t *testing.T) {
		var statuses []string
		for _, b := range w.invoices(t, r) {
			statuses = append(statuses, b.Status)
		}
		require.Contains(t, statuses, "cancelled")
	})
}

func TestMoneyIsolation(t *testing.T) {
	env := testenv.New(t)
	a := newWorld(t, env, "0812775")
	b := newWorld(t, env, "0812776")
	ra, rb := a.newRenter(t, "Rina Wulandari"), b.newRenter(t, "Budi Santoso")
	a.fillLedger(t, ra)
	lb := b.fillLedger(t, rb)

	t.Run("the views read as the caller", func(t *testing.T) {
		for _, view := range moneyViews {
			var opts []string
			a.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				return tx.QueryRow(ctx, "SELECT coalesce(reloptions, '{}') FROM pg_class WHERE relname = $1 AND relkind = 'v'", view).Scan(&opts)
			})
			require.Contains(t, opts, "security_invoker=true", view)
		}
	})

	t.Run("each organization sees only its own rows, in every table and view", func(t *testing.T) {
		for _, w := range []*world{a, b} {
			for _, rel := range slices.Concat(moneyTables, moneyViews) {
				var all, foreign int
				w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
					return tx.QueryRow(ctx, fmt.Sprintf(
						"SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM %s", rel,
					)).Scan(&all, &foreign)
				})
				require.Positive(t, all, "%s should have rows of its own organization", rel)
				require.Zero(t, foreign, "%s shows another organization's rows", rel)
			}
		}
	})

	t.Run("another organization's invoice, payment and balances are not found", func(t *testing.T) {
		a.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			for _, check := range []struct {
				rel string
				col string
				id  uuid.UUID
			}{
				{"invoice", "id", lb.invoice}, {"invoice_line", "invoice_id", lb.invoice},
				{"payment", "id", lb.payment}, {"payment_allocation", "payment_id", lb.payment},
				{"correction", "invoice_id", lb.invoice}, {"correction", "payment_id", lb.payment},
				{"invoice_balance", "invoice_id", lb.invoice}, {"tenant_balance", "tenant_id", rb.tenantID},
			} {
				var n int
				require.NoError(t, tx.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s WHERE %s = $1", check.rel, check.col), check.id).Scan(&n))
				require.Zero(t, n, "%s.%s of the other organization", check.rel, check.col)
			}
			_, err := q.GetTenantBalance(ctx, dbgen.GetTenantBalanceParams{OrganizationID: b.org.ID, TenantID: rb.tenantID})
			require.ErrorIs(t, err, pgx.ErrNoRows)
			return nil
		})
	})

	t.Run("no row can be written for another organization", func(t *testing.T) {
		inserts := map[string]func(ctx context.Context, q *dbgen.Queries) error{
			"invoice": func(ctx context.Context, q *dbgen.Queries) error {
				_, err := b.insertInvoice(ctx, q, rb, monthly(50), 700_000)
				return err
			},
			"invoice_line": func(ctx context.Context, q *dbgen.Queries) error {
				return b.insertLine(ctx, q, lb.invoice, "charge", "Wi-Fi", 1)
			},
			"payment": func(ctx context.Context, q *dbgen.Queries) error {
				_, err := b.insertPayment(ctx, q, rb, 1, at(2026, 10, 30, 9))
				return err
			},
			"payment_allocation": func(ctx context.Context, q *dbgen.Queries) error {
				return q.CreatePaymentAllocation(ctx, dbgen.CreatePaymentAllocationParams{
					ID: uuid.New(), OrganizationID: b.org.ID, PaymentID: lb.payment, InvoiceID: lb.invoice, TenantID: rb.tenantID, Amount: 1,
				})
			},
			"correction": func(ctx context.Context, q *dbgen.Queries) error {
				return b.insertCorrection(ctx, q, lb.invoice, uuid.Nil, 1, "koreksi")
			},
		}
		for _, table := range moneyTables {
			t.Run(table, func(t *testing.T) {
				// Organization A's scope, writing rows that name organization B.
				err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error { return inserts[table](ctx, q) })
				pgErr := requireCode(t, err, "42501")
				require.Contains(t, pgErr.Message, "row-level security")
			})
		}
	})

	t.Run("allocation in one organization leaves the other's untouched", func(t *testing.T) {
		before := summarize(b.invoices(t, rb))
		a.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
			if _, err := a.insertPayment(ctx, q, ra, 5_000_000, at(2026, 11, 1, 9)); err != nil {
				return err
			}
			_, err := billing.Allocate(ctx, tx, a.org.ID, ra.tenantID)
			return err
		})
		require.Equal(t, before, summarize(b.invoices(t, rb)))
		require.Equal(t, "paid", a.invoices(t, ra)[0].Status)
	})
}

func TestActiveOrganizationIDs(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	var want []uuid.UUID
	for _, prefix := range []string{"0812781", "0812782", "0812783"} {
		want = append(want, env.CreateOrg(t, "Kos "+prefix, prefix).ID)
	}
	slices.SortFunc(want, func(x, y uuid.UUID) int { return slices.Compare(x[:], y[:]) })

	// The app role, with no organization scope at all, as a scheduled job starts.
	app, err := pgx.Connect(ctx, env.URLs.App)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, app.Close(ctx)) })
	owner, err := pgx.Connect(ctx, env.URLs.Owner)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close(ctx)) })

	ids := func(t *testing.T) []uuid.UUID {
		t.Helper()
		got, err := dbgen.New(app).ActiveOrganizationIDs(ctx)
		require.NoError(t, err)
		return got
	}

	t.Run("returns every organization's id, in order, to the app role without a scope", func(t *testing.T) {
		require.Equal(t, want, ids(t))
	})

	t.Run("the app role still cannot read organizations itself", func(t *testing.T) {
		var n int
		require.NoError(t, app.QueryRow(ctx, "SELECT count(*) FROM organization").Scan(&n))
		require.Zero(t, n)
		// Not even by setting the function's flag: the policy is for the function's owner only.
		tx, err := app.Begin(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback(ctx) })
		_, err = tx.Exec(ctx, "SELECT set_config('app.list_org_ids', 'on', true)")
		require.NoError(t, err)
		require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM organization").Scan(&n))
		require.Zero(t, n)
		require.NoError(t, tx.Rollback(ctx))
	})

	t.Run("it returns ids only and is executable only by the app role", func(t *testing.T) {
		var result string
		var definer bool
		var config []string
		require.NoError(t, owner.QueryRow(ctx, `
			SELECT pg_get_function_result(p.oid), p.prosecdef, coalesce(p.proconfig, '{}')
			FROM pg_proc p WHERE p.proname = 'active_organization_ids'`).Scan(&result, &definer, &config))
		require.Equal(t, "SETOF uuid", result)
		require.True(t, definer)
		require.Contains(t, config, "search_path=pg_catalog, public")

		_, err := owner.Exec(ctx, "CREATE ROLE stranger NOLOGIN")
		require.NoError(t, err)
		var app, stranger bool
		require.NoError(t, owner.QueryRow(ctx, `
			SELECT has_function_privilege('cashflow_app', 'active_organization_ids()', 'EXECUTE'),
			       has_function_privilege('stranger', 'active_organization_ids()', 'EXECUTE')`).Scan(&app, &stranger))
		require.True(t, app)
		require.False(t, stranger, "PUBLIC must not have EXECUTE")
	})

	t.Run("it works for an owner held to row-level security, as in production", func(t *testing.T) {
		// The test database's owner is a superuser, which skips row-level security, so hand
		// the function to an ordinary role first. Without the policy it would see nothing.
		for _, stmt := range []string{
			"CREATE ROLE fn_owner NOLOGIN NOSUPERUSER NOBYPASSRLS",
			"GRANT SELECT ON organization, membership TO fn_owner", // an organization policy reads membership
			"ALTER FUNCTION active_organization_ids() OWNER TO fn_owner",
		} {
			_, err := owner.Exec(ctx, stmt)
			require.NoError(t, err, stmt)
		}
		require.Empty(t, ids(t), "an owner without the policy is held to the organization scope")

		_, err := owner.Exec(ctx, "ALTER POLICY organization_list_ids ON organization TO fn_owner")
		require.NoError(t, err)
		require.Equal(t, want, ids(t))
	})
}

// TestActiveOrganizationIDsIsTheOnlySecurityDefiner guards the policies that let the migration role read
// across organizations while a function's own setting is on: organization_list_ids
// (app.list_org_ids, 00011_money.sql), payment_link_lookup (app.payment_link_lookup,
// 00017_payment_link.sql), autogopay_account_webhook_lookup (app.autogopay_webhook_lookup,
// 00020_autogopay_webhook_lookup.sql), telegram_link_code_lookup (app.telegram_link_code_lookup)
// and telegram_link_chat_organizations (app.telegram_chat_organizations, both
// 00025_telegram_link.sql). Any role can turn a custom setting on for its own session, so every
// SECURITY DEFINER function owned by the migration role could read under any of them. Today there
// are five: active_organization_ids() reads organization ids only; payment_link_lookup(bytea)
// reads one link's organization and id by its token hash; autogopay_webhook_lookup(text) reads one
// AutoGoPay account's organization id by its webhook id; telegram_link_code_lookup(bytea) reads
// one Telegram linking code's organization and id by its hash; and
// telegram_chat_organizations(bigint) reads the organization ids where one Telegram chat has a
// live link. Each touches only its own table. A new one must be reviewed against all five
// policies, and this test says so. (Its name predates the second function; 00011_money.sql refers
// to it by this name.)
func TestActiveOrganizationIDsIsTheOnlySecurityDefiner(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	owner, err := pgx.Connect(ctx, env.URLs.Owner)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close(ctx)) })

	rows, err := owner.Query(ctx, `
		SELECT p.proname FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' AND p.prosecdef
		ORDER BY p.proname`)
	require.NoError(t, err)
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	require.Equal(t, []string{
		"active_organization_ids", "autogopay_webhook_lookup", "payment_link_lookup",
		"telegram_chat_organizations", "telegram_link_code_lookup",
	}, names,
		"a new SECURITY DEFINER function must be checked against the organization_list_ids policy in 00011_money.sql, "+
			"the payment_link_lookup policy in 00017_payment_link.sql, the autogopay_account_webhook_lookup policy "+
			"in 00020_autogopay_webhook_lookup.sql and the telegram_link_code_lookup and "+
			"telegram_link_chat_organizations policies in 00025_telegram_link.sql")
}
