package billing_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
)

func TestFieldReasonAt(t *testing.T) {
	t.Parallel()
	date := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	jakarta := func(y int, m time.Month, d, hour, minute int) time.Time {
		return time.Date(y, m, d, hour, minute, 0, 0, clock.Jakarta)
	}
	tests := []struct {
		name   string
		due    time.Time
		now    time.Time
		want   billing.FieldReason
		listed bool
	}{
		{name: "due today", due: date(2026, 11, 14), now: jakarta(2026, 11, 14, 9, 0),
			want: billing.FieldReason{Code: billing.FieldDueToday}, listed: true},
		{name: "due tomorrow", due: date(2026, 11, 15), now: jakarta(2026, 11, 14, 9, 0),
			want: billing.FieldReason{Code: billing.FieldDueInDays, Days: 1}, listed: true},
		{name: "due the day after tomorrow", due: date(2026, 11, 16), now: jakarta(2026, 11, 14, 9, 0),
			want: billing.FieldReason{Code: billing.FieldDueInDays, Days: 2}, listed: true},
		{name: "due in three days is not listed", due: date(2026, 11, 17), now: jakarta(2026, 11, 14, 9, 0)},
		{name: "one day overdue on the day after the due date", due: date(2026, 11, 13), now: jakarta(2026, 11, 14, 9, 0),
			want: billing.FieldReason{Code: billing.FieldOverdue, Days: 1}, listed: true},
		{name: "overdue across a month end", due: date(2026, 10, 28), now: jakarta(2026, 11, 4, 9, 0),
			want: billing.FieldReason{Code: billing.FieldOverdue, Days: 7}, listed: true},
		{name: "due in two days across a month end", due: date(2026, 12, 1), now: jakarta(2026, 11, 29, 9, 0),
			want: billing.FieldReason{Code: billing.FieldDueInDays, Days: 2}, listed: true},
		{name: "due in two days across a year end", due: date(2027, 1, 1), now: jakarta(2026, 12, 30, 9, 0),
			want: billing.FieldReason{Code: billing.FieldDueInDays, Days: 2}, listed: true},
		{name: "overdue across February", due: date(2027, 2, 28), now: jakarta(2027, 3, 1, 9, 0),
			want: billing.FieldReason{Code: billing.FieldOverdue, Days: 1}, listed: true},
		{name: "one minute before midnight in Jakarta it is still due today", due: date(2026, 11, 30), now: jakarta(2026, 11, 30, 23, 59),
			want: billing.FieldReason{Code: billing.FieldDueToday}, listed: true},
		{name: "at midnight in Jakarta it is a day overdue", due: date(2026, 11, 30), now: jakarta(2026, 12, 1, 0, 0),
			want: billing.FieldReason{Code: billing.FieldOverdue, Days: 1}, listed: true},
		{name: "the Jakarta day, not the UTC day, decides (07:00 Jakarta is 00:00 UTC the same day)",
			due: date(2026, 11, 15), now: jakarta(2026, 11, 15, 6, 59),
			want: billing.FieldReason{Code: billing.FieldDueToday}, listed: true},
		{name: "a UTC moment late in the evening is already tomorrow in Jakarta",
			due: date(2026, 11, 15), now: time.Date(2026, 11, 14, 18, 0, 0, 0, time.UTC),
			want: billing.FieldReason{Code: billing.FieldDueToday}, listed: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, listed := billing.FieldReasonAt(tc.due, tc.now)
			require.Equal(t, tc.listed, listed)
			require.Equal(t, tc.want, got)
		})
	}
}
