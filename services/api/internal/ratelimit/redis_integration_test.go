//go:build integration

package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func TestRedisLimiter(t *testing.T) {
	ctx := context.Background()
	ctr, err := tcredis.Run(ctx, "redis:7-alpine")
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)
	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)
	opts, err := redis.ParseURL(uri)
	require.NoError(t, err)
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })

	l := NewRedis(rdb)
	for i := 1; i <= 3; i++ {
		ok, err := l.Allow(ctx, "otp:request:+6281", 3, time.Second)
		require.NoError(t, err)
		require.True(t, ok, "attempt %d", i)
	}
	ok, err := l.Allow(ctx, "otp:request:+6281", 3, time.Second)
	require.NoError(t, err)
	require.False(t, ok)

	ttl, err := rdb.TTL(ctx, "ratelimit:otp:request:+6281").Result()
	require.NoError(t, err)
	require.Positive(t, ttl, "the window must expire")

	require.Eventually(t, func() bool {
		ok, err := l.Allow(ctx, "otp:request:+6281", 3, time.Second)
		return err == nil && ok
	}, 5*time.Second, 200*time.Millisecond, "a new window opens once the old one expires")
}
