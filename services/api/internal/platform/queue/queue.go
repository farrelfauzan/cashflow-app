// Package queue wires asynq, the Redis-backed job queue, for the API (which enqueues) and the
// worker (which runs jobs and the cron scheduler).
//
// A task ID only blocks a duplicate while the first task is still pending. The guarantee that
// money is recorded once comes from database unique keys and idempotent handlers, never from
// the queue.
package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// Queue names, highest priority first. The weights below give them 6, 3 and 1 shares of the
// workers.
const (
	Critical = "critical"
	Default  = "default"
	Low      = "low"
)

func redisOpt(addr string) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: addr}
}

// NewClient returns a client for enqueueing tasks.
func NewClient(addr string) *asynq.Client {
	return asynq.NewClient(redisOpt(addr))
}

// NewServer returns the worker server. Register handlers on the mux passed to Start.
func NewServer(addr string, log *slog.Logger) *asynq.Server {
	return asynq.NewServer(redisOpt(addr), asynq.Config{
		Concurrency: 10,
		Queues:      map[string]int{Critical: 6, Default: 3, Low: 1},
		Logger:      logAdapter{log: log},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			// The payload is left out of the log because it can hold personal data.
			log.ErrorContext(ctx, "task failed", "type", task.Type(), "error", err)
		}),
	})
}

// NewScheduler returns the cron scheduler. Pass clock.Jakarta so every cron spec means
// Jakarta time.
func NewScheduler(addr string, loc *time.Location, log *slog.Logger) *asynq.Scheduler {
	return asynq.NewScheduler(redisOpt(addr), &asynq.SchedulerOpts{
		Location: loc,
		Logger:   logAdapter{log: log},
	})
}

// logAdapter lets asynq write through slog.
type logAdapter struct{ log *slog.Logger }

func (a logAdapter) Debug(args ...any) { a.log.Debug(fmt.Sprint(args...)) }
func (a logAdapter) Info(args ...any)  { a.log.Info(fmt.Sprint(args...)) }
func (a logAdapter) Warn(args ...any)  { a.log.Warn(fmt.Sprint(args...)) }
func (a logAdapter) Error(args ...any) { a.log.Error(fmt.Sprint(args...)) }
func (a logAdapter) Fatal(args ...any) { a.log.Error(fmt.Sprint(args...)) }
