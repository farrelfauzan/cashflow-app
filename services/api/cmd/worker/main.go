// Command worker runs queued jobs and the cron scheduler. It is the same Go module as the API,
// started as a separate process.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/logging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/queue"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadWorker(ctx)
	if err != nil {
		return err
	}
	log, err := logging.New(os.Stdout, cfg.LogLevel)
	if err != nil {
		return err
	}
	log = log.With("service", "worker", "env", cfg.Env)

	// Handlers and cron entries are registered here as each slice adds jobs.
	mux := asynq.NewServeMux()
	srv := queue.NewServer(cfg.RedisAddr, log)
	if err := srv.Start(mux); err != nil {
		return fmt.Errorf("starting job server: %w", err)
	}

	sched := queue.NewScheduler(cfg.RedisAddr, clock.Jakarta, log)
	if err := sched.Start(); err != nil {
		srv.Shutdown()
		return fmt.Errorf("starting scheduler: %w", err)
	}

	log.Info("worker started", "redis", cfg.RedisAddr)
	<-ctx.Done()

	log.Info("worker stopping")
	sched.Shutdown()
	srv.Shutdown()
	return nil
}
