// Package receipt sends the tenant's receipt for cash a karyawan collected (slice 6, S6-2,
// decision 46.5): the payment_receipt row recorded with the cash payment (billing.RecordCashPayment)
// is sent once, through the MessagingProvider, by the receipt:send task (package receipttask).
// Cash payments only for now; it is not the PDF kwitansi of decision 41(7).
//
// The receipt goes out the moment the cash is recorded, at any hour, because the tenant has just
// handed the money over: if the karyawan records less than they took, the tenant sees it at once.
package receipt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// SweepAfter is how long a receipt may stay pending before the sweep enqueues its task again: its
// first enqueue was lost (the API or Redis went away between the commit and the enqueue).
const SweepAfter = 15 * time.Minute

// sweepBatch bounds how many receipts one sweep enqueues per organization.
const sweepBatch = 500

// ErrReceiptNotFound means the organization has no receipt for that payment.
var ErrReceiptNotFound = errors.New("receipt: receipt not found")

// Outcome is what one Send did.
type Outcome string

// Outcomes of Send.
const (
	// OutcomeSent: the receipt is marked sent and the provider accepted the message.
	OutcomeSent Outcome = "sent"
	// OutcomeUndelivered: the receipt is marked sent, then the provider refused the message. It is
	// not tried again (at most once); the error is logged.
	OutcomeUndelivered Outcome = "undelivered"
	// OutcomeSkipped: the payment no longer counts anything (corrected to zero); the receipt is
	// marked skipped and nothing is sent.
	OutcomeSkipped Outcome = "skipped"
	// OutcomeSettled: an earlier delivery already sent or skipped it; nothing is done.
	OutcomeSettled Outcome = "settled"
)

// SenderDeps are what a Sender needs.
type SenderDeps struct {
	Store *store.Store
	// Provider sends the message. With TELEGRAM_MESSAGES=linked it is the router, which sends to a
	// tenant's linked Telegram chat instead of WhatsApp (decision 42).
	Provider messaging.Provider
	// Clock decides today in Asia/Jakarta and sent_at.
	Clock clock.Clock
	Log   *slog.Logger
}

// Sender sends recorded receipts.
//
// Sending is the platform's own work: it checks no member's permission and writes no audit row;
// the receipt row is the record. Every read and write runs in the receipt's organization scope.
//
// It is at most once, like reminder.Sender: in one transaction the receipt is locked, its message
// is built from the payment as it stands and the row is marked sent, and only after that commits is
// the provider called. A crash after the commit loses the message instead of sending it twice, and
// a provider error is logged, never retried. Nothing here logs the message text or the phone.
type Sender struct {
	store    *store.Store
	provider messaging.Provider
	clock    clock.Clock
	log      *slog.Logger
}

// NewSender returns a Sender.
func NewSender(d SenderDeps) *Sender {
	return &Sender{store: d.Store, provider: d.Provider, clock: d.Clock, log: d.Log}
}

// prepared is a message ready to go once its row is marked sent.
type prepared struct {
	outcome Outcome // OutcomeSent when msg is to be sent; otherwise why not
	reason  string  // why it was skipped, for the log
	msg     messaging.Message
}

// Send sends the receipt of one cash payment of org. A receipt that is not pending does nothing
// (OutcomeSettled), so a second delivery of its task sends nothing. It says what the payment paid
// and what the tenant owes when it is sent, under the tenant's allocation lock; a payment
// corrected to zero meanwhile is skipped.
func (s *Sender) Send(ctx context.Context, org, paymentID uuid.UUID) (Outcome, error) {
	var p prepared
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.LockPaymentReceipt(ctx, dbgen.LockPaymentReceiptParams{OrganizationID: org, PaymentID: paymentID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrReceiptNotFound
		}
		if err != nil {
			return fmt.Errorf("receipt: loading the receipt: %w", err)
		}
		if row.Status != billing.ReceiptPending {
			p = prepared{outcome: OutcomeSettled}
			return nil
		}
		if p, err = s.prepare(ctx, tx, org, row); err != nil {
			return err
		}
		status, at := billing.ReceiptSkipped, (*time.Time)(nil)
		if p.outcome == OutcomeSent {
			now := s.clock.Now()
			status, at = billing.ReceiptSent, &now
		}
		n, err := q.MarkPaymentReceipt(ctx, dbgen.MarkPaymentReceiptParams{OrganizationID: org, PaymentID: paymentID, Status: status, SentAt: at})
		if err != nil {
			return fmt.Errorf("receipt: marking it %s: %w", status, err)
		}
		if n != 1 {
			return fmt.Errorf("receipt: marking it %s changed %d rows", status, n)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return s.deliver(ctx, org, paymentID, p), nil
}

// prepare builds, inside tx under the tenant's allocation lock, the message of a locked pending
// receipt, checked with messaging.Render so a mistake fails here, before the row is marked.
func (s *Sender) prepare(ctx context.Context, tx pgx.Tx, org uuid.UUID, row dbgen.LockPaymentReceiptRow) (prepared, error) {
	q := dbgen.New(tx)
	if err := billing.LockTenant(ctx, q, org, row.TenantID); err != nil {
		return prepared{}, err
	}
	res, err := billing.LoadPaymentResultTx(ctx, tx, org, row.TenantID, row.PaymentID, tenant.Today(s.clock))
	if err != nil {
		return prepared{}, fmt.Errorf("receipt: loading the payment: %w", err)
	}
	if res.Payment.Net <= 0 {
		return prepared{outcome: OutcomeSkipped, reason: "the payment was corrected to zero"}, nil
	}
	who, err := q.GetMessageRecipient(ctx, dbgen.GetMessageRecipientParams{OrganizationID: org, TenantID: row.TenantID})
	if err != nil {
		return prepared{}, fmt.Errorf("receipt: loading the recipient: %w", err)
	}
	msg := messaging.Message{
		OrganizationID: org, TenantID: row.TenantID, To: who.Phone,
		Template: messaging.TemplatePaymentReceipt, Params: Params(who.DisplayName, who.Name, res),
	}
	if _, err := messaging.Render(msg); err != nil {
		return prepared{}, fmt.Errorf("receipt: building the message: %w", err)
	}
	return prepared{outcome: OutcomeSent, msg: msg}, nil
}

// deliver calls the provider for a prepared message whose row is committed as sent, and logs what
// happened. It never logs the message or the phone.
func (s *Sender) deliver(ctx context.Context, org, paymentID uuid.UUID, p prepared) Outcome {
	attrs := []any{"payment", paymentID, "organization", org}
	switch p.outcome {
	case OutcomeSettled:
		s.log.InfoContext(ctx, "payment receipt already settled", attrs...)
		return p.outcome
	case OutcomeSkipped:
		s.log.InfoContext(ctx, "payment receipt skipped", append(attrs, "reason", p.reason)...)
		return p.outcome
	}
	_, err := s.provider.SendTemplate(ctx, p.msg)
	if err == nil {
		s.log.InfoContext(ctx, "payment receipt sent", attrs...)
		return OutcomeSent
	}
	// The adapters' errors carry the provider's code, never the message text.
	attrs = append(attrs, "error", err)
	switch {
	case errors.Is(err, messaging.ErrUndeliverable):
		s.log.WarnContext(ctx, "payment receipt undeliverable: the tenant's WhatsApp number or Telegram chat cannot receive it", attrs...)
	case errors.Is(err, messaging.ErrRateLimited):
		s.log.WarnContext(ctx, "payment receipt not sent: WhatsApp asked us to slow down", attrs...)
	case errors.Is(err, messaging.ErrNotConnected):
		s.log.ErrorContext(ctx, "payment receipt not sent: the WhatsApp session is not connected; an operator must re-link it", attrs...)
	default:
		s.log.ErrorContext(ctx, "payment receipt not sent", attrs...)
	}
	return OutcomeUndelivered
}

// Pending lists org's receipts still pending SweepAfter after they were recorded, oldest first and
// at most sweepBatch: the sweep enqueues their tasks again.
func (s *Sender) Pending(ctx context.Context, org uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		ids, err = dbgen.New(tx).ListPendingPaymentReceipts(ctx, dbgen.ListPendingPaymentReceiptsParams{
			OrganizationID: org, Before: s.clock.Now().Add(-SweepAfter), RowLimit: sweepBatch,
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("receipt: listing pending receipts: %w", err)
	}
	return ids, nil
}
