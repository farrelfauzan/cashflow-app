package tenant

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestAnniversary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		start time.Time
		n     int
		want  time.Time
	}{
		{"n=0 is the start date", date(2026, 10, 15), 0, date(2026, 10, 15)},
		{"n=0 on a 31st is the start date", date(2027, 1, 31), 0, date(2027, 1, 31)},
		{"the next month, same day", date(2026, 10, 15), 1, date(2026, 11, 15)},
		{"across the new year", date(2026, 12, 15), 2, date(2027, 2, 15)},
		{"a whole year later", date(2026, 10, 15), 12, date(2027, 10, 15)},
		{"31 Jan clamps to 28 Feb", date(2027, 1, 31), 1, date(2027, 2, 28)},
		{"31 Jan in a leap year clamps to 29 Feb", date(2028, 1, 31), 1, date(2028, 2, 29)},
		{"30 Jan clamps to 28 Feb", date(2027, 1, 30), 1, date(2027, 2, 28)},
		{"29 Jan clamps to 28 Feb", date(2027, 1, 29), 1, date(2027, 2, 28)},
		{"29 Jan in a leap year is 29 Feb", date(2028, 1, 29), 1, date(2028, 2, 29)},
		{"counted from the start, not the previous anniversary", date(2027, 1, 31), 2, date(2027, 3, 31)},
		{"31 Aug clamps to 30 Sep", date(2026, 8, 31), 1, date(2026, 9, 30)},
		{"29 Feb start in a common year is 28 Feb", date(2028, 2, 29), 12, date(2029, 2, 28)},
		{"29 Feb start in the next leap year is 29 Feb", date(2028, 2, 29), 48, date(2032, 2, 29)},
		{"a time of day is ignored", time.Date(2026, 10, 15, 23, 59, 0, 0, time.UTC), 1, date(2026, 11, 15)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, Anniversary(tc.start, tc.n))
		})
	}

	t.Run("a 31st start through a whole year", func(t *testing.T) {
		t.Parallel()
		start := date(2027, 1, 31)
		want := []time.Time{
			date(2027, 1, 31), date(2027, 2, 28), date(2027, 3, 31), date(2027, 4, 30),
			date(2027, 5, 31), date(2027, 6, 30), date(2027, 7, 31), date(2027, 8, 31),
			date(2027, 9, 30), date(2027, 10, 31), date(2027, 11, 30), date(2027, 12, 31),
			date(2028, 1, 31),
		}
		for n, w := range want {
			require.Equal(t, w, Anniversary(start, n), "n=%d", n)
		}
	})
}

func TestIsAnniversary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		start time.Time
		d     time.Time
		want  bool
	}{
		{"the start date itself", date(2026, 9, 15), date(2026, 9, 15), true},
		{"the next month", date(2026, 9, 15), date(2026, 10, 15), true},
		{"a year later", date(2026, 9, 15), date(2027, 9, 15), true},
		{"the day before an anniversary", date(2026, 9, 15), date(2026, 10, 14), false},
		{"the day after an anniversary", date(2026, 9, 15), date(2026, 10, 16), false},
		{"another day of the month", date(2026, 9, 15), date(2026, 10, 20), false},
		{"before the start, even on the same day of an earlier month", date(2026, 9, 15), date(2026, 8, 15), false},
		{"the day before the start", date(2026, 9, 15), date(2026, 9, 14), false},
		{"a month-end start: 28 Feb is its anniversary", date(2027, 1, 31), date(2027, 2, 28), true},
		{"a month-end start: 15 Feb is not", date(2027, 1, 31), date(2027, 2, 15), false},
		{"a month-end start: 27 Feb is not", date(2027, 1, 31), date(2027, 2, 27), false},
		{"a month-end start: 31 Mar is back on the 31st", date(2027, 1, 31), date(2027, 3, 31), true},
		{"a month-end start: 30 Mar is not", date(2027, 1, 31), date(2027, 3, 30), false},
		{"a month-end start: 30 Apr is its anniversary", date(2027, 1, 31), date(2027, 4, 30), true},
		{"a leap year: 29 Feb is an anniversary of 31 Jan", date(2028, 1, 31), date(2028, 2, 29), true},
		{"a leap year: 28 Feb is not", date(2028, 1, 31), date(2028, 2, 28), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, IsAnniversary(tc.start, tc.d))
		})
	}
}

func TestFirstAnniversaryAfter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		start time.Time
		d     time.Time
		want  time.Time
	}{
		{"d before the start is the start", date(2026, 12, 1), date(2026, 11, 2), date(2026, 12, 1)},
		{"d the day before the start is the start", date(2026, 12, 1), date(2026, 11, 30), date(2026, 12, 1)},
		{"d on the start is the next anniversary", date(2026, 12, 1), date(2026, 12, 1), date(2027, 1, 1)},
		{"d on an anniversary is the next one", date(2026, 9, 15), date(2026, 10, 15), date(2026, 11, 15)},
		{"d between two anniversaries", date(2026, 9, 15), date(2026, 11, 2), date(2026, 11, 15)},
		{"d the day before an anniversary", date(2026, 9, 15), date(2026, 10, 14), date(2026, 10, 15)},
		{"d the day after an anniversary", date(2026, 9, 15), date(2026, 10, 16), date(2026, 11, 15)},
		{"across the new year", date(2026, 11, 20), date(2026, 12, 25), date(2027, 1, 20)},
		{"a month-end start: d on 31 Jan", date(2027, 1, 31), date(2027, 1, 31), date(2027, 2, 28)},
		{"a month-end start: d on 15 Feb", date(2027, 1, 31), date(2027, 2, 15), date(2027, 2, 28)},
		{"a month-end start: d on 28 Feb", date(2027, 1, 31), date(2027, 2, 28), date(2027, 3, 31)},
		{"a month-end start: d on 1 Mar", date(2027, 1, 31), date(2027, 3, 1), date(2027, 3, 31)},
		{"a month-end start: d on 31 Mar", date(2027, 1, 31), date(2027, 3, 31), date(2027, 4, 30)},
		{"a leap year: d on 29 Feb", date(2028, 1, 31), date(2028, 2, 29), date(2028, 3, 31)},
		{"a leap year: d on 28 Feb", date(2028, 1, 31), date(2028, 2, 28), date(2028, 2, 29)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FirstAnniversaryAfter(tc.start, tc.d)
			require.Equal(t, tc.want, got)
			if !tc.d.Before(tc.start) {
				require.True(t, got.After(tc.d), "strictly after d")
			}
			require.True(t, IsAnniversary(tc.start, got), "the result is an anniversary")
		})
	}
}
