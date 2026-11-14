// Command worker runs queued jobs and the cron scheduler. It is the same Go module as the API,
// started as a separate process.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/provider"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/logging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/queue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise/promisetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris/qristask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/ratelimit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder/remindertask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/scan/clamd"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/staffnotice"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/staffnotice/noticetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/storage/s3store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtask"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}

// registerUploads adds the upload scan and its reconcile (S6-1) when the worker has a bucket and
// clamd. Without STORAGE_* settings, which config.LoadWorker allows only in development, it
// registers nothing.
func registerUploads(ctx context.Context, cfg config.Worker, st *store.Store, az *authz.Service, client *asynq.Client,
	clk clock.Clock, log *slog.Logger, mux *asynq.ServeMux, sched *asynq.Scheduler) error {
	if !cfg.Storage.Enabled() {
		log.Info("upload scans off: no STORAGE_* settings")
		return nil
	}
	bucket, err := s3store.New(s3store.Config{
		Endpoint: cfg.Storage.Endpoint, PresignEndpoint: cfg.Storage.PresignEndpoint, Bucket: cfg.Storage.Bucket,
		Region: cfg.Storage.Region, AccessKeyID: cfg.Storage.AccessKeyID, SecretAccessKey: cfg.Storage.SecretAccessKey,
		PathStyle: cfg.Storage.PathStyle,
	})
	if err != nil {
		return err
	}
	scanner, err := clamd.New(cfg.ClamAVAddr)
	if err != nil {
		return err
	}
	// clamd loads its signatures for a minute or two after it starts, so a failed ping only
	// warns: scans fail and are retried until it answers.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := scanner.Ping(pingCtx); err != nil {
		log.Warn("clamd does not answer yet; upload scans wait for it", "clamav", cfg.ClamAVAddr, "error", err)
	}
	log.Info("upload scans on", "storage", cfg.Storage, "clamav", cfg.ClamAVAddr)
	return uploadtask.Worker{
		Checker: upload.NewChecker(upload.CheckerDeps{
			Store: st, Objects: bucket, Scanner: scanner, Audit: az, Clock: clk, Log: log,
		}),
		Orgs:     st,
		Enqueuer: uploadtask.NewClient(client),
		Clock:    clk,
		Log:      log,
	}.Register(mux, sched)
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
	// Staff notices (TG-5) are recorded and sent only with TELEGRAM_MESSAGES=linked, and then only
	// to staff with a live Telegram link; otherwise the closer and the decider record none.
	var closerOpts []promise.CloserOption
	var deciderOpts []reminder.DeciderOption
	if cfg.Telegram.SendsToLinked() {
		notices := staffnotice.NewNotices(clk, noticetask.NewClient(client), log)
		closerOpts = append(closerOpts, promise.NotifyStaff(notices))
		deciderOpts = append(deciderOpts, reminder.NotifyStaff(notices))
	}
	closer := promise.NewCloser(st, clk, log, closerOpts...)
	invoices := invoicetask.Worker{
		Generator: billing.NewService(st, az, clk, log, closer),
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
	promises := promisetask.Worker{
		Closer:   closer,
		Orgs:     st,
		Enqueuer: promisetask.NewClient(client),
		Clock:    clk,
		Log:      log,
	}
	if err := promises.Register(mux, sched); err != nil {
		return err
	}
	// Reminders and promise confirmations go out through the configured WhatsApp provider. With
	// TELEGRAM_MESSAGES=linked a tenant with a linked chat gets them on Telegram (decision 42).
	msg, err := provider.New(cfg.WhatsApp(), log)
	if err != nil {
		return err
	}
	// Staff notices (TG-5) go to staff users' linked chats, so the router reads staff links too.
	// The worker never makes link codes, so their limiter is never used.
	tgStaff := auth.NewTelegramStaff(st, ratelimit.NewMemory(clk), clk, log)
	msg, err = provider.WithTelegram(msg, cfg.Telegram, telegramlink.NewChats(st, clk), tgStaff, log)
	if err != nil {
		return err
	}
	sendQueue := remindertask.NewSendClient(client)
	reminders := remindertask.Worker{
		Decider:  reminder.NewDecider(st, clk, log, sendQueue, deciderOpts...),
		Orgs:     st,
		Enqueuer: remindertask.NewClient(client),
		Clock:    clk,
		Log:      log,
	}
	if err := reminders.Register(mux, sched); err != nil {
		return err
	}
	sending := remindertask.SendWorker{
		Sender: reminder.NewSender(reminder.SenderDeps{
			Store: st, Links: paylink.NewService(st, az, clk, cfg.PublicWebOrigin), Provider: msg, Clock: clk, Log: log,
		}),
		Orgs:  st,
		Queue: sendQueue,
		Log:   log,
	}
	if cfg.Telegram.SendsToLinked() {
		// msg is the router here, so a notice reaches only linked chats, never WhatsApp.
		notices := staffnotice.NewSender(staffnotice.SenderDeps{Store: st, Principals: az, Provider: msg, Clock: clk, Log: log})
		noticetask.Worker{Sender: notices, Log: log}.Register(mux)
		sending.Also = append(sending.Also, noticetask.Sweeper{Sender: notices, Queue: noticetask.NewClient(client)})
	}
	if err := sending.Register(mux, sched); err != nil {
		return err
	}
	if err := registerUploads(ctx, cfg, st, az, client, clk, log, mux, sched); err != nil {
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
