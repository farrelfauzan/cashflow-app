// Package config loads each binary's settings from environment variables and fails fast
// when one is missing or invalid.
package config

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sethvargo/go-envconfig/v2"
)

// Common holds the settings every binary needs.
type Common struct {
	Env      string `env:"APP_ENV, default=development"`
	LogLevel string `env:"LOG_LEVEL, default=info"`
}

func (c Common) validate() error {
	switch c.Env {
	case "development", "staging", "production":
		return nil
	}
	return fmt.Errorf("APP_ENV must be development, staging or production, got %q", c.Env)
}

// API is the configuration of cmd/api.
type API struct {
	Common
	HTTPAddr    string `env:"HTTP_ADDR, default=:8080"`
	DatabaseURL string `env:"DATABASE_URL, required"`
	RedisAddr   string `env:"REDIS_ADDR, default=localhost:6379"`
	// AuthSecret keys the HMAC of login codes. At least 32 bytes, random, never committed.
	AuthSecret string `env:"AUTH_SECRET, required"`
	// AllowedOrigins is a comma-separated list of browser origins allowed to send
	// state-changing requests.
	AllowedOrigins string `env:"ALLOWED_ORIGINS, default=http://localhost:3000"`
	// WhatsAppProvider picks the messaging adapter: "gowa" (launch) or "devlog" (development
	// only). "cloudapi" is reserved for the WhatsApp Cloud API adapter, not built yet.
	WhatsAppProvider string `env:"WHATSAPP_PROVIDER, default=devlog"`
	// DevlogFile, when set, makes the devlog provider also append each message to this file as
	// one JSON line, so end-to-end tests can read login codes. Development with devlog only.
	DevlogFile string `env:"DEVLOG_FILE"`
	GOWA       GOWA   `env:", prefix=GOWA_"`
	// OpenAPISpec is the spec file Swagger UI shows at /docs. Only development serves it; the
	// default works when the API runs from services/api, as `make api-run` does.
	OpenAPISpec string `env:"OPENAPI_SPEC, default=../../api/openapi.yaml"`
}

// DocsSpecPath returns the spec to serve at /docs, or "" outside development.
func (c API) DocsSpecPath() string {
	if c.Env != "development" {
		return ""
	}
	return c.OpenAPISpec
}

// GOWA points the API at the self-hosted GOWA server.
type GOWA struct {
	URL      string `env:"URL"`       // GOWA_URL, for example http://gowa:3000
	Username string `env:"USERNAME"`  // GOWA_USERNAME, one of the server's APP_BASIC_AUTH users
	Password string `env:"PASSWORD"`  // GOWA_PASSWORD
	DeviceID string `env:"DEVICE_ID"` // GOWA_DEVICE_ID, empty uses the server's only device
}

// Origins returns AllowedOrigins as a list.
func (c API) Origins() []string {
	var out []string
	for _, o := range strings.Split(c.AllowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// CookieSecure reports whether cookies must be marked Secure. Only local development, which
// runs over plain HTTP, turns it off.
func (c API) CookieSecure() bool { return c.Env != "development" }

func (c API) validate() error {
	if err := c.Common.validate(); err != nil {
		return err
	}
	if len(c.AuthSecret) < 32 {
		return fmt.Errorf("AUTH_SECRET must be at least 32 bytes, got %d", len(c.AuthSecret))
	}
	switch c.WhatsAppProvider {
	case "devlog":
		if c.Env != "development" {
			return fmt.Errorf("WHATSAPP_PROVIDER=devlog only runs in development; use gowa in %s", c.Env)
		}
	case "gowa":
		if c.GOWA.URL == "" || c.GOWA.Username == "" || c.GOWA.Password == "" {
			return errors.New("WHATSAPP_PROVIDER=gowa needs GOWA_URL, GOWA_USERNAME and GOWA_PASSWORD")
		}
	case "cloudapi":
		return errors.New("WHATSAPP_PROVIDER=cloudapi is not built yet; use gowa")
	default:
		return fmt.Errorf("WHATSAPP_PROVIDER must be gowa or devlog, got %q", c.WhatsAppProvider)
	}
	// Login codes in a file are only acceptable on a developer's machine.
	if c.DevlogFile != "" && (c.Env != "development" || c.WhatsAppProvider != "devlog") {
		return errors.New("DEVLOG_FILE only works in development with WHATSAPP_PROVIDER=devlog; unset it")
	}
	return nil
}

// Admin is the configuration of cmd/admin. It connects as the app role, like the API.
type Admin struct {
	Common
	DatabaseURL string `env:"DATABASE_URL, required"`
}

// LoadAdmin reads the admin command configuration from the process environment.
func LoadAdmin(ctx context.Context) (Admin, error) {
	var c Admin
	if err := process(ctx, envconfig.OsLookuper(), &c); err != nil {
		return Admin{}, fmt.Errorf("loading admin config: %w", err)
	}
	if err := c.validate(); err != nil {
		return Admin{}, fmt.Errorf("loading admin config: %w", err)
	}
	return c, nil
}

// Worker is the configuration of cmd/worker.
type Worker struct {
	Common
	RedisAddr string `env:"REDIS_ADDR, default=localhost:6379"`
}

// Migrate is the configuration of cmd/migrate. It connects as the owner role, never as the
// role the API uses, so the API cannot change the schema or bypass row-level security.
type Migrate struct {
	Common
	DatabaseURL string `env:"MIGRATE_DATABASE_URL, required"`
}

// LoadAPI reads the API configuration from the process environment.
func LoadAPI(ctx context.Context) (API, error) {
	return loadAPI(ctx, envconfig.OsLookuper())
}

// LoadWorker reads the worker configuration from the process environment.
func LoadWorker(ctx context.Context) (Worker, error) {
	return loadWorker(ctx, envconfig.OsLookuper())
}

// LoadMigrate reads the migration runner configuration from the process environment.
func LoadMigrate(ctx context.Context) (Migrate, error) {
	return loadMigrate(ctx, envconfig.OsLookuper())
}

func loadAPI(ctx context.Context, l envconfig.Lookuper) (API, error) {
	var c API
	if err := process(ctx, l, &c); err != nil {
		return API{}, fmt.Errorf("loading api config: %w", err)
	}
	if err := c.validate(); err != nil {
		return API{}, fmt.Errorf("loading api config: %w", err)
	}
	return c, nil
}

func loadWorker(ctx context.Context, l envconfig.Lookuper) (Worker, error) {
	var c Worker
	if err := process(ctx, l, &c); err != nil {
		return Worker{}, fmt.Errorf("loading worker config: %w", err)
	}
	if err := c.validate(); err != nil {
		return Worker{}, fmt.Errorf("loading worker config: %w", err)
	}
	return c, nil
}

func loadMigrate(ctx context.Context, l envconfig.Lookuper) (Migrate, error) {
	var c Migrate
	if err := process(ctx, l, &c); err != nil {
		return Migrate{}, fmt.Errorf("loading migrate config: %w", err)
	}
	if err := c.validate(); err != nil {
		return Migrate{}, fmt.Errorf("loading migrate config: %w", err)
	}
	return c, nil
}

func process(ctx context.Context, l envconfig.Lookuper, target any) error {
	return envconfig.ProcessWith(ctx, &envconfig.Config{Target: target, Lookuper: l})
}
