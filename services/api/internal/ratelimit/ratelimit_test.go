package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
)

func TestMemoryLimiter(t *testing.T) {
	ctx := context.Background()
	c := clock.NewFake(time.Date(2026, 11, 1, 9, 0, 0, 0, clock.Jakarta))
	l := NewMemory(c)

	for i := 1; i <= 3; i++ {
		ok, err := l.Allow(ctx, "otp:+6281", 3, 15*time.Minute)
		require.NoError(t, err)
		require.True(t, ok, "attempt %d", i)
	}
	ok, err := l.Allow(ctx, "otp:+6281", 3, 15*time.Minute)
	require.NoError(t, err)
	require.False(t, ok, "the fourth attempt is over the limit")

	ok, err = l.Allow(ctx, "otp:+6282", 3, 15*time.Minute)
	require.NoError(t, err)
	require.True(t, ok, "keys are counted separately")

	c.Advance(15 * time.Minute)
	ok, err = l.Allow(ctx, "otp:+6281", 3, 15*time.Minute)
	require.NoError(t, err)
	require.True(t, ok, "a new window starts after the old one ends")
}
