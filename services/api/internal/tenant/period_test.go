package tenant

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPeriodAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		start, enteredOn time.Time
		term, n          int
		from, to, due    time.Time
	}{
		{"monthly, first period", date(2026, 10, 15), date(2026, 10, 15), 1, 0, date(2026, 10, 15), date(2026, 11, 14), date(2026, 10, 15)},
		{"monthly, second period", date(2026, 10, 15), date(2026, 10, 15), 1, 1, date(2026, 11, 15), date(2026, 12, 14), date(2026, 11, 15)},
		{"3 months from 15 Oct", date(2026, 10, 15), date(2026, 10, 15), 3, 0, date(2026, 10, 15), date(2027, 1, 14), date(2026, 10, 15)},
		{"3 months, second period", date(2026, 10, 15), date(2026, 10, 15), 3, 1, date(2027, 1, 15), date(2027, 4, 14), date(2027, 1, 15)},
		{"6 months across the year", date(2026, 10, 15), date(2026, 10, 15), 6, 1, date(2027, 4, 15), date(2027, 10, 14), date(2027, 4, 15)},
		{"12 months", date(2026, 10, 15), date(2026, 10, 15), 12, 0, date(2026, 10, 15), date(2027, 10, 14), date(2026, 10, 15)},
		{"12 months, second year", date(2026, 10, 15), date(2026, 10, 15), 12, 1, date(2027, 10, 15), date(2028, 10, 14), date(2027, 10, 15)},
		{"monthly from 31 Jan ends 27 Feb", date(2027, 1, 31), date(2027, 1, 31), 1, 0, date(2027, 1, 31), date(2027, 2, 27), date(2027, 1, 31)},
		{"monthly from 31 Jan, February period", date(2027, 1, 31), date(2027, 1, 31), 1, 1, date(2027, 2, 28), date(2027, 3, 30), date(2027, 2, 28)},
		{"monthly from 31 Jan in a leap year", date(2028, 1, 31), date(2028, 1, 31), 1, 1, date(2028, 2, 29), date(2028, 3, 30), date(2028, 2, 29)},
		{"3 months from 31 Jan: 30 Apr, counted from the start", date(2027, 1, 31), date(2027, 1, 31), 3, 1, date(2027, 4, 30), date(2027, 7, 30), date(2027, 4, 30)},
		{"3 months from 31 Jan: then 31 Jul", date(2027, 1, 31), date(2027, 1, 31), 3, 2, date(2027, 7, 31), date(2027, 10, 30), date(2027, 7, 31)},
		{"3 months from 30 Nov: 28 Feb", date(2026, 11, 30), date(2026, 11, 30), 3, 1, date(2027, 2, 28), date(2027, 5, 29), date(2027, 2, 28)},
		{"3 months from 30 Nov in a leap year: 29 Feb", date(2027, 11, 30), date(2027, 11, 30), 3, 1, date(2028, 2, 29), date(2028, 5, 29), date(2028, 2, 29)},
		{"12 months from 29 Feb: 28 Feb next year", date(2028, 2, 29), date(2028, 2, 29), 12, 1, date(2029, 2, 28), date(2030, 2, 27), date(2029, 2, 28)},
		{"12 months from 29 Feb: 29 Feb four years on", date(2028, 2, 29), date(2028, 2, 29), 12, 4, date(2032, 2, 29), date(2033, 2, 27), date(2032, 2, 29)},
		{"a booking: due on its start, not the entry day", date(2026, 11, 15), date(2026, 10, 28), 1, 0, date(2026, 11, 15), date(2026, 12, 14), date(2026, 11, 15)},
		// Decision 24's example: started 15 Sep, entered 28 Oct.
		{"late monthly: a past period is due on the entry day", date(2026, 9, 15), date(2026, 10, 28), 1, 0, date(2026, 9, 15), date(2026, 10, 14), date(2026, 10, 28)},
		{"late monthly: the running period is due 7 days after entry", date(2026, 9, 15), date(2026, 10, 28), 1, 1, date(2026, 10, 15), date(2026, 11, 14), date(2026, 11, 4)},
		{"late monthly: the next period is due as usual", date(2026, 9, 15), date(2026, 10, 28), 1, 2, date(2026, 11, 15), date(2026, 12, 14), date(2026, 11, 15)},
		{"late 3 months: the running period is due 7 days after entry", date(2026, 9, 15), date(2026, 10, 28), 3, 0, date(2026, 9, 15), date(2026, 12, 14), date(2026, 11, 4)},
		{"late 12 months: a past year is due on the entry day", date(2025, 3, 1), date(2026, 10, 28), 12, 0, date(2025, 3, 1), date(2026, 2, 28), date(2026, 10, 28)},
		{"late 12 months: the running year is due 7 days after entry", date(2025, 3, 1), date(2026, 10, 28), 12, 1, date(2026, 3, 1), date(2027, 2, 28), date(2026, 11, 4)},
		{"late: a period starting on the entry day is the current one, due 7 days after", date(2026, 9, 15), date(2026, 10, 15), 1, 1, date(2026, 10, 15), date(2026, 11, 14), date(2026, 10, 22)},
		{"late: entered a day later, the same period is due 7 days after that", date(2026, 9, 15), date(2026, 10, 16), 1, 1, date(2026, 10, 15), date(2026, 11, 14), date(2026, 10, 23)},
		{"late: the period after the current one is due as usual", date(2026, 9, 15), date(2026, 10, 15), 1, 2, date(2026, 11, 15), date(2026, 12, 14), date(2026, 11, 15)},
		{"on time: a lease entered on its start date is due that day", date(2026, 10, 15), date(2026, 10, 15), 12, 0, date(2026, 10, 15), date(2027, 10, 14), date(2026, 10, 15)},
		{"late: the period that ended the day before entry is due on entry", date(2026, 9, 15), date(2026, 10, 15), 1, 0, date(2026, 9, 15), date(2026, 10, 14), date(2026, 10, 15)},
		{"a time of day is ignored", time.Date(2026, 10, 15, 22, 0, 0, 0, time.UTC), time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC), 3, 0, date(2026, 10, 15), date(2027, 1, 14), date(2026, 10, 15)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := PeriodAt(tc.start, tc.enteredOn, tc.term, tc.n)
			require.Equal(t, Period{N: tc.n, From: tc.from, To: tc.to, Due: tc.due}, got)
		})
	}
}

func TestPeriodsTileWithoutGapsOrOverlap(t *testing.T) {
	t.Parallel()
	starts := []time.Time{date(2026, 10, 15), date(2027, 1, 31), date(2026, 8, 30), date(2028, 2, 29), date(2026, 12, 1)}
	for _, start := range starts {
		for _, term := range TermMonths {
			prev := PeriodAt(start, start, term, 0)
			require.Equal(t, start, prev.From)
			for n := 1; n < 30; n++ {
				p := PeriodAt(start, start, term, n)
				require.Equal(t, prev.To.AddDate(0, 0, 1), p.From, "start %s term %d n %d", start.Format(time.DateOnly), term, n)
				require.True(t, p.To.After(p.From))
				prev = p
			}
		}
	}
}

func TestPeriodIndex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		start time.Time
		term  int
		d     time.Time
		want  int
	}{
		{"before the start date", date(2026, 10, 15), 1, date(2026, 10, 14), -1},
		{"the start date", date(2026, 10, 15), 3, date(2026, 10, 15), 0},
		{"the last day of the first quarter", date(2026, 10, 15), 3, date(2027, 1, 14), 0},
		{"the first day of the second quarter", date(2026, 10, 15), 3, date(2027, 1, 15), 1},
		{"a day early in the month, monthly", date(2026, 10, 15), 1, date(2026, 12, 3), 1},
		{"from 31 Jan, on 28 Feb", date(2027, 1, 31), 1, date(2027, 2, 28), 1},
		{"from 31 Jan, on 27 Feb", date(2027, 1, 31), 1, date(2027, 2, 27), 0},
		{"from 31 Jan, quarterly, on 30 Apr", date(2027, 1, 31), 3, date(2027, 4, 30), 1},
		{"from 31 Jan, quarterly, on 29 Apr", date(2027, 1, 31), 3, date(2027, 4, 29), 0},
		{"yearly, years later", date(2026, 10, 15), 12, date(2031, 10, 16), 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, PeriodIndex(tc.start, tc.term, tc.d))
		})
	}
}

func TestIsPeriodStartAndNextPeriodStart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		start   time.Time
		term    int
		d       time.Time
		isStart bool
		next    time.Time
	}{
		{"before the start date", date(2026, 10, 15), 3, date(2026, 10, 1), false, date(2026, 10, 15)},
		{"the start date", date(2026, 10, 15), 3, date(2026, 10, 15), true, date(2027, 1, 15)},
		{"a monthly anniversary inside a quarter", date(2026, 10, 15), 3, date(2026, 11, 15), false, date(2027, 1, 15)},
		{"a quarter start", date(2026, 10, 15), 3, date(2027, 1, 15), true, date(2027, 4, 15)},
		{"a 6-month start", date(2026, 10, 15), 6, date(2027, 4, 15), true, date(2027, 10, 15)},
		{"a year start", date(2026, 10, 15), 12, date(2027, 10, 15), true, date(2028, 10, 15)},
		{"not a year start", date(2026, 10, 15), 12, date(2027, 4, 15), false, date(2027, 10, 15)},
		{"a clamped quarter start from 31 Jan", date(2027, 1, 31), 3, date(2027, 4, 30), true, date(2027, 7, 31)},
		{"monthly matches the anniversary helpers", date(2027, 1, 31), 1, date(2027, 2, 28), true, date(2027, 3, 31)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.isStart, IsPeriodStart(tc.start, tc.term, tc.d))
			require.Equal(t, tc.next, NextPeriodStart(tc.start, tc.term, tc.d))
			if tc.term == 1 {
				require.Equal(t, IsAnniversary(tc.start, tc.d), IsPeriodStart(tc.start, 1, tc.d))
				require.Equal(t, FirstAnniversaryAfter(tc.start, tc.d), NextPeriodStart(tc.start, 1, tc.d))
			}
		})
	}
}

func TestValidTerm(t *testing.T) {
	t.Parallel()
	for _, m := range []int{1, 3, 6, 12} {
		require.True(t, ValidTerm(m), m)
	}
	for _, m := range []int{0, 2, 4, 24, -1} {
		require.False(t, ValidTerm(m), m)
	}
}
