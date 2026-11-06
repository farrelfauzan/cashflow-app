package autogopay

// This file reads AutoGoPay's callbacks to /webhooks/autogopay/<webhook id>. What the real
// service sends (captured 2026-11-06):
//
//   - Saving the callback URL in AutoGoPay's dashboard sends a verification call:
//     {"challenge": ..., "event": "verification.challenge", "timestamp": ...}. The dashboard
//     accepts the URL only if the answer is JSON with a "success" field.
//   - A paid QR sends {"event": "transaction.received", "timestamp": ..., "transaction":
//     {"transaction_id", "order_id", "amount", "status": "PAID", "payment_method": "QRIS",
//     "paid_at", "settlement_time"}}, times in WIB with no zone.
//   - Both carry X-Signature: lowercase hex HMAC-SHA256 of the raw body, keyed with the
//     organization's API key. Neither body names the account, which is why each organization has
//     its own URL.
//
// The signature is checked on the raw bytes before anything is parsed. order_id is never read:
// its format differs between generate (AUTOGOPAY-<unix>-<n>) and the webhook
// (AUTOGOPAY-<uuid>); everything is keyed on transaction_id.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

// SignatureHeader carries the callback's signature.
const SignatureHeader = "X-Signature"

// The callback events we know.
const (
	EventChallenge           = "verification.challenge"
	EventTransactionReceived = "transaction.received"
)

// ErrBadCallback means a signed callback's body is not one we can read.
var ErrBadCallback = errors.New("autogopay: unreadable callback")

var webhookIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// ValidWebhookID reports whether id has the shape of a webhook id (43 characters of base64url),
// so a malformed one is refused before any lookup.
func ValidWebhookID(id string) bool { return webhookIDPattern.MatchString(id) }

// ValidSignature reports whether signature is the hex HMAC-SHA256 of body keyed with key. The
// comparison is constant-time; a signature that is not 64 hex characters never matches.
func ValidSignature(body []byte, key secret.Value, signature string) bool {
	got, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil || len(got) != sha256.Size || key.IsZero() {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key.Reveal()))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// Callback is a signed callback, read.
type Callback struct {
	Event string
	// Transaction is set for EventTransactionReceived.
	Transaction *PaidTransaction
}

// PaidTransaction is what a transaction.received callback says about a QR.
type PaidTransaction struct {
	ID     string
	Status TransactionStatus
	Amount int64     // whole rupiah
	PaidAt time.Time // Asia/Jakarta; zero when not sent
}

type callbackBody struct {
	Event       string               `json:"event"`
	Transaction *callbackTransaction `json:"transaction"`
}

type callbackTransaction struct {
	TransactionID string          `json:"transaction_id"`
	Status        string          `json:"status"`
	Amount        json.RawMessage `json:"amount"`
	PaidAt        string          `json:"paid_at"`
}

// ParseCallback reads a callback body whose signature already passed. Fields we do not use are
// ignored, so AutoGoPay can add some. An event other than the two known ones comes back with only
// Event set. A transaction.received without a transaction id, with an amount that is not a
// positive whole number, or with a status we do not know is ErrBadCallback.
func ParseCallback(body []byte) (Callback, error) {
	var b callbackBody
	if err := json.Unmarshal(body, &b); err != nil {
		return Callback{}, fmt.Errorf("%w: %w", ErrBadCallback, err)
	}
	out := Callback{Event: b.Event}
	if b.Event != EventTransactionReceived {
		return out, nil
	}
	t := b.Transaction
	if t == nil || strings.TrimSpace(t.TransactionID) == "" {
		return Callback{}, fmt.Errorf("%w: no transaction id", ErrBadCallback)
	}
	var amount int64
	if err := json.Unmarshal(t.Amount, &amount); err != nil || amount <= 0 {
		return Callback{}, fmt.Errorf("%w: amount is not a positive whole number", ErrBadCallback)
	}
	status, ok := parseStatus(t.Status)
	if !ok {
		return Callback{}, fmt.Errorf("%w: unknown status %q", ErrBadCallback, truncate(t.Status))
	}
	paidAt, err := parseTime(t.PaidAt)
	if err != nil {
		return Callback{}, fmt.Errorf("%w: paid_at unreadable", ErrBadCallback)
	}
	out.Transaction = &PaidTransaction{ID: t.TransactionID, Status: status, Amount: amount, PaidAt: paidAt}
	return out, nil
}
