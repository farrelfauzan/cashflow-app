package config

import (
	"context"
	"encoding/base64"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/sethvargo/go-envconfig/v2"
	"github.com/stretchr/testify/require"
)

var testSecret = strings.Repeat("s", 32)

// testKey is a valid SECRETS_KEY: the id "test" and 32 bytes of 'k', base64-encoded.
var testKey = "test:" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))

// noWhatsApp is the error of every WhatsApp provider outside development: none runs there until
// the Cloud API adapter (slice 5).
const noWhatsApp = "no WhatsApp provider runs in"

// requireOnlyNoWhatsApp checks that err is the missing WhatsApp provider. The API and the worker
// check the provider last, so a staging or production configuration that passes every other
// check fails only on it.
func requireOnlyNoWhatsApp(t *testing.T, err error) {
	t.Helper()
	require.ErrorContains(t, err, noWhatsApp)
}

func TestLoadAPI(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    API
		wantErr string
	}{
		{
			name: "defaults apply when only the required values are set",
			env:  map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey},
			want: API{
				Common:           Common{Env: "development", LogLevel: "info"},
				HTTPAddr:         ":8080",
				DatabaseURL:      "postgres://app@db/cashflow",
				RedisAddr:        "localhost:6379",
				AuthSecret:       testSecret,
				Secrets:          Secrets{Key: testKey},
				DatabaseMaxConns: 20, QRISMaxConcurrent: 10,
				AllowedOrigins:   "http://localhost:3000",
				PublicWebOrigin:  "http://localhost:3000",
				WhatsAppProvider: "devlog",
				OpenAPISpec:      "../../api/openapi.yaml",
			},
		},
		{
			name: "explicit values override defaults",
			env: map[string]string{
				"APP_ENV":      "development",
				"LOG_LEVEL":    "warn",
				"HTTP_ADDR":    ":9000",
				"DATABASE_URL": "postgres://app@db/cashflow",
				"REDIS_ADDR":   "redis:6379",
				"AUTH_SECRET":  testSecret, "SECRETS_KEY": testKey,
				"ALLOWED_ORIGINS":   "https://app.example.id",
				"PUBLIC_WEB_ORIGIN": "https://app.example.id",
				"WHATSAPP_PROVIDER": "devlog",
				"DEVLOG_FILE":       "/tmp/devlog.jsonl",
			},
			want: API{
				Common:           Common{Env: "development", LogLevel: "warn"},
				HTTPAddr:         ":9000",
				DatabaseURL:      "postgres://app@db/cashflow",
				RedisAddr:        "redis:6379",
				AuthSecret:       testSecret,
				Secrets:          Secrets{Key: testKey},
				DatabaseMaxConns: 20, QRISMaxConcurrent: 10,
				AllowedOrigins:   "https://app.example.id",
				PublicWebOrigin:  "https://app.example.id",
				WhatsAppProvider: "devlog",
				DevlogFile:       "/tmp/devlog.jsonl",
				OpenAPISpec:      "../../api/openapi.yaml",
			},
		},
		{
			name: "production has no WhatsApp provider until the Cloud API adapter",
			env: map[string]string{
				"APP_ENV": "production", "DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
				"PUBLIC_WEB_ORIGIN": "https://app.example.id",
			},
			wantErr: "no WhatsApp provider runs in production",
		},
		{
			name:    "missing database url is rejected",
			env:     map[string]string{"AUTH_SECRET": testSecret, "SECRETS_KEY": testKey},
			wantErr: "DATABASE_URL",
		},
		{
			name:    "missing auth secret is rejected",
			env:     map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "SECRETS_KEY": testKey},
			wantErr: "AUTH_SECRET",
		},
		{
			name:    "short auth secret is rejected",
			env:     map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": "short", "SECRETS_KEY": testKey},
			wantErr: "at least 32 bytes",
		},
		{
			name: "unknown environment is rejected",
			env: map[string]string{
				"APP_ENV":      "prod",
				"DATABASE_URL": "postgres://app@db/cashflow",
				"AUTH_SECRET":  testSecret, "SECRETS_KEY": testKey,
			},
			wantErr: "APP_ENV",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loadAPI(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestWhatsAppProviderChoice(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
		"PUBLIC_WEB_ORIGIN": "https://app.example.id",
	}
	with := func(extra map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"devlog in development", with(nil), ""},
		{"devlog refused in staging", with(map[string]string{"APP_ENV": "staging"}), "WHATSAPP_PROVIDER=devlog only runs in development; " + noWhatsApp + " staging"},
		{"devlog refused in production", with(map[string]string{"APP_ENV": "production"}), "WHATSAPP_PROVIDER=devlog only runs in development; " + noWhatsApp + " production"},
		{"cloud api not built", with(map[string]string{"WHATSAPP_PROVIDER": "cloudapi"}), "WHATSAPP_PROVIDER=cloudapi is not built yet"},
		{"cloud api not built in production", with(map[string]string{"APP_ENV": "production", "WHATSAPP_PROVIDER": "cloudapi"}), "WHATSAPP_PROVIDER=cloudapi is not built yet"},
		{"unknown provider", with(map[string]string{"WHATSAPP_PROVIDER": "messagebird"}), "devlog, twilio or cloudapi"},
		{"devlog file in development", with(map[string]string{"DEVLOG_FILE": "/tmp/devlog.jsonl"}), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadAPI(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestTwilioSettings(t *testing.T) {
	// Built at run time so no source line looks like a real Twilio identifier to secret scanning.
	fakeHex := strings.Repeat("0123456789abcdef", 2)
	sid := "AC" + fakeHex
	const token = "test-auth-token" //nolint:gosec // G101: a test placeholder, not a credential
	env := func(appEnv string, twilio map[string]string) map[string]string {
		m := map[string]string{
			"APP_ENV": appEnv, "DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
			"PUBLIC_WEB_ORIGIN": "https://app.example.id", "WHATSAPP_PROVIDER": "twilio",
		}
		for k, v := range twilio {
			m[k] = v
		}
		return m
	}
	sandbox := map[string]string{
		"TWILIO_ACCOUNT_SID": sid, "TWILIO_AUTH_TOKEN": token, "TWILIO_WHATSAPP_FROM": "+14155238886",
	}
	without := func(key string) map[string]string {
		m := map[string]string{}
		for k, v := range sandbox {
			if k != key {
				m[k] = v
			}
		}
		return m
	}
	withValue := func(key, value string) map[string]string {
		m := without(key)
		m[key] = value
		return m
	}
	allThree := "TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN and TWILIO_WHATSAPP_FROM"
	tests := []struct {
		name    string
		env     map[string]string
		want    Twilio
		wantErr string
	}{
		{name: "sandbox in development", env: env("development", sandbox),
			want: Twilio{AccountSID: sid, AuthToken: token, WhatsAppFrom: "+14155238886"}},
		{name: "another account's sandbox number in development", env: env("development", withValue("TWILIO_WHATSAPP_FROM", "+17372508034")),
			want: Twilio{AccountSID: sid, AuthToken: token, WhatsAppFrom: "+17372508034"}},
		{name: "nothing set", env: env("development", nil), wantErr: allThree},
		{name: "missing account sid", env: env("development", without("TWILIO_ACCOUNT_SID")), wantErr: allThree},
		{name: "missing auth token", env: env("development", without("TWILIO_AUTH_TOKEN")), wantErr: allThree},
		{name: "missing sender", env: env("development", without("TWILIO_WHATSAPP_FROM")), wantErr: allThree},
		{name: "account sid with wrong prefix", env: env("development", withValue("TWILIO_ACCOUNT_SID", "SK"+fakeHex)), wantErr: "TWILIO_ACCOUNT_SID must be"},
		{name: "account sid too short", env: env("development", withValue("TWILIO_ACCOUNT_SID", "AC0123")), wantErr: "TWILIO_ACCOUNT_SID must be"},
		{name: "sender without plus", env: env("development", withValue("TWILIO_WHATSAPP_FROM", "14155238886")), wantErr: "TWILIO_WHATSAPP_FROM must be an E.164"},
		{name: "sender with whatsapp prefix", env: env("development", withValue("TWILIO_WHATSAPP_FROM", "whatsapp:+14155238886")), wantErr: "TWILIO_WHATSAPP_FROM must be an E.164"},
		{name: "twilio refused in staging", env: env("staging", sandbox), wantErr: "WHATSAPP_PROVIDER=twilio only runs in development"},
		{name: "twilio refused in production", env: env("production", sandbox), wantErr: "WHATSAPP_PROVIDER=twilio only runs in development"},
		{name: "twilio refused in staging whatever the sender", env: env("staging", withValue("TWILIO_WHATSAPP_FROM", "+17372508034")),
			wantErr: "WHATSAPP_PROVIDER=twilio only runs in development"},
		{name: "devlog file refused with twilio", env: env("development", withValue("DEVLOG_FILE", "/tmp/devlog.jsonl")), wantErr: "DEVLOG_FILE only works"},
		{name: "content sids are kept", env: env("development", withValue("TWILIO_CONTENT_SIDS", "login_code=HX"+fakeHex)),
			want: Twilio{AccountSID: sid, AuthToken: token, WhatsAppFrom: "+14155238886", ContentSIDs: "login_code=HX" + fakeHex}},
		{name: "content sids without a template name", env: env("development", withValue("TWILIO_CONTENT_SIDS", "HX"+fakeHex)), wantErr: "template=ContentSid pairs"},
		{name: "content sids are ignored with devlog", env: map[string]string{
			"APP_ENV": "development", "DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
			"WHATSAPP_PROVIDER": "devlog", "TWILIO_CONTENT_SIDS": "not a pair",
		}, want: Twilio{ContentSIDs: "not a pair"}},
		{name: "content sids with a template named twice", env: env("development", withValue("TWILIO_CONTENT_SIDS", "login_code=HX"+fakeHex+",login_code=HX"+fakeHex)), wantErr: "names login_code twice"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := loadAPI(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.NotContains(t, err.Error(), token, "the auth token never appears in an error")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got.Twilio)
		})
	}
}

func TestTwilioContentSIDMap(t *testing.T) {
	t.Parallel()
	hx := "HX" + strings.Repeat("0123456789abcdef", 2)
	tests := []struct {
		name    string
		value   string
		want    map[string]string
		wantErr string
	}{
		{name: "unset", value: "", want: map[string]string{}},
		{name: "only spaces", value: "  ", want: map[string]string{}},
		{name: "one pair", value: "login_code=" + hx, want: map[string]string{"login_code": hx}},
		{name: "two pairs with spaces", value: " login_code = " + hx + " , member_invite=" + hx + " ",
			want: map[string]string{"login_code": hx, "member_invite": hx}},
		{name: "missing sid", value: "login_code=", wantErr: "pair 1 has no Content SID after '='"},
		{name: "missing name", value: "=" + hx, wantErr: "pair 1 has no template name before '='"},
		{name: "only the sid", value: hx, wantErr: "pair 1 has no '='"},
		{name: "trailing comma", value: "login_code=" + hx + ",", wantErr: "pair 2 has no '='"},
		{name: "duplicate name", value: "login_code=" + hx + ",login_code=" + hx, wantErr: "names login_code twice"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Twilio{ContentSIDs: tc.value}.ContentSIDMap()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestPublicWebOrigin(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
	}
	env := func(appEnv, origin string) map[string]string {
		m := map[string]string{"APP_ENV": appEnv}
		for k, v := range base {
			m[k] = v
		}
		if origin != "" {
			m["PUBLIC_WEB_ORIGIN"] = origin
		}
		return m
	}
	tests := []struct {
		name      string
		env       map[string]string
		wantLogin string
		wantErr   string
	}{
		{name: "development defaults to the Next.js dev server", env: env("development", ""), wantLogin: "http://localhost:3000/login"},
		{name: "http is fine in development", env: env("development", "http://192.168.1.5:3000"), wantLogin: "http://192.168.1.5:3000/login"},
		{name: "https in production", env: env("production", "https://app.example.id"), wantLogin: "https://app.example.id/login"},
		{name: "missing outside development", env: env("staging", ""), wantErr: "PUBLIC_WEB_ORIGIN is required in staging"},
		{name: "http outside development", env: env("production", "http://app.example.id"), wantErr: "must use https in production"},
		{name: "a path", env: env("production", "https://app.example.id/app"), wantErr: "origin only"},
		{name: "a trailing slash", env: env("production", "https://app.example.id/"), wantErr: "origin only"},
		{name: "a query", env: env("production", "https://app.example.id?x=1"), wantErr: "origin only"},
		{name: "credentials", env: env("production", "https://u:p@app.example.id"), wantErr: "origin only"},
		{name: "not absolute", env: env("production", "app.example.id"), wantErr: "absolute http(s) origin"},
		{name: "another scheme", env: env("development", "ftp://app.example.id"), wantErr: "absolute http(s) origin"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loadAPI(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			if tc.env["APP_ENV"] != "development" {
				// The origin passed; outside development the load then stops at WhatsApp.
				requireOnlyNoWhatsApp(t, err)
				require.Equal(t, tc.wantLogin, API{PublicWebOrigin: tc.env["PUBLIC_WEB_ORIGIN"]}.LoginURL())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantLogin, got.LoginURL())
		})
	}
}

func TestAPIOriginsAndCookies(t *testing.T) {
	c := API{Common: Common{Env: "development"}, AllowedOrigins: " http://localhost:3000 , http://localhost:8080,,"}
	require.Equal(t, []string{"http://localhost:3000", "http://localhost:8080"}, c.Origins())
	require.False(t, c.CookieSecure())

	c.Env = "staging"
	require.True(t, c.CookieSecure())
}

func TestDocsOnlyInDevelopment(t *testing.T) {
	c := API{Common: Common{Env: "development"}, OpenAPISpec: "spec.yaml"}
	require.Equal(t, "spec.yaml", c.DocsSpecPath())
	c.Env = "production"
	require.Empty(t, c.DocsSpecPath())
}

func TestLoadWorker(t *testing.T) {
	// Built at run time so no source line looks like a real Twilio identifier to secret scanning.
	sid := "AC" + strings.Repeat("0123456789abcdef", 2)
	const token = "test-auth-token" //nolint:gosec // G101: a test placeholder, not a credential
	tests := []struct {
		name    string
		env     map[string]string
		want    Worker
		wantErr string
	}{
		{
			name: "defaults apply when only the database url and secrets key are set",
			env:  map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "SECRETS_KEY": testKey},
			want: Worker{
				Common:           Common{Env: "development", LogLevel: "info"},
				DatabaseURL:      "postgres://app@db/cashflow",
				RedisAddr:        "localhost:6379",
				Secrets:          Secrets{Key: testKey},
				DatabaseMaxConns: 20,
				PublicWebOrigin:  "http://localhost:3000",
				WhatsAppProvider: "devlog",
			},
		},
		{
			name: "the worker sends through the configured provider",
			env: map[string]string{
				"APP_ENV": "development", "DATABASE_URL": "postgres://app@db/cashflow", "SECRETS_KEY": testKey,
				"PUBLIC_WEB_ORIGIN": "https://app.example.id", "WHATSAPP_PROVIDER": "twilio",
				"TWILIO_ACCOUNT_SID": sid, "TWILIO_AUTH_TOKEN": token, "TWILIO_WHATSAPP_FROM": "+14155238886",
			},
			want: Worker{
				Common:           Common{Env: "development", LogLevel: "info"},
				DatabaseURL:      "postgres://app@db/cashflow",
				RedisAddr:        "localhost:6379",
				Secrets:          Secrets{Key: testKey},
				DatabaseMaxConns: 20,
				PublicWebOrigin:  "https://app.example.id",
				WhatsAppProvider: "twilio",
				Twilio:           Twilio{AccountSID: sid, AuthToken: token, WhatsAppFrom: "+14155238886"},
			},
		},
		{
			name:    "a payment link origin is required outside development",
			env:     map[string]string{"APP_ENV": "staging", "DATABASE_URL": "postgres://app@db/cashflow", "SECRETS_KEY": testKey},
			wantErr: "PUBLIC_WEB_ORIGIN is required",
		},
		{
			name: "devlog is refused outside development",
			env: map[string]string{
				"APP_ENV": "staging", "DATABASE_URL": "postgres://app@db/cashflow", "SECRETS_KEY": testKey,
				"PUBLIC_WEB_ORIGIN": "https://app.example.id",
			},
			wantErr: "WHATSAPP_PROVIDER=devlog only runs in development",
		},
		{
			name: "twilio needs its settings",
			env: map[string]string{
				"DATABASE_URL": "postgres://app@db/cashflow", "SECRETS_KEY": testKey, "WHATSAPP_PROVIDER": "twilio",
			},
			wantErr: "TWILIO_ACCOUNT_SID",
		},
		{
			name: "production has no WhatsApp provider until the Cloud API adapter",
			env: map[string]string{
				"APP_ENV": "production", "DATABASE_URL": "postgres://app@db/cashflow", "SECRETS_KEY": testKey,
				"PUBLIC_WEB_ORIGIN": "https://app.example.id",
			},
			wantErr: "no WhatsApp provider runs in production",
		},
		{
			name:    "missing database url is rejected",
			env:     map[string]string{"SECRETS_KEY": testKey},
			wantErr: "DATABASE_URL",
		},
		{
			name:    "an unknown environment is rejected",
			env:     map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "SECRETS_KEY": testKey, "APP_ENV": "prod"},
			wantErr: "APP_ENV",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := loadWorker(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLoadMigrateRequiresOwnerURL(t *testing.T) {
	_, err := loadMigrate(context.Background(), envconfig.MapLookuper(map[string]string{
		"DATABASE_URL": "postgres://app@db/cashflow",
	}))
	require.ErrorContains(t, err, "MIGRATE_DATABASE_URL")
}

// TestSecretsKeyEveryBinary checks that the API, the worker and the admin command each refuse to
// start without a valid SECRETS_KEY, and that no error repeats a key.
func TestSecretsKeyEveryBinary(t *testing.T) {
	t.Parallel()
	b64 := func(n int) string { return base64.StdEncoding.EncodeToString([]byte(strings.Repeat("p", n))) }
	short := "k1:" + b64(16)
	loaders := map[string]func(map[string]string) (Secrets, error){
		"api": func(env map[string]string) (Secrets, error) {
			c, err := loadAPI(context.Background(), envconfig.MapLookuper(env))
			return c.Secrets, err
		},
		"worker": func(env map[string]string) (Secrets, error) {
			c, err := loadWorker(context.Background(), envconfig.MapLookuper(env))
			return c.Secrets, err
		},
		"admin": func(env map[string]string) (Secrets, error) {
			c, err := loadAdmin(context.Background(), envconfig.MapLookuper(env))
			return c.Secrets, err
		},
	}
	tests := []struct {
		name    string
		vars    map[string]string // added to a valid base without SECRETS_*
		want    Secrets
		wantErr string
	}{
		{name: "missing", vars: map[string]string{}, wantErr: "SECRETS_KEY"},
		{name: "empty", vars: map[string]string{"SECRETS_KEY": ""}, wantErr: "SECRETS_KEY is required"},
		{name: "no key id", vars: map[string]string{"SECRETS_KEY": b64(32)}, wantErr: "SECRETS_KEY has no ':'"},
		{name: "16-byte key", vars: map[string]string{"SECRETS_KEY": short}, wantErr: "SECRETS_KEY has a key of 16 bytes"},
		{name: "bad base64", vars: map[string]string{"SECRETS_KEY": "k1:%%%"}, wantErr: "SECRETS_KEY has a key that is not valid standard base64"},
		{name: "bad previous key", vars: map[string]string{"SECRETS_KEY": testKey, "SECRETS_KEY_PREVIOUS": short},
			wantErr: "SECRETS_KEY_PREVIOUS entry 1 has a key of 16 bytes"},
		{name: "previous key reusing the current id", vars: map[string]string{"SECRETS_KEY": testKey, "SECRETS_KEY_PREVIOUS": "test:" + b64(32)},
			wantErr: "uses the id \"test\" of SECRETS_KEY"},
		{name: "valid", vars: map[string]string{"SECRETS_KEY": testKey}, want: Secrets{Key: testKey}},
		{name: "valid with a previous key", vars: map[string]string{"SECRETS_KEY": testKey, "SECRETS_KEY_PREVIOUS": "old:" + b64(32)},
			want: Secrets{Key: testKey, Previous: "old:" + b64(32)}},
	}
	for binary, load := range loaders {
		for _, tc := range tests {
			t.Run(binary+": "+tc.name, func(t *testing.T) {
				t.Parallel()
				env := map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret}
				for k, v := range tc.vars {
					env[k] = v
				}
				got, err := load(env)
				if tc.wantErr != "" {
					require.ErrorContains(t, err, tc.wantErr)
					for _, v := range tc.vars {
						if _, key, ok := strings.Cut(v, ":"); ok && key != "" {
							require.NotContains(t, err.Error(), key, "an error never repeats a key")
						}
					}
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
				ring, err := got.Keyring()
				require.NoError(t, err)
				require.Equal(t, "test", ring.CurrentKeyID())
			})
		}
	}
}

// TestEnvExampleSecretsKey keeps the development key in .env.example loadable, so `make env`
// gives a working setup.
func TestEnvExampleSecretsKey(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../../.env.example")
	require.NoError(t, err)
	vars := map[string]string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(k, "#") {
			vars[k] = v
		}
	}
	require.Contains(t, vars, "SECRETS_KEY")
	ring, err := Secrets{Key: vars["SECRETS_KEY"], Previous: vars["SECRETS_KEY_PREVIOUS"]}.Keyring()
	require.NoError(t, err)
	require.Equal(t, "dev-only", ring.CurrentKeyID())
}

func TestAutoGoPayBaseURL(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
		"PUBLIC_WEB_ORIGIN": "https://app.example.id",
	}
	env := func(appEnv, url string) map[string]string {
		m := map[string]string{"APP_ENV": appEnv, "AUTOGOPAY_BASE_URL": url}
		for k, v := range base {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{name: "empty uses AutoGoPay's gateway", env: env("production", "")},
		{name: "https in production", env: env("production", "https://v1-gateway.autogopay.site")},
		{name: "a trailing slash", env: env("production", "https://v1-gateway.autogopay.site/")},
		{name: "a local stub over http in development", env: env("development", "http://localhost:9090")},
		{name: "http outside development", env: env("staging", "http://gateway.test"), wantErr: "must use https in staging"},
		{name: "a path", env: env("production", "https://gateway.test/v1"), wantErr: "no path"},
		{name: "credentials", env: env("production", "https://u:p@gateway.test"), wantErr: "no path"},
		{name: "a query", env: env("production", "https://gateway.test?x=1"), wantErr: "no path"},
		{name: "not absolute", env: env("production", "gateway.test"), wantErr: "no path"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, apiErr := loadAPI(context.Background(), envconfig.MapLookuper(tc.env))
			_, workerErr := loadWorker(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr != "" {
				require.ErrorContains(t, apiErr, tc.wantErr)
				require.ErrorContains(t, workerErr, tc.wantErr)
				return
			}
			if tc.env["APP_ENV"] != "development" {
				// The URL passed; outside development the load then stops at WhatsApp.
				requireOnlyNoWhatsApp(t, apiErr)
				requireOnlyNoWhatsApp(t, workerErr)
				return
			}
			require.NoError(t, apiErr)
			require.NoError(t, workerErr)
			require.Equal(t, tc.env["AUTOGOPAY_BASE_URL"], got.AutoGoPayBaseURL)
		})
	}
}

func TestPoolAndQRISBound(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
	}
	env := func(kv ...string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	tests := []struct {
		name      string
		env       map[string]string
		wantConns int32
		wantQRIS  int
		wantErr   string
	}{
		{name: "defaults", env: env(), wantConns: 20, wantQRIS: 10},
		{name: "set both", env: env("DATABASE_MAX_CONNS", "20", "QRIS_MAX_CONCURRENT", "5"), wantConns: 20, wantQRIS: 5},
		{name: "QRIS as many as the pool", env: env("DATABASE_MAX_CONNS", "4", "QRIS_MAX_CONCURRENT", "4"), wantErr: "QRIS_MAX_CONCURRENT must be 1 to DATABASE_MAX_CONNS-1 (3)"},
		{name: "QRIS zero", env: env("QRIS_MAX_CONCURRENT", "0"), wantErr: "QRIS_MAX_CONCURRENT"},
		{name: "a pool of one", env: env("DATABASE_MAX_CONNS", "1"), wantErr: "DATABASE_MAX_CONNS must be at least 2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := loadAPI(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantConns, got.DatabaseMaxConns)
			require.Equal(t, tc.wantQRIS, got.QRISMaxConcurrent)
		})
	}
	_, err := loadWorker(context.Background(), envconfig.MapLookuper(env("DATABASE_MAX_CONNS", "1")))
	require.ErrorContains(t, err, "DATABASE_MAX_CONNS must be at least 2")
}

func TestLoadFakeAutoGoPay(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		wantAddr string
		wantErr  string
	}{
		{name: "loopback by default in development", env: map[string]string{}, wantAddr: "127.0.0.1:8091"},
		{name: "another address", env: map[string]string{"FAKE_AUTOGOPAY_ADDR": "127.0.0.1:9999"}, wantAddr: "127.0.0.1:9999"},
		{name: "refused in staging", env: map[string]string{"APP_ENV": "staging"}, wantErr: "only runs in development"},
		{name: "refused in production", env: map[string]string{"APP_ENV": "production"}, wantErr: "only runs in development"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := loadFakeAutoGoPay(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantAddr, got.Addr)
		})
	}
}

func TestTelegramSettings(t *testing.T) {
	// A placeholder in the Bot API token shape, built at run time so no source line looks like a
	// real token to secret scanning.
	token := "7000000001:" + strings.Repeat("AbCdEfGhIj", 4)
	_, tokenSecret, _ := strings.Cut(token, ":")
	webhookSecret := strings.Repeat("w-_9", 8)
	env := func(appEnv string, kv ...string) map[string]string {
		m := map[string]string{
			"APP_ENV": appEnv, "DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
			"PUBLIC_WEB_ORIGIN": "https://app.example.id",
		}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	bot := []string{"TELEGRAM_BOT_TOKEN", token, "TELEGRAM_BOT_USERNAME", "CashflowDevBot"}
	with := func(kv ...string) []string { return append(append([]string{}, bot...), kv...) }
	tests := []struct {
		name    string
		env     map[string]string
		want    Telegram
		wantErr string
	}{
		{name: "nothing set", env: env("staging")},
		{name: "nothing set in production", env: env("production")},
		{name: "messages off in production", env: env("production", "TELEGRAM_MESSAGES", "off"), want: Telegram{Messages: "off"}},
		{name: "messages to linked chats in production is refused (demo channel)", env: env("production", "TELEGRAM_MESSAGES", "linked"),
			wantErr: "Telegram is a demo channel"},
		{name: "messages to linked chats in development", env: env("development", with("TELEGRAM_MESSAGES", "linked")...),
			want: Telegram{BotToken: token, BotUsername: "CashflowDevBot", Messages: "linked"}},
		{name: "messages to linked chats without a bot", env: env("development", "TELEGRAM_MESSAGES", "linked"),
			wantErr: "TELEGRAM_MESSAGES=linked needs TELEGRAM_BOT_TOKEN"},
		{name: "an unknown messages value", env: env("development", with("TELEGRAM_MESSAGES", "all")...),
			wantErr: "TELEGRAM_MESSAGES must be off or linked"},
		{name: "a token in production is refused (demo channel)", env: env("production", bot...), wantErr: "Telegram is a demo channel"},
		{name: "only a webhook secret in production is refused", env: env("production", "TELEGRAM_WEBHOOK_SECRET", webhookSecret), wantErr: "Telegram is a demo channel"},
		{name: "token and username in development", env: env("development", bot...), want: Telegram{BotToken: token, BotUsername: "CashflowDevBot"}},
		{name: "token and username in staging without a webhook secret", env: env("staging", bot...),
			wantErr: "TELEGRAM_WEBHOOK_SECRET is required in staging"},
		{name: "username ending in lowercase bot", env: env("development", "TELEGRAM_BOT_TOKEN", token, "TELEGRAM_BOT_USERNAME", "cashflow_dev_bot"),
			want: Telegram{BotToken: token, BotUsername: "cashflow_dev_bot"}},
		{name: "all four", env: env("staging", with("TELEGRAM_WEBHOOK_SECRET", webhookSecret, "TELEGRAM_API_BASE_URL", "https://api.telegram.org")...),
			want: Telegram{BotToken: token, BotUsername: "CashflowDevBot", WebhookSecret: webhookSecret, APIBaseURL: "https://api.telegram.org"}},
		{name: "a local Bot API server over http in development", env: env("development", with("TELEGRAM_API_BASE_URL", "http://127.0.0.1:8081")...),
			want: Telegram{BotToken: token, BotUsername: "CashflowDevBot", APIBaseURL: "http://127.0.0.1:8081"}},
		{name: "token without a colon", env: env("staging", "TELEGRAM_BOT_TOKEN", strings.ReplaceAll(token, ":", ""), "TELEGRAM_BOT_USERNAME", "CashflowDevBot"),
			wantErr: "TELEGRAM_BOT_TOKEN must be"},
		{name: "token secret too short", env: env("staging", "TELEGRAM_BOT_TOKEN", "7000000001:"+tokenSecret[:29], "TELEGRAM_BOT_USERNAME", "CashflowDevBot"),
			wantErr: "TELEGRAM_BOT_TOKEN must be"},
		{name: "token with letters in the bot id", env: env("staging", "TELEGRAM_BOT_TOKEN", "70a0:"+tokenSecret, "TELEGRAM_BOT_USERNAME", "CashflowDevBot"),
			wantErr: "TELEGRAM_BOT_TOKEN must be"},
		{name: "token with a bad character", env: env("staging", "TELEGRAM_BOT_TOKEN", token+"!", "TELEGRAM_BOT_USERNAME", "CashflowDevBot"),
			wantErr: "TELEGRAM_BOT_TOKEN must be"},
		{name: "token without a username", env: env("staging", "TELEGRAM_BOT_TOKEN", token), wantErr: "TELEGRAM_BOT_USERNAME is required"},
		{name: "username with @", env: env("staging", "TELEGRAM_BOT_TOKEN", token, "TELEGRAM_BOT_USERNAME", "@CashflowDevBot"),
			wantErr: "TELEGRAM_BOT_USERNAME must be"},
		{name: "username not ending in bot", env: env("staging", "TELEGRAM_BOT_TOKEN", token, "TELEGRAM_BOT_USERNAME", "CashflowDev"),
			wantErr: "TELEGRAM_BOT_USERNAME must be"},
		{name: "token pasted as the username", env: env("staging", "TELEGRAM_BOT_TOKEN", token, "TELEGRAM_BOT_USERNAME", token),
			wantErr: "TELEGRAM_BOT_USERNAME must be"},
		{name: "webhook secret with a bad character", env: env("staging", with("TELEGRAM_WEBHOOK_SECRET", "abc def")...),
			wantErr: "TELEGRAM_WEBHOOK_SECRET must be"},
		{name: "webhook secret too long", env: env("staging", with("TELEGRAM_WEBHOOK_SECRET", strings.Repeat("a", 257))...),
			wantErr: "TELEGRAM_WEBHOOK_SECRET must be"},
		{name: "webhook secret of 256 characters", env: env("staging", with("TELEGRAM_WEBHOOK_SECRET", strings.Repeat("a", 256))...),
			want: Telegram{BotToken: token, BotUsername: "CashflowDevBot", WebhookSecret: strings.Repeat("a", 256)}},
		{name: "http base url outside development", env: env("staging", with("TELEGRAM_API_BASE_URL", "http://api.telegram.org")...),
			wantErr: "TELEGRAM_API_BASE_URL must use https in staging"},
		{name: "base url with the token in its path", env: env("staging", with("TELEGRAM_API_BASE_URL", "https://api.telegram.org/bot"+token)...),
			wantErr: "TELEGRAM_API_BASE_URL must be an absolute http(s) URL"},
		{name: "relative base url", env: env("staging", with("TELEGRAM_API_BASE_URL", "api.telegram.org")...),
			wantErr: "TELEGRAM_API_BASE_URL must be an absolute http(s) URL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, apiErr := loadAPI(context.Background(), envconfig.MapLookuper(tc.env))
			worker, workerErr := loadWorker(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr != "" {
				for _, err := range []error{apiErr, workerErr} {
					require.ErrorContains(t, err, tc.wantErr)
					require.NotContains(t, err.Error(), tokenSecret, "the token never appears in an error")
					require.NotContains(t, err.Error(), "abc def", "the webhook secret never appears in an error")
				}
				return
			}
			if tc.env["APP_ENV"] != "development" {
				// Telegram passed; outside development the load then stops at WhatsApp.
				requireOnlyNoWhatsApp(t, apiErr)
				requireOnlyNoWhatsApp(t, workerErr)
				return
			}
			require.NoError(t, apiErr)
			require.NoError(t, workerErr)
			require.Equal(t, tc.want, got.Telegram)
			require.Equal(t, tc.want, worker.Telegram)
		})
	}
}

func TestTelegramLogValueHidesSecrets(t *testing.T) {
	t.Parallel()
	token := "7000000001:" + strings.Repeat("AbCdEfGhIj", 4)
	var buf strings.Builder
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	log.Info("config", "telegram", Telegram{BotToken: token, BotUsername: "CashflowDevBot", WebhookSecret: "hook-secret-value"})
	require.NotContains(t, buf.String(), "AbCdEfGhIj")
	require.NotContains(t, buf.String(), "hook-secret-value")
	require.Contains(t, buf.String(), `"enabled":true`)
}

func TestLoadTelegramPoll(t *testing.T) {
	token := "7000000001:" + strings.Repeat("AbCdEfGhIj", 4)
	const db = "postgres://app@db/cashflow"
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{name: "development with a bot", env: map[string]string{"DATABASE_URL": db, "TELEGRAM_BOT_TOKEN": token, "TELEGRAM_BOT_USERNAME": "cashflow_dev_bot"}},
		{name: "no token", env: map[string]string{"DATABASE_URL": db}, wantErr: "needs TELEGRAM_BOT_TOKEN"},
		{name: "no database", env: map[string]string{"TELEGRAM_BOT_TOKEN": token, "TELEGRAM_BOT_USERNAME": "cashflow_dev_bot"}, wantErr: "DATABASE_URL"},
		{name: "a pool too small", env: map[string]string{"DATABASE_URL": db, "DATABASE_MAX_CONNS": "1", "TELEGRAM_BOT_TOKEN": token, "TELEGRAM_BOT_USERNAME": "cashflow_dev_bot"},
			wantErr: "DATABASE_MAX_CONNS must be at least 2"},
		{name: "token without username", env: map[string]string{"DATABASE_URL": db, "TELEGRAM_BOT_TOKEN": token}, wantErr: "TELEGRAM_BOT_USERNAME is required"},
		{name: "malformed token", env: map[string]string{"DATABASE_URL": db, "TELEGRAM_BOT_TOKEN": "nope", "TELEGRAM_BOT_USERNAME": "cashflow_dev_bot"}, wantErr: "TELEGRAM_BOT_TOKEN must be"},
		{name: "refused in staging", env: map[string]string{"DATABASE_URL": db, "APP_ENV": "staging", "TELEGRAM_BOT_TOKEN": token, "TELEGRAM_BOT_USERNAME": "cashflow_dev_bot"},
			wantErr: "only runs in development"},
		{name: "refused in production", env: map[string]string{"DATABASE_URL": db, "APP_ENV": "production", "TELEGRAM_BOT_TOKEN": token, "TELEGRAM_BOT_USERNAME": "cashflow_dev_bot"},
			wantErr: "only runs in development"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := loadTelegramPoll(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.NotContains(t, err.Error(), "AbCdEfGhIj")
				return
			}
			require.NoError(t, err)
			require.Equal(t, token, got.Telegram.BotToken)
			require.Equal(t, db, got.DatabaseURL)
			require.EqualValues(t, 4, got.DatabaseMaxConns)
		})
	}
}

// TestAdminRunDaySending: run-day sends only with WHATSAPP_PROVIDER=devlog in development; unset
// does not mean devlog for it, and its settings never stop another admin command from loading.
func TestAdminRunDaySending(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		vars    map[string]string
		want    DevlogSending
		sends   bool
		wantErr string
	}{
		{name: "unset provider records only", vars: map[string]string{"APP_ENV": "development"}},
		{name: "cloudapi records only", vars: map[string]string{"APP_ENV": "development", "WHATSAPP_PROVIDER": "cloudapi"}},
		{name: "twilio records only", vars: map[string]string{"APP_ENV": "development", "WHATSAPP_PROVIDER": "twilio"}},
		{name: "devlog outside development records only", vars: map[string]string{"APP_ENV": "staging", "WHATSAPP_PROVIDER": "devlog"}},
		{
			name:  "devlog defaults the origin to the dev server",
			vars:  map[string]string{"APP_ENV": "development", "WHATSAPP_PROVIDER": "devlog"},
			want:  DevlogSending{WhatsApp: WhatsApp{Env: "development", Provider: "devlog"}, WebOrigin: "http://localhost:3000"},
			sends: true,
		},
		{
			name: "devlog with a file and an origin",
			vars: map[string]string{
				"APP_ENV": "development", "WHATSAPP_PROVIDER": "devlog",
				"DEVLOG_FILE": "/tmp/devlog.jsonl", "PUBLIC_WEB_ORIGIN": "http://127.0.0.1:3001",
			},
			want: DevlogSending{
				WhatsApp:  WhatsApp{Env: "development", Provider: "devlog", DevlogFile: "/tmp/devlog.jsonl"},
				WebOrigin: "http://127.0.0.1:3001",
			},
			sends: true,
		},
		{
			name:    "devlog with an origin that has a path",
			vars:    map[string]string{"APP_ENV": "development", "WHATSAPP_PROVIDER": "devlog", "PUBLIC_WEB_ORIGIN": "http://localhost:3000/app"},
			wantErr: "PUBLIC_WEB_ORIGIN must be an origin only",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "SECRETS_KEY": testKey}
			for k, v := range tc.vars {
				env[k] = v
			}
			c, err := loadAdmin(context.Background(), envconfig.MapLookuper(env))
			require.NoError(t, err, "run-day's settings never stop the admin config from loading")
			got, sends, err := c.RunDaySending()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.sends, sends)
			require.Equal(t, tc.want, got)
		})
	}
}

// storageTestSecret is the development MinIO password of infra/compose.dev.yaml.
const storageTestSecret = "cashflow-dev-secret" //nolint:gosec // G101: a development placeholder, not a credential

func TestStorageSettings(t *testing.T) {
	t.Parallel()
	minio := Storage{
		Endpoint: "http://localhost:9100", Bucket: "cashflow-dev", Region: "us-east-1",
		AccessKeyID: "cashflow", SecretAccessKey: storageTestSecret, PathStyle: true,
	}
	r2 := Storage{
		Endpoint: "https://0123456789abcdef.r2.cloudflarestorage.com", Bucket: "cashflow-prod",
		AccessKeyID: "id", SecretAccessKey: "secret",
	}
	with := func(s Storage, f func(*Storage)) Storage { f(&s); return s }
	tests := []struct {
		name    string
		env     string
		s       Storage
		wantErr string
	}{
		{name: "nothing set in development turns uploads off", env: "development"},
		{name: "nothing set in staging", env: "staging", wantErr: "uploads need a bucket in staging"},
		{name: "nothing set in production", env: "production", wantErr: "uploads need a bucket in production"},
		{name: "MinIO in development", env: "development", s: minio},
		{name: "MinIO with a browser endpoint", env: "development", s: with(minio, func(s *Storage) { s.PresignEndpoint = "http://localhost:9100" })},
		{name: "R2 in production", env: "production", s: r2},
		{name: "only a bucket", env: "development", s: Storage{Bucket: "cashflow-dev"},
			wantErr: "STORAGE_ENDPOINT, STORAGE_ACCESS_KEY_ID, STORAGE_SECRET_ACCESS_KEY"},
		{name: "no secret", env: "production", s: with(r2, func(s *Storage) { s.SecretAccessKey = "" }), wantErr: "STORAGE_SECRET_ACCESS_KEY"},
		{name: "http in production", env: "production", s: with(r2, func(s *Storage) { s.Endpoint = "http://r2.test" }), wantErr: "STORAGE_ENDPOINT must use https"},
		{name: "a browser endpoint over http in staging", env: "staging", s: with(r2, func(s *Storage) { s.PresignEndpoint = "http://files.test" }),
			wantErr: "STORAGE_PRESIGN_ENDPOINT must use https"},
		{name: "an endpoint with the bucket in the path", env: "production", s: with(r2, func(s *Storage) { s.Endpoint += "/cashflow-prod" }), wantErr: "no path"},
		{name: "a bucket name with capitals", env: "production", s: with(r2, func(s *Storage) { s.Bucket = "Cashflow" }), wantErr: "STORAGE_BUCKET"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.s.validate(tc.env)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
			if tc.s.SecretAccessKey != "" {
				require.NotContains(t, err.Error(), tc.s.SecretAccessKey)
			}
		})
	}
}

func TestStorageLogValueHidesTheSecret(t *testing.T) {
	t.Parallel()
	s := Storage{Endpoint: "https://r2.test", Bucket: "cashflow-prod", AccessKeyID: "id", SecretAccessKey: "very-secret-value"}
	var b strings.Builder
	slog.New(slog.NewTextHandler(&b, nil)).Info("x", "storage", s)
	require.NotContains(t, b.String(), "very-secret-value")
	require.Contains(t, b.String(), "cashflow-prod")
}

func TestWorkerUploadSettings(t *testing.T) {
	t.Parallel()
	base := map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "SECRETS_KEY": testKey}
	minio := map[string]string{
		"STORAGE_ENDPOINT": "http://localhost:9100", "STORAGE_BUCKET": "cashflow-dev",
		"STORAGE_ACCESS_KEY_ID": "cashflow", "STORAGE_SECRET_ACCESS_KEY": storageTestSecret, "STORAGE_PATH_STYLE": "true",
	}
	env := func(maps ...map[string]string) map[string]string {
		out := map[string]string{}
		for _, m := range append([]map[string]string{base}, maps...) {
			for k, v := range m {
				out[k] = v
			}
		}
		return out
	}
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
		want    func(t *testing.T, w Worker)
	}{
		{name: "no uploads", env: env(), want: func(t *testing.T, w Worker) { require.False(t, w.Storage.Enabled()) }},
		{name: "a bucket and clamd", env: env(minio, map[string]string{"CLAMAV_ADDR": "localhost:3310"}), want: func(t *testing.T, w Worker) {
			require.True(t, w.Storage.Enabled())
			require.True(t, w.Storage.PathStyle)
			require.Equal(t, "localhost:3310", w.ClamAVAddr)
		}},
		{name: "a bucket without clamd", env: env(minio), wantErr: "CLAMAV_ADDR"},
		{name: "clamd without a port", env: env(minio, map[string]string{"CLAMAV_ADDR": "clamav"}), wantErr: "CLAMAV_ADDR must be host:port"},
		{name: "clamd alone is ignored", env: env(map[string]string{"CLAMAV_ADDR": "localhost:3310"}), want: func(t *testing.T, w Worker) {
			require.False(t, w.Storage.Enabled())
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := loadWorker(context.Background(), envconfig.MapLookuper(tc.env))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.want(t, got)
		})
	}
}

func TestAPIUploadSettings(t *testing.T) {
	t.Parallel()
	got, err := loadAPI(context.Background(), envconfig.MapLookuper(map[string]string{
		"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
		"STORAGE_ENDPOINT": "http://minio:9000", "STORAGE_PRESIGN_ENDPOINT": "http://localhost:9100", "STORAGE_BUCKET": "cashflow-dev",
		"STORAGE_ACCESS_KEY_ID": "cashflow", "STORAGE_SECRET_ACCESS_KEY": storageTestSecret,
	}))
	require.NoError(t, err)
	require.True(t, got.Storage.Enabled())
	require.Equal(t, "http://localhost:9100", got.Storage.PresignEndpoint)

	_, err = loadAPI(context.Background(), envconfig.MapLookuper(map[string]string{
		"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
		"STORAGE_BUCKET": "cashflow-dev",
	}))
	require.ErrorContains(t, err, "STORAGE_ENDPOINT")
}
