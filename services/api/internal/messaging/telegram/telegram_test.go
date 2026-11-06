package telegram_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/telegram"
)

// botToken is a placeholder in the Bot API token shape, built at run time so no source line
// looks like a real token to secret scanning.
var botToken = "7000000001:" + strings.Repeat("AbCdEfGhIj", 4)

const (
	chatID    = int64(5550001112)
	loginCode = "482913"
)

var loginMessage = messaging.Message{
	To:       "+6281234567890",
	Template: messaging.TemplateLoginCode,
	Params:   map[string]string{"code": loginCode},
}

// sentBody is a trimmed copy of the Bot API's answer to sendMessage in a private chat.
const sentBody = `{"ok":true,"result":{"message_id":42,"from":{"id":7000000001,"is_bot":true,` +
	`"first_name":"Cashflow Dev","username":"cashflow_dev_bot"},"chat":{"id":5550001112,` +
	`"first_name":"Budi","last_name":"Santoso","type":"private"},"date":1762400000,"text":"Kode masuk Anda"}}`

func newClient(t *testing.T, h http.HandlerFunc) *telegram.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := telegram.New(telegram.Config{Token: botToken, BaseURL: srv.URL})
	require.NoError(t, err)
	return c
}

type sendRequest struct {
	ChatID      int64  `json:"chat_id"`
	Text        string `json:"text"`
	ParseMode   string `json:"parse_mode"`
	ReplyMarkup *struct {
		InlineKeyboard [][]map[string]string `json:"inline_keyboard"`
	} `json:"reply_markup"`
}

func TestSend(t *testing.T) {
	t.Parallel()
	var got struct {
		method, path, contentType string
		raw                       map[string]any
		req                       sendRequest
	}
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path, got.contentType = r.Method, r.URL.Path, r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got.raw)
		_ = json.Unmarshal(body, &got.req)
		_, _ = io.WriteString(w, sentBody)
	})

	receipt, err := c.Send(t.Context(), chatID, loginMessage)
	require.NoError(t, err)
	require.Equal(t, "42", receipt.ProviderMessageID)

	want, err := messaging.Render(loginMessage)
	require.NoError(t, err)
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/bot"+botToken+"/sendMessage", got.path)
	require.Equal(t, "application/json", got.contentType)
	require.Equal(t, chatID, got.req.ChatID)
	require.Equal(t, want, got.req.Text)
	require.Len(t, got.raw, 2, "only chat_id and text: no parse_mode, so template text is never markup")
	require.NotContains(t, got.raw, "parse_mode")
}

func TestSendButtonsBecomeAnInlineKeyboard(t *testing.T) {
	t.Parallel()
	var got sendRequest
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, sentBody)
	})
	msg := messaging.Message{
		Template: messaging.TemplateMemberInvite,
		Params: map[string]string{
			"name": "Mbak Sari", "role": "Karyawan", "organization": "Griya Sejahtera", "inviter": "Pak Budi",
		},
		Buttons: []messaging.Button{
			{Title: "Masuk", Payload: "member_invite", URL: "https://app.example.id/login"},
			{Title: "Bantuan", Payload: "help", URL: "https://app.example.id/bantuan"},
		},
	}
	_, err := c.Send(t.Context(), chatID, msg)
	require.NoError(t, err)

	require.Contains(t, got.Text, "*Griya Sejahtera*")
	require.NotContains(t, got.Text, "https://", "links travel as buttons, not in the text")
	require.NotNil(t, got.ReplyMarkup)
	require.Equal(t, [][]map[string]string{
		{{"text": "Masuk", "url": "https://app.example.id/login"}},
		{{"text": "Bantuan", "url": "https://app.example.id/bantuan"}},
	}, got.ReplyMarkup.InlineKeyboard, "one URL button per row, no callback_data")
}

func TestBadMessagesNeverReachTelegram(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	c := newClient(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })
	code := map[string]string{"code": loginCode}
	tests := []struct {
		name   string
		chatID int64
		msg    messaging.Message
	}{
		{"unknown template", chatID, messaging.Message{Template: "nope"}},
		{"missing parameter", chatID, messaging.Message{Template: messaging.TemplateLoginCode}},
		{"extra parameter", chatID, messaging.Message{Template: messaging.TemplateLoginCode, Params: map[string]string{"code": loginCode, "x": "y"}}},
		{"button without url", chatID, messaging.Message{Template: messaging.TemplateLoginCode, Params: code,
			Buttons: []messaging.Button{{Title: "Bayar", Payload: "pay"}}}},
		{"no chat", 0, messaging.Message{Template: messaging.TemplateLoginCode, Params: code}},
		{"text over 4096 characters", chatID, messaging.Message{Template: messaging.TemplateLoginCode,
			Params: map[string]string{"code": strings.Repeat("é", 4100)}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Send(t.Context(), tc.chatID, tc.msg)
			require.Error(t, err)
			require.NotContains(t, err.Error(), loginCode)
		})
	}
	require.Zero(t, calls.Load())
}

func TestErrorsMapToMessagingErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		status  int
		body    string
		wantIs  error
		wantMsg string
	}{
		{"bot blocked by the user", 403, `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`,
			messaging.ErrUndeliverable, "status 403, error_code 403"},
		{"user deactivated", 403, `{"ok":false,"error_code":403,"description":"Forbidden: user is deactivated"}`,
			messaging.ErrUndeliverable, "error_code 403"},
		{"bot cannot initiate the conversation", 403, `{"ok":false,"error_code":403,"description":"Forbidden: bot can't initiate conversation with a user"}`,
			messaging.ErrUndeliverable, "error_code 403"},
		{"chat not found", 400, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
			messaging.ErrUndeliverable, "chat not found"},
		{"unauthorized token", 401, `{"ok":false,"error_code":401,"description":"Unauthorized"}`,
			messaging.ErrNotConnected, "error_code 401"},
		{"revoked token", 404, `{"ok":false,"error_code":404,"description":"Not Found"}`,
			messaging.ErrNotConnected, "error_code 404"},
		{"401 without JSON", 401, `Unauthorized`, messaging.ErrNotConnected, "status 401, error_code 401"},
		{"other bad request", 400, `{"ok":false,"error_code":400,"description":"Bad Request: message text is empty"}`,
			nil, "request refused (telegram sendMessage, status 400, error_code 400)"},
		{"gateway error without JSON", 502, `<html>502 Bad Gateway</html>`, nil, "status 502, error_code 502"},
		{"malformed success", 200, `{"ok":true,"result":`, nil, "unreadable success response"},
		{"ok false with status 200", 200, `{"ok":false}`, nil, "unreadable success response"},
		{"result without a message id", 200, `{"ok":true,"result":{"chat":{"id":1}}}`, nil, "without a message_id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := c.Send(t.Context(), chatID, loginMessage)
			require.Error(t, err)
			for _, sentinel := range []error{messaging.ErrRateLimited, messaging.ErrUndeliverable, messaging.ErrNotConnected, telegram.ErrConflict} {
				require.Equal(t, errors.Is(tc.wantIs, sentinel), errors.Is(err, sentinel), "errors.Is(err, %v)", sentinel)
			}
			require.ErrorContains(t, err, tc.wantMsg)
			requireNoLeak(t, err)
			for _, desc := range []string{"blocked", "deactivated", "initiate", "text is empty", "Bad Gateway"} {
				require.NotContains(t, err.Error(), desc, "the Bot API's description stays out of errors")
			}
		})
	}
}

func TestRateLimitExposesRetryAfter(t *testing.T) {
	t.Parallel()
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 7","parameters":{"retry_after":7}}`)
	})
	_, err := c.Send(t.Context(), chatID, loginMessage)
	require.ErrorIs(t, err, messaging.ErrRateLimited)
	rl, ok := errors.AsType[*telegram.RateLimitError](err)
	require.True(t, ok)
	require.Equal(t, 7*time.Second, rl.RetryAfter)
	require.Equal(t, "sendMessage", rl.Method)
	requireNoLeak(t, err)
}

func TestGetMe(t *testing.T) {
	t.Parallel()
	var path string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = io.WriteString(w, `{"ok":true,"result":{"id":7000000001,"is_bot":true,"first_name":"Cashflow Dev",`+
			`"username":"cashflow_dev_bot","can_join_groups":true,"can_read_all_group_messages":false,"supports_inline_queries":false}}`)
	})
	bot, err := c.GetMe(t.Context())
	require.NoError(t, err)
	require.Equal(t, telegram.Bot{ID: 7000000001, Username: "cashflow_dev_bot"}, bot)
	require.Equal(t, "/bot"+botToken+"/getMe", path)
}

func TestGetMeRefusesANonBot(t *testing.T) {
	t.Parallel()
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":false,"first_name":"Budi"}}`)
	})
	_, err := c.GetMe(t.Context())
	require.ErrorContains(t, err, "without a bot user")
}

func TestGetUpdates(t *testing.T) {
	t.Parallel()
	var got map[string]any
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/bot"+botToken+"/getUpdates", r.URL.Path)
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"ok":true,"result":[`+
			`{"update_id":900001,"message":{"message_id":1,"from":{"id":5550001112,"is_bot":false,"first_name":"Budi","language_code":"id"},`+
			`"chat":{"id":5550001112,"first_name":"Budi","type":"private"},"date":1762400000,"text":"/start abc123",`+
			`"entities":[{"offset":0,"length":6,"type":"bot_command"}]}},`+
			`{"update_id":900002,"message":{"message_id":2,"from":{"id":5550001112,"is_bot":false,"first_name":"Budi"},`+
			`"chat":{"id":5550001112,"first_name":"Budi","type":"private"},"date":1762400001,`+
			`"contact":{"phone_number":"6281234567890","first_name":"Budi","user_id":5550001112}}},`+
			`{"update_id":900003,"edited_message":{"message_id":1,"chat":{"id":5550001112,"type":"private"},"date":1762400002,"text":"x"}}]}`)
	})
	updates, err := c.GetUpdates(t.Context(), 900001, 25*time.Second)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"offset": float64(900001), "timeout": float64(25)}, got)
	require.Len(t, updates, 3)

	require.Equal(t, int64(900001), updates[0].UpdateID)
	require.Equal(t, telegram.KindMessage, updates[0].Kind())
	require.Equal(t, "/start abc123", updates[0].Message.Text)
	require.Equal(t, chatID, updates[0].Message.Chat.ID)
	require.Equal(t, chatID, updates[0].Message.From.ID)

	require.Equal(t, telegram.KindContact, updates[1].Kind())
	require.Equal(t, "6281234567890", updates[1].Message.Contact.PhoneNumber)
	require.Equal(t, chatID, updates[1].Message.Contact.UserID)

	require.Equal(t, telegram.KindOther, updates[2].Kind(), "an edit is not a new message")
	require.Nil(t, updates[2].Message)
}

func TestGetUpdatesConflict(t *testing.T) {
	t.Parallel()
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":409,"description":"Conflict: can't use getUpdates method while webhook is active; use deleteWebhook to delete the webhook first"}`)
	})
	_, err := c.GetUpdates(t.Context(), 0, 0)
	require.ErrorIs(t, err, telegram.ErrConflict)
	requireNoLeak(t, err)
}

// TestTheTokenNeverLeaks forces each kind of failure and checks no error string carries the
// token, which is in every request URL.
func TestTheTokenNeverLeaks(t *testing.T) {
	t.Parallel()
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(slow.Close)
	t.Cleanup(func() { close(release) })

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example/", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)

	// A server that quotes the request path, and so the token, in its description.
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":500,"description":"Internal Server Error at /bot`+botToken+`/sendMessage"}`)
	}))
	t.Cleanup(refusing.Close)

	tests := []struct {
		name    string
		baseURL string
		timeout time.Duration
		wantIs  error
	}{
		{name: "connection refused", baseURL: closedURL},
		{name: "timeout", baseURL: slow.URL, timeout: 50 * time.Millisecond, wantIs: context.DeadlineExceeded},
		{name: "redirect is not followed", baseURL: redirect.URL},
		{name: "server error echoing the path", baseURL: refusing.URL},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := telegram.New(telegram.Config{Token: botToken, BaseURL: tc.baseURL, Timeout: tc.timeout})
			require.NoError(t, err)
			calls := map[string]func() error{
				"sendMessage": func() error { _, err := c.Send(t.Context(), chatID, loginMessage); return err },
				"getMe":       func() error { _, err := c.GetMe(t.Context()); return err },
				"getUpdates":  func() error { _, err := c.GetUpdates(t.Context(), 1, 0); return err },
			}
			for method, call := range calls {
				err := call()
				require.Error(t, err, method)
				require.ErrorContains(t, err, method)
				requireNoLeak(t, err)
				if tc.wantIs != nil {
					require.ErrorIs(t, err, tc.wantIs)
				}
			}
		})
	}
}

func TestContextCancelStopsTheSend(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithCancel(t.Context())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	_, err := c.Send(ctx, chatID, loginMessage)
	require.ErrorIs(t, err, context.Canceled)
	requireNoLeak(t, err)
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     telegram.Config
		wantErr string
	}{
		{"valid", telegram.Config{Token: botToken}, ""},
		{"valid with base url", telegram.Config{Token: botToken, BaseURL: "http://127.0.0.1:8081"}, ""},
		{"missing token", telegram.Config{}, "bot token must be"},
		{"token without colon", telegram.Config{Token: strings.ReplaceAll(botToken, ":", "")}, "bot token must be"},
		{"token secret too short", telegram.Config{Token: "123:" + strings.Repeat("a", 29)}, "bot token must be"},
		{"token with a space", telegram.Config{Token: botToken + " x"}, "bot token must be"},
		{"relative base url", telegram.Config{Token: botToken, BaseURL: "api.telegram.org"}, "base URL"},
		{"base url with credentials", telegram.Config{Token: botToken, BaseURL: "https://u:p@api.telegram.org"}, "base URL"}, //nolint:gosec // G101: a test placeholder, not a credential
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := telegram.New(tc.cfg)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
			if tc.cfg.Token != "" {
				require.NotContains(t, err.Error(), tc.cfg.Token)
			}
			requireNoLeak(t, err)
		})
	}
}

// requireNoLeak checks that an error carries neither the token (or its secret half), the chat
// id, nor the message's login code, since callers log errors as they are.
func requireNoLeak(t *testing.T, err error) {
	t.Helper()
	msg := err.Error()
	_, secretHalf, _ := strings.Cut(botToken, ":")
	require.NotContains(t, msg, secretHalf, "errors must never carry the bot token")
	require.NotContains(t, msg, "/bot", "errors must never carry the request URL")
	require.NotContains(t, msg, "5550001112", "errors must never carry the chat id")
	require.NotContains(t, msg, loginCode, "errors must never carry the message text")
}
