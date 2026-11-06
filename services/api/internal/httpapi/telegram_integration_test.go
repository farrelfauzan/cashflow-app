//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/telegram"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

const (
	tgBotUsername   = "cashflow_test_bot"
	tgWebhookSecret = "webhook-secret_for-tests-0123456789"
	tgSecretHeader  = "X-Telegram-Bot-Api-Secret-Token"
)

// tgBotToken is a placeholder in the Bot API token shape, built at run time so no source line
// looks like a real token to secret scanning.
var tgBotToken = "7000000002:" + strings.Repeat("KlMnOpQrSt", 4)

var deepLinkPattern = regexp.MustCompile(`^https://t\.me/` + tgBotUsername + `\?start=([A-Za-z0-9_-]{43})$`)

// fakeBotAPI replays the Bot API's sendMessage and records each reply the bot sends.
type fakeBotAPI struct {
	srv  *httptest.Server
	mu   sync.Mutex
	sent []botReply
	next atomic.Int64
}

type botReply struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

func newFakeBotAPI(t *testing.T) *fakeBotAPI {
	t.Helper()
	b := &fakeBotAPI{}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/bot"+tgBotToken+"/sendMessage" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"ok":false,"error_code":404,"description":"Not Found"}`)
			return
		}
		var raw map[string]any
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &raw)
		var m botReply
		_ = json.Unmarshal(body, &m)
		if _, ok := raw["parse_mode"]; ok || len(raw) != 2 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: plain text only"}`)
			return
		}
		b.mu.Lock()
		b.sent = append(b.sent, m)
		b.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"from":{"id":7000000002,"is_bot":true,"first_name":"Cashflow Test",`+
			`"username":"%s"},"chat":{"id":%d,"type":"private"},"date":1762400000,"text":"ok"}}`, b.next.Add(1), tgBotUsername, m.ChatID)
	}))
	t.Cleanup(b.srv.Close)
	return b
}

func (b *fakeBotAPI) replies() []botReply {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]botReply(nil), b.sent...)
}

// lastReply is the newest reply, which must have gone to chat.
func (b *fakeBotAPI) lastReply(t *testing.T, chat int64) string {
	t.Helper()
	rs := b.replies()
	require.NotEmpty(t, rs, "the bot replied nothing")
	last := rs[len(rs)-1]
	require.Equal(t, chat, last.ChatID)
	return last.Text
}

// tgOptions picks which Telegram parts a test server has.
type tgOptions struct {
	bot    *fakeBotAPI // nil: no bot in this environment
	secret string      // empty: no webhook
}

// newTelegramServer serves the API with the payment link services and, as configured, the
// Telegram demo channel, logging to logs.
func newTelegramServer(t *testing.T, env *testenv.Env, logs io.Writer, opt tgOptions) *httptest.Server {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	links := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin)
	deps := httpapi.Deps{
		Log: log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing, PayLinks: links,
		PublicLimiter: env.Limiter, CookieSecure: false, AllowedOrigins: []string{origin},
		TelegramWebhookSecret: opt.secret,
	}
	if opt.bot != nil {
		client, err := telegram.New(telegram.Config{Token: tgBotToken, BaseURL: opt.bot.srv.URL})
		require.NoError(t, err)
		deps.TelegramCodes = telegramlink.NewCodes(env.Store, links, env.Clock, tgBotUsername)
		deps.TelegramUpdates = telegramlink.NewHandler(env.Store, client, env.Clock, log)
	}
	srv := httptest.NewServer(httpapi.New(deps).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// postTelegram asks for a deep link as the payment page does.
func postTelegram(t *testing.T, base, token string) (result, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+"/api/t/"+token+"/telegram", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", origin)
	req.Header.Set("X-Forwarded-For", "203.0.113.40")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return result{StatusCode: resp.StatusCode, Header: resp.Header}, data
}

// deliver posts body to the webhook as Telegram would, with secret in the header (none when empty).
func deliver(t *testing.T, base, secret string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+"/webhooks/telegram", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set(tgSecretHeader, secret)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, data
}

var updateID atomic.Int64

// message is a Bot API update with a text message from a person in a chat of chatType.
func message(chatID int64, chatType, text string) []byte {
	u := map[string]any{
		"update_id": 700000 + updateID.Add(1),
		"message": map[string]any{
			"message_id": 1,
			"from":       map[string]any{"id": chatID, "is_bot": false, "first_name": "Rina", "language_code": "id"},
			"chat":       map[string]any{"id": chatID, "first_name": "Rina", "type": chatType},
			"date":       1762400000,
			"text":       text,
		},
	}
	data, _ := json.Marshal(u)
	return data
}

// say delivers a private message from chat through the webhook and returns the bot's reply.
func say(t *testing.T, base string, bot *fakeBotAPI, chat int64, text string) string {
	t.Helper()
	before := len(bot.replies())
	status, body := deliver(t, base, tgWebhookSecret, message(chat, "private", text))
	require.Equal(t, http.StatusOK, status)
	require.Empty(t, body, "the webhook answers an empty body: Telegram would run a JSON one as a Bot API call")
	require.Len(t, bot.replies(), before+1, "one reply per message")
	return bot.lastReply(t, chat)
}

func telegramState(t *testing.T, base, token string) apigen.PaymentPageTelegram {
	t.Helper()
	resp, body := getPage(t, base, token, "203.0.113.41")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	return decode[apigen.PaymentPage](t, body).Telegram
}

func tokenOf(t *testing.T, l paylink.Link) string {
	t.Helper()
	token, ok := strings.CutPrefix(l.URL, linkOrigin+"/t/")
	require.True(t, ok)
	return token
}

const (
	replyInvalid = "Link ini sudah tidak berlaku. Buka lagi halaman pembayaran dari link tagihan Anda, lalu tekan Hubungkan Telegram."
	replyStopped = "Berhenti. Pesan tidak lagi dikirim ke Telegram."
)

// TestTelegramLinkOverHTTP covers the tenant's Telegram opt-in end to end (decision 42): the
// payment page's deep link, /start and /stop through the webhook, the page's linked state, and
// the secrets kept out of the logs.
func TestTelegramLinkOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta)
	env.Clock.Set(start)
	logs := &lockedBuffer{}
	bot := newFakeBotAPI(t)
	srv := newTelegramServer(t, env, logs, tgOptions{bot: bot, secret: tgWebhookSecret})

	a := newLinkOrg(t, env, "Griya Sejahtera", "0812641")
	b := newLinkOrg(t, env, "Kos Bulanan Sejahtera", "0812642")
	rina, rinaInvoice := a.rent(t, "Rina Wulandari", uuid.Nil)
	_, dewiInvoice := b.rent(t, "Dewi Lestari", uuid.Nil)
	links := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin)
	mint := func(lo *linkOrg, invoice uuid.UUID) string {
		l, err := links.Mint(ctx, lo.admin, invoice)
		require.NoError(t, err)
		return tokenOf(t, l)
	}
	rinaToken, dewiToken := mint(a, rinaInvoice), mint(b, dewiInvoice)
	var secrets []string // every token and code, to look for in the logs at the end
	secrets = append(secrets, rinaToken, dewiToken)

	deepLink := func(t *testing.T, token string) string {
		t.Helper()
		resp, body := postTelegram(t, srv.URL, token)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		requirePublicHeaders(t, resp)
		require.NotContains(t, string(body), token, "the token never comes back in the deep link")
		m := deepLinkPattern.FindStringSubmatch(decode[apigen.TelegramDeepLink](t, body).DeepLink)
		require.NotNil(t, m, string(body))
		secrets = append(secrets, m[1])
		return m[1]
	}
	const rinaChat, otherChat, sharedChat = int64(5550008001), int64(5550008002), int64(5550008003)
	confirmA := "Terhubung. Pesan dari Griya Sejahtera untuk Rina Wulandari akan dikirim ke sini."

	liveLinks := func(t *testing.T, org uuid.UUID) map[uuid.UUID]int64 {
		t.Helper()
		out := map[uuid.UUID]int64{}
		require.NoError(t, env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, "SELECT tenant_id, chat_id FROM telegram_link WHERE unlinked_at IS NULL")
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var tenantID uuid.UUID
				var chat int64
				if err := rows.Scan(&tenantID, &chat); err != nil {
					return err
				}
				out[tenantID] = chat
			}
			return rows.Err()
		}))
		return out
	}

	var firstCode string
	t.Run("the deep link carries a fresh code, never the token or anything derived from it", func(t *testing.T) {
		require.Equal(t, apigen.PaymentPageTelegram{Available: true, Linked: false}, telegramState(t, srv.URL, rinaToken))

		firstCode = deepLink(t, rinaToken)
		second := deepLink(t, rinaToken)
		require.NotEqual(t, firstCode, second, "every request makes a new code")
		require.NotEqual(t, rinaToken, firstCode)

		tokenHash, ok := paylink.TokenHash(rinaToken)
		require.True(t, ok)
		raw, err := base64.RawURLEncoding.DecodeString(firstCode)
		require.NoError(t, err)
		codeHash := sha256.Sum256(raw)
		var stored [][]byte
		require.NoError(t, env.Store.WithOrgTx(ctx, a.o.ID, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, "SELECT code_hash FROM telegram_link_code WHERE tenant_id = $1 ORDER BY created_at, id", rina)
			if err != nil {
				return err
			}
			stored, err = pgx.CollectRows(rows, pgx.RowTo[[]byte])
			return err
		}))
		require.Len(t, stored, 2)
		require.Contains(t, stored, codeHash[:], "the stored hash is the code's")
		for _, h := range stored {
			require.NotEqual(t, tokenHash, h, "a code's hash is never the token's")
			require.NotContains(t, string(h), rinaToken)
		}
		tokenRaw, err := base64.RawURLEncoding.DecodeString(rinaToken)
		require.NoError(t, err)
		require.False(t, bytes.Equal(tokenRaw, raw), "the code's bytes are not the token's")
		require.False(t, bytes.Contains(raw, tokenRaw[:8]) || bytes.Contains(tokenRaw, raw[:8]), "no part of one is in the other")
	})

	t.Run("/start with the code links the tenant and the page shows it", func(t *testing.T) {
		require.Equal(t, confirmA, say(t, srv.URL, bot, rinaChat, "/start "+firstCode))
		require.Equal(t, apigen.PaymentPageTelegram{Available: true, Linked: true}, telegramState(t, srv.URL, rinaToken))
		require.Equal(t, map[uuid.UUID]int64{rina: rinaChat}, liveLinks(t, a.o.ID))
	})

	t.Run("the code works once", func(t *testing.T) {
		require.Equal(t, "Sudah terhubung. Pesan dari Griya Sejahtera untuk Rina Wulandari akan dikirim ke sini.",
			say(t, srv.URL, bot, rinaChat, "/start "+firstCode), "a replay from the same chat answers, not fails")
		require.Equal(t, replyInvalid, say(t, srv.URL, bot, otherChat, "/start "+firstCode), "another chat cannot reuse it")
		require.Equal(t, map[uuid.UUID]int64{rina: rinaChat}, liveLinks(t, a.o.ID))
	})

	t.Run("an expired code is refused", func(t *testing.T) {
		code := deepLink(t, rinaToken)
		env.Clock.Advance(telegramlink.CodeLifetime)
		t.Cleanup(func() { env.Clock.Set(start) })
		require.Equal(t, replyInvalid, say(t, srv.URL, bot, otherChat, "/start "+code))
		require.Equal(t, map[uuid.UUID]int64{rina: rinaChat}, liveLinks(t, a.o.ID), "nothing changed")
	})

	t.Run("an unknown or malformed code is refused alike", func(t *testing.T) {
		for _, code := range []string{"Xk3v9q2LrT7mWb8NcY1pZs4uHf6gJd0eKa5oVi2nQwE", "abc", rinaToken} {
			require.Equal(t, replyInvalid, say(t, srv.URL, bot, otherChat, "/start "+code))
		}
	})

	t.Run("a second link replaces the first for the same tenant", func(t *testing.T) {
		env.Clock.Advance(time.Minute)
		code := deepLink(t, rinaToken)
		before := len(bot.replies())
		status, body := deliver(t, srv.URL, tgWebhookSecret, message(sharedChat, "private", "/start "+code))
		require.Equal(t, http.StatusOK, status)
		require.Empty(t, body)
		sent := bot.replies()[before:]
		require.Len(t, sent, 2, "the confirmation to the new chat, then a notice to the replaced one")
		require.Equal(t, sharedChat, sent[0].ChatID)
		require.Equal(t, confirmA, sent[0].Text)
		require.Equal(t, rinaChat, sent[1].ChatID)
		require.Equal(t, "Telegram ini dilepas dari tagihan Griya Sejahtera. Pesan tagihan sekarang dikirim ke Telegram lain yang dihubungkan dari halaman pembayaran.",
			sent[1].Text)
		require.Equal(t, map[uuid.UUID]int64{rina: sharedChat}, liveLinks(t, a.o.ID))
		var all, unlinked int
		require.NoError(t, env.Store.WithOrgTx(ctx, a.o.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*), count(unlinked_at) FROM telegram_link WHERE tenant_id = $1", rina).Scan(&all, &unlinked)
		}))
		require.Equal(t, 2, all)
		require.Equal(t, 1, unlinked, "the first link is kept, unlinked")
		require.Equal(t, replyInvalid, say(t, srv.URL, bot, rinaChat, "/start "+firstCode),
			"the replaced link's code no longer answers as linked")
	})

	t.Run("/stop unlinks the chat in every organization", func(t *testing.T) {
		code := deepLink(t, dewiToken)
		require.Equal(t, "Terhubung. Pesan dari Kos Bulanan Sejahtera untuk Dewi Lestari akan dikirim ke sini.",
			say(t, srv.URL, bot, sharedChat, "/start "+code))
		orgs, err := env.Store.TelegramChatOrganizations(ctx, sharedChat)
		require.NoError(t, err)
		require.ElementsMatch(t, []uuid.UUID{a.o.ID, b.o.ID}, orgs, "one chat, two organizations")

		require.Equal(t, replyStopped, say(t, srv.URL, bot, sharedChat, "/stop"))
		require.Empty(t, liveLinks(t, a.o.ID))
		require.Empty(t, liveLinks(t, b.o.ID))
		require.False(t, telegramState(t, srv.URL, rinaToken).Linked)
		require.False(t, telegramState(t, srv.URL, dewiToken).Linked)
		require.Equal(t, replyStopped, say(t, srv.URL, bot, sharedChat, "/stop"), "stopping again answers the same")
	})

	t.Run("organization B never sees A's codes or links", func(t *testing.T) {
		for _, table := range []string{"telegram_link_code", "telegram_link"} {
			for _, pair := range [][2]uuid.UUID{{b.o.ID, a.o.ID}, {a.o.ID, b.o.ID}} {
				var n int
				require.NoError(t, env.Store.WithOrgTx(ctx, pair[0], func(tx pgx.Tx) error {
					return tx.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE organization_id = $1", pair[1]).Scan(&n)
				}))
				require.Zero(t, n, table)
			}
		}
	})

	t.Run("other messages get help, and what is not a private message is ignored", func(t *testing.T) {
		require.Contains(t, say(t, srv.URL, bot, otherChat, "/start"), "buka halaman pembayaran")
		require.Contains(t, say(t, srv.URL, bot, otherChat, "halo, saya mau bayar"), "/stop")

		before := len(bot.replies())
		ignored := [][]byte{
			message(otherChat, "group", "/start "+firstCode),
			message(otherChat, "channel", "/stop"),
			[]byte(`{"update_id":799001,"edited_message":{"message_id":1,"chat":{"id":5550008002,"type":"private"},"date":1762400000,"text":"x"}}`),
			[]byte(`{"update_id":799002,"message":{"message_id":1,"from":{"id":7000000099,"is_bot":true,"first_name":"Bot"},` +
				`"chat":{"id":7000000099,"type":"private"},"date":1762400000,"text":"/stop"}}`),
		}
		for _, body := range ignored {
			status, resp := deliver(t, srv.URL, tgWebhookSecret, body)
			require.Equal(t, http.StatusOK, status)
			require.Empty(t, resp)
		}
		require.Len(t, bot.replies(), before, "no reply to an ignored update")
	})

	t.Run("the webhook refuses a wrong or missing secret before reading the body", func(t *testing.T) {
		before := len(bot.replies())
		tests := []struct {
			name   string
			secret string
			body   []byte
			want   int
		}{
			{"wrong secret, malformed body", "wrong-secret", []byte(`{not json`), http.StatusUnauthorized},
			{"missing secret, malformed body", "", []byte(`{not json`), http.StatusUnauthorized},
			{"secret with a suffix", tgWebhookSecret + "x", message(rinaChat, "private", "/stop"), http.StatusUnauthorized},
			{"secret cut short", tgWebhookSecret[:10], message(rinaChat, "private", "/stop"), http.StatusUnauthorized},
			{"right secret, malformed body", tgWebhookSecret, []byte(`{not json`), http.StatusOK},
			{"right secret, body over 64 KiB", tgWebhookSecret, bytes.Repeat([]byte("a"), 65<<10), http.StatusOK},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				status, body := deliver(t, srv.URL, tc.secret, tc.body)
				require.Equal(t, tc.want, status)
				require.Empty(t, body, "no detail in the answer")
			})
		}
		require.Len(t, bot.replies(), before, "nothing refused or unreadable was handled")
	})

	t.Run("the deep link answers link errors like the page", func(t *testing.T) {
		for _, token := range []string{"not-a-token", strings.Repeat("A", 43)} {
			resp, body := postTelegram(t, srv.URL, token)
			require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
			require.Equal(t, "payment_link_not_found", errorCode(t, body))
			requirePublicHeaders(t, resp)
		}
		_, err := links.RevokeAll(ctx, a.admin, rinaInvoice)
		require.NoError(t, err)
		resp, body := postTelegram(t, srv.URL, rinaToken)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		require.Equal(t, "payment_link_not_found", errorCode(t, body))
	})

	t.Run("tokens, codes, chat ids and message text stay out of the logs", func(t *testing.T) {
		out := logs.String()
		require.Contains(t, out, `"outcome":"linked"`, "outcomes are logged")
		for _, s := range secrets {
			require.NotContains(t, out, s)
		}
		for _, chat := range []int64{rinaChat, otherChat, sharedChat} {
			require.NotContains(t, out, strconv.FormatInt(chat, 10))
		}
		for _, text := range []string{"/start", "halo", "Rina Wulandari", "Dewi Lestari", tgWebhookSecret, tgBotToken[11:]} {
			require.NotContains(t, out, text)
		}
	})
}

// TestTelegramUnavailable: without a bot the page hides Telegram, the deep link answers
// telegram_unavailable and there is no webhook; with a bot but no webhook secret there is no
// webhook either.
func TestTelegramUnavailable(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	a := newLinkOrg(t, env, "Griya Sejahtera", "0812643")
	_, invoice := a.rent(t, "Rina Wulandari", uuid.Nil)
	l, err := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin).Mint(ctx, a.admin, invoice)
	require.NoError(t, err)
	token := tokenOf(t, l)

	t.Run("no bot", func(t *testing.T) {
		srv := newTelegramServer(t, env, io.Discard, tgOptions{secret: tgWebhookSecret})
		require.Equal(t, apigen.PaymentPageTelegram{Available: false, Linked: false}, telegramState(t, srv.URL, token))
		resp, body := postTelegram(t, srv.URL, token)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		require.Equal(t, "telegram_unavailable", errorCode(t, body))
		requirePublicHeaders(t, resp)
		status, _ := deliver(t, srv.URL, tgWebhookSecret, message(5550008009, "private", "/stop"))
		require.Equal(t, http.StatusNotFound, status)
	})

	t.Run("a bot but no webhook secret", func(t *testing.T) {
		bot := newFakeBotAPI(t)
		srv := newTelegramServer(t, env, io.Discard, tgOptions{bot: bot})
		require.Equal(t, apigen.PaymentPageTelegram{Available: true, Linked: false}, telegramState(t, srv.URL, token))
		for _, secret := range []string{"", tgWebhookSecret} {
			status, _ := deliver(t, srv.URL, secret, message(5550008009, "private", "/stop"))
			require.Equal(t, http.StatusNotFound, status)
		}
		require.Empty(t, bot.replies())
	})
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var e apigen.Error
	require.NoError(t, json.Unmarshal(body, &e), string(body))
	return e.Error.Code
}
