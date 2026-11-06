package provider_test

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/devlog"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/gowa"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/messagingtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/provider"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/router"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/twilio"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
)

func TestNewBuildsTheConfiguredProvider(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sid := "AC" + strings.Repeat("0123456789abcdef", 2)
	tests := []struct {
		name    string
		in      config.WhatsApp
		check   func(t *testing.T, got any)
		wantErr bool
	}{
		{name: "devlog", in: config.WhatsApp{Env: "development", Provider: "devlog"},
			check: func(t *testing.T, got any) { require.IsType(t, &devlog.Provider{}, got) }},
		{name: "gowa", in: config.WhatsApp{Provider: "gowa", GOWA: config.GOWA{URL: "http://gowa:3000", Username: "u", Password: "p"}},
			check: func(t *testing.T, got any) { require.IsType(t, &gowa.Client{}, got) }},
		{name: "twilio", in: config.WhatsApp{Provider: "twilio", Twilio: config.Twilio{AccountSID: sid, AuthToken: "t", WhatsAppFrom: "+14155238886"}},
			check: func(t *testing.T, got any) { require.IsType(t, &twilio.Client{}, got) }},
		{name: "none", in: config.WhatsApp{Provider: "cloudapi"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := provider.New(tc.in, log)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			tc.check(t, got)
		})
	}
}

func TestWithTelegramWrapsOnlyWhenSendingToLinkedChats(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	token := "7000000001:" + strings.Repeat("AbCdEfGhIj", 4)
	whatsapp := &messagingtest.Fake{}

	for _, messages := range []string{"", "off"} {
		got, err := provider.WithTelegram(whatsapp, config.Telegram{BotToken: token, BotUsername: "CashflowDevBot", Messages: messages}, nil, nil, log)
		require.NoError(t, err)
		require.Same(t, whatsapp, got, "TELEGRAM_MESSAGES=%q leaves WhatsApp as it is", messages)
	}

	got, err := provider.WithTelegram(whatsapp, config.Telegram{BotToken: token, BotUsername: "CashflowDevBot", Messages: "linked"}, nil, nil, log)
	require.NoError(t, err)
	require.IsType(t, &router.Router{}, got)

	_, err = provider.WithTelegram(whatsapp, config.Telegram{BotToken: "not-a-token", Messages: "linked"}, nil, nil, log)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "not-a-token")
}
