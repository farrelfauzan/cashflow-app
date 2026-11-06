// Package telegram talks to the Telegram Bot API (https://core.telegram.org/bots/api) for the
// platform's one bot (decision 42): sendMessage to a chat, getMe for a startup check, and
// getUpdates for the development polling runner (cmd/telegrampoll).
//
// Client does not implement messaging.Provider. A Provider sends to a phone number, and a bot
// can only message a chat that pressed Start on it, so which chat (or whether WhatsApp instead)
// is chosen per recipient by the router that comes later (TG-3). Send takes the chat id. The
// router is the messaging.Provider: callers keep calling SendTemplate, and through it this
// adapter is one of the MessagingProvider adapters CLAUDE.md lists.
//
// Messages use the same templates as WhatsApp (messaging.Render) and go out as plain text, with
// no parse_mode, so template text is never read as markup. Buttons become inline keyboard URL
// buttons; a button's Payload is not sent as callback data yet (nothing handles callbacks), so
// every button needs its URL, as on GOWA.
//
// The bot token is part of every request URL (/bot<token>/<method>), and Go's *url.Error
// prints the URL. Every error this package returns names the method, the HTTP status and the
// Bot API error code only: never the token, the URL, the chat id, the message text or the Bot
// API's description.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

const (
	// DefaultBaseURL is the Bot API server.
	DefaultBaseURL   = "https://api.telegram.org"
	defaultTimeout   = 15 * time.Second
	maxResponseBytes = 1 << 20
	// maxTextLength is sendMessage's limit: 1-4096 characters after entities parsing. With no
	// parse_mode there are no entities to parse, so it is the text's length in characters.
	maxTextLength = 4096
)

// tokenPattern is the shape BotFather gives: the bot id, a colon, then a secret.
var tokenPattern = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]{30,}$`)

// ValidToken reports whether token has the shape of a Bot API token. Config uses it so a
// malformed value stops the process without echoing it.
func ValidToken(token string) bool { return tokenPattern.MatchString(token) }

var (
	// ErrConflict is getUpdates' 409: a webhook is set for the bot, or another getUpdates poller
	// is running with the same token. Polling cannot continue until one of them is removed.
	ErrConflict = errors.New("telegram: getUpdates conflict (a webhook is set, or another poller runs with this token)")
)

// RateLimitError is a 429 from the Bot API. It matches messaging.ErrRateLimited with
// errors.Is; RetryAfter is how long Telegram asked us to wait (zero when it did not say).
type RateLimitError struct {
	Method     string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%s (telegram %s, status 429, retry after %s)", messaging.ErrRateLimited, e.Method, e.RetryAfter)
}

// Unwrap makes errors.Is(err, messaging.ErrRateLimited) true.
func (e *RateLimitError) Unwrap() error { return messaging.ErrRateLimited }

// Config is the bot's token and where the Bot API lives.
type Config struct {
	// Token is the bot token from BotFather. A secret: never log it.
	Token string
	// BaseURL overrides DefaultBaseURL (tests, or a local Bot API server).
	BaseURL string
	// Timeout bounds every call except getUpdates, which waits up to its long-polling timeout
	// plus this.
	Timeout time.Duration
}

// Client calls the Bot API for one bot.
type Client struct {
	base    *url.URL
	token   string
	timeout time.Duration
	http    *http.Client
}

// New returns a client for the bot in cfg. Its errors never quote the token.
func New(cfg Config) (*Client, error) {
	if !ValidToken(cfg.Token) {
		return nil, errors.New("telegram: bot token must be digits, a colon and at least 30 letters, digits, _ or -")
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	base, err := url.Parse(baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil {
		return nil, fmt.Errorf("telegram: base URL must be an absolute http(s) URL, got %q", baseURL)
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &Client{
		base:    base,
		token:   cfg.Token,
		timeout: timeout,
		// No client-wide timeout: each call sets its own deadline, since getUpdates waits longer.
		// Redirects are not followed, so the token in the path never goes to another host.
		http: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

// Bot is what getMe tells about the bot.
type Bot struct {
	ID       int64
	Username string
}

// GetMe returns the bot's id and username: a startup check that the token works.
func (c *Client) GetMe(ctx context.Context) (Bot, error) {
	var u User
	if err := c.call(ctx, "getMe", struct{}{}, &u, c.timeout); err != nil {
		return Bot{}, err
	}
	if u.ID == 0 || !u.IsBot {
		return Bot{}, errors.New("telegram: getMe answered without a bot user")
	}
	return Bot{ID: u.ID, Username: u.Username}, nil
}

type inlineButton struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

type sendMessageRequest struct {
	ChatID      int64           `json:"chat_id"`
	Text        string          `json:"text"`
	ReplyMarkup *inlineKeyboard `json:"reply_markup,omitempty"`
}

type sentMessage struct {
	MessageID int64 `json:"message_id"`
}

// Send renders m with messaging.Render and sends it to chatID as plain text, each button as an
// inline keyboard URL button on its own row. m.To is ignored. The receipt holds Telegram's
// message_id.
func (c *Client) Send(ctx context.Context, chatID int64, m messaging.Message) (messaging.Receipt, error) {
	// The full render checks the template, its parameters and that every button has a link.
	if _, err := messaging.Render(m); err != nil {
		return messaging.Receipt{}, err
	}
	// The text itself leaves the links out: they are the keyboard's buttons.
	bare := m
	bare.Buttons = nil
	text, err := messaging.Render(bare)
	if err != nil {
		return messaging.Receipt{}, err
	}
	if chatID == 0 {
		return messaging.Receipt{}, errors.New("telegram: chat id is required")
	}
	if n := utf8.RuneCountInString(text); n > maxTextLength {
		return messaging.Receipt{}, fmt.Errorf("telegram: template %s renders to %d characters, over sendMessage's %d", m.Template, n, maxTextLength)
	}
	req := sendMessageRequest{ChatID: chatID, Text: text}
	if len(m.Buttons) > 0 {
		kb := &inlineKeyboard{InlineKeyboard: make([][]inlineButton, 0, len(m.Buttons))}
		for _, b := range m.Buttons {
			kb.InlineKeyboard = append(kb.InlineKeyboard, []inlineButton{{Text: b.Title, URL: b.URL}})
		}
		req.ReplyMarkup = kb
	}
	var sent sentMessage
	if err := c.call(ctx, "sendMessage", req, &sent, c.timeout); err != nil {
		return messaging.Receipt{}, err
	}
	if sent.MessageID == 0 {
		return messaging.Receipt{}, errors.New("telegram: sendMessage answered without a message_id")
	}
	return messaging.Receipt{ProviderMessageID: strconv.FormatInt(sent.MessageID, 10)}, nil
}

// SendText sends text to chatID as plain text (no parse_mode, so nothing in it is read as
// markup), for the bot's own replies to commands, which are not templates. The receipt holds
// Telegram's message_id. Like every error here, a failure never quotes the text or the chat id.
func (c *Client) SendText(ctx context.Context, chatID int64, text string) (messaging.Receipt, error) {
	if chatID == 0 {
		return messaging.Receipt{}, errors.New("telegram: chat id is required")
	}
	if strings.TrimSpace(text) == "" {
		return messaging.Receipt{}, errors.New("telegram: text is required")
	}
	if n := utf8.RuneCountInString(text); n > maxTextLength {
		return messaging.Receipt{}, fmt.Errorf("telegram: text is %d characters, over sendMessage's %d", n, maxTextLength)
	}
	var sent sentMessage
	if err := c.call(ctx, "sendMessage", sendMessageRequest{ChatID: chatID, Text: text}, &sent, c.timeout); err != nil {
		return messaging.Receipt{}, err
	}
	if sent.MessageID == 0 {
		return messaging.Receipt{}, errors.New("telegram: sendMessage answered without a message_id")
	}
	return messaging.Receipt{ProviderMessageID: strconv.FormatInt(sent.MessageID, 10)}, nil
}

type getUpdatesRequest struct {
	Offset  int64 `json:"offset,omitempty"`
	Timeout int   `json:"timeout"`
}

// GetUpdates long-polls for updates from offset (the last update_id handled plus one; 0 for
// the first call), waiting up to timeout (whole seconds) when there are none. A 409 is
// ErrConflict.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]Update, error) {
	if timeout < 0 {
		timeout = 0
	}
	req := getUpdatesRequest{Offset: offset, Timeout: int(timeout / time.Second)}
	var updates []Update
	if err := c.call(ctx, "getUpdates", req, &updates, timeout+c.timeout); err != nil {
		return nil, err
	}
	return updates, nil
}

// apiResponse is the Bot API's envelope for every answer.
type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// call posts params as JSON to method and decodes the result into out.
func (c *Client) call(ctx context.Context, method string, params, out any, timeout time.Duration) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("telegram %s: encoding request: %w", method, err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	endpoint := c.base.JoinPath("bot"+c.token, method).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return c.transportError(method, "building request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return c.transportError(method, "calling Bot API", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return c.transportError(method, "reading response", err)
	}

	var res apiResponse
	decodeErr := json.Unmarshal(data, &res)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if decodeErr != nil || !res.OK {
			return fmt.Errorf("telegram %s: unreadable success response (status %d)", method, resp.StatusCode)
		}
		if err := json.Unmarshal(res.Result, out); err != nil {
			return fmt.Errorf("telegram %s: unreadable result (status %d)", method, resp.StatusCode)
		}
		return nil
	}
	// A proxy in front of the Bot API may answer without JSON; the HTTP status decides then.
	code := res.ErrorCode
	if decodeErr != nil || code == 0 {
		code = resp.StatusCode
	}
	retryAfter := 0
	if res.Parameters != nil {
		retryAfter = res.Parameters.RetryAfter
	}
	return apiError(method, resp.StatusCode, code, res.Description, retryAfter)
}

// apiError maps a Bot API refusal to the shared messaging errors. The description is read to
// tell "chat not found" apart, but never copied into the error.
func apiError(method string, status, code int, description string, retryAfter int) error {
	detail := fmt.Sprintf("telegram %s, status %d, error_code %d", method, status, code)
	switch {
	case code == http.StatusTooManyRequests:
		return &RateLimitError{Method: method, RetryAfter: time.Duration(retryAfter) * time.Second}
	// 403: the user blocked the bot, deleted their account, or the bot cannot write to the chat.
	case code == http.StatusForbidden:
		return fmt.Errorf("%w (%s)", messaging.ErrUndeliverable, detail)
	case code == http.StatusBadRequest && strings.Contains(strings.ToLower(description), "chat not found"):
		return fmt.Errorf("%w (%s, chat not found)", messaging.ErrUndeliverable, detail)
	// 401 and 404: the token is wrong or was revoked in BotFather. An operator has to fix it.
	case code == http.StatusUnauthorized || code == http.StatusNotFound:
		return fmt.Errorf("%w (%s)", messaging.ErrNotConnected, detail)
	case code == http.StatusConflict:
		return fmt.Errorf("%w (%s)", ErrConflict, detail)
	default:
		return fmt.Errorf("telegram: request refused (%s)", detail)
	}
}

// transportError wraps a failure to reach the Bot API. http.Client wraps it in a *url.Error,
// whose text holds the request URL and so the token; that layer is dropped, and the token is
// also removed from whatever text remains. errors.Is still sees the cause (a deadline, say).
func (c *Client) transportError(method, step string, err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		err = ue.Err
	}
	return &redactedError{
		msg: fmt.Sprintf("telegram %s: %s: %s", method, step, strings.ReplaceAll(err.Error(), c.token, "[token]")),
		err: err,
	}
}

type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }
