package demoseed

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

func TestBilledFrom(t *testing.T) {
	t.Parallel()
	today := day(2026, time.November, 2) // today - 70 days is 24 August
	tests := []struct {
		name  string
		start time.Time
		term  int
		want  time.Time // zero means the lease's own start date
	}{
		{name: "monthly lease bills from the period running two months back", start: day(2026, 7, 1), term: 1, want: day(2026, 8, 1)},
		{name: "a lease from last year bills from the same period", start: day(2025, 11, 1), term: 1, want: day(2026, 8, 1)},
		{name: "a mid-month start keeps its own day", start: day(2026, 3, 15), term: 1, want: day(2026, 8, 15)},
		{name: "a yearly lease bills only its current year", start: day(2026, 1, 1), term: 12, want: day(2026, 1, 1)},
		{name: "a quarterly lease bills from the quarter running then", start: day(2026, 1, 10), term: 3, want: day(2026, 7, 10)},
		{name: "a start on the 31st is counted from the start date", start: day(2026, 1, 31), term: 1, want: day(2026, 7, 31)},
		{name: "a lease that began after that day bills from its start", start: day(2026, 10, 1), term: 1},
		{name: "a booking bills from its start", start: day(2027, 6, 1), term: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := billedFrom(tc.start, tc.term, today)
			require.Equal(t, tc.want, got)
			if got.IsZero() {
				return
			}
			from := today.AddDate(0, 0, -EntryDays) // the entry day, or the start when later
			if tc.start.After(from) {
				from = tc.start
			}
			require.True(t, tenant.IsPeriodStart(tc.start, tc.term, got), "a period start of the lease's term")
			require.False(t, got.Before(tc.start))
			require.False(t, got.After(tenant.NextPeriodStart(tc.start, tc.term, from)),
				"no later than the lease service allows on the entry day")
		})
	}
}

func TestBehind(t *testing.T) {
	t.Parallel()
	fake := clock.NewFake(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	past := Behind(fake, EntryDays)
	require.Equal(t, day(2026, 9, 23), tenant.Today(past))
	fake.Advance(24 * time.Hour)
	require.Equal(t, day(2026, 9, 24), tenant.Today(past), "it follows the clock it wraps")
}
