package autogopay

// This file is the AutoGoPay QRIS client. Its errors and what callers do with them:
//
//   - ErrAmountOutOfRange: the amount is outside MinAmount..MaxAmount; no request was sent.
//     Invoices above the limit must be paid by bank transfer.
//   - ErrTransactionNotFound: AutoGoPay does not know the transaction id.
//   - ErrUnavailable: AutoGoPay could not be reached or answered with a server error, 429, a body
//     that is not JSON or one larger than 1 MiB. Trying later may work. For Generate the QR may
//     still have been created: check before making another, never retry in a loop (AutoGoPay's
//     docs warn that repeated generates can get the merchant account blocked).
//   - ErrRejected: AutoGoPay refused the call (401, 403, any other 4xx, or success:false on
//     generate). The landlord's key or IP whitelist needs attention.
//   - ErrBadResponse: AutoGoPay answered 200 with a shape we do not understand.
//   - ErrInvalidAPIKey, ErrInvalidTransactionID: caller bugs; no request was sent.
//
// The payment page (slice 3, S3-5) shows every error except ErrTransactionNotFound and
// ErrAmountOutOfRange as "QRIS gangguan" and offers bank transfer.
//
// When the caller's context is cancelled, the error satisfies errors.Is(err, context.Canceled)
// and is NOT ErrUnavailable: the caller gave up, AutoGoPay did nothing wrong. Any deadline (the
// client's Timeout or the caller's context deadline) is ErrUnavailable.
//
// No error text ever contains the API key: it travels only in the Authorization header, and a
// message echoed back by AutoGoPay has the key replaced before it is kept.

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
	"unicode"
	"unicode/utf8"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

const (
	// DefaultBaseURL is AutoGoPay's gateway.
	DefaultBaseURL = "https://v1-gateway.autogopay.site"
	// MinAmount and MaxAmount bound a QRIS amount in rupiah, as AutoGoPay does.
	MinAmount int64 = 1
	MaxAmount int64 = 10_000_000

	defaultTimeout   = 10 * time.Second
	maxResponseBytes = 1 << 20
	maxMessageRunes  = 120
	// timeLayout is how AutoGoPay writes times: no zone, in Asia/Jakarta (WIB).
	timeLayout = "2006-01-02 15:04:05"
)

var (
	// ErrUnavailable means AutoGoPay could not be reached or failed on its side.
	ErrUnavailable = errors.New("autogopay: unavailable")
	// ErrRejected means AutoGoPay refused the request.
	ErrRejected = errors.New("autogopay: request rejected")
	// ErrTransactionNotFound means AutoGoPay does not know the transaction id.
	ErrTransactionNotFound = errors.New("autogopay: transaction not found")
	// ErrBadResponse means AutoGoPay answered 200 with a response we do not understand.
	ErrBadResponse = errors.New("autogopay: unexpected response")
	// ErrAmountOutOfRange means the amount is outside MinAmount..MaxAmount.
	ErrAmountOutOfRange = errors.New("autogopay: QRIS amount must be Rp1 to Rp10.000.000")
	// ErrInvalidTransactionID means the transaction id is empty.
	ErrInvalidTransactionID = errors.New("autogopay: transaction id is required")
)

// TransactionStatus is a QRIS transaction's state.
type TransactionStatus string

// The states AutoGoPay reports, in our words.
const (
	StatusPending   TransactionStatus = "pending"
	StatusPaid      TransactionStatus = "paid"
	StatusExpired   TransactionStatus = "expired"
	StatusCancelled TransactionStatus = "cancelled"
)

// parseStatus maps AutoGoPay's status text. The status endpoint documents "settlement" for a
// paid QR and the webhook documents "PAID"; no paid QR has been seen yet, so both count.
func parseStatus(s string) (TransactionStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pending":
		return StatusPending, true
	case "settlement", "paid":
		return StatusPaid, true
	case "expire":
		return StatusExpired, true
	case "cancel":
		return StatusCancelled, true
	default:
		return "", false
	}
}

// QRIS is a newly generated QR. AutoGoPay also returns qr_url and checkout_url, links to its own
// pages; they are deliberately not parsed, because tenants only ever see our page and the QR
// image we render from QRString.
type QRIS struct {
	TransactionID string
	OrderID       string
	Amount        int64
	QRString      string
	Status        TransactionStatus
	CreatedAt     time.Time // Asia/Jakarta; zero if AutoGoPay sent no transaction_time
	ExpiresAt     time.Time // Asia/Jakarta
}

// Transaction is a QRIS transaction's state as AutoGoPay reports it.
type Transaction struct {
	ID     string
	Status TransactionStatus
	Time   time.Time // the transaction's creation time, Asia/Jakarta; zero if not sent
}

// Config points the client at AutoGoPay.
type Config struct {
	BaseURL string        // default DefaultBaseURL
	Timeout time.Duration // per request, default 10s
}

// Client calls AutoGoPay's QRIS endpoints. One Client serves every organization: each call takes
// that organization's API key. It never retries.
type Client struct {
	base *url.URL
	http *http.Client
}

// NewClient returns a client for cfg.
func NewClient(cfg Config) (*Client, error) {
	raw := cfg.BaseURL
	if raw == "" {
		raw = DefaultBaseURL
	}
	base, err := url.Parse(raw)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("autogopay: base URL must be an absolute http(s) URL, got %q", raw)
	}
	if cfg.Timeout < 0 {
		return nil, fmt.Errorf("autogopay: timeout must not be negative, got %s", cfg.Timeout)
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return &Client{
		base: base,
		http: &http.Client{
			Timeout: timeout,
			// Never follow a redirect: the key would be sent again to wherever it points, and
			// AutoGoPay's API does not redirect.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

type generateRequest struct {
	Amount int64 `json:"amount"`
}

type transactionRequest struct {
	TransactionID string `json:"transaction_id"`
}

type envelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type generateData struct {
	TransactionID     string `json:"transaction_id"`
	OrderID           string `json:"order_id"`
	Amount            int64  `json:"amount"`
	TransactionStatus string `json:"transaction_status"`
	QRString          string `json:"qr_string"`
	TransactionTime   string `json:"transaction_time"`
	ExpiryTime        string `json:"expiry_time"`
}

type transactionData struct {
	TransactionID     string `json:"transaction_id"`
	TransactionStatus string `json:"transaction_status"`
	TransactionTime   string `json:"transaction_time"`
}

// Generate creates a QRIS for amount rupiah with POST /qris/generate. It sends exactly one
// request; see the file comment before calling it again after an error.
func (c *Client) Generate(ctx context.Context, key secret.Value, amount int64) (QRIS, error) {
	const op = "generate"
	if key.IsZero() {
		return QRIS{}, fmt.Errorf("autogopay: %s: %w", op, ErrInvalidAPIKey)
	}
	if amount < MinAmount || amount > MaxAmount {
		return QRIS{}, fmt.Errorf("autogopay: %s: %w (got %d)", op, ErrAmountOutOfRange, amount)
	}
	env, err := c.post(ctx, op, key, "generate", generateRequest{Amount: amount})
	if err != nil {
		return QRIS{}, err
	}
	if !env.Success {
		return QRIS{}, fmt.Errorf("autogopay: %s: %w (success false: %q)", op, ErrRejected, cleanMessage(env.Message, key))
	}
	var d generateData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return QRIS{}, fmt.Errorf("autogopay: %s: %w (data: %w)", op, ErrBadResponse, err)
	}
	return d.qris(op, amount)
}

func (d generateData) qris(op string, amount int64) (QRIS, error) {
	bad := func(detail string) error { return fmt.Errorf("autogopay: %s: %w (%s)", op, ErrBadResponse, detail) }
	if d.TransactionID == "" {
		return QRIS{}, bad("no transaction_id")
	}
	if d.QRString == "" {
		return QRIS{}, bad("no qr_string")
	}
	if d.Amount != amount {
		return QRIS{}, bad(fmt.Sprintf("amount %d, asked for %d", d.Amount, amount))
	}
	status, ok := parseStatus(d.TransactionStatus)
	if !ok || status != StatusPending {
		return QRIS{}, bad(fmt.Sprintf("status %q, want pending", truncate(d.TransactionStatus)))
	}
	expires, err := parseTime(d.ExpiryTime)
	if err != nil || expires.IsZero() {
		return QRIS{}, bad("expiry_time missing or unreadable")
	}
	created, err := parseTime(d.TransactionTime)
	if err != nil {
		return QRIS{}, bad("transaction_time unreadable")
	}
	return QRIS{
		TransactionID: d.TransactionID,
		OrderID:       d.OrderID,
		Amount:        d.Amount,
		QRString:      d.QRString,
		Status:        status,
		CreatedAt:     created,
		ExpiresAt:     expires,
	}, nil
}

// Status reports a transaction's state with POST /qris/status. AutoGoPay answers success:false
// for a pending or cancelled QR, so only data.transaction_status is read.
func (c *Client) Status(ctx context.Context, key secret.Value, transactionID string) (Transaction, error) {
	return c.transaction(ctx, "status", key, transactionID)
}

// Cancel cancels a QR with POST /qris/cancel and returns the state AutoGoPay reports afterwards.
// That is not always StatusCancelled: a QR paid just before the cancel comes back StatusPaid,
// and the caller must treat it as paid.
func (c *Client) Cancel(ctx context.Context, key secret.Value, transactionID string) (Transaction, error) {
	return c.transaction(ctx, "cancel", key, transactionID)
}

func (c *Client) transaction(ctx context.Context, op string, key secret.Value, id string) (Transaction, error) {
	if key.IsZero() {
		return Transaction{}, fmt.Errorf("autogopay: %s: %w", op, ErrInvalidAPIKey)
	}
	if strings.TrimSpace(id) == "" {
		return Transaction{}, fmt.Errorf("autogopay: %s: %w", op, ErrInvalidTransactionID)
	}
	env, err := c.post(ctx, op, key, op, transactionRequest{TransactionID: id})
	if err != nil {
		return Transaction{}, err
	}
	bad := func(detail string) error { return fmt.Errorf("autogopay: %s: %w (%s)", op, ErrBadResponse, detail) }
	var d transactionData
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return Transaction{}, bad("no data")
	}
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return Transaction{}, fmt.Errorf("autogopay: %s: %w (data: %w)", op, ErrBadResponse, err)
	}
	if d.TransactionStatus == "" {
		return Transaction{}, fmt.Errorf("autogopay: %s: %w", op, ErrTransactionNotFound)
	}
	if d.TransactionID != "" && d.TransactionID != id {
		return Transaction{}, bad("answer is for another transaction")
	}
	status, ok := parseStatus(d.TransactionStatus)
	if !ok {
		return Transaction{}, bad(fmt.Sprintf("unknown status %q", truncate(d.TransactionStatus)))
	}
	at, err := parseTime(d.TransactionTime)
	if err != nil {
		return Transaction{}, bad("transaction_time unreadable")
	}
	return Transaction{ID: id, Status: status, Time: at}, nil
}

// post sends body to /qris/<path> and returns the decoded envelope of a 200 answer.
func (c *Client) post(ctx context.Context, op string, key secret.Value, path string, body any) (envelope, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return envelope{}, fmt.Errorf("autogopay: %s: encoding request: %w", op, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base.JoinPath("qris", path).String(), bytes.NewReader(payload))
	if err != nil {
		return envelope{}, fmt.Errorf("autogopay: %s: building request: %w", op, err)
	}
	req.Header.Set("Authorization", "Bearer "+key.Reveal())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return envelope{}, transportError(ctx, op, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return envelope{}, transportError(ctx, op, err)
	}
	if len(data) > maxResponseBytes {
		return envelope{}, fmt.Errorf("autogopay: %s: %w (HTTP %d, response larger than 1 MiB)", op, ErrUnavailable, resp.StatusCode)
	}

	var env envelope
	jsonErr := json.Unmarshal(data, &env)
	switch {
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return envelope{}, fmt.Errorf("autogopay: %s: %w (HTTP %d)", op, ErrUnavailable, resp.StatusCode)
	case resp.StatusCode >= 400:
		// A body that is not JSON leaves the message empty; the status still decides.
		return envelope{}, fmt.Errorf("autogopay: %s: %w (HTTP %d: %q)", op, ErrRejected, resp.StatusCode, cleanMessage(env.Message, key))
	case resp.StatusCode != http.StatusOK:
		return envelope{}, fmt.Errorf("autogopay: %s: %w (HTTP %d)", op, ErrBadResponse, resp.StatusCode)
	case jsonErr != nil:
		return envelope{}, fmt.Errorf("autogopay: %s: %w (HTTP 200, response is not JSON)", op, ErrUnavailable)
	}
	return env, nil
}

// transportError classifies a failed round trip: the caller's cancellation stays
// context.Canceled, anything else (deadline, connection) is ErrUnavailable. The wrapped error
// names the URL but never the key, which only travels in a header.
func transportError(ctx context.Context, op string, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("autogopay: %s: %w", op, context.Canceled)
	}
	return fmt.Errorf("autogopay: %s: %w: %w", op, ErrUnavailable, err)
}

// parseTime reads AutoGoPay's zone-less time as Asia/Jakarta. An empty string is the zero time.
func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.ParseInLocation(timeLayout, s, clock.Jakarta)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing time: %w", err)
	}
	return t, nil
}

// cleanMessage makes AutoGoPay's message safe to keep in an error: the key is removed should it
// be echoed back, control characters are dropped and the text is cut to 120 runes.
func cleanMessage(msg string, key secret.Value) string {
	if k := key.Reveal(); k != "" {
		msg = strings.ReplaceAll(msg, k, "[redacted]")
	}
	return truncate(msg)
}

func truncate(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if utf8.RuneCountInString(s) <= maxMessageRunes {
		return s
	}
	return string([]rune(s)[:maxMessageRunes]) + "…"
}
