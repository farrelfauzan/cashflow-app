// Package config loads each binary's settings from environment variables and fails fast
// when one is missing or invalid.
package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/sethvargo/go-envconfig/v2"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
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

// Secrets holds the master keys that encrypt per-organization secrets, such as AutoGoPay API
// keys, at rest (internal/secret). Both are secrets: never log them or a struct that holds them.
// The API, the worker and the admin command all need them, so a bad key stops each at startup.
type Secrets struct {
	// Key is SECRETS_KEY, "<id>:<base64 of 32 random bytes>": the key that encrypts.
	Key string `env:"KEY, required"`
	// Previous is SECRETS_KEY_PREVIOUS, optional: comma-separated keys of the same form that only
	// decrypt, kept during a rotation until every row is encrypted under the current key.
	Previous string `env:"KEY_PREVIOUS"`
}

// Keyring parses the keys. Load* has already checked them, so it fails only on a Secrets built
// by hand.
func (s Secrets) Keyring() (*secret.Keyring, error) {
	k, err := secret.ParseKeyring(s.Key, s.Previous)
	if err != nil {
		return nil, fmt.Errorf("reading secrets keys: %w", err)
	}
	return k, nil
}

func (s Secrets) validate() error {
	_, err := secret.ParseKeyring(s.Key, s.Previous)
	return err
}

// API is the configuration of cmd/api.
type API struct {
	Common
	HTTPAddr    string `env:"HTTP_ADDR, default=:8080"`
	DatabaseURL string `env:"DATABASE_URL, required"`
	RedisAddr   string `env:"REDIS_ADDR, default=localhost:6379"`
	// AuthSecret keys the HMAC of login codes. At least 32 bytes, random, never committed.
	AuthSecret string  `env:"AUTH_SECRET, required"`
	Secrets    Secrets `env:", prefix=SECRETS_"`
	// AutoGoPayBaseURL is AUTOGOPAY_BASE_URL, AutoGoPay's gateway; empty uses
	// autogopay.DefaultBaseURL. It must be https outside development.
	AutoGoPayBaseURL string `env:"AUTOGOPAY_BASE_URL"`
	// DatabaseMaxConns is DATABASE_MAX_CONNS, the size of the API's connection pool.
	DatabaseMaxConns int32 `env:"DATABASE_MAX_CONNS, default=20"`
	// QRISMaxConcurrent is QRIS_MAX_CONCURRENT: how many payment page requests may wait on
	// AutoGoPay at once, each holding a database connection. It stays below DatabaseMaxConns, so a
	// slow AutoGoPay can never take every connection and the page (and its bank transfer fallback)
	// keeps loading; a request past it answers qris_overloaded at once. A normal AutoGoPay call
	// takes about a second, so the places fill only when AutoGoPay hangs.
	QRISMaxConcurrent int `env:"QRIS_MAX_CONCURRENT, default=10"`
	// AllowedOrigins is a comma-separated list of browser origins allowed to send
	// state-changing requests.
	AllowedOrigins string `env:"ALLOWED_ORIGINS, default=http://localhost:3000"`
	// PublicWebOrigin is where people open the web app, for links in WhatsApp messages, for
	// example https://app.example.id. Required outside development, where it must be https;
	// development defaults to the Next.js dev server.
	PublicWebOrigin string `env:"PUBLIC_WEB_ORIGIN"`
	// WhatsAppProvider picks the messaging adapter: "gowa", "twilio" (development only: only
	// a Twilio sandbox is supported until a verified sender with approved templates exists) or
	// "devlog" (development only). "cloudapi" is reserved for the WhatsApp
	// Cloud API adapter, not built yet.
	WhatsAppProvider string `env:"WHATSAPP_PROVIDER, default=devlog"`
	// DevlogFile, when set, makes the devlog provider also append each message to this file as
	// one JSON line, so end-to-end tests can read login codes. Development with devlog only.
	DevlogFile string `env:"DEVLOG_FILE"`
	GOWA       GOWA   `env:", prefix=GOWA_"`
	Twilio     Twilio `env:", prefix=TWILIO_"`
	// Telegram is the platform bot (decision 42). Optional; validated when set.
	Telegram Telegram `env:", prefix=TELEGRAM_"`
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

// Twilio holds the Twilio account and WhatsApp sender. AuthToken is a secret: never log it or
// the struct that holds it.
type Twilio struct {
	AccountSID   string `env:"ACCOUNT_SID"`   // TWILIO_ACCOUNT_SID, AC followed by 32 hex characters
	AuthToken    string `env:"AUTH_TOKEN"`    // TWILIO_AUTH_TOKEN
	WhatsAppFrom string `env:"WHATSAPP_FROM"` // TWILIO_WHATSAPP_FROM, the sender in E.164
	// ContentSIDs is TWILIO_CONTENT_SIDS: optional comma-separated template=ContentSid pairs,
	// for example "login_code=HX...". The Twilio adapter checks the names and SIDs.
	ContentSIDs string `env:"CONTENT_SIDS"`
}

// ContentSIDMap parses ContentSIDs into template name -> Content SID.
func (t Twilio) ContentSIDMap() (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(t.ContentSIDs) == "" {
		return out, nil
	}
	// Errors name the pair by position and say what is missing, without echoing the value, in
	// case a secret was pasted into the wrong setting.
	const want = "TWILIO_CONTENT_SIDS must be template=ContentSid pairs separated by commas, such as login_code=HX followed by 32 hex characters"
	i := 0
	for pair := range strings.SplitSeq(t.ContentSIDs, ",") {
		i++
		name, sid, ok := strings.Cut(strings.TrimSpace(pair), "=")
		name, sid = strings.TrimSpace(name), strings.TrimSpace(sid)
		switch {
		case !ok:
			return nil, fmt.Errorf("%s; pair %d has no '='", want, i)
		case name == "":
			return nil, fmt.Errorf("%s; pair %d has no template name before '='", want, i)
		case sid == "":
			return nil, fmt.Errorf("%s; pair %d has no Content SID after '='", want, i)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("TWILIO_CONTENT_SIDS names %s twice", name)
		}
		out[name] = sid
	}
	return out, nil
}

var (
	twilioAccountSID = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)
	e164             = regexp.MustCompile(`^\+[0-9]{8,15}$`)
)

func (t Twilio) validate() error {
	if t.AccountSID == "" || t.AuthToken == "" || t.WhatsAppFrom == "" {
		return errors.New("WHATSAPP_PROVIDER=twilio needs TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN and TWILIO_WHATSAPP_FROM")
	}
	if !twilioAccountSID.MatchString(t.AccountSID) {
		return errors.New("TWILIO_ACCOUNT_SID must be AC followed by 32 hex characters")
	}
	if !e164.MatchString(t.WhatsAppFrom) {
		return fmt.Errorf("TWILIO_WHATSAPP_FROM must be an E.164 number such as +14155238886, got %q", t.WhatsAppFrom)
	}
	if _, err := t.ContentSIDMap(); err != nil {
		return err
	}
	return nil
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

// LoginURL is the web login page, linked from the member invitation. It carries no token: the
// invitee signs in with the usual WhatsApp code.
func (c API) LoginURL() string { return c.PublicWebOrigin + "/login" }

// devWebOrigin is the Next.js dev server, the PUBLIC_WEB_ORIGIN default in development.
const devWebOrigin = "http://localhost:3000"

// validWebOrigin checks that origin is an absolute http(s) origin with no path, query or
// fragment, and https outside development.
func validWebOrigin(origin, env string) error {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("PUBLIC_WEB_ORIGIN must be an absolute http(s) origin such as https://app.example.id, got %q", origin)
	}
	if u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.HasSuffix(origin, "?") || strings.HasSuffix(origin, "#") {
		return fmt.Errorf("PUBLIC_WEB_ORIGIN must be an origin only, with no path, query or credentials, got %q", origin)
	}
	if env != "development" && u.Scheme != "https" {
		return fmt.Errorf("PUBLIC_WEB_ORIGIN must use https in %s, got %q", env, origin)
	}
	return nil
}

// CookieSecure reports whether cookies must be marked Secure. Only local development, which
// runs over plain HTTP, turns it off.
func (c API) CookieSecure() bool { return c.Env != "development" }

func (c API) validate() error {
	if err := c.Common.validate(); err != nil {
		return err
	}
	if err := c.Secrets.validate(); err != nil {
		return err
	}
	if err := validAutoGoPayURL(c.AutoGoPayBaseURL, c.Env); err != nil {
		return err
	}
	if err := c.Telegram.validate(c.Env); err != nil {
		return err
	}
	if c.DatabaseMaxConns < 2 {
		return fmt.Errorf("DATABASE_MAX_CONNS must be at least 2, got %d", c.DatabaseMaxConns)
	}
	if c.QRISMaxConcurrent < 1 || int64(c.QRISMaxConcurrent) >= int64(c.DatabaseMaxConns) {
		return fmt.Errorf("QRIS_MAX_CONCURRENT must be 1 to DATABASE_MAX_CONNS-1 (%d), got %d",
			c.DatabaseMaxConns-1, c.QRISMaxConcurrent)
	}
	if len(c.AuthSecret) < 32 {
		return fmt.Errorf("AUTH_SECRET must be at least 32 bytes, got %d", len(c.AuthSecret))
	}
	if c.PublicWebOrigin == "" {
		return fmt.Errorf("PUBLIC_WEB_ORIGIN is required in %s", c.Env)
	}
	if err := validWebOrigin(c.PublicWebOrigin, c.Env); err != nil {
		return err
	}
	return c.WhatsApp().validate()
}

// WhatsApp is the WhatsApp provider choice and its settings, as cmd/api and cmd/worker read them
// (the API sends login codes and invitations, the worker reminders).
type WhatsApp struct {
	Env        string
	Provider   string
	DevlogFile string
	GOWA       GOWA
	Twilio     Twilio
}

// WhatsApp returns the API's WhatsApp settings.
func (c API) WhatsApp() WhatsApp {
	return WhatsApp{Env: c.Env, Provider: c.WhatsAppProvider, DevlogFile: c.DevlogFile, GOWA: c.GOWA, Twilio: c.Twilio}
}

func (w WhatsApp) validate() error {
	switch w.Provider {
	case "devlog":
		if w.Env != "development" {
			return fmt.Errorf("WHATSAPP_PROVIDER=devlog only runs in development; use gowa or twilio in %s", w.Env)
		}
	case "gowa":
		if w.GOWA.URL == "" || w.GOWA.Username == "" || w.GOWA.Password == "" {
			return errors.New("WHATSAPP_PROVIDER=gowa needs GOWA_URL, GOWA_USERNAME and GOWA_PASSWORD")
		}
	case "twilio":
		// Only a Twilio sandbox is supported, and sandbox numbers differ per account, so the
		// provider itself stays in development until a verified sender exists.
		if w.Env != "development" {
			return fmt.Errorf("WHATSAPP_PROVIDER=twilio only runs in development (a Twilio sandbox); use gowa in %s", w.Env)
		}
		if err := w.Twilio.validate(); err != nil {
			return err
		}
	case "cloudapi":
		return errors.New("WHATSAPP_PROVIDER=cloudapi is not built yet; use gowa")
	default:
		return fmt.Errorf("WHATSAPP_PROVIDER must be gowa, twilio or devlog, got %q", w.Provider)
	}
	// Login codes in a file are only acceptable on a developer's machine.
	if w.DevlogFile != "" && (w.Env != "development" || w.Provider != "devlog") {
		return errors.New("DEVLOG_FILE only works in development with WHATSAPP_PROVIDER=devlog; unset it")
	}
	return nil
}

// Admin is the configuration of cmd/admin. It connects as the app role, like the API.
type Admin struct {
	Common
	DatabaseURL string  `env:"DATABASE_URL, required"`
	Secrets     Secrets `env:", prefix=SECRETS_"`
}

func (c Admin) validate() error {
	if err := c.Common.validate(); err != nil {
		return err
	}
	return c.Secrets.validate()
}

// LoadAdmin reads the admin command configuration from the process environment.
func LoadAdmin(ctx context.Context) (Admin, error) {
	return loadAdmin(ctx, envconfig.OsLookuper())
}

func loadAdmin(ctx context.Context, l envconfig.Lookuper) (Admin, error) {
	var c Admin
	if err := process(ctx, l, &c); err != nil {
		return Admin{}, fmt.Errorf("loading admin config: %w", err)
	}
	if err := c.validate(); err != nil {
		return Admin{}, fmt.Errorf("loading admin config: %w", err)
	}
	return c, nil
}

// Worker is the configuration of cmd/worker. It connects to the database as the app role, like
// the API, and every job runs inside one organization's scope.
type Worker struct {
	Common
	DatabaseURL string  `env:"DATABASE_URL, required"`
	RedisAddr   string  `env:"REDIS_ADDR, default=localhost:6379"`
	Secrets     Secrets `env:", prefix=SECRETS_"`
	// AutoGoPayBaseURL is as in API: reconcile asks AutoGoPay about pending QRs.
	AutoGoPayBaseURL string `env:"AUTOGOPAY_BASE_URL"`
	// DatabaseMaxConns is DATABASE_MAX_CONNS, the size of the worker's connection pool.
	DatabaseMaxConns int32 `env:"DATABASE_MAX_CONNS, default=20"`
	// PublicWebOrigin is as in API: the base of the payment links in reminders.
	PublicWebOrigin string `env:"PUBLIC_WEB_ORIGIN"`
	// WhatsAppProvider, DevlogFile, GOWA and Twilio are as in API: the worker sends reminders and
	// promise confirmations through the same provider as the API's login codes (though only
	// devlog and twilio send them until slice 5's pacing, decision 41.6).
	WhatsAppProvider string `env:"WHATSAPP_PROVIDER, default=devlog"`
	DevlogFile       string `env:"DEVLOG_FILE"`
	GOWA             GOWA   `env:", prefix=GOWA_"`
	Twilio           Twilio `env:", prefix=TWILIO_"`
	// Telegram is as in API.
	Telegram Telegram `env:", prefix=TELEGRAM_"`
}

// WhatsApp returns the worker's WhatsApp settings.
func (c Worker) WhatsApp() WhatsApp {
	return WhatsApp{Env: c.Env, Provider: c.WhatsAppProvider, DevlogFile: c.DevlogFile, GOWA: c.GOWA, Twilio: c.Twilio}
}

func (c Worker) validate() error {
	if err := c.Common.validate(); err != nil {
		return err
	}
	if err := validAutoGoPayURL(c.AutoGoPayBaseURL, c.Env); err != nil {
		return err
	}
	if err := c.Telegram.validate(c.Env); err != nil {
		return err
	}
	if c.DatabaseMaxConns < 2 {
		return fmt.Errorf("DATABASE_MAX_CONNS must be at least 2, got %d", c.DatabaseMaxConns)
	}
	if err := c.Secrets.validate(); err != nil {
		return err
	}
	if c.PublicWebOrigin == "" {
		return fmt.Errorf("PUBLIC_WEB_ORIGIN is required in %s", c.Env)
	}
	if err := validWebOrigin(c.PublicWebOrigin, c.Env); err != nil {
		return err
	}
	return c.WhatsApp().validate()
}

// validAutoGoPayURL checks AUTOGOPAY_BASE_URL: empty (AutoGoPay's own gateway), or an absolute
// URL with no path, query or credentials, https outside development. The organization's key is
// sent to it.
func validAutoGoPayURL(raw, env string) error {
	return validBaseURL("AUTOGOPAY_BASE_URL", raw, env, true)
}

// validBaseURL checks a provider base URL setting named name: empty (the provider's default), or
// an absolute URL with no path, query or credentials, https outside development, since a secret
// is sent to it. echo quotes the value in the error; it is off where a secret could have been
// pasted into the URL (the Bot API puts its token in the path).
func validBaseURL(name, raw, env string, echo bool) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") ||
		u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		if !echo {
			return fmt.Errorf("%s must be an absolute http(s) URL with no path", name)
		}
		return fmt.Errorf("%s must be an absolute http(s) URL with no path, got %q", name, raw)
	}
	if env != "development" && u.Scheme != "https" {
		return fmt.Errorf("%s must use https in %s", name, env)
	}
	return nil
}

// Telegram is the platform's Telegram bot (decision 42). Every setting is optional until the bot
// is used; a value that is set is checked at startup. BotToken and WebhookSecret are secrets:
// never log them (LogValue hides them) and never echo them in an error.
type Telegram struct {
	// BotToken is TELEGRAM_BOT_TOKEN, from BotFather: digits, a colon, then the secret.
	BotToken string `env:"BOT_TOKEN"`
	// BotUsername is TELEGRAM_BOT_USERNAME, without the @; required with a token, for deep links.
	BotUsername string `env:"BOT_USERNAME"`
	// WebhookSecret is TELEGRAM_WEBHOOK_SECRET, which Telegram sends back in the
	// X-Telegram-Bot-Api-Secret-Token header of each webhook call (used from TG-2).
	WebhookSecret string `env:"WEBHOOK_SECRET"`
	// APIBaseURL is TELEGRAM_API_BASE_URL; empty uses https://api.telegram.org. It must be https
	// outside development, since the token is in every request path.
	APIBaseURL string `env:"API_BASE_URL"`
	// Messages is TELEGRAM_MESSAGES: "off" or empty (the default) sends every message on WhatsApp;
	// "linked" sends a tenant's messages to their linked chat (the router, TG-3). Needs the bot.
	Messages string `env:"MESSAGES"`
}

// SendsToLinked reports whether tenants' messages go to their linked Telegram chats.
func (t Telegram) SendsToLinked() bool { return t.Messages == "linked" }

var (
	// telegramToken is the Bot API token shape; telegram.ValidToken applies the same rule.
	telegramToken         = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]{30,}$`)
	telegramUsername      = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	telegramWebhookSecret = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)
)

// Enabled reports whether a bot token is set.
func (t Telegram) Enabled() bool { return t.BotToken != "" }

// LogValue keeps the token and the webhook secret out of logs.
func (t Telegram) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("enabled", t.Enabled()),
		slog.String("bot_username", t.BotUsername),
		slog.Bool("webhook_secret_set", t.WebhookSecret != ""),
		slog.String("api_base_url", t.APIBaseURL),
		slog.String("messages", t.Messages),
	)
}

// validate checks each setting that is set. Errors name the setting, never its secret value.
func (t Telegram) validate(env string) error {
	// Telegram is a demo channel (decision 42), like the Twilio sandbox: development and staging
	// with test accounts only. Production stays WhatsApp.
	if env == "production" && (t.BotToken != "" || t.BotUsername != "" || t.WebhookSecret != "" || t.APIBaseURL != "" || t.SendsToLinked()) {
		return errors.New("Telegram is a demo channel for development and staging (decision 42); unset the TELEGRAM_* settings in production")
	}
	if t.BotToken != "" && !telegramToken.MatchString(t.BotToken) {
		return errors.New("TELEGRAM_BOT_TOKEN must be the token BotFather gives: digits, a colon, then at least 30 letters, digits, _ or -")
	}
	if t.BotToken != "" && t.BotUsername == "" {
		return errors.New("TELEGRAM_BOT_USERNAME is required when TELEGRAM_BOT_TOKEN is set")
	}
	if t.BotUsername != "" &&
		(!telegramUsername.MatchString(t.BotUsername) || !strings.HasSuffix(strings.ToLower(t.BotUsername), "bot")) {
		// The value is not echoed, in case a token was pasted into the wrong setting.
		return errors.New("TELEGRAM_BOT_USERNAME must be letters, digits or _ ending in \"bot\", without the @")
	}
	if t.WebhookSecret != "" && !telegramWebhookSecret.MatchString(t.WebhookSecret) {
		return errors.New("TELEGRAM_WEBHOOK_SECRET must be 1 to 256 characters of A-Z, a-z, 0-9, _ and -")
	}
	if err := validBaseURL("TELEGRAM_API_BASE_URL", t.APIBaseURL, env, false); err != nil {
		return err
	}
	switch t.Messages {
	case "", "off":
	case "linked":
		if t.BotToken == "" {
			return errors.New("TELEGRAM_MESSAGES=linked needs TELEGRAM_BOT_TOKEN")
		}
	default:
		return errors.New("TELEGRAM_MESSAGES must be off or linked")
	}
	// Staging receives the bot's updates only on the webhook, which needs its secret; without it
	// the bot could send but never link anyone.
	if env == "staging" && t.BotToken != "" && t.WebhookSecret == "" {
		return errors.New("TELEGRAM_WEBHOOK_SECRET is required in staging when TELEGRAM_BOT_TOKEN is set")
	}
	return nil
}

// TelegramPoll is the configuration of cmd/telegrampoll, which long-polls the development bot.
// It runs in development only and needs the bot token and the database.
type TelegramPoll struct {
	Common
	// DatabaseURL is DATABASE_URL, the app role: the update handler links and unlinks chats.
	DatabaseURL string `env:"DATABASE_URL, required"`
	// DatabaseMaxConns is DATABASE_MAX_CONNS; the poller handles one update at a time.
	DatabaseMaxConns int32    `env:"DATABASE_MAX_CONNS, default=4"`
	Telegram         Telegram `env:", prefix=TELEGRAM_"`
}

func (c TelegramPoll) validate() error {
	if err := c.Common.validate(); err != nil {
		return err
	}
	if c.Env != "development" {
		return fmt.Errorf("the Telegram poller only runs in development, not %s; staging uses the webhook", c.Env)
	}
	if !c.Telegram.Enabled() {
		return errors.New("the Telegram poller needs TELEGRAM_BOT_TOKEN (and TELEGRAM_BOT_USERNAME) in services/api/.env")
	}
	if c.DatabaseMaxConns < 2 {
		return fmt.Errorf("DATABASE_MAX_CONNS must be at least 2, got %d", c.DatabaseMaxConns)
	}
	return c.Telegram.validate(c.Env)
}

// LoadTelegramPoll reads the poller's configuration from the process environment.
func LoadTelegramPoll(ctx context.Context) (TelegramPoll, error) {
	return loadTelegramPoll(ctx, envconfig.OsLookuper())
}

func loadTelegramPoll(ctx context.Context, l envconfig.Lookuper) (TelegramPoll, error) {
	var c TelegramPoll
	if err := process(ctx, l, &c); err != nil {
		return TelegramPoll{}, fmt.Errorf("loading telegram poller config: %w", err)
	}
	if err := c.validate(); err != nil {
		return TelegramPoll{}, fmt.Errorf("loading telegram poller config: %w", err)
	}
	return c, nil
}

// FakeAutoGoPay is the configuration of cmd/fakeautogopay, the AutoGoPay stand-in for local runs
// and the web's end-to-end tests. It runs in development only.
type FakeAutoGoPay struct {
	Common
	// Addr is FAKE_AUTOGOPAY_ADDR, where the stand-in listens; loopback by default, since its
	// control routes have no authentication.
	Addr string `env:"FAKE_AUTOGOPAY_ADDR, default=127.0.0.1:8091"`
}

func (c FakeAutoGoPay) validate() error {
	if err := c.Common.validate(); err != nil {
		return err
	}
	if c.Env != "development" {
		return fmt.Errorf("the AutoGoPay stand-in only runs in development, not %s", c.Env)
	}
	return nil
}

// LoadFakeAutoGoPay reads the stand-in's configuration from the process environment.
func LoadFakeAutoGoPay(ctx context.Context) (FakeAutoGoPay, error) {
	return loadFakeAutoGoPay(ctx, envconfig.OsLookuper())
}

func loadFakeAutoGoPay(ctx context.Context, l envconfig.Lookuper) (FakeAutoGoPay, error) {
	var c FakeAutoGoPay
	if err := process(ctx, l, &c); err != nil {
		return FakeAutoGoPay{}, fmt.Errorf("loading fake AutoGoPay config: %w", err)
	}
	if err := c.validate(); err != nil {
		return FakeAutoGoPay{}, fmt.Errorf("loading fake AutoGoPay config: %w", err)
	}
	return c, nil
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
	if c.PublicWebOrigin == "" && c.Env == "development" {
		c.PublicWebOrigin = devWebOrigin
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
	if c.PublicWebOrigin == "" && c.Env == "development" {
		c.PublicWebOrigin = devWebOrigin
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
