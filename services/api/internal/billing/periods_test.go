package billing_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

func d(y int, m time.Month, dd int) time.Time { return time.Date(y, m, dd, 0, 0, 0, 0, time.UTC) }

func ptr(t time.Time) *time.Time { return &t }

func TestDuePeriods(t *testing.T) {
	t.Parallel()
	// period is a period as tenant.PeriodAt returns it.
	type period struct {
		n             int
		from, to, due time.Time
	}
	monthly := func(start, entered time.Time) billing.LeaseDates {
		return billing.LeaseDates{Start: start, EnteredOn: entered, BilledFrom: start, TermMonths: 1}
	}
	tests := []struct {
		name  string
		lease billing.LeaseDates
		today time.Time
		want  []period
	}{
		{
			name:  "a lease saved on its start date is invoiced that day, due that day",
			lease: monthly(d(2026, 10, 28), d(2026, 10, 28)),
			today: d(2026, 10, 28),
			want:  []period{{0, d(2026, 10, 28), d(2026, 11, 27), d(2026, 10, 28)}},
		},
		{
			name:  "the next period is not due within the lead time on 20 Nov",
			lease: monthly(d(2026, 10, 28), d(2026, 10, 28)),
			today: d(2026, 11, 20),
			want:  []period{{0, d(2026, 10, 28), d(2026, 11, 27), d(2026, 10, 28)}},
		},
		{
			name:  "the next period is due within the lead time on 21 Nov",
			lease: monthly(d(2026, 10, 28), d(2026, 10, 28)),
			today: d(2026, 11, 21),
			want: []period{
				{0, d(2026, 10, 28), d(2026, 11, 27), d(2026, 10, 28)},
				{1, d(2026, 11, 28), d(2026, 12, 27), d(2026, 11, 28)},
			},
		},
		{
			name:  "a booking starting 1 Dec is not invoiced on 23 Nov",
			lease: monthly(d(2026, 12, 1), d(2026, 10, 28)),
			today: d(2026, 11, 23),
		},
		{
			name:  "a booking starting 1 Dec is invoiced on 24 Nov",
			lease: monthly(d(2026, 12, 1), d(2026, 10, 28)),
			today: d(2026, 11, 24),
			want:  []period{{0, d(2026, 12, 1), d(2026, 12, 31), d(2026, 12, 1)}},
		},
		{
			name:  "a lease entered late has its past period due on entry and the current one 7 days after",
			lease: monthly(d(2026, 9, 15), d(2026, 10, 28)),
			today: d(2026, 10, 28),
			want: []period{
				{0, d(2026, 9, 15), d(2026, 10, 14), d(2026, 10, 28)},
				{1, d(2026, 10, 15), d(2026, 11, 14), d(2026, 11, 4)},
			},
		},
		{
			name: "billed from 15 Oct skips the period the owner collected by hand",
			lease: billing.LeaseDates{
				Start: d(2026, 9, 15), EnteredOn: d(2026, 10, 28), BilledFrom: d(2026, 10, 15), TermMonths: 1,
			},
			today: d(2026, 10, 28),
			want:  []period{{1, d(2026, 10, 15), d(2026, 11, 14), d(2026, 11, 4)}},
		},
		{
			name:  "a late lease is an ordinary lease from its next anniversary",
			lease: monthly(d(2026, 9, 15), d(2026, 10, 28)),
			today: d(2026, 11, 8),
			want: []period{
				{0, d(2026, 9, 15), d(2026, 10, 14), d(2026, 10, 28)},
				{1, d(2026, 10, 15), d(2026, 11, 14), d(2026, 11, 4)},
				{2, d(2026, 11, 15), d(2026, 12, 14), d(2026, 11, 15)},
			},
		},
		{
			name: "no period that starts after the planned end date",
			lease: billing.LeaseDates{
				Start: d(2026, 10, 15), EnteredOn: d(2026, 10, 15), BilledFrom: d(2026, 10, 15), TermMonths: 1,
				PlannedEnd: ptr(d(2026, 11, 14)),
			},
			today: d(2026, 12, 8),
			want:  []period{{0, d(2026, 10, 15), d(2026, 11, 14), d(2026, 10, 15)}},
		},
		{
			name: "a period that starts on the planned end date is billed in full",
			lease: billing.LeaseDates{
				Start: d(2026, 10, 15), EnteredOn: d(2026, 10, 15), BilledFrom: d(2026, 10, 15), TermMonths: 1,
				PlannedEnd: ptr(d(2026, 11, 15)),
			},
			today: d(2026, 11, 8),
			want: []period{
				{0, d(2026, 10, 15), d(2026, 11, 14), d(2026, 10, 15)},
				{1, d(2026, 11, 15), d(2026, 12, 14), d(2026, 11, 15)},
			},
		},
		{
			name: "the earlier of the end date and the planned end date wins",
			lease: billing.LeaseDates{
				Start: d(2026, 10, 15), EnteredOn: d(2026, 10, 15), BilledFrom: d(2026, 10, 15), TermMonths: 1,
				PlannedEnd: ptr(d(2026, 12, 31)), End: ptr(d(2026, 11, 10)),
			},
			today: d(2026, 11, 10),
			want:  []period{{0, d(2026, 10, 15), d(2026, 11, 14), d(2026, 10, 15)}},
		},
		{
			name: "a cancelled booking, ended on its start date, is never invoiced",
			lease: billing.LeaseDates{
				Start: d(2026, 11, 1), EnteredOn: d(2026, 10, 28), BilledFrom: d(2026, 11, 1), TermMonths: 1,
				End: ptr(d(2026, 11, 1)),
			},
			today: d(2026, 11, 1),
		},
		{
			name: "a 3-month lease bills one period per term",
			lease: billing.LeaseDates{
				Start: d(2026, 10, 15), EnteredOn: d(2026, 10, 15), BilledFrom: d(2026, 10, 15), TermMonths: 3,
			},
			today: d(2027, 1, 8),
			want: []period{
				{0, d(2026, 10, 15), d(2027, 1, 14), d(2026, 10, 15)},
				{1, d(2027, 1, 15), d(2027, 4, 14), d(2027, 1, 15)},
			},
		},
		{
			name: "a 3-month lease with a planned end inside its first term stops after it",
			lease: billing.LeaseDates{
				Start: d(2026, 10, 15), EnteredOn: d(2026, 10, 15), BilledFrom: d(2026, 10, 15), TermMonths: 3,
				PlannedEnd: ptr(d(2027, 1, 14)),
			},
			today: d(2027, 1, 8),
			want:  []period{{0, d(2026, 10, 15), d(2027, 1, 14), d(2026, 10, 15)}},
		},
		{
			name:  "a 31 Jan start is due on the last day of February",
			lease: monthly(d(2027, 1, 31), d(2027, 1, 31)),
			today: d(2027, 2, 21),
			want: []period{
				{0, d(2027, 1, 31), d(2027, 2, 27), d(2027, 1, 31)},
				{1, d(2027, 2, 28), d(2027, 3, 30), d(2027, 2, 28)},
			},
		},
		{
			name:  "a 31 Jan start in a leap year is due on 29 February",
			lease: monthly(d(2028, 1, 31), d(2028, 1, 31)),
			today: d(2028, 2, 22),
			want: []period{
				{0, d(2028, 1, 31), d(2028, 2, 28), d(2028, 1, 31)},
				{1, d(2028, 2, 29), d(2028, 3, 30), d(2028, 2, 29)},
			},
		},
		{
			name:  "a 31 Oct start is due 30 Nov, generated on 23 Nov",
			lease: monthly(d(2026, 10, 31), d(2026, 10, 31)),
			today: d(2026, 11, 23),
			want: []period{
				{0, d(2026, 10, 31), d(2026, 11, 29), d(2026, 10, 31)},
				{1, d(2026, 11, 30), d(2026, 12, 30), d(2026, 11, 30)},
			},
		},
		{
			name:  "a 31 Oct start is not due within the lead time on 22 Nov",
			lease: monthly(d(2026, 10, 31), d(2026, 10, 31)),
			today: d(2026, 11, 22),
			want:  []period{{0, d(2026, 10, 31), d(2026, 11, 29), d(2026, 10, 31)}},
		},
		{
			name: "a custom 60-month term is one period",
			lease: billing.LeaseDates{
				Start: d(2026, 11, 2), EnteredOn: d(2026, 11, 2), BilledFrom: d(2026, 11, 2), TermMonths: 60,
			},
			today: d(2026, 11, 2),
			want:  []period{{0, d(2026, 11, 2), d(2031, 11, 1), d(2026, 11, 2)}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := billing.DuePeriods(tc.lease, tc.today)
			var want []tenant.Period
			for _, p := range tc.want {
				want = append(want, tenant.Period{N: p.n, From: p.from, To: p.to, Due: p.due})
			}
			require.Equal(t, want, got)
		})
	}
}

func TestNextInvoiceOf(t *testing.T) {
	t.Parallel()
	monthly := func(start, entered time.Time) billing.LeaseDates {
		return billing.LeaseDates{Start: start, EnteredOn: entered, BilledFrom: start, TermMonths: 1}
	}
	with := func(l billing.LeaseDates, f func(*billing.LeaseDates)) billing.LeaseDates {
		f(&l)
		return l
	}
	oct28 := monthly(d(2026, 10, 28), d(2026, 10, 28))
	tests := []struct {
		name     string
		lease    billing.LeaseDates
		today    time.Time
		invoiced []time.Time
		want     *billing.NextInvoice
	}{
		{
			name: "the next month's invoice is made seven days before it is due", lease: oct28,
			today: d(2026, 11, 3), invoiced: []time.Time{d(2026, 10, 28)},
			want: &billing.NextInvoice{Period: d(2026, 11, 28), IssuedOn: d(2026, 11, 21)},
		},
		{
			name: "on the day it is made it is still the next one", lease: oct28,
			today: d(2026, 11, 21), invoiced: []time.Time{d(2026, 10, 28)},
			want: &billing.NextInvoice{Period: d(2026, 11, 28), IssuedOn: d(2026, 11, 21)},
		},
		{
			name: "once it exists the one after it is next", lease: oct28,
			today: d(2026, 11, 21), invoiced: []time.Time{d(2026, 10, 28), d(2026, 11, 28)},
			want: &billing.NextInvoice{Period: d(2026, 12, 28), IssuedOn: d(2026, 12, 21)},
		},
		{
			name: "a lease not invoiced yet is invoiced today", lease: oct28,
			today: d(2026, 10, 28),
			want:  &billing.NextInvoice{Period: d(2026, 10, 28), IssuedOn: d(2026, 10, 28)},
		},
		{
			name: "a booking further ahead is invoiced seven days before it starts", lease: monthly(d(2026, 12, 1), d(2026, 11, 3)),
			today: d(2026, 11, 3),
			want:  &billing.NextInvoice{Period: d(2026, 12, 1), IssuedOn: d(2026, 11, 24)},
		},
		{
			name:  "a lease entered late gets its past periods at once; amounts go on the newest",
			lease: monthly(d(2026, 9, 15), d(2026, 10, 28)), today: d(2026, 10, 28),
			want: &billing.NextInvoice{Period: d(2026, 10, 15), IssuedOn: d(2026, 10, 28)},
		},
		{
			name:  "after the late entry the next is the regular one",
			lease: monthly(d(2026, 9, 15), d(2026, 10, 28)), today: d(2026, 10, 28),
			invoiced: []time.Time{d(2026, 9, 15), d(2026, 10, 15)},
			want:     &billing.NextInvoice{Period: d(2026, 11, 15), IssuedOn: d(2026, 11, 8)},
		},
		{
			name: "invoices that fell behind are made today, the amounts on the newest", lease: oct28,
			today: d(2027, 1, 22), invoiced: []time.Time{d(2026, 10, 28)},
			want: &billing.NextInvoice{Period: d(2027, 1, 28), IssuedOn: d(2027, 1, 22)},
		},
		{
			name:  "periods skipped with Mulai tagih dari are never next",
			lease: with(monthly(d(2026, 6, 15), d(2026, 10, 28)), func(l *billing.LeaseDates) { l.BilledFrom = d(2026, 10, 15) }),
			today: d(2026, 10, 28),
			want:  &billing.NextInvoice{Period: d(2026, 10, 15), IssuedOn: d(2026, 10, 28)},
		},
		{
			name: "a three-month lease's next period", lease: billing.LeaseDates{Start: d(2026, 10, 15), EnteredOn: d(2026, 10, 15), BilledFrom: d(2026, 10, 15), TermMonths: 3},
			today: d(2026, 11, 3), invoiced: []time.Time{d(2026, 10, 15)},
			want: &billing.NextInvoice{Period: d(2027, 1, 15), IssuedOn: d(2027, 1, 8)},
		},
		{
			name:  "a period starting on the planned end date is still billed",
			lease: with(oct28, func(l *billing.LeaseDates) { l.PlannedEnd = ptr(d(2026, 11, 28)) }),
			today: d(2026, 11, 3), invoiced: []time.Time{d(2026, 10, 28)},
			want: &billing.NextInvoice{Period: d(2026, 11, 28), IssuedOn: d(2026, 11, 21)},
		},
		{
			name:  "none after the planned end date",
			lease: with(oct28, func(l *billing.LeaseDates) { l.PlannedEnd = ptr(d(2026, 11, 27)) }),
			today: d(2026, 11, 3), invoiced: []time.Time{d(2026, 10, 28)},
		},
		{
			name:  "none for an ended lease",
			lease: with(oct28, func(l *billing.LeaseDates) { l.End = ptr(d(2026, 12, 31)) }),
			today: d(2026, 11, 3), invoiced: []time.Time{d(2026, 10, 28)},
		},
		{
			name:  "none for a cancelled booking",
			lease: with(monthly(d(2026, 12, 1), d(2026, 11, 3)), func(l *billing.LeaseDates) { l.End = ptr(d(2026, 12, 1)) }),
			today: d(2026, 11, 3),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := billing.NextInvoiceOf(tc.lease, tc.today, tc.invoiced)
			if tc.want == nil {
				require.False(t, ok, "got %+v", got)
				return
			}
			require.True(t, ok)
			require.Equal(t, *tc.want, got)
		})
	}
}
