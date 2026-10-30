//go:build integration

package property_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestUnitPrices(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // the clock reads 2 November 2026, 09:00 in Jakarta
	o := env.CreateOrg(t, "Griya Sejahtera", "0812651")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	price := func(v int64) *int64 { return &v }

	melati, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	kamar1, err := env.Properties.CreateUnit(ctx, admin, melati.ID, property.UnitInput{
		Label: "Kamar 1", Terms: []property.TermInput{{Months: 1, Price: price(850_000)}, {Months: 12}},
	})
	require.NoError(t, err)

	t.Run("a new price from a date", func(t *testing.T) {
		tests := []struct {
			name    string
			in      property.PriceInput
			wantErr error
		}{
			{"from a later day", property.PriceInput{Months: 1, Amount: 900_000, EffectiveFrom: day(2027, 1, 1)}, nil},
			{"from today", property.PriceInput{Months: 12, Amount: 9_000_000, EffectiveFrom: day(2026, 11, 2)}, nil},
			{"from yesterday", property.PriceInput{Months: 1, Amount: 900_000, EffectiveFrom: day(2026, 11, 1)}, property.ErrPriceTooEarly},
			{"for a term the unit does not offer", property.PriceInput{Months: 3, Amount: 2_400_000, EffectiveFrom: day(2027, 1, 1)}, tenant.ErrTermNotOffered},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				got, err := env.Properties.CreateUnitPrice(ctx, admin, kamar1.ID, tc.in)
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.in.Months, got.Months)
				require.Equal(t, tc.in.Amount, got.Amount)
				require.Equal(t, tc.in.EffectiveFrom, got.EffectiveFrom)
				require.Equal(t, o.Admin.Name, *got.RecordedByName)
			})
		}
	})

	t.Run("a term not offered names the months field", func(t *testing.T) {
		_, err := env.Properties.CreateUnitPrice(ctx, admin, kamar1.ID, property.PriceInput{Months: 6, Amount: 1, EffectiveFrom: day(2027, 1, 1)})
		var termErr *tenant.TermError
		require.ErrorAs(t, err, &termErr)
		require.Equal(t, "months", termErr.Field)
		require.Equal(t, 6, termErr.Months)
	})

	t.Run("bad input names the field", func(t *testing.T) {
		for field, in := range map[string]property.PriceInput{
			"months":        {Months: 61, Amount: 1, EffectiveFrom: day(2027, 1, 1)},
			"amount":        {Months: 1, EffectiveFrom: day(2027, 1, 1)},
			"effectiveFrom": {Months: 1, Amount: 1},
		} {
			_, err := env.Properties.CreateUnitPrice(ctx, admin, kamar1.ID, in)
			var inputErr *property.InputError
			require.ErrorAs(t, err, &inputErr, field)
			require.Equal(t, field, inputErr.Field)
		}
	})

	t.Run("the history is newest first, with who recorded each row", func(t *testing.T) {
		// A second price on the same later day: the later recorded comes first and wins.
		_, err := env.Properties.CreateUnitPrice(ctx, admin, kamar1.ID, property.PriceInput{Months: 1, Amount: 950_000, EffectiveFrom: day(2027, 1, 1)})
		require.NoError(t, err)
		history, err := env.Properties.ListUnitPrices(ctx, admin, kamar1.ID)
		require.NoError(t, err)
		type row struct {
			Months int
			Amount int64
			From   time.Time
		}
		var got []row
		for _, p := range history {
			got = append(got, row{p.Months, p.Amount, p.EffectiveFrom})
			require.Equal(t, o.Admin.Name, *p.RecordedByName)
			require.False(t, p.RecordedAt.IsZero())
		}
		require.Equal(t, []row{
			{1, 950_000, day(2027, 1, 1)},
			{1, 900_000, day(2027, 1, 1)},
			{12, 9_000_000, day(2026, 11, 2)},
			{1, 850_000, day(2026, 11, 2)},
		}, got)
	})

	t.Run("a unit shows today's price and the next one", func(t *testing.T) {
		units, err := env.AllUnits(ctx, admin, melati.ID)
		require.NoError(t, err)
		require.Equal(t, []property.UnitTerm{
			{Months: 1, Price: price(850_000), Next: &property.ScheduledPrice{Amount: 950_000, EffectiveFrom: day(2027, 1, 1)}},
			{Months: 12, Price: price(9_000_000)},
		}, units[0].Terms)

		env.Clock.Set(time.Date(2027, 1, 1, 0, 30, 0, 0, clock.Jakarta))
		t.Cleanup(func() { env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta)) })
		u, err := env.Properties.UpdateUnit(ctx, admin, kamar1.ID, property.UnitInput{Label: "Kamar 1"})
		require.NoError(t, err)
		require.Equal(t, []property.UnitTerm{{Months: 1, Price: price(950_000)}, {Months: 12, Price: price(9_000_000)}}, u.Terms,
			"on 1 January in Jakarta the scheduled price is in force")
	})

	t.Run("an unknown unit is not found", func(t *testing.T) {
		_, err := env.Properties.ListUnitPrices(ctx, admin, melati.ID)
		require.ErrorIs(t, err, property.ErrNotFound)
	})
}
