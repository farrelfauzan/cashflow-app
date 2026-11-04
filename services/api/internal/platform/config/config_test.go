package config

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"github.com/sethvargo/go-envconfig/v2"
	"github.com/stretchr/testify/require"
)

var testSecret = strings.Repeat("s", 32)

// testKey is a valid SECRETS_KEY: the id "test" and 32 bytes of 'k', base64-encoded.
var testKey = "test:" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))

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
				AllowedOrigins:   "http://localhost:3000",
				PublicWebOrigin:  "http://localhost:3000",
				WhatsAppProvider: "devlog",
				OpenAPISpec:      "../../api/openapi.yaml",
			},
		},
		{
			name: "explicit values override defaults",
			env: map[string]string{
				"APP_ENV":      "production",
				"LOG_LEVEL":    "warn",
				"HTTP_ADDR":    ":9000",
				"DATABASE_URL": "postgres://app@db/cashflow",
				"REDIS_ADDR":   "redis:6379",
				"AUTH_SECRET":  testSecret, "SECRETS_KEY": testKey,
				"ALLOWED_ORIGINS":   "https://app.example.id",
				"PUBLIC_WEB_ORIGIN": "https://app.example.id",
				"WHATSAPP_PROVIDER": "gowa",
				"GOWA_URL":          "http://gowa:3000",
				"GOWA_USERNAME":     "cashflow",
				"GOWA_PASSWORD":     "gowa-secret",
				"GOWA_DEVICE_ID":    "platform",
			},
			want: API{
				Common:           Common{Env: "production", LogLevel: "warn"},
				HTTPAddr:         ":9000",
				DatabaseURL:      "postgres://app@db/cashflow",
				RedisAddr:        "redis:6379",
				AuthSecret:       testSecret,
				Secrets:          Secrets{Key: testKey},
				AllowedOrigins:   "https://app.example.id",
				PublicWebOrigin:  "https://app.example.id",
				WhatsAppProvider: "gowa",
				GOWA:             GOWA{URL: "http://gowa:3000", Username: "cashflow", Password: "gowa-secret", DeviceID: "platform"},
				OpenAPISpec:      "../../api/openapi.yaml",
			},
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
		{"devlog refused in staging", with(map[string]string{"APP_ENV": "staging"}), "only runs in development"},
		{"gowa needs its settings", with(map[string]string{"WHATSAPP_PROVIDER": "gowa"}), "GOWA_URL"},
		{"gowa with settings", with(map[string]string{"APP_ENV": "staging", "WHATSAPP_PROVIDER": "gowa",
			"GOWA_URL": "http://gowa:3000", "GOWA_USERNAME": "u", "GOWA_PASSWORD": "p"}), ""},
		{"cloud api not built", with(map[string]string{"WHATSAPP_PROVIDER": "cloudapi"}), "not built yet"},
		{"unknown provider", with(map[string]string{"WHATSAPP_PROVIDER": "messagebird"}), "gowa, twilio or devlog"},
		{"devlog file in development", with(map[string]string{"DEVLOG_FILE": "/tmp/devlog.jsonl"}), ""},
		{"devlog file refused in staging", with(map[string]string{"APP_ENV": "staging", "WHATSAPP_PROVIDER": "gowa",
			"GOWA_URL": "http://gowa:3000", "GOWA_USERNAME": "u", "GOWA_PASSWORD": "p", "DEVLOG_FILE": "/tmp/devlog.jsonl"}), "DEVLOG_FILE only works"},
		{"devlog file refused in production", with(map[string]string{"APP_ENV": "production", "WHATSAPP_PROVIDER": "gowa",
			"GOWA_URL": "http://gowa:3000", "GOWA_USERNAME": "u", "GOWA_PASSWORD": "p", "DEVLOG_FILE": "/tmp/devlog.jsonl"}), "DEVLOG_FILE only works"},
		{"devlog file refused with gowa in development", with(map[string]string{"WHATSAPP_PROVIDER": "gowa",
			"GOWA_URL": "http://gowa:3000", "GOWA_USERNAME": "u", "GOWA_PASSWORD": "p", "DEVLOG_FILE": "/tmp/devlog.jsonl"}), "DEVLOG_FILE only works"},
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
	gowa := map[string]string{
		"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret, "SECRETS_KEY": testKey,
		"WHATSAPP_PROVIDER": "gowa", "GOWA_URL": "http://gowa:3000", "GOWA_USERNAME": "u", "GOWA_PASSWORD": "p",
	}
	env := func(appEnv, origin string) map[string]string {
		m := map[string]string{"APP_ENV": appEnv}
		for k, v := range gowa {
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
				Common:      Common{Env: "development", LogLevel: "info"},
				DatabaseURL: "postgres://app@db/cashflow",
				RedisAddr:   "localhost:6379",
				Secrets:     Secrets{Key: testKey},
			},
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
