package qris_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
)

func TestPublicStatus(t *testing.T) {
	t.Parallel()
	expires := time.Date(2026, 11, 5, 10, 15, 0, 0, clock.Jakarta)
	before := expires.Add(-time.Minute)
	after := expires.Add(time.Minute)
	checked := expires.Add(-2 * time.Minute)
	attempt := func(status string, check *time.Time) *qris.Attempt {
		return &qris.Attempt{Amount: 700_000, QRString: "000201", Status: status, ExpiresAt: expires, CheckRequestedAt: check}
	}

	tests := []struct {
		name string
		a    *qris.Attempt
		now  time.Time
		want qris.Status
	}{
		{"no attempt", nil, before, qris.StatusNone},
		{"pending before expiry", attempt(qris.AttemptPending, nil), before, qris.StatusPending},
		{"pending before expiry with a check requested", attempt(qris.AttemptPending, &checked), before, qris.StatusChecking},
		{"pending past expiry with no check", attempt(qris.AttemptPending, nil), after, qris.StatusExpired},
		{"pending exactly at expiry is expired", attempt(qris.AttemptPending, nil), expires, qris.StatusExpired},
		{"pending past expiry with a check requested", attempt(qris.AttemptPending, &checked), after, qris.StatusChecking},
		{"paid", attempt(qris.AttemptPaid, nil), before, qris.StatusPaid},
		{"paid past expiry", attempt(qris.AttemptPaid, &checked), after, qris.StatusPaid},
		{"expired", attempt(qris.AttemptExpired, nil), before, qris.StatusExpired},
		{"cancelled", attempt(qris.AttemptCancelled, nil), before, qris.StatusExpired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, qris.PublicStatus(tc.a, tc.now))
		})
	}
}
