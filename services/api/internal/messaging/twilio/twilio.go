// Package twilio sends WhatsApp messages through Twilio's Programmable Messaging API
// (POST /2010-04-01/Accounts/{AccountSid}/Messages.json).
//
// Today it sends from a Twilio sandbox for WhatsApp, in development only. The sandbox number is
// shown on the console's sandbox page and can differ per account. The sandbox reaches only
// phones that sent "join <keyword>" to it. A verified Twilio WhatsApp sender of our own would
// lift the join step but not the next rule.
//
// WhatsApp lets a business send free-form text only inside the 24-hour session that starts
// when the person last wrote to the sender; outside it, only approved templates get through.
// Twilio may also insist on a Content template (error 21654, "ContentSid Required"). So a
// message template can be mapped to a Twilio Content template (Config.ContentSIDs): it is then
// sent as ContentSid with ContentVariables numbered in the template's parameter order, then its
// button links. Any other template is sent as text rendered by messaging.Render, links included.
//
// Twilio refuses some sends at once (21654 maps to ErrNotConnected) and fails others later
// (63016 outside the window, 63015 for a phone that has not joined the sandbox), reported only
// through status callbacks, which arrive with slice 5. Until then such a send looks successful
// and never arrives.
package twilio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

const (
	defaultBaseURL   = "https://api.twilio.com"
	defaultTimeout   = 15 * time.Second
	maxResponseBytes = 1 << 20
)

var (
	accountSIDPattern = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)
	contentSIDPattern = regexp.MustCompile(`^HX[0-9a-fA-F]{32}$`)
)

// Config holds the Twilio account and the WhatsApp sender.
type Config struct {
	AccountSID string // AC followed by 32 hex characters
	AuthToken  string
	From       string // the sender in E.164, as the Twilio console shows it
	// ContentSIDs maps a template name (messaging.TemplateLoginCode, ...) to the Content
	// template Twilio sends for it (HX followed by 32 hex characters). A template listed here
	// is sent as ContentSid and ContentVariables; any other template is sent as rendered text.
	ContentSIDs map[string]string
	// BaseURL overrides https://api.twilio.com. Tests only.
	BaseURL string
	Timeout time.Duration
}

// Client is a messaging.Provider backed by Twilio.
type Client struct {
	endpoint    string
	accountSID  string
	authToken   string
	from        string
	contentSIDs map[string]string
	http        *http.Client
}

var _ messaging.Provider = (*Client)(nil)

// New returns a client for the Twilio account in cfg.
func New(cfg Config) (*Client, error) {
	if !accountSIDPattern.MatchString(cfg.AccountSID) {
		return nil, errors.New("twilio: account SID must be AC followed by 32 hex characters")
	}
	if cfg.AuthToken == "" {
		return nil, errors.New("twilio: auth token is required")
	}
	if !isE164(cfg.From) {
		return nil, fmt.Errorf("twilio: sender must be an E.164 number such as +14155238886, got %q", cfg.From)
	}
	for name, sid := range cfg.ContentSIDs {
		if _, ok := messaging.Lookup(name); !ok {
			return nil, fmt.Errorf("twilio: content template for unknown message template %q", name)
		}
		if !contentSIDPattern.MatchString(sid) {
			return nil, fmt.Errorf("twilio: content SID for %s must be HX followed by 32 hex characters", name)
		}
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	base, err := url.Parse(baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("twilio: base URL must be an absolute http(s) URL, got %q", baseURL)
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &Client{
		endpoint:    base.JoinPath("2010-04-01", "Accounts", cfg.AccountSID, "Messages.json").String(),
		accountSID:  cfg.AccountSID,
		authToken:   cfg.AuthToken,
		from:        cfg.From,
		contentSIDs: maps.Clone(cfg.ContentSIDs),
		http:        &http.Client{Timeout: timeout},
	}, nil
}

// apiResponse holds the fields we read from both a created message and an error body.
type apiResponse struct {
	SID  string `json:"sid"`
	Code int    `json:"code"`
}

// SendTemplate creates a Twilio message: the Content template configured for m.Template, or
// else the rendered text.
func (c *Client) SendTemplate(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	// Render also checks the parameters and buttons, so a template sent by ContentSid obeys the
	// same rules as one sent as text.
	text, err := messaging.Render(m)
	if err != nil {
		return messaging.Receipt{}, err
	}
	if !isE164(m.To) {
		// The number stays out of the error: callers log it.
		return messaging.Receipt{}, errors.New("twilio: recipient must be an E.164 number")
	}
	form := url.Values{
		"To":   {"whatsapp:" + m.To},
		"From": {"whatsapp:" + c.from},
	}
	if sid, ok := c.contentSIDs[m.Template]; ok {
		vars, err := contentVariables(m)
		if err != nil {
			return messaging.Receipt{}, err
		}
		form.Set("ContentSid", sid)
		if vars != "" {
			form.Set("ContentVariables", vars)
		}
	} else {
		form.Set("Body", text)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("twilio: building request: %w", err)
	}
	req.SetBasicAuth(c.accountSID, c.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("twilio: calling API: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("twilio: reading response: %w", err)
	}
	var res apiResponse
	// A failed decode leaves res empty; the status code still decides below. Errors from a
	// proxy or load balancer in front of Twilio are not JSON.
	_ = json.Unmarshal(data, &res)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if res.SID == "" {
			return messaging.Receipt{}, fmt.Errorf("twilio: success response without a message sid (status %d)", resp.StatusCode)
		}
		return messaging.Receipt{ProviderMessageID: res.SID}, nil
	}
	return messaging.Receipt{}, sendError(resp.StatusCode, res.Code)
}

// contentVariables numbers a message's values the way a Content template reads them: the
// template's parameters in their registered order ({{1}}, {{2}}, ...), then each button's link.
// It returns "" when there is nothing to fill.
func contentVariables(m messaging.Message) (string, error) {
	t, ok := messaging.Lookup(m.Template)
	if !ok {
		return "", fmt.Errorf("twilio: unknown template %q", m.Template)
	}
	values := make([]string, 0, len(t.Params)+len(m.Buttons))
	for _, p := range t.Params {
		values = append(values, m.Params[p])
	}
	for _, b := range m.Buttons {
		values = append(values, b.URL)
	}
	if len(values) == 0 {
		return "", nil
	}
	vars := make(map[string]string, len(values))
	for i, v := range values {
		vars[strconv.Itoa(i+1)] = v
	}
	data, err := json.Marshal(vars)
	if err != nil {
		return "", fmt.Errorf("twilio: encoding content variables for %s: %w", m.Template, err)
	}
	return string(data), nil
}

// sendError maps a failed response to the shared messaging errors. It reports only the HTTP
// status and Twilio's numeric error code: never Twilio's message or detail text, which can quote
// the recipient's number, and never our message text, which can hold a login code.
func sendError(status, code int) error {
	detail := fmt.Sprintf("status %d, code %d", status, code)
	switch {
	// 20429: too many requests. 63018: the sender's WhatsApp rate limit.
	case code == 20429 || code == 63018:
		return fmt.Errorf("%w (%s)", messaging.ErrRateLimited, detail)
	// 21211: invalid To. 21614: not a mobile number. 63003: invalid destination.
	// 21610: the recipient replied STOP. 63015: a sandbox recipient that has not joined; in
	// practice Twilio usually reports this later, asynchronously, after answering 201.
	case code == 21211 || code == 21614 || code == 63003 || code == 21610 || code == 63015:
		return fmt.Errorf("%w (%s)", messaging.ErrUndeliverable, detail)
	// 20003: wrong Account SID or Auth Token. 63007: no WhatsApp channel for the From address
	// (sender not enabled, or not the sandbox number). 21606: From is not valid for this
	// account. An operator must fix the credentials or the sender.
	// 21654: Twilio refused the free-form Body and wants a Content template (ContentSid),
	// which WhatsApp requires outside the recipient's 24-hour window. 63016: the same rule
	// when Twilio reports it at once. A template without a Content SID, or a window that lapsed,
	// needs an operator (or, in the sandbox, the person writing to the sandbox number first).
	case code == 20003 || code == 63007 || code == 21606 || code == 21654 || code == 63016:
		return fmt.Errorf("%w (%s)", messaging.ErrNotConnected, detail)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%w (%s)", messaging.ErrRateLimited, detail)
	// Any other refusal, or Twilio failing on its side: retrying the same request will not
	// help soon and someone has to look at the logged code, so callers treat it like a
	// provider that is not usable (a 503, logged as an error) instead of an unexplained 500.
	case status >= 400:
		return fmt.Errorf("%w (%s)", messaging.ErrNotConnected, detail)
	default:
		return fmt.Errorf("twilio: send failed (%s)", detail)
	}
}

// isE164 reports whether s is a + followed by 8 to 15 digits: E.164 with its length limit.
func isE164(s string) bool {
	digits, ok := strings.CutPrefix(s, "+")
	return ok && len(digits) >= 8 && len(digits) <= 15 &&
		strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) == -1
}
