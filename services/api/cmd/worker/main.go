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

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/logging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/queue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris/qristask"
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

	st, err := store.Open(ctx, cfg.DatabaseURL, log, store.WithMaxConns(cfg.DatabaseMaxConns))
	if err != nil {
		return err
	}
	defer st.Close()

	client := queue.NewClient(cfg.RedisAddr)
	defer func() {
		if err := client.Close(); err != nil {
			log.Error("closing queue client", "error", err)
		}
	}()

	clk := clock.System()
	az := authz.NewService(st, clk, log)

	// Handlers and cron entries are registered here as each slice adds jobs.
	mux := asynq.NewServeMux()
	sched := queue.NewScheduler(cfg.RedisAddr, clock.Jakarta, log)
	invoices := invoicetask.Worker{
		Generator: billing.NewService(st, az, clk, log),
		Orgs:      st,
		Enqueuer:  invoicetask.NewClient(client),
		Clock:     clk,
		Log:       log,
	}
	if err := invoices.Register(mux, sched); err != nil {
		return err
	}
	keys, err := cfg.Secrets.Keyring()
	if err != nil {
		return err
	}
	gateway, err := autogopay.NewClient(autogopay.Config{BaseURL: cfg.AutoGoPayBaseURL})
	if err != nil {
		return err
	}
	qrisSvc := qris.NewService(qris.Deps{
		Store: st, Clock: clk, Gateway: gateway, Keys: autogopay.NewService(st, keys, az, clk), Audit: az, Log: log,
	})
	reconcile := qristask.Worker{
		Reconciler: qrisSvc,
		Settler:    qrisSvc,
		Orgs:       st,
		Enqueuer:   qristask.NewClient(client),
		Clock:      clk,
		Log:        log,
	}
	if err := reconcile.Register(mux, sched); err != nil {
		return err
	}

	srv := queue.NewServer(cfg.RedisAddr, log)
	if err := srv.Start(mux); err != nil {
		return fmt.Errorf("starting job server: %w", err)
	}
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
