package config

import (
	"context"
	"strings"
	"testing"

	"github.com/sethvargo/go-envconfig/v2"
	"github.com/stretchr/testify/require"
)

var testSecret = strings.Repeat("s", 32)

func TestLoadAPI(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    API
		wantErr string
	}{
		{
			name: "defaults apply when only the required values are set",
			env:  map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret},
			want: API{
				Common:           Common{Env: "development", LogLevel: "info"},
				HTTPAddr:         ":8080",
				DatabaseURL:      "postgres://app@db/cashflow",
				RedisAddr:        "localhost:6379",
				AuthSecret:       testSecret,
				AllowedOrigins:   "http://localhost:3000",
				WhatsAppProvider: "devlog",
				OpenAPISpec:      "../../api/openapi.yaml",
			},
		},
		{
			name: "explicit values override defaults",
			env: map[string]string{
				"APP_ENV":           "production",
				"LOG_LEVEL":         "warn",
				"HTTP_ADDR":         ":9000",
				"DATABASE_URL":      "postgres://app@db/cashflow",
				"REDIS_ADDR":        "redis:6379",
				"AUTH_SECRET":       testSecret,
				"ALLOWED_ORIGINS":   "https://app.example.id",
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
				AllowedOrigins:   "https://app.example.id",
				WhatsAppProvider: "gowa",
				GOWA:             GOWA{URL: "http://gowa:3000", Username: "cashflow", Password: "gowa-secret", DeviceID: "platform"},
				OpenAPISpec:      "../../api/openapi.yaml",
			},
		},
		{
			name:    "missing database url is rejected",
			env:     map[string]string{"AUTH_SECRET": testSecret},
			wantErr: "DATABASE_URL",
		},
		{
			name:    "missing auth secret is rejected",
			env:     map[string]string{"DATABASE_URL": "postgres://app@db/cashflow"},
			wantErr: "AUTH_SECRET",
		},
		{
			name:    "short auth secret is rejected",
			env:     map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": "short"},
			wantErr: "at least 32 bytes",
		},
		{
			name: "unknown environment is rejected",
			env: map[string]string{
				"APP_ENV":      "prod",
				"DATABASE_URL": "postgres://app@db/cashflow",
				"AUTH_SECRET":  testSecret,
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
	base := map[string]string{"DATABASE_URL": "postgres://app@db/cashflow", "AUTH_SECRET": testSecret}
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
		{"unknown provider", with(map[string]string{"WHATSAPP_PROVIDER": "twilio"}), "gowa or devlog"},
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

func TestLoadWorkerDefaults(t *testing.T) {
	got, err := loadWorker(context.Background(), envconfig.MapLookuper(map[string]string{}))
	require.NoError(t, err)
	require.Equal(t, "localhost:6379", got.RedisAddr)
	require.Equal(t, "development", got.Env)
}

func TestLoadMigrateRequiresOwnerURL(t *testing.T) {
	_, err := loadMigrate(context.Background(), envconfig.MapLookuper(map[string]string{
		"DATABASE_URL": "postgres://app@db/cashflow",
	}))
	require.ErrorContains(t, err, "MIGRATE_DATABASE_URL")
}
