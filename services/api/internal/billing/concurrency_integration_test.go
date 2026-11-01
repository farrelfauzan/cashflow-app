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

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// TestGenerationRacesLeaseChanges proves the lock order (lease, then unit, then tenant) on the
// race that matters: invoice generation and a planned end date on the same lease and tenant.
// Whichever commits first, the other sees it; neither deadlocks.
func TestGenerationRacesLeaseChanges(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	t.Run("a planned end set while generation holds the lease cancels the invoice it made", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812831")
		r := w.rent(t, "Rina Wulandari", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		setNow(env, 2026, 11, 10, 9, 0) // the 15 Nov invoice is due within the lead time

		done := make(chan error, 1)
		err := env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
			n, err := env.Billing.GenerateLease(ctx, tx, w.org.ID, r.leaseID, day(2026, 11, 10))
			require.NoError(t, err)
			require.Equal(t, 1, n)
			go func() {
				_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, r.leaseID, ptr(day(2026, 11, 14)))
				done <- err
			}()
			testenv.WaitUntilBlockedOn(ctx, t, tx)
			return nil // commit: the planned end goes on and must see the new invoice
		})
		require.NoError(t, err)
		require.NoError(t, <-done)
		invs := w.invoices(t, r.renter)
		require.Len(t, invs, 2)
		require.Equal(t, "cancelled", invs[1].Status, "the invoice generated first is cancelled by the planned end")
	})

	t.Run("generation waiting on a planned end being set makes no invoice after it", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812832")
		r := w.rent(t, "Dewi Lestari", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		setNow(env, 2026, 11, 10, 9, 0)

		done := make(chan error, 1)
		// The same statements SetPlannedEnd runs first: lock the lease, store the date.
		err := env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
			q := dbgen.New(tx)
			if _, err := q.LockLease(ctx, dbgen.LockLeaseParams{OrganizationID: w.org.ID, ID: r.leaseID}); err != nil {
				return err
			}
			if _, err := q.SetLeasePlannedEnd(ctx, dbgen.SetLeasePlannedEndParams{
				OrganizationID: w.org.ID, ID: r.leaseID, PlannedEndDate: ptr(day(2026, 11, 14)),
			}); err != nil {
				return err
			}
			go func() {
				_, err := env.Billing.GenerateOrg(ctx, w.org.ID)
				done <- err
			}()
			testenv.WaitUntilBlockedOn(ctx, t, tx)
			return nil
		})
		require.NoError(t, err)
		require.NoError(t, <-done)
		require.Len(t, w.invoices(t, r.renter), 1, "generation read the lease after the lock and saw the planned end")
	})

	t.Run("generation and lease changes on one tenant's two leases never deadlock", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newWorld(t, env, "0812833")
		const rounds = 8
		type pair struct{ a, b rental }
		pairs := make([]pair, rounds)
		for i := range pairs {
			a := w.rent(t, fmt.Sprintf("Penyewa %d", i), day(2026, 10, 15), rentOpts{deposit: 300_000})
			b := w.secondLease(t, a.renter, day(2026, 10, 15))
			pairs[i] = pair{a, b}
		}
		setNow(env, 2026, 11, 10, 9, 0)

		for i, p := range pairs {
			var wg sync.WaitGroup
			errs := make(chan error, 4)
			run := func(fn func() error) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					errs <- fn()
				}()
			}
			run(func() error {
				for _, id := range []uuid.UUID{p.a.leaseID, p.b.leaseID} {
					err := env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
						_, err := env.Billing.GenerateLease(ctx, tx, w.org.ID, id, day(2026, 11, 10))
						return err
					})
					if err != nil {
						return err
					}
				}
				return nil
			})
			run(func() error {
				_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, p.b.leaseID, ptr(day(2026, 11, 14)))
				return err
			})
			run(func() error {
				_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, p.a.leaseID, ptr(day(2026, 11, 14)))
				return err
			})
			run(func() error {
				_, err := env.Billing.GenerateOrg(ctx, w.org.ID)
				return err
			})
			wg.Wait()
			close(errs)
			for err := range errs {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) {
					require.NotEqual(t, "40P01", pgErr.Code, "round %d deadlocked: %v", i, err)
				}
				require.NoError(t, err, "round %d", i)
			}
			// Whatever the order, no period after the planned end bills anything.
			for _, inv := range w.invoices(t, p.a.renter) {
				if inv.Period.After(day(2026, 11, 14)) {
					require.Zero(t, inv.Billed, "round %d: invoice for %s still bills", i, inv.Period)
				}
			}
		}
	})
}

// secondLease gives the tenant of r a monthly lease on a new unit of the kos.
func (w *world) secondLease(t *testing.T, r renter, start time.Time) rental {
	t.Helper()
	ctx := context.Background()
	w.n++
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{
		Label: fmt.Sprintf("Kamar %d", w.n), Terms: testenv.Monthly(700_000),
	})
	require.NoError(t, err)
	l, err := w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: r.tenantID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: start},
	})
	require.NoError(t, err)
	return rental{renter: renter{tenantID: r.tenantID, leaseID: l.ID}, unitID: unit.ID}
}
