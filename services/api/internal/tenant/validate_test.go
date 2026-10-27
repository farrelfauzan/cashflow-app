package tenant

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
)

func TestValidTerms(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		in        Terms
		wantField string
	}{
		{"minimal terms", Terms{StartDate: start, RentAmount: 1, DueDay: 1}, ""},
		{"due on the 28th", Terms{StartDate: start, RentAmount: 850_000, DueDay: 28, DepositAmount: 850_000}, ""},
		{"no start date", Terms{RentAmount: 1, DueDay: 1}, "startDate"},
		{"free rent", Terms{StartDate: start, DueDay: 1}, "rentAmount"},
		{"due day 0", Terms{StartDate: start, RentAmount: 1}, "dueDay"},
		{"due day 29 does not exist every month", Terms{StartDate: start, RentAmount: 1, DueDay: 29}, "dueDay"},
		{"negative deposit", Terms{StartDate: start, RentAmount: 1, DueDay: 1, DepositAmount: -1}, "depositAmount"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ValidTerms(tc.in)
			if tc.wantField == "" {
				require.NoError(t, err)
				return
			}
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, tc.wantField, inputErr.Field)
		})
	}
}

func TestValidTenant(t *testing.T) {
	t.Parallel()
	got, err := ValidTenant(Input{Name: " Rina ", Phone: "0812 3456 7890", Note: " pindahan "})
	require.NoError(t, err)
	require.Equal(t, Input{Name: "Rina", Phone: "+6281234567890", Note: "pindahan"}, got)

	for field, in := range map[string]Input{
		"name":  {Name: strings.Repeat("a", 101), Phone: "081234567890"},
		"phone": {Name: "Rina", Phone: "021-555-1234"},
		"note":  {Name: "Rina", Phone: "081234567890", Note: strings.Repeat("a", 501)},
	} {
		_, err := ValidTenant(in)
		var inputErr *InputError
		require.ErrorAs(t, err, &inputErr, field)
		require.Equal(t, field, inputErr.Field)
	}
}

func TestTodayIsTheJakartaCalendarDay(t *testing.T) {
	t.Parallel()
	// 23:30 UTC on 1 November is already 06:30 on 2 November in Jakarta.
	c := clock.NewFake(time.Date(2026, 11, 1, 23, 30, 0, 0, time.UTC))
	require.Equal(t, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), today(c))
	require.Equal(t, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		calendarDay(time.Date(2026, 11, 2, 23, 59, 0, 0, clock.Jakarta)), "a date keeps its own calendar day")
}
