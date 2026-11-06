package qris

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

// ErrWebhookRefused means a callback was not accepted: the webhook id is malformed or unknown, the
// signature does not match, or the signed body cannot be read. The answer is the same for all, so
// a caller learns nothing about which organizations exist.
var ErrWebhookRefused = errors.New("qris: webhook refused")

// WebhookKeys finds the organization behind a webhook id and its AutoGoPay key.
// autogopay.Service implements it.
type WebhookKeys interface {
	WebhookKey(ctx context.Context, webhookID string) (uuid.UUID, secret.Value, error)
}

// PaidQueue enqueues the qris-paid task for a transaction AutoGoPay reported paid. A task for the
// same transaction already queued counts as enqueued.
type PaidQueue interface {
	EnqueuePaid(ctx context.Context, org uuid.UUID, transactionID string, amount int64) error
}

// Webhook handles AutoGoPay's callbacks to /webhooks/autogopay/{webhookId}.
type Webhook struct {
	keys  WebhookKeys
	queue PaidQueue
	log   *slog.Logger
}

// NewWebhook returns a Webhook.
func NewWebhook(keys WebhookKeys, queue PaidQueue, log *slog.Logger) *Webhook {
	return &Webhook{keys: keys, queue: queue, log: log}
}

// Handle checks and acts on one callback: body is the raw request body and signature the
// X-Signature header. The organization comes from the webhook id alone, and the signature is
// checked with that organization's key before anything in the body is read. Then:
//
//   - verification.challenge (AutoGoPay checking the URL when it is saved) is accepted;
//   - transaction.received with a paid status enqueues qris-paid for the transaction, keyed on its
//     id; the task records the payment once, however often the callback comes;
//   - any other event or status is accepted and ignored, so AutoGoPay does not send it again.
//
// nil means answer success. ErrWebhookRefused means refuse with nothing more said. Any other
// error is ours (the database, the queue): answer a failure so AutoGoPay may send it again, and
// reconcile covers it if it does not. The webhook id is never logged: it is in the URL AutoGoPay
// holds, and knowing it is half of what a forged callback would need.
func (w *Webhook) Handle(ctx context.Context, webhookID, signature string, body []byte) error {
	org, key, err := w.keys.WebhookKey(ctx, webhookID)
	if errors.Is(err, autogopay.ErrUnknownWebhook) || errors.Is(err, autogopay.ErrNoAccount) {
		w.log.WarnContext(ctx, "AutoGoPay callback to an unknown webhook id")
		return ErrWebhookRefused
	}
	if err != nil {
		return fmt.Errorf("qris: reading the webhook's account: %w", err)
	}
	if !autogopay.ValidSignature(body, key, signature) {
		w.log.WarnContext(ctx, "AutoGoPay callback with a bad signature", "organization", org)
		return ErrWebhookRefused
	}
	cb, err := autogopay.ParseCallback(body)
	if err != nil {
		w.log.ErrorContext(ctx, "signed AutoGoPay callback we cannot read; reconcile will find a payment", "organization", org, "error", err)
		return ErrWebhookRefused
	}
	switch {
	case cb.Event == autogopay.EventChallenge:
		w.log.InfoContext(ctx, "AutoGoPay verified the webhook URL", "organization", org)
		return nil
	case cb.Event == autogopay.EventTransactionReceived && cb.Transaction.Status == autogopay.StatusPaid:
		t := cb.Transaction
		if err := w.queue.EnqueuePaid(ctx, org, t.ID, t.Amount); err != nil {
			return fmt.Errorf("qris: enqueueing qris-paid: %w", err)
		}
		w.log.InfoContext(ctx, "AutoGoPay reported a QR paid", "organization", org, "transaction", t.ID, "amount", t.Amount)
		return nil
	default:
		status := ""
		if cb.Transaction != nil {
			status = string(cb.Transaction.Status)
		}
		w.log.InfoContext(ctx, "AutoGoPay callback ignored", "organization", org, "event", cb.Event, "status", status)
		return nil
	}
}
