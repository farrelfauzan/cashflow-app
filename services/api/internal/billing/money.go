package billing

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
)

// MaxAmount is the largest payment or correction a member may record, in rupiah (one trillion).
const MaxAmount int64 = 1_000_000_000_000

// maxNoteLength is the longest payment note or correction reason, in letters.
const maxNoteLength = 500

// Payment methods and sources as stored on a payment row.
const (
	MethodCash     = "cash"
	MethodTransfer = "transfer"
	MethodQRIS     = "qris"
	MethodVA       = "va"
	SourceManual   = "manual"
)

var (
	// ErrNotFound means there is no such tenant, invoice or payment in the organization. The
	// error returned wraps it in a *NotFoundError naming the input field, if one named it.
	ErrNotFound = errors.New("billing: not found")
	// ErrRequestIDReused means a requestId already recorded a payment or correction that is
	// not the one now asked for.
	ErrRequestIDReused = errors.New("billing: request id used for something else")
	// ErrCorrectionNoChange means a correction's new amount equals the current amount.
	ErrCorrectionNoChange = errors.New("billing: correction changes nothing")
	// ErrCorrectionBelowZero means a correction would take an invoice's billed amount or a
	// payment's net amount below zero.
	ErrCorrectionBelowZero = errors.New("billing: correction below zero")
)

// NotFoundError is ErrNotFound for the input field that named the missing record; Field is
// empty when the record came from the path.
type NotFoundError struct {
	Field string
}

func (e *NotFoundError) Error() string { return fmt.Sprintf("%v: %s", ErrNotFound, e.Field) }

func (e *NotFoundError) Unwrap() error { return ErrNotFound }

// InputError names the field that is wrong. Message is safe to show to the user.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return fmt.Sprintf("billing: %s: %s", e.Field, e.Message) }

// PaymentInput is a cash or transfer payment a member records by hand.
type PaymentInput struct {
	// RequestID is made by the client once per form; the same one again with the same input
	// records nothing new.
	RequestID uuid.UUID
	TenantID  uuid.UUID
	Amount    int64
	Method    string // MethodCash or MethodTransfer
	// ReceivedOn is the calendar day the money was received in Asia/Jakarta, as midnight UTC
	// like a date column.
	ReceivedOn time.Time
	Note       string
}

// CorrectionInput corrects exactly one invoice or one payment.
type CorrectionInput struct {
	RequestID uuid.UUID
	InvoiceID uuid.UUID // uuid.Nil when it corrects a payment
	PaymentID uuid.UUID // uuid.Nil when it corrects an invoice
	// Exactly one of AmountDelta and NewAmount. AmountDelta is signed and never 0; NewAmount is
	// what the invoice should bill or the payment count ("Nominal baru"), and the service takes
	// the difference from the current amount inside the transaction.
	AmountDelta *int64
	NewAmount   *int64
	Reason      string
}

// ValidPayment checks a payment and returns it with its note trimmed and ReceivedOn as a
// calendar day. today is the current day in Asia/Jakarta (tenant.Today). Errors are *InputError.
func ValidPayment(in PaymentInput, today time.Time) (PaymentInput, error) {
	in.Note = strings.TrimSpace(in.Note)
	in.ReceivedOn = calendarDay(in.ReceivedOn)
	switch {
	case in.RequestID == uuid.Nil:
		return in, &InputError{Field: "requestId", Message: "Permintaan tidak valid. Muat ulang halaman lalu coba lagi."}
	case in.Amount < 1 || in.Amount > MaxAmount:
		return in, &InputError{Field: "amount", Message: "Nominal harus antara Rp1 dan Rp1.000.000.000.000."}
	case in.Method != MethodCash && in.Method != MethodTransfer:
		return in, &InputError{Field: "method", Message: "Pilih tunai atau transfer."}
	case in.ReceivedOn.IsZero() || in.ReceivedOn.Year() < 2000:
		return in, &InputError{Field: "receivedOn", Message: "Tanggal terima wajib diisi."}
	case in.ReceivedOn.After(today):
		return in, &InputError{Field: "receivedOn", Message: "Tanggal terima tidak boleh setelah hari ini."}
	case len([]rune(in.Note)) > maxNoteLength:
		return in, &InputError{Field: "note", Message: fmt.Sprintf("Catatan paling panjang %d huruf.", maxNoteLength)}
	}
	return in, nil
}

// ValidCorrection checks a correction and returns it with its reason trimmed. Errors are
// *InputError.
func ValidCorrection(in CorrectionInput) (CorrectionInput, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	switch {
	case in.RequestID == uuid.Nil:
		return in, &InputError{Field: "requestId", Message: "Permintaan tidak valid. Muat ulang halaman lalu coba lagi."}
	case (in.InvoiceID == uuid.Nil) == (in.PaymentID == uuid.Nil):
		return in, &InputError{Field: "invoiceId", Message: "Pilih satu tagihan atau satu pembayaran yang dikoreksi."}
	case (in.AmountDelta == nil) == (in.NewAmount == nil):
		return in, &InputError{Field: "newAmount", Message: "Isi nominal baru."}
	case in.NewAmount != nil && (*in.NewAmount < 0 || *in.NewAmount > MaxAmount):
		return in, &InputError{Field: "newAmount", Message: "Nominal baru harus antara Rp0 dan Rp1.000.000.000.000."}
	case in.AmountDelta != nil && *in.AmountDelta == 0:
		return in, &InputError{Field: "amountDelta", Message: "Nominal koreksi tidak boleh nol."}
	case in.AmountDelta != nil && (*in.AmountDelta < -MaxAmount || *in.AmountDelta > MaxAmount):
		return in, &InputError{Field: "amountDelta", Message: "Nominal koreksi paling besar Rp1.000.000.000.000."}
	case in.Reason == "":
		return in, &InputError{Field: "reason", Message: "Alasan koreksi wajib diisi."}
	case len([]rune(in.Reason)) > maxNoteLength:
		return in, &InputError{Field: "reason", Message: fmt.Sprintf("Alasan koreksi paling panjang %d huruf.", maxNoteLength)}
	}
	return in, nil
}

// receivedAt is the moment a payment received on day is stored with: 00:00 in Asia/Jakarta.
func receivedAt(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, clock.Jakarta)
}

// calendarDay keeps only the year, month and day of t, as midnight UTC like a date column.
func calendarDay(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// receivedOn is the Asia/Jakarta day of a stored received_at, as midnight UTC.
func receivedOn(at time.Time) time.Time {
	return calendarDay(at.In(clock.Jakarta))
}
