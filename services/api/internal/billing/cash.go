package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// Statuses of a cash payment's receipt (payment_receipt.status).
const (
	ReceiptPending = "pending" // recorded with the payment; the receipt:send task sends it
	ReceiptSent    = "sent"    // handed to the provider (at most once)
	ReceiptSkipped = "skipped" // nothing to send any more, such as a payment corrected to zero
)

// ReceiptQueue enqueues the receipt:send task of a cash payment that has committed. A task
// already queued counts as enqueued. receipttask.Client implements it.
type ReceiptQueue interface {
	EnqueueReceipt(ctx context.Context, org, paymentID uuid.UUID) error
}

// CashInput is cash a member collected from a tenant in the field ("Terima tunai").
type CashInput struct {
	// RequestID is made by the client once per form; the same one again with the same input
	// records nothing new.
	RequestID uuid.UUID
	TenantID  uuid.UUID
	Amount    int64
	Note      string
	// PhotoUploadID is the photo of the cash (decision 50): a completed cash_receipt upload for
	// the tenant made by the caller, not used by another payment and not refused by the scan. It is
	// required: field cash is never recorded without its photo.
	PhotoUploadID uuid.UUID
}

// CashResult is a recorded cash payment with the status of the tenant's receipt.
type CashResult struct {
	PaymentResult
	// ReceiptStatus is ReceiptPending right after recording; a repeated request answers the
	// receipt's status now.
	ReceiptStatus string
	// Photo is the photo recorded with the payment, its upload's status then (pending while it
	// is scanned); a repeated request answers the payment's photo now, which AttachReceiptPhoto
	// replaces only once the scan refused it.
	Photo ReceiptPhoto
}

// RecordCashPayment records cash a member collected from a tenant, received now, exactly as
// RecordPayment records a payment (FIFO over all the tenant's invoices, credit for the rest, the
// promise hook, requestId idempotency), but authorized by cash:record (audited) with the amount on
// the tenant through each of its active leases (decision 47): in the lease's property and assigned
// to its collector, so a karyawan records cash only for tenants they collect from now, whoever
// collected their ended leases. A tenant with no active lease needs the organization-wide grant.
// A missing tenant is a *NotFoundError with no field.
//
// The photo comes with the payment, never after it (decision 50): in.PhotoUploadID is required (an
// *InputError on photoUploadId) and is linked in the payment's transaction, checked as
// AttachReceiptPhoto checks an upload (a *NotFoundError, or ErrReceiptPhotoInvalid or
// ErrReceiptPhotoRefused in a *FieldError, on photoUploadId); any of them records nothing. A
// pending photo is accepted: neither the payment nor the receipt waits for the scan.
//
// In the same transaction it records the tenant's receipt (payment_receipt, pending) and, once
// that commits, enqueues its receipt:send task; a lost enqueue is caught by the sweep.
//
// The same requestId with the same tenant, amount, note and photo, recorded by the same member
// through this method, records nothing and answers the payment with Created false and the
// receipt's and photo's status now; anything else under that requestId is ErrRequestIDReused.
func (s *Service) RecordCashPayment(ctx context.Context, p authz.Principal, in CashInput) (CashResult, error) {
	today := tenant.Today(s.clock)
	valid, err := ValidPayment(PaymentInput{
		RequestID: in.RequestID, TenantID: in.TenantID, Amount: in.Amount, Method: MethodCash, ReceivedOn: today, Note: in.Note,
	}, today)
	if err != nil {
		return CashResult{}, err
	}
	if in.PhotoUploadID == uuid.Nil {
		return CashResult{}, &InputError{Field: "photoUploadId", Message: "Foto uang tunai wajib diunggah sebelum menyimpan."}
	}
	org := p.OrganizationID
	now := s.clock.Now()
	status := ReceiptPending
	var photo ReceiptPhoto
	res, err := s.recordManual(ctx, p, valid, manualPayment{
		action: authz.ActionCashRecord, field: true, receivedAt: now,
		same: func(row dbgen.Payment) bool {
			note := ""
			if row.Note != nil {
				note = *row.Note
			}
			return row.TenantID == valid.TenantID && row.Amount == valid.Amount && row.Method == MethodCash &&
				note == valid.Note && row.RecordedByStaffUserID.UUID == p.UserID
		},
		inTx: func(ctx context.Context, tx pgx.Tx, row dbgen.Payment, created bool) error {
			q := dbgen.New(tx)
			if created {
				if err := q.InsertPaymentReceipt(ctx, dbgen.InsertPaymentReceiptParams{
					PaymentID: row.ID, OrganizationID: org, TenantID: row.TenantID, CreatedAt: now,
				}); err != nil {
					return fmt.Errorf("recording the cash payment's receipt: %w", err)
				}
				// The photo's link names the receipt, so it follows it; an unusable photo rolls both
				// back with the payment.
				var err error
				photo, err = linkReceiptPhoto(ctx, q, org, p.UserID, row.ID, row.TenantID, in.PhotoUploadID, "photoUploadId", now)
				return err
			}
			st, err := q.GetPaymentReceiptStatus(ctx, dbgen.GetPaymentReceiptStatusParams{OrganizationID: org, PaymentID: row.ID})
			if errors.Is(err, pgx.ErrNoRows) {
				// The requestId recorded a payment some other way (recordPayment), not this cash.
				return ErrRequestIDReused
			}
			if err != nil {
				return fmt.Errorf("loading the cash payment's receipt: %w", err)
			}
			status = st
			// The same payment only with the photo it was recorded with: its first link. A payment
			// recorded before photos were required (decision 50) has none, so any photo repeats it.
			links, err := q.ListPaymentReceiptPhotoLinks(ctx, dbgen.ListPaymentReceiptPhotoLinksParams{OrganizationID: org, PaymentID: row.ID})
			if err != nil {
				return fmt.Errorf("listing the cash payment's photos: %w", err)
			}
			if first, ok := firstLink(links); ok && first != in.PhotoUploadID {
				return ErrRequestIDReused
			}
			photos, err := currentReceiptPhotos(ctx, q, org, []uuid.UUID{row.ID})
			if err != nil {
				return err
			}
			photo = photos[row.ID]
			return nil
		},
	})
	if err != nil {
		return CashResult{}, err
	}
	if status == ReceiptPending {
		s.enqueueReceipt(ctx, org, res.Payment.ID)
	}
	return CashResult{PaymentResult: res, ReceiptStatus: status, Photo: photo}, nil
}

// firstLink returns the upload of a payment's first photo link; links are latest first.
func firstLink(links []dbgen.ListPaymentReceiptPhotoLinksRow) (uuid.UUID, bool) {
	if len(links) == 0 {
		return uuid.Nil, false
	}
	return links[len(links)-1].UploadID, true
}

// enqueueReceipt enqueues the receipt of a committed cash payment. A failure is logged, not
// returned: the payment is recorded, and the sweep (reminders:sweep) enqueues a receipt still
// pending after a lost enqueue.
func (s *Service) enqueueReceipt(ctx context.Context, org, paymentID uuid.UUID) {
	if s.receipts == nil {
		return
	}
	if err := s.receipts.EnqueueReceipt(ctx, org, paymentID); err != nil {
		s.log.WarnContext(ctx, "enqueueing a cash payment's receipt; the sweep retries it",
			"organization", org, "payment", paymentID, "error", err)
	}
}
