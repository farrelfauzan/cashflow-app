//go:build integration

package tenant_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// A unit's custom term (decision 33) works like the standard ones: offered with a price, picked
// by a lease and kept, priced from a date, and replaced without touching the leases on it.
func TestCustomTerm(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // the clock reads 2 November 2026, 09:00 in Jakarta
	w := newWorld(t, env, "0812765")
	price := func(v int64) *int64 { return &v }

	ruko, err := env.Properties.CreateUnit(ctx, w.admin, w.melati.ID, property.UnitInput{
		Label: "Ruko 1", Terms: []property.TermInput{{Months: 12, Price: price(54_000_000)}, {Months: 24, Price: price(100_000_000)}},
	})
	require.NoError(t, err)
	require.Equal(t, []property.UnitTerm{{Months: 12, Price: price(54_000_000)}, {Months: 24, Price: price(100_000_000)}}, ruko.Terms)

	tn, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Hendra Gunawan", Phone: "0812-7650-5001"})
	require.NoError(t, err)
	lease, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tn.ID, UnitID: ruko.ID, TermMonths: 24, Terms: tenant.Terms{StartDate: day(2026, 11, 1)},
	})
	require.NoError(t, err)
	require.Equal(t, 24, lease.TermMonths)
	require.Equal(t, int64(100_000_000), *lease.PeriodRent)

	t.Run("a later price for the custom term", func(t *testing.T) {
		got, err := env.Properties.CreateUnitPrice(ctx, w.admin, ruko.ID, property.PriceInput{Months: 24, Amount: 110_000_000, EffectiveFrom: day(2027, 1, 1)})
		require.NoError(t, err)
		require.Equal(t, 24, got.Months)
	})

	t.Run("a second custom term is refused", func(t *testing.T) {
		_, err := env.Properties.UpdateUnit(ctx, w.admin, ruko.ID, property.UnitInput{
			Label: ruko.Label, Terms: []property.TermInput{{Months: 12}, {Months: 24}, {Months: 36}},
		})
		var inputErr *property.InputError
		require.ErrorAs(t, err, &inputErr)
		require.Equal(t, "terms", inputErr.Field)
	})

	t.Run("replacing the custom term keeps the lease and the history", func(t *testing.T) {
		u, err := env.Properties.UpdateUnit(ctx, w.admin, ruko.ID, property.UnitInput{
			Label: ruko.Label, Terms: []property.TermInput{{Months: 12}, {Months: 36, Price: price(150_000_000)}},
		})
		require.NoError(t, err)
		require.Equal(t, []property.UnitTerm{{Months: 12, Price: price(54_000_000)}, {Months: 36, Price: price(150_000_000)}}, u.Terms)

		got, err := env.Tenants.Get(ctx, w.admin, tn.ID)
		require.NoError(t, err)
		require.Equal(t, 24, got.Leases[0].TermMonths)
		require.Equal(t, int64(100_000_000), *got.Leases[0].PeriodRent, "the 24-month price still in force today")

		history, err := env.Properties.ListUnitPrices(ctx, w.admin, ruko.ID)
		require.NoError(t, err)
		months := map[int]int{}
		for _, p := range history {
			months[p.Months]++
		}
		require.Equal(t, map[int]int{12: 1, 24: 2, 36: 1}, months)

		_, err = env.Properties.CreateUnitPrice(ctx, w.admin, ruko.ID, property.PriceInput{Months: 24, Amount: 1, EffectiveFrom: day(2027, 2, 1)})
		require.ErrorIs(t, err, tenant.ErrTermNotOffered, "no longer offered")
	})

	t.Run("the database refuses terms outside decision 33", func(t *testing.T) {
		checkViolation := func(t *testing.T, name string, f func(q *dbgen.Queries) error) {
			t.Helper()
			err := env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error { return f(dbgen.New(tx)) })
			var pgErr *pgconn.PgError
			require.True(t, errors.As(err, &pgErr), "%s: expected a Postgres error, got %v", name, err)
			require.Equal(t, "23514", pgErr.Code, "%s: expected a check violation", name)
		}
		for name, terms := range map[string][]int16{
			"two custom terms": {1, 2, 24},
			"61 months":        {1, 61},
			"a zero term":      {0},
			"a repeated term":  {12, 12},
			"no term at all":   {},
		} {
			checkViolation(t, name, func(q *dbgen.Queries) error {
				_, err := q.UpdateUnit(ctx, dbgen.UpdateUnitParams{OrganizationID: w.org.ID, ID: ruko.ID, Label: ruko.Label, Terms: terms})
				return err
			})
		}
		checkViolation(t, "a 61-month price", func(q *dbgen.Queries) error {
			_, err := q.CreateUnitPrice(ctx, dbgen.CreateUnitPriceParams{
				ID: uuid.New(), OrganizationID: w.org.ID, UnitID: ruko.ID, TermMonths: 61, Amount: 1, EffectiveFrom: day(2027, 1, 1),
			})
			return err
		})

		err := env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
			_, err := dbgen.New(tx).UpdateUnit(ctx, dbgen.UpdateUnitParams{OrganizationID: w.org.ID, ID: ruko.ID, Label: ruko.Label, Terms: []int16{1, 3, 6, 12, 60}})
			return err
		})
		require.NoError(t, err, "every standard term and one custom term")
	})
}
