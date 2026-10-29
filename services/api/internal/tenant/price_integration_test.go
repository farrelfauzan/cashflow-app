//go:build integration

package tenant_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// at sets the test clock to 09:00 in Jakarta on a day.
func at(env *testenv.Env, d time.Time) {
	env.Clock.Set(time.Date(d.Year(), d.Month(), d.Day(), 9, 0, 0, 0, clock.Jakarta))
}

func TestPriceInForce(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	a := newWorld(t, env, "0812761")
	b := newWorld(t, env, "0812762")
	today := day(2026, 11, 2) // the test clock's day

	unit, err := env.Properties.CreateUnit(ctx, a.admin, a.melati.ID, property.UnitInput{
		Label: "Kamar 9", Terms: []property.TermInput{{Months: 1}, {Months: 12}},
	})
	require.NoError(t, err)
	bare, err := env.Properties.CreateUnit(ctx, a.admin, a.melati.ID, property.UnitInput{Label: "Kamar 10"})
	require.NoError(t, err)
	price := func(months int, amount int64, from time.Time) {
		t.Helper()
		_, err := env.Properties.CreateUnitPrice(ctx, a.admin, unit.ID, property.PriceInput{Months: months, Amount: amount, EffectiveFrom: from})
		require.NoError(t, err)
	}
	// Past prices are recorded on their own day, as an admin would have.
	at(env, day(2026, 10, 1))
	price(1, 800_000, day(2026, 10, 1))
	at(env, day(2026, 10, 15))
	price(12, 9_000_000, day(2026, 10, 15))
	at(env, today)
	price(1, 900_000, day(2026, 12, 1))
	price(1, 950_000, day(2027, 1, 1))
	price(1, 1_000_000, day(2027, 1, 1)) // recorded later on the same date: it wins

	tests := []struct {
		name   string
		org    uuid.UUID
		unitID uuid.UUID
		months int
		day    time.Time
		want   int64 // 0 means ErrNoPrice
	}{
		{"a unit with no price rows", a.org.ID, bare.ID, 1, today, 0},
		{"the day before the first effective date", a.org.ID, unit.ID, 1, day(2026, 9, 30), 0},
		{"on the first effective date", a.org.ID, unit.ID, 1, day(2026, 10, 1), 800_000},
		{"after it", a.org.ID, unit.ID, 1, today, 800_000},
		{"a future price is not in force before its date", a.org.ID, unit.ID, 1, day(2026, 11, 30), 800_000},
		{"a future price on its date", a.org.ID, unit.ID, 1, day(2026, 12, 1), 900_000},
		{"two prices on the same date: the later recorded wins", a.org.ID, unit.ID, 1, day(2027, 1, 1), 1_000_000},
		{"another term has its own history: before it", a.org.ID, unit.ID, 12, day(2026, 10, 14), 0},
		{"another term has its own history: in force", a.org.ID, unit.ID, 12, day(2027, 1, 1), 9_000_000},
		{"a term without rows", a.org.ID, unit.ID, 3, today, 0},
		{"another organization's unit is not visible", a.org.ID, b.kamar1.ID, 1, today, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got int64
			err := env.Store.WithOrgTx(ctx, tc.org, func(tx pgx.Tx) error {
				var err error
				got, err = tenant.PriceInForce(ctx, dbgen.New(tx), tc.org, tc.unitID, tc.months, tc.day)
				return err
			})
			if tc.want == 0 {
				require.ErrorIs(t, err, tenant.ErrNoPrice)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestUnitPriceIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "0812763")

	for name, stmt := range map[string]string{
		"update": "UPDATE unit_price SET amount = 1",
		"delete": "DELETE FROM unit_price",
	} {
		t.Run(name, func(t *testing.T) {
			err := env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, stmt)
				return err
			})
			var pgErr *pgconn.PgError
			require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
			require.Equal(t, "42501", pgErr.Code)
		})
	}
}

func TestLeaseTerms(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "0812764")
	newTenant := func(t *testing.T, phone string) tenant.Tenant {
		t.Helper()
		tn, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Sari " + phone, Phone: phone})
		require.NoError(t, err)
		return tn
	}
	newUnit := func(t *testing.T, label string, terms ...property.TermInput) property.Unit {
		t.Helper()
		u, err := env.Properties.CreateUnit(ctx, w.admin, w.melati.ID, property.UnitInput{Label: label, Terms: terms})
		require.NoError(t, err)
		return u
	}
	price := func(v int64) *int64 { return &v }

	t.Run("refused", func(t *testing.T) {
		unpriced := newUnit(t, "Kamar 20", property.TermInput{Months: 1})
		tn := newTenant(t, "0812-7640-5001")
		tests := []struct {
			name    string
			unitID  uuid.UUID
			months  int
			wantErr error
		}{
			{"a term the unit does not offer", w.kamar2.ID, 12, tenant.ErrTermNotOffered},
			{"a term with no price in force today", unpriced.ID, 1, tenant.ErrNoPrice},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
					TenantID: tn.ID, UnitID: tc.unitID, TermMonths: tc.months, Terms: tenant.Terms{StartDate: day(2026, 11, 1)},
				})
				require.ErrorIs(t, err, tc.wantErr)
				var termErr *tenant.TermError
				require.ErrorAs(t, err, &termErr)
				require.Equal(t, "termMonths", termErr.Field)
				require.Equal(t, tc.months, termErr.Months)
			})
		}
	})

	t.Run("periodRent is today's price and follows a new price", func(t *testing.T) {
		u := newUnit(t, "Kamar 21", property.TermInput{Months: 1, Price: price(850_000)}, property.TermInput{Months: 12, Price: price(9_000_000)})
		tn := newTenant(t, "0812-7640-5002")
		l, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
			TenantID: tn.ID, UnitID: u.ID, TermMonths: 12, Terms: tenant.Terms{StartDate: day(2026, 11, 1)},
		})
		require.NoError(t, err)
		require.Equal(t, 12, l.TermMonths)
		require.Equal(t, int64(9_000_000), *l.PeriodRent)

		_, err = env.Properties.UpdateUnit(ctx, w.admin, u.ID, property.UnitInput{
			Label: u.Label, Terms: []property.TermInput{{Months: 1}, {Months: 12, Price: price(9_500_000)}},
		})
		require.NoError(t, err)
		got, err := env.Tenants.Get(ctx, w.admin, tn.ID)
		require.NoError(t, err)
		require.Equal(t, int64(9_500_000), *got.Leases[0].PeriodRent)
	})

	t.Run("a 3-month lease bills from a quarter start and keeps its term", func(t *testing.T) {
		u := newUnit(t, "Kamar 22", property.TermInput{Months: 3, Price: price(2_400_000)})
		tn := newTenant(t, "0812-7640-5003")
		create := func(billedFrom time.Time) (tenant.Lease, error) {
			return env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
				TenantID: tn.ID, UnitID: u.ID, TermMonths: 3,
				Terms: tenant.Terms{StartDate: day(2026, 8, 15), BilledFrom: billedFrom},
			})
		}
		_, err := create(day(2026, 9, 15))
		requireField(t, err, "billedFrom")
		_, err = create(day(2027, 2, 15))
		requireField(t, err, "billedFrom")
		l, err := create(day(2026, 11, 15))
		require.NoError(t, err)
		require.Equal(t, day(2026, 11, 15), l.BilledFrom)

		updated, err := env.Tenants.UpdateLease(ctx, w.admin, l.ID, tenant.Terms{StartDate: day(2026, 8, 15), DepositAmount: 500_000})
		require.NoError(t, err)
		require.Equal(t, 3, updated.TermMonths)
		require.Equal(t, day(2026, 11, 15), updated.BilledFrom)

		_, err = env.Tenants.UpdateLease(ctx, w.admin, l.ID, tenant.Terms{StartDate: day(2026, 8, 15), BilledFrom: day(2026, 10, 15)})
		requireField(t, err, "billedFrom")
	})

	t.Run("a lease keeps its term after the unit stops offering it", func(t *testing.T) {
		u := newUnit(t, "Kamar 23", property.TermInput{Months: 1, Price: price(850_000)}, property.TermInput{Months: 12, Price: price(9_000_000)})
		tn := newTenant(t, "0812-7640-5004")
		l, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
			TenantID: tn.ID, UnitID: u.ID, TermMonths: 12, Terms: tenant.Terms{StartDate: day(2026, 11, 1)},
		})
		require.NoError(t, err)
		_, err = env.Properties.UpdateUnit(ctx, w.admin, u.ID, property.UnitInput{Label: u.Label, Terms: []property.TermInput{{Months: 1}}})
		require.NoError(t, err)
		updated, err := env.Tenants.UpdateLease(ctx, w.admin, l.ID, tenant.Terms{StartDate: day(2026, 11, 1)})
		require.NoError(t, err)
		require.Equal(t, 12, updated.TermMonths)
		require.Equal(t, int64(9_000_000), *updated.PeriodRent, "the price history stays")
	})
}
