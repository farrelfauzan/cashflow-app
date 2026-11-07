// Command api serves the HTTP API.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/provider"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/telegram"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/logging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/queue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris/qristask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/ratelimit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder/remindertask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenantimport"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

func run() error {
	// `api healthcheck` lets the container image check itself without curl.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		return healthcheck()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadAPI(ctx)
	if err != nil {
		return err
	}
	log, err := logging.New(os.Stdout, cfg.LogLevel)
	if err != nil {
		return err
	}
	log = log.With("service", "api", "env", cfg.Env)

	msg, err := provider.New(cfg.WhatsApp(), log)
	if err != nil {
		return err
	}

	st, err := store.Open(ctx, cfg.DatabaseURL, log, store.WithMaxConns(cfg.DatabaseMaxConns))
	if err != nil {
		return err
	}
	defer st.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Error("closing redis client", "error", err)
		}
	}()
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("pinging redis: %w", err)
	}

	// The queue client enqueues invoice generation once a lease is saved; the worker runs it.
	queueClient := queue.NewClient(cfg.RedisAddr)
	defer func() {
		if err := queueClient.Close(); err != nil {
			log.Error("closing queue client", "error", err)
		}
	}()

	clk := clock.System()
	// Login codes and invitations name no tenant, so the router sends them on WhatsApp.
	msg, err = provider.WithTelegram(msg, cfg.Telegram, telegramlink.NewChats(st, clk), log)
	if err != nil {
		return err
	}
	limiter := ratelimit.NewRedis(rdb)
	authSvc, err := auth.NewService(st, msg, limiter, clk, log, []byte(cfg.AuthSecret))
	if err != nil {
		return err
	}
	az := authz.NewService(st, clk, log)
	props := property.NewService(st, az, clk)
	leaseQueue := invoicetask.NewLeaseQueue(invoicetask.NewClient(queueClient), log)
	// A manual payment that keeps a tenant's promise closes it in the payment's transaction.
	billingSvc := billing.NewService(st, az, clk, log, promise.NewCloser(st, clk, log))
	tenants := tenant.NewService(st, az, clk, billingSvc, leaseQueue)
	imports := tenantimport.NewService(st, az, props, tenants, clk)
	cookieSecure := cfg.CookieSecure()
	allowedOrigins := cfg.Origins()
	docsSpecPath := cfg.DocsSpecPath()

	members := member.NewService(st, az, member.Invitations{
		Messenger: msg, Limiter: limiter, Clock: clk, Log: log, LoginURL: cfg.LoginURL(),
	})

	payLinks := paylink.NewService(st, az, clk, cfg.PublicWebOrigin)
	keys, err := cfg.Secrets.Keyring()
	if err != nil {
		return err
	}
	accounts := autogopay.NewService(st, keys, az, clk)
	qrisSvc, err := newQRIS(cfg, st, payLinks, accounts, az, limiter, clk, log)
	if err != nil {
		return err
	}
	webhooks := qris.NewWebhook(accounts, qristask.NewPaidClient(queueClient), log)
	// Each saved promise version enqueues its "janji tercatat" message; the worker sends it.
	promises := promise.NewService(promise.Deps{
		Store: st, Links: payLinks, Tenants: billingSvc, Clock: clk,
		Confirms: remindertask.NewSendClient(queueClient), Log: log,
	})
	tgCodes, tgUpdates, err := newTelegram(cfg, st, payLinks, clk, log)
	if err != nil {
		return err
	}

	srv := httpapi.New(httpapi.Deps{
		Log:           log,
		DB:            st,
		Auth:          authSvc,
		Authz:         az,
		Members:       members,
		Properties:    props,
		Tenants:       tenants,
		Imports:       imports,
		Billing:       billingSvc,
		PayLinks:      payLinks,
		QRIS:          qrisSvc,
		Promises:      promises,
		Webhooks:      webhooks,
		TelegramCodes: tgCodes,
		// A nil *Handler must stay a nil interface, so the route is not registered without a bot.
		TelegramUpdates:       updateHandler(tgUpdates),
		TelegramWebhookSecret: cfg.Telegram.WebhookSecret,
		PublicLimiter:         limiter,
		CookieSecure:          cookieSecure,
		AllowedOrigins:        allowedOrigins,
		DocsSpecPath:          docsSpecPath,
	})
	if docsSpecPath != "" {
		log.Info("API docs at /docs", "spec", docsSpecPath)
	}
	log.Info("api starting", "addr", cfg.HTTPAddr)
	return srv.Run(ctx, cfg.HTTPAddr)
}

// newTelegram builds the Telegram demo channel (decision 42) when the environment has a bot: the
// payment page's deep-link codes and the webhook's update handler. Without a bot both are nil, so
// the page offers no Telegram and /webhooks/telegram does not exist. config.LoadAPI has already
// refused a bot in production.
func newTelegram(cfg config.API, st *store.Store, links *paylink.Service, clk clock.Clock, log *slog.Logger) (*telegramlink.Codes, *telegramlink.Handler, error) {
	if !cfg.Telegram.Enabled() {
		return nil, nil, nil
	}
	bot, err := telegram.New(telegram.Config{Token: cfg.Telegram.BotToken, BaseURL: cfg.Telegram.APIBaseURL})
	if err != nil {
		return nil, nil, err
	}
	log.Info("telegram demo channel on", "telegram", cfg.Telegram)
	return telegramlink.NewCodes(st, links, clk, cfg.Telegram.BotUsername),
		telegramlink.NewHandler(st, bot, clk, log), nil
}

// updateHandler turns a nil *telegramlink.Handler into a nil interface.
func updateHandler(h *telegramlink.Handler) telegram.UpdateHandler {
	if h == nil {
		return nil
	}
	return h
}

func healthcheck() error {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parsing HTTP_ADDR: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return fmt.Errorf("parsing port in HTTP_ADDR: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// G704 is a false positive here: the host is fixed to loopback and the port is an integer
	// from our own HTTP_ADDR setting.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/healthz", port), nil) //nolint:gosec // see comment above
	if err != nil {
		return fmt.Errorf("building healthcheck request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // same loopback request as above
	if err != nil {
		return fmt.Errorf("healthcheck request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck returned status %d", resp.StatusCode)
	}
	return nil
}

// newQRIS builds the QRIS service: AutoGoPay's client, the organizations' keys, and the Redis
// counters that cap generates and checks per tenant.
func newQRIS(cfg config.API, st *store.Store, links qris.LinkOpener, accounts *autogopay.Service, az *authz.Service,
	limiter qris.Limiter, clk clock.Clock, log *slog.Logger) (*qris.Service, error) {
	gateway, err := autogopay.NewClient(autogopay.Config{BaseURL: cfg.AutoGoPayBaseURL})
	if err != nil {
		return nil, err
	}
	return qris.NewService(qris.Deps{
		Store: st, Links: links, Clock: clk, Gateway: gateway,
		Keys: accounts, Limiter: limiter, Audit: az, Log: log,
		MaxConcurrentCalls: cfg.QRISMaxConcurrent,
	}), nil
}
