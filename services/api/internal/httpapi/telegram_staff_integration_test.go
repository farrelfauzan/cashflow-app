//go:build integration

package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/router"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/telegram"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink/linkcode"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// telegramLoginCode finds the code in a login code message as Telegram shows it (plain text).
var telegramLoginCode = regexp.MustCompile(`^Kode masuk Anda: ([0-9]{6})\n`)

const replyStaffInvalid = "Link ini sudah tidak berlaku. Buka lagi aplikasi, lalu tekan Hubungkan Telegram di akun Anda."

// newStaffTelegramServer serves the API as cmd/api wires it: the staff links are always built;
// with a bot, TELEGRAM_MESSAGES=linked puts the router (WhatsApp is env.Messenger) in front of the
// login codes, and the bot's webhook links tenants and staff. Without a bot, codes go to WhatsApp
// and /api/me/telegram says Telegram is unavailable.
func newStaffTelegramServer(t *testing.T, env *testenv.Env, logs io.Writer, bot *fakeBotAPI) *httptest.Server {
	t.Helper()
	return newStaffTelegramServerWith(t, env, logs, bot, true)
}

// newStaffTelegramServerWith is newStaffTelegramServer with TELEGRAM_MESSAGES chosen: with a bot
// and messagesLinked false, the bot answers but login codes and tenant messages go to WhatsApp, so
// neither the account nor the payment page offers a link.
func newStaffTelegramServerWith(t *testing.T, env *testenv.Env, logs io.Writer, bot *fakeBotAPI, messagesLinked bool) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	links := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin)
	staff := auth.NewTelegramStaff(env.Store, env.Limiter, env.Clock, log)
	deps := httpapi.Deps{
		Log: log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing, PayLinks: links,
		PublicLimiter: env.Limiter, CookieSecure: false, AllowedOrigins: []string{origin},
		TelegramStaff: staff,
	}
	if bot != nil {
		client, err := telegram.New(telegram.Config{Token: tgBotToken, BaseURL: bot.srv.URL})
		require.NoError(t, err)
		rtr := router.New(env.Messenger, client, telegramlink.NewChats(env.Store, env.Clock), staff, log)
		deps.Auth, err = auth.NewService(env.Store, rtr, env.Limiter, env.Clock, log, testenv.Secret)
		require.NoError(t, err)
		if messagesLinked {
			deps.TelegramCodes = telegramlink.NewCodes(env.Store, links, env.Clock, tgBotUsername)
		}
		deps.TelegramUpdates = telegramlink.NewHandler(env.Store, client, staff, env.Clock, log)
		deps.TelegramWebhookSecret = tgWebhookSecret
		deps.TelegramBotUsername = tgBotUsername
		deps.LoginCodesViaTelegram = messagesLinked
	}
	srv := httptest.NewServer(httpapi.New(deps).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// requestCode asks for a login code as the login screen does and returns the raw answer.
func requestCode(t *testing.T, base, phone string, whatsappOnly bool) (int, []byte) {
	t.Helper()
	body := apigen.LoginCodeRequest{Phone: phone}
	if whatsappOnly {
		body.WhatsappOnly = &whatsappOnly
	}
	resp, data := newClient(t, base).do(http.MethodPost, "/api/auth/otp/request", body)
	return resp.StatusCode, data
}

// TestStaffTelegramOverHTTP covers TG-4 end to end: a signed-in staff member links their chat from
// the app, the bot's /start and /stop, login codes on Telegram with "Kirim lewat WhatsApp", a code
// request that answers the same for every number, unlinking, the limits, and the logs.
func TestStaffTelegramOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta)
	env.Clock.Set(start)
	logs := &lockedBuffer{}
	bot := newFakeBotAPI(t)
	srv := newStaffTelegramServer(t, env, logs, bot)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812661")
	const adminChat, otherChat, sharedChat = int64(5550010001), int64(5550010002), int64(5550010003)
	var secrets []string

	admin := newClient(t, srv.URL)
	myTelegram := func(t *testing.T) apigen.MyTelegram {
		t.Helper()
		resp, body := admin.do(http.MethodGet, "/api/me/telegram", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		return decode[apigen.MyTelegram](t, body)
	}
	deepLink := func(t *testing.T) string {
		t.Helper()
		resp, body := admin.do(http.MethodPost, "/api/me/telegram", nil)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
		m := deepLinkPattern.FindStringSubmatch(decode[apigen.TelegramDeepLink](t, body).DeepLink)
		require.NotNil(t, m, string(body))
		secrets = append(secrets, m[1])
		return m[1]
	}
	lastTelegramCode := func(t *testing.T, chat int64) string {
		t.Helper()
		m := telegramLoginCode.FindStringSubmatch(bot.lastReply(t, chat))
		require.NotNil(t, m, "the last message to the chat is a login code")
		return m[1]
	}
	linked := "Terhubung. Kode masuk untuk " + o.Admin.Name + " akan dikirim ke sini."

	t.Run("the routes need a session", func(t *testing.T) {
		anon := newClient(t, srv.URL)
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			resp, body := anon.do(method, "/api/me/telegram", nil)
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode, method)
			require.Equal(t, "unauthenticated", errorCode(t, body))
		}
	})

	var firstCode string
	t.Run("a signed-in staff member gets a deep link with a fresh code", func(t *testing.T) {
		admin.signIn(env, o.Admin.Phone)
		require.Equal(t, apigen.MyTelegram{Available: true, Linked: false}, myTelegram(t))
		firstCode = deepLink(t)
		require.NotEqual(t, firstCode, deepLink(t), "every request makes a new code")
	})

	t.Run("a POST from another site is refused", func(t *testing.T) {
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+"/api/me/telegram", nil)
			require.NoError(t, err)
			req.Header.Set("Origin", "https://evil.example.com")
			resp, err := admin.http.Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusForbidden, resp.StatusCode, method)
		}
	})

	t.Run("/start with the code links the chat and names the staff member", func(t *testing.T) {
		require.Equal(t, linked, say(t, srv.URL, bot, adminChat, "/start "+firstCode))
		require.Equal(t, apigen.MyTelegram{Available: true, Linked: true}, myTelegram(t))
	})

	t.Run("the code works once; a used or expired staff code gets the staff reply", func(t *testing.T) {
		require.Equal(t, "Sudah terhubung. Kode masuk untuk "+o.Admin.Name+" akan dikirim ke sini.",
			say(t, srv.URL, bot, adminChat, "/start "+firstCode))
		require.Equal(t, replyStaffInvalid, say(t, srv.URL, bot, otherChat, "/start "+firstCode))
		code := deepLink(t)
		env.Clock.Advance(linkcode.Lifetime)
		require.Equal(t, replyStaffInvalid, say(t, srv.URL, bot, otherChat, "/start "+code))
		require.Equal(t, replyInvalid, say(t, srv.URL, bot, otherChat, "/start Xk3v9q2LrT7mWb8NcY1pZs4uHf6gJd0eKa5oVi2nQwE"),
			"a code nobody has gets the reply it always got")
	})

	t.Run("the login code goes to the linked chat and signs in", func(t *testing.T) {
		before := len(env.Messenger.Sent())
		status, _ := requestCode(t, srv.URL, o.Admin.Phone, false)
		require.Equal(t, http.StatusAccepted, status)
		require.Len(t, env.Messenger.Sent(), before, "nothing on WhatsApp")
		code := lastTelegramCode(t, adminChat)
		phone := newClient(t, srv.URL)
		resp, body := phone.do(http.MethodPost, "/api/auth/otp/verify", apigen.LoginCodeVerify{Phone: o.Admin.Phone, Code: code})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	})

	t.Run("whatsappOnly sends the code on WhatsApp", func(t *testing.T) {
		replies := len(bot.replies())
		status, _ := requestCode(t, srv.URL, o.Admin.Phone, true)
		require.Equal(t, http.StatusAccepted, status)
		require.Len(t, bot.replies(), replies, "nothing on Telegram")
		sent := env.Messenger.Sent()
		require.Equal(t, messaging.TemplateLoginCode, sent[len(sent)-1].Template)
		require.Equal(t, "+62"+strings.TrimPrefix(o.Admin.Phone, "0"), sent[len(sent)-1].To)
	})

	t.Run("the 202 is byte-identical for linked, unlinked and unknown numbers, with or without whatsappOnly", func(t *testing.T) {
		env.Clock.Advance(16 * time.Minute) // a fresh per-phone window
		want := `{"status":"sent","telegramAvailable":true}` + "\n"
		replies, whatsapp := len(bot.replies()), len(env.Messenger.Sent())
		for _, phone := range []string{o.Admin.Phone, o.Supervisor.Phone, "0899-1234-5678"} {
			for _, whatsappOnly := range []bool{false, true} {
				status, body := requestCode(t, srv.URL, phone, whatsappOnly)
				require.Equal(t, http.StatusAccepted, status)
				require.Equal(t, want, string(body), "phone %s whatsappOnly %v", phone, whatsappOnly)
			}
		}
		require.Len(t, bot.replies(), replies+1, "only the linked number without whatsappOnly went to Telegram")
		require.Len(t, env.Messenger.Sent(), whatsapp+3, "the rest of the registered numbers went to WhatsApp")
	})

	t.Run("whatsappOnly shares the per-number limit, and the 429 has the usual shape", func(t *testing.T) {
		for _, whatsappOnly := range []bool{false, true, true} {
			status, body := requestCode(t, srv.URL, o.Karyawan.Phone, whatsappOnly)
			require.Equal(t, http.StatusAccepted, status, string(body))
		}
		for _, whatsappOnly := range []bool{true, false} {
			status, body := requestCode(t, srv.URL, o.Karyawan.Phone, whatsappOnly)
			require.Equal(t, http.StatusTooManyRequests, status)
			e := decode[apigen.Error](t, body)
			require.Equal(t, "rate_limited", e.Error.Code)
			require.NotEmpty(t, e.Error.Message)
		}
	})

	t.Run("a second link replaces the first and tells the old chat", func(t *testing.T) {
		code := deepLink(t)
		before := len(bot.replies())
		status, body := deliver(t, srv.URL, tgWebhookSecret, message(sharedChat, "private", "/start "+code))
		require.Equal(t, http.StatusOK, status)
		require.Empty(t, body)
		sent := bot.replies()[before:]
		require.Len(t, sent, 2, "the confirmation to the new chat, then a notice to the replaced one")
		require.Equal(t, botReply{ChatID: sharedChat, Text: linked}, sent[0])
		require.Equal(t, botReply{ChatID: adminChat, Text: "Telegram ini dilepas dari akun " + o.Admin.Name +
			". Kode masuk sekarang dikirim ke Telegram lain yang dihubungkan dari aplikasi."}, sent[1])

		status, _ = requestCode(t, srv.URL, o.Admin.Phone, false)
		require.Equal(t, http.StatusAccepted, status)
		lastTelegramCode(t, sharedChat)
	})

	t.Run("/stop ends the chat's staff and tenant links alike", func(t *testing.T) {
		lo := newLinkOrg(t, env, "Kos Bulanan Sejahtera", "0812662")
		_, invoice := lo.rent(t, "Rina Wulandari", uuid.Nil)
		l, err := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin).Mint(ctx, lo.admin, invoice)
		require.NoError(t, err)
		token := tokenOf(t, l)
		resp, body := postTelegram(t, srv.URL, token)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		tenantCode := deepLinkPattern.FindStringSubmatch(decode[apigen.TelegramDeepLink](t, body).DeepLink)[1]
		secrets = append(secrets, token, tenantCode)
		require.Contains(t, say(t, srv.URL, bot, sharedChat, "/start "+tenantCode), "Rina Wulandari")
		require.True(t, telegramState(t, srv.URL, token).Linked)

		require.Equal(t, replyStopped, say(t, srv.URL, bot, sharedChat, "/stop"))
		require.Equal(t, apigen.MyTelegram{Available: true, Linked: false}, myTelegram(t))
		require.False(t, telegramState(t, srv.URL, token).Linked)
		require.Equal(t, replyStopped, say(t, srv.URL, bot, sharedChat, "/stop"), "stopping again answers the same")
	})

	t.Run("DELETE unlinks, so codes go to WhatsApp again", func(t *testing.T) {
		require.Equal(t, linked, say(t, srv.URL, bot, adminChat, "/start "+deepLink(t)))
		require.True(t, myTelegram(t).Linked)
		for range 2 {
			resp, _ := admin.do(http.MethodDelete, "/api/me/telegram", nil)
			require.Equal(t, http.StatusNoContent, resp.StatusCode, "also when nothing is linked")
		}
		require.Equal(t, apigen.MyTelegram{Available: true, Linked: false}, myTelegram(t))

		env.Clock.Advance(16 * time.Minute)
		replies := len(bot.replies())
		status, _ := requestCode(t, srv.URL, o.Admin.Phone, false)
		require.Equal(t, http.StatusAccepted, status)
		require.Len(t, bot.replies(), replies)
		sent := env.Messenger.Sent()
		require.Equal(t, o.Admin.UserID, sent[len(sent)-1].StaffUserID)
	})

	t.Run("at most 5 deep links per staff member per 15 minutes", func(t *testing.T) {
		env.Clock.Advance(16 * time.Minute)
		for range 5 {
			deepLink(t)
		}
		resp, body := admin.do(http.MethodPost, "/api/me/telegram", nil)
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		require.Equal(t, "rate_limited", errorCode(t, body))
	})

	t.Run("codes and chat ids stay out of the logs", func(t *testing.T) {
		out := logs.String()
		require.Contains(t, out, `"outcome":"staff_linked"`)
		require.Contains(t, out, `"channel":"telegram"`)
		for _, s := range secrets {
			require.NotContains(t, out, s)
		}
		for _, chat := range []int64{adminChat, otherChat, sharedChat} {
			require.NotContains(t, out, strconv.FormatInt(chat, 10))
		}
		require.NotContains(t, out, o.Admin.Name, "the reply text is not logged")
	})
}

// TestStaffTelegramUnavailable: without a bot (production), the account card is hidden with a 200,
// the deep link answers telegram_unavailable, DELETE still answers 204, and the login screen is
// told Telegram is not available, for every number.
func TestStaffTelegramUnavailable(t *testing.T) {
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812663")
	srv := newStaffTelegramServer(t, env, io.Discard, nil)
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)

	resp, body := admin.do(http.MethodGet, "/api/me/telegram", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "never 404")
	require.Equal(t, apigen.MyTelegram{Available: false, Linked: false}, decode[apigen.MyTelegram](t, body))

	resp, body = admin.do(http.MethodPost, "/api/me/telegram", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.Equal(t, "telegram_unavailable", errorCode(t, body))

	resp, _ = admin.do(http.MethodDelete, "/api/me/telegram", nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	for _, phone := range []string{o.Supervisor.Phone, "0899-1234-5679"} {
		for _, whatsappOnly := range []bool{false, true} {
			status, body := requestCode(t, srv.URL, phone, whatsappOnly)
			require.Equal(t, http.StatusAccepted, status)
			require.Equal(t, `{"status":"sent","telegramAvailable":false}`+"\n", string(body))
		}
	}
}

// With a bot but TELEGRAM_MESSAGES off, login codes go to WhatsApp, so the account offers no link:
// GET says unavailable, POST answers telegram_unavailable, and the login screen is told the same.
func TestStaffTelegramNeedsMessagesLinked(t *testing.T) {
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812664")
	srv := newStaffTelegramServerWith(t, env, io.Discard, newFakeBotAPI(t), false)
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)

	resp, body := admin.do(http.MethodGet, "/api/me/telegram", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, apigen.MyTelegram{Available: false, Linked: false}, decode[apigen.MyTelegram](t, body))

	resp, body = admin.do(http.MethodPost, "/api/me/telegram", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.Equal(t, "telegram_unavailable", errorCode(t, body))

	status, body := requestCode(t, srv.URL, o.Supervisor.Phone, false)
	require.Equal(t, http.StatusAccepted, status)
	require.Equal(t, `{"status":"sent","telegramAvailable":false}`+"\n", string(body))
}
