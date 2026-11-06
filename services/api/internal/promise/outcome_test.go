package promise_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
)

func TestOutcome(t *testing.T) {
	t.Parallel()
	amount := func(v int64) *int64 { return &v }

	tests := []struct {
		name string
		in   promise.Progress
		want string
	}{
		// A promised amount: the target is paying now plus the promised amount.
		{
			name: "an amount paid exactly to the target is kept",
			in:   promise.Progress{PayingNow: 200_000, PromisedAmount: amount(500_000), Paid: 700_000},
			want: promise.StatusKept,
		},
		{
			name: "an amount paid over the target is kept",
			in:   promise.Progress{PayingNow: 200_000, PromisedAmount: amount(500_000), Paid: 900_000},
			want: promise.StatusKept,
		},
		{
			name: "an amount paid one rupiah short is partially kept",
			in:   promise.Progress{PayingNow: 200_000, PromisedAmount: amount(500_000), Paid: 699_999},
			want: promise.StatusPartiallyKept,
		},
		{
			name: "paying now, then nothing more by the date, is broken",
			in:   promise.Progress{PayingNow: 200_000, PromisedAmount: amount(500_000), Paid: 200_000},
			want: promise.StatusBroken,
		},
		{
			name: "Rp1 paid now and nothing after is broken",
			in:   promise.Progress{PayingNow: 1, PromisedAmount: amount(500_000), Paid: 1},
			want: promise.StatusBroken,
		},
		{
			name: "paying now, then less than the promised amount, is partially kept",
			in:   promise.Progress{PayingNow: 200_000, PromisedAmount: amount(500_000), Paid: 450_000},
			want: promise.StatusPartiallyKept,
		},
		{
			name: "Rp1 beyond what was paid now is partially kept",
			in:   promise.Progress{PayingNow: 200_000, PromisedAmount: amount(500_000), Paid: 200_001},
			want: promise.StatusPartiallyKept,
		},
		{
			name: "less than what was said to be paid now is broken",
			in:   promise.Progress{PayingNow: 200_000, PromisedAmount: amount(500_000), Paid: 150_000},
			want: promise.StatusBroken,
		},
		{
			name: "an amount with nothing paid is broken",
			in:   promise.Progress{PromisedAmount: amount(500_000)},
			want: promise.StatusBroken,
		},
		{
			name: "an amount is kept once nothing it covers is owed, even with less paid",
			in:   promise.Progress{PromisedAmount: amount(500_000), CoveredSettled: true},
			want: promise.StatusKept,
		},
		{
			name: "an amount with no paying now is kept at the amount",
			in:   promise.Progress{PromisedAmount: amount(500_000), Paid: 500_000},
			want: promise.StatusKept,
		},
		// No amount: everything the version covers.
		{
			name: "no amount with every covered invoice paid is kept",
			in:   promise.Progress{Paid: 700_000, CoveredSettled: true},
			want: promise.StatusKept,
		},
		{
			name: "no amount with a covered invoice cancelled and nothing paid is kept",
			in:   promise.Progress{CoveredSettled: true},
			want: promise.StatusKept,
		},
		{
			name: "no amount with a covered invoice still owing is partially kept when something was paid",
			in:   promise.Progress{PayingNow: 100_000, Paid: 600_000},
			want: promise.StatusPartiallyKept,
		},
		{
			name: "no amount with only what was paid now is broken",
			in:   promise.Progress{PayingNow: 100_000, Paid: 100_000},
			want: promise.StatusBroken,
		},
		{
			name: "no amount with nothing paid is broken",
			in:   promise.Progress{PayingNow: 100_000},
			want: promise.StatusBroken,
		},
		{
			name: "payments corrected below zero count as nothing paid",
			in:   promise.Progress{Paid: -50_000},
			want: promise.StatusBroken,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.in.Outcome())
		})
	}
}
