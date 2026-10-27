// Package ratelimit counts attempts per key in fixed windows, for login codes and, later,
// WhatsApp sends per organization.
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
)

// Limiter reports whether one more attempt for key fits in the window.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// Redis is the production limiter. The window starts at the first attempt and every attempt,
// allowed or not, counts.
type Redis struct {
	rdb *redis.Client
}

var _ Limiter = (*Redis)(nil)

// NewRedis returns a limiter backed by rdb.
func NewRedis(rdb *redis.Client) *Redis { return &Redis{rdb: rdb} }

// Allow counts the attempt and reports whether it is within the limit.
func (r *Redis) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	k := "ratelimit:" + key
	var incr *redis.IntCmd
	_, err := r.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		incr = pipe.Incr(ctx, k)
		pipe.ExpireNX(ctx, k, window) // only the first attempt starts the window
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("counting attempt: %w", err)
	}
	return incr.Val() <= int64(limit), nil
}

// Memory is an in-process limiter for tests and single-process tools.
type Memory struct {
	mu      sync.Mutex
	clock   clock.Clock
	windows map[string]*memoryWindow
}

type memoryWindow struct {
	count   int
	resetAt time.Time
}

var _ Limiter = (*Memory)(nil)

// NewMemory returns an in-process limiter that reads time from c.
func NewMemory(c clock.Clock) *Memory {
	return &Memory{clock: c, windows: map[string]*memoryWindow{}}
}

// Allow counts the attempt and reports whether it is within the limit.
func (m *Memory) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	w, ok := m.windows[key]
	if !ok || !now.Before(w.resetAt) {
		w = &memoryWindow{resetAt: now.Add(window)}
		m.windows[key] = w
	}
	w.count++
	return w.count <= limit, nil
}
