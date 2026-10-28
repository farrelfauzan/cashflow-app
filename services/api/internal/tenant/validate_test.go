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
		{"minimal terms", Terms{StartDate: start, RentAmount: 1}, ""},
		{"with a deposit", Terms{StartDate: start, RentAmount: 850_000, DepositAmount: 850_000}, ""},
		{"no start date", Terms{RentAmount: 1}, "startDate"},
		{"free rent", Terms{StartDate: start}, "rentAmount"},
		{"negative deposit", Terms{StartDate: start, RentAmount: 1, DepositAmount: -1}, "depositAmount"},
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

func TestMoveOutDates(t *testing.T) {
	t.Parallel()
	d := func(month, day int) time.Time { return time.Date(2026, time.Month(month), day, 0, 0, 0, 0, time.UTC) }
	now := d(11, 2)
	for _, tc := range []struct {
		name       string
		start, end time.Time
		ok         bool
	}{
		{"today", d(10, 1), d(11, 2), true},
		{"on the start day", d(10, 1), d(10, 1), true},
		{"before the start", d(10, 1), d(9, 30), false},
		{"tomorrow", d(10, 1), d(11, 3), false},
		{"a lease starting today ends today", d(11, 2), d(11, 2), true},
		{"a booking is cancelled on its start date", d(11, 10), d(11, 10), true},
		{"a booking cannot end today", d(11, 10), d(11, 2), false},
		{"a booking cannot end after its start", d(11, 10), d(11, 11), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkEndDate(tc.start, tc.end, now)
			if tc.ok {
				require.NoError(t, err)
				return
			}
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, "endDate", inputErr.Field)
		})
	}
}

func TestValidTermsBilledFrom(t *testing.T) {
	t.Parallel()
	start := date(2026, 9, 15)
	tests := []struct {
		name       string
		start      time.Time
		billedFrom time.Time
		want       time.Time
		wantField  string
	}{
		{"left out defaults to the start date", start, time.Time{}, start, ""},
		{"the start date itself", start, start, start, ""},
		{"a later anniversary", start, date(2026, 11, 15), date(2026, 11, 15), ""},
		{"a time of day is dropped", start, time.Date(2026, 10, 15, 17, 0, 0, 0, time.UTC), date(2026, 10, 15), ""},
		{"a month-end start allows the clamped day", date(2027, 1, 31), date(2027, 2, 28), date(2027, 2, 28), ""},
		{"not an anniversary", start, date(2026, 10, 20), time.Time{}, "billedFrom"},
		{"the day before an anniversary", start, date(2026, 10, 14), time.Time{}, "billedFrom"},
		{"before the start date", start, date(2026, 8, 15), time.Time{}, "billedFrom"},
		{"the day before the start date", start, date(2026, 9, 14), time.Time{}, "billedFrom"},
		{"a month-end start refuses the middle of the month", date(2027, 1, 31), date(2027, 2, 15), time.Time{}, "billedFrom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidTerms(Terms{StartDate: tc.start, RentAmount: 850_000, BilledFrom: tc.billedFrom})
			if tc.wantField == "" {
				require.NoError(t, err)
				require.Equal(t, tc.want, got.BilledFrom)
				return
			}
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, tc.wantField, inputErr.Field)
		})
	}
}

func TestCheckBilledFrom(t *testing.T) {
	t.Parallel()
	today := date(2026, 11, 2)
	tests := []struct {
		name       string
		start      time.Time
		billedFrom time.Time
		enteredOn  time.Time
		wantLatest string // "" when allowed, else the date the message names
	}{
		{"late lease: the start date", date(2026, 9, 15), date(2026, 9, 15), today, ""},
		{"late lease: a past period", date(2026, 9, 15), date(2026, 10, 15), today, ""},
		{"late lease: the current period", date(2026, 9, 15), date(2026, 11, 15), today, ""},
		{"late lease: one period too far", date(2026, 9, 15), date(2026, 12, 15), today, "15/11/2026"},
		{"late lease: far in the future", date(2026, 9, 15), date(2027, 9, 15), today, "15/11/2026"},
		{"booking: the start date", date(2026, 12, 1), date(2026, 12, 1), today, ""},
		{"booking: the second period", date(2026, 12, 1), date(2027, 1, 1), today, ""},
		{"booking: the third period", date(2026, 12, 1), date(2027, 2, 1), today, "01/01/2027"},
		{"starting today: the start date", today, today, today, ""},
		{"starting today: the next period", today, date(2026, 12, 2), today, ""},
		{"starting today: two periods ahead", today, date(2027, 1, 2), today, "02/12/2026"},
		{"entered on an anniversary: the next one is allowed", date(2026, 9, 15), date(2026, 12, 15), date(2026, 11, 15), ""},
		{"entered on an anniversary: not the one after", date(2026, 9, 15), date(2027, 1, 15), date(2026, 11, 15), "15/12/2026"},
		{"month-end start: the clamped current period", date(2026, 8, 31), date(2026, 11, 30), today, ""},
		{"month-end start: past the clamped current period", date(2026, 8, 31), date(2026, 12, 31), today, "30/11/2026"},
		{"entered earlier: bounded by the stored day, not today", date(2026, 9, 15), date(2026, 11, 15), date(2026, 9, 20), "15/10/2026"},
		{"entered earlier: the stored day's current period", date(2026, 9, 15), date(2026, 10, 15), date(2026, 9, 20), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkBilledFrom(Terms{StartDate: tc.start, BilledFrom: tc.billedFrom}, tc.enteredOn)
			if tc.wantLatest == "" {
				require.NoError(t, err)
				return
			}
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, "billedFrom", inputErr.Field)
			require.Contains(t, inputErr.Message, tc.wantLatest)
		})
	}
}

func TestCheckPlannedEnd(t *testing.T) {
	t.Parallel()
	now := date(2026, 11, 2)
	tests := []struct {
		name       string
		start, end time.Time
		ok         bool
	}{
		{"today", date(2026, 10, 1), date(2026, 11, 2), true},
		{"a later day", date(2026, 10, 1), date(2026, 11, 14), true},
		{"yesterday", date(2026, 10, 1), date(2026, 11, 1), false},
		{"before the start", date(2026, 10, 1), date(2026, 9, 30), false},
		{"a booking planned to end on its start date", date(2026, 12, 10), date(2026, 12, 10), true},
		{"a booking planned to end before its start", date(2026, 12, 10), date(2026, 12, 9), false},
		{"a lease starting today, ending today", now, now, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkPlannedEnd(tc.start, tc.end, now)
			if tc.ok {
				require.NoError(t, err)
				return
			}
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, "plannedEndDate", inputErr.Field)
		})
	}
}
