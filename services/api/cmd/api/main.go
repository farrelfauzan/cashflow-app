// Command api serves the HTTP API.
package main

import (
	"context"
	"errors"
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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/devlog"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/gowa"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/logging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/ratelimit"
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

	msg, err := messenger(cfg, log)
	if err != nil {
		return err
	}

	st, err := store.Open(ctx, cfg.DatabaseURL, log)
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

	clk := clock.System()
	limiter := ratelimit.NewRedis(rdb)
	authSvc, err := auth.NewService(st, msg, limiter, clk, log, []byte(cfg.AuthSecret))
	if err != nil {
		return err
	}
	az := authz.NewService(st, clk, log)
	props := property.NewService(st, az)
	tenants := tenant.NewService(st, az, clk)

	members := member.NewService(st, az, member.Invitations{
		Messenger: msg, Limiter: limiter, Clock: clk, Log: log, LoginURL: cfg.LoginURL(),
	})

	srv := httpapi.New(httpapi.Deps{
		Log:            log,
		DB:             st,
		Auth:           authSvc,
		Authz:          az,
		Members:        members,
		Properties:     props,
		Tenants:        tenants,
		Imports:        tenantimport.NewService(st, az, props, tenants),
		CookieSecure:   cfg.CookieSecure(),
		AllowedOrigins: cfg.Origins(),
		DocsSpecPath:   cfg.DocsSpecPath(),
	})
	if path := cfg.DocsSpecPath(); path != "" {
		log.Info("API docs at /docs", "spec", path)
	}
	log.Info("api starting", "addr", cfg.HTTPAddr)
	return srv.Run(ctx, cfg.HTTPAddr)
}

// messenger builds the WhatsApp provider chosen in config: GOWA, or in development the log
// provider, which writes messages (login codes included) to the log instead of sending them.
func messenger(cfg config.API, log *slog.Logger) (messaging.Provider, error) {
	switch cfg.WhatsAppProvider {
	case "gowa":
		return gowa.New(gowa.Config{
			BaseURL:  cfg.GOWA.URL,
			Username: cfg.GOWA.Username,
			Password: cfg.GOWA.Password,
			DeviceID: cfg.GOWA.DeviceID,
		})
	case "devlog":
		return devlog.New(log, cfg.DevlogFile), nil
	default:
		// config.LoadAPI has already rejected anything else.
		return nil, errors.New("no WhatsApp provider configured")
	}
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
