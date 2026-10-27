// Package gowa sends WhatsApp messages through a self-hosted GOWA server
// (github.com/aldinokemal/go-whatsapp-web-multidevice). GOWA runs as its own container and
// drives a WhatsApp account that is linked to it like WhatsApp Web, by scanning a QR code.
//
// It is the launch provider because the WhatsApp Cloud API needs a registered business behind
// the account. GOWA uses the unofficial WhatsApp Web protocol, so the number can be restricted
// or banned for bulk or unsolicited messages: senders must throttle, and only message people who
// expect to hear from us. There are no approved templates and no native buttons, so messages
// are rendered to plain text with links (messaging.Render).
package gowa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

const (
	defaultTimeout   = 15 * time.Second
	maxResponseBytes = 1 << 20
)

// Config points the client at a GOWA server.
type Config struct {
	BaseURL  string // for example http://gowa:3000
	Username string // APP_BASIC_AUTH user on the GOWA server
	Password string
	// DeviceID selects the linked account (X-Device-Id). Empty uses GOWA's only device.
	DeviceID string
	Timeout  time.Duration
}

// Client is a messaging.Provider backed by GOWA.
type Client struct {
	base     *url.URL
	username string
	password string
	deviceID string
	http     *http.Client
}

var _ messaging.Provider = (*Client)(nil)

// New returns a client for the GOWA server in cfg.
func New(cfg Config) (*Client, error) {
	base, err := url.Parse(cfg.BaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("gowa: base URL must be an absolute http(s) URL, got %q", cfg.BaseURL)
	}
	if cfg.Username == "" || cfg.Password == "" {
		return nil, errors.New("gowa: username and password are required")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &Client{
		base:     base,
		username: cfg.Username,
		password: cfg.Password,
		deviceID: cfg.DeviceID,
		http:     &http.Client{Timeout: timeout},
	}, nil
}

type sendMessageRequest struct {
	Phone   string `json:"phone"`
	Message string `json:"message"`
}

// envelope is GOWA's response shape for every endpoint.
type envelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Results json.RawMessage `json:"results"`
}

type sendResult struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// SendTemplate renders the message to text and sends it with POST /send/message.
func (c *Client) SendTemplate(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	text, err := messaging.Render(m)
	if err != nil {
		return messaging.Receipt{}, err
	}
	phone, err := gowaPhone(m.To)
	if err != nil {
		return messaging.Receipt{}, err
	}
	body, err := json.Marshal(sendMessageRequest{Phone: phone, Message: text})
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("gowa: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base.JoinPath("send", "message").String(), bytes.NewReader(body))
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("gowa: building request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	if c.deviceID != "" {
		req.Header.Set("X-Device-Id", c.deviceID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("gowa: calling server: %w", err)
	}
	defer resp.Body.Close()

	var env envelope
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("gowa: reading response: %w", err)
	}
	// A failed decode leaves env empty; the status code still decides below. Error responses
	// from the basic-auth layer are not JSON.
	_ = json.Unmarshal(data, &env)

	if resp.StatusCode == http.StatusOK && env.Code == "SUCCESS" {
		var res sendResult
		if err := json.Unmarshal(env.Results, &res); err != nil || res.MessageID == "" {
			return messaging.Receipt{}, errors.New("gowa: success response without a message id")
		}
		return messaging.Receipt{ProviderMessageID: res.MessageID}, nil
	}
	return messaging.Receipt{}, sendError(resp.StatusCode, env.Code)
}

// sendError maps a failed response to the shared messaging errors. It reports only the status
// and GOWA's error code: never the message text, which can hold a login code.
func sendError(status int, code string) error {
	detail := fmt.Sprintf("status %d, code %q", status, code)
	switch {
	case status == http.StatusTooManyRequests || code == "WA_REACHOUT_TIMELOCK":
		return fmt.Errorf("%w (%s)", messaging.ErrRateLimited, detail)
	case code == "INVALID_JID":
		return fmt.Errorf("%w (%s)", messaging.ErrUndeliverable, detail)
	case code == "AUTHENTICATION_ERROR" || code == "DEVICE_NOT_FOUND" || code == "DEVICE_ID_REQUIRED":
		return fmt.Errorf("%w (%s)", messaging.ErrNotConnected, detail)
	case status == http.StatusUnauthorized:
		return fmt.Errorf("gowa: the server rejected our credentials; check GOWA_USERNAME and GOWA_PASSWORD (%s)", detail)
	default:
		return fmt.Errorf("gowa: send failed (%s)", detail)
	}
}

// gowaPhone turns an E.164 number into the digits-only form GOWA expects.
func gowaPhone(e164 string) (string, error) {
	digits, ok := strings.CutPrefix(e164, "+")
	if !ok || len(digits) < 8 || strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
		return "", fmt.Errorf("gowa: recipient must be an E.164 number, got %q", e164)
	}
	return digits, nil
}
