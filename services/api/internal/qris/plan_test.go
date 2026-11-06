package qris_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
)

func TestPlanMake(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, clock.Jakarta)
	attempt := func(status string, amount int64, expires time.Time) *qris.Attempt {
		return &qris.Attempt{Amount: amount, QRString: "000201", Status: status, ExpiresAt: expires}
	}
	fresh := now.Add(15 * time.Minute)

	tests := []struct {
		name   string
		a      *qris.Attempt
		amount int64
		want   string
	}{
		{"no attempt generates", nil, 700_000, "generate"},
		{"paid latest generates", attempt(qris.AttemptPaid, 700_000, fresh), 700_000, "generate"},
		{"expired latest generates", attempt(qris.AttemptExpired, 700_000, fresh), 700_000, "generate"},
		{"cancelled latest generates", attempt(qris.AttemptCancelled, 700_000, fresh), 700_000, "generate"},
		{"pending, same amount, plenty of time reuses", attempt(qris.AttemptPending, 700_000, fresh), 700_000, "reuse"},
		{"pending with exactly ReuseMargin left replaces", attempt(qris.AttemptPending, 700_000, now.Add(qris.ReuseMargin)), 700_000, "replace"},
		{"pending with just over ReuseMargin left reuses", attempt(qris.AttemptPending, 700_000, now.Add(qris.ReuseMargin+time.Second)), 700_000, "reuse"},
		{"pending with less than ReuseMargin left replaces", attempt(qris.AttemptPending, 700_000, now.Add(time.Minute)), 700_000, "replace"},
		{"pending for another amount replaces", attempt(qris.AttemptPending, 700_000, fresh), 300_000, "replace"},
		{"pending past expiry replaces", attempt(qris.AttemptPending, 700_000, now.Add(-time.Minute)), 700_000, "replace"},
		{"pending exactly at expiry replaces", attempt(qris.AttemptPending, 700_000, now), 700_000, "replace"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, qris.PlanMake(tc.a, tc.amount, now))
		})
	}
}
