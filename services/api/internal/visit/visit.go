// Package visit is the karyawan's visit list, "Kunjungan hari ini", and the outcomes they record
// on it, "Catat hasil kunjungan" (slice 6, S6-4, decision 46.6). The list is built from the open
// visit tasks the reminder rules and the promise closer open (h7, broken_promises, and from slice 5
// undeliverable). An outcome points at what the field app already recorded (a cash payment, a
// promise version) or carries a new number for the admin, and resolves the tasks it answers.
package visit

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
)

// Outcomes of a visit.
const (
	OutcomePaid          = "paid"
	OutcomePromise       = "promise"
	OutcomeNotHome       = "not_home"
	OutcomeNumberChanged = "number_changed"
)

// Reasons of a visit task (visit_task.reason).
const (
	TaskH7             = "h7"
	TaskBrokenPromises = "broken_promises"
	TaskUndeliverable  = "undeliverable"
)

// Reasons a tenant is on the visit list, as the client sees them.
const (
	ReasonOverdue        = "overdue"         // an h7 task: debt a week past due
	ReasonBrokenPromises = "broken_promises" // promises broken since the last such visit
	ReasonUndeliverable  = "undeliverable"   // WhatsApp cannot reach the tenant's number
)

// Statuses of a row of the visit list.
const (
	StatusOpen = "open" // an open visit task
	StatusDone = "done" // visited today, nothing left open
)

const maxNote = 500

// resolves lists the task reasons each outcome resolves (decision 46.6): paid and promise answer
// the debt and the broken promises, a new number answers an unreachable number, not home answers
// nothing.
var resolves = map[string][]string{
	OutcomePaid:          {TaskH7, TaskBrokenPromises},
	OutcomePromise:       {TaskH7, TaskBrokenPromises},
	OutcomeNumberChanged: {TaskUndeliverable},
	OutcomeNotHome:       nil,
}

var (
	// ErrNotOpen means the tenant has no open visit task.
	ErrNotOpen = errors.New("visit: no open visit")
	// ErrPaymentInvalid means the payment cannot back a paid outcome: it is not field cash the
	// member took from this tenant today, or another outcome names it already.
	ErrPaymentInvalid = errors.New("visit: the payment cannot back this outcome")
	// ErrPromiseInvalid means the promise version cannot back a promise outcome: the member did not
	// make it from the field for this tenant today, or another outcome names it already.
	ErrPromiseInvalid = errors.New("visit: the promise cannot back this outcome")
	// ErrInvalidPhone means the new number is not a phone number.
	ErrInvalidPhone = errors.New("visit: invalid phone number")
	// ErrRequestIDReused means a requestId already recorded a different outcome.
	ErrRequestIDReused = errors.New("visit: request id used for another outcome")
)

// InputError is invalid input on one field, with a message for the user.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return "visit: invalid " + e.Field }

// Input is an outcome a member records on a tenant.
type Input struct {
	RequestID        uuid.UUID
	Outcome          string
	PaymentID        uuid.UUID // OutcomePaid only
	PromiseVersionID uuid.UUID // OutcomePromise only
	NewPhone         string    // OutcomeNumberChanged only, as typed
	Note             string
}

// Outcome is a recorded outcome.
type Outcome struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Outcome   string
	PaymentID uuid.UUID // uuid.Nil unless paid
	// Amount is what the payment counts now, net of its corrections; 0 unless paid.
	Amount           int64
	PromiseVersionID uuid.UUID // uuid.Nil unless promise
	NewPhone         string    // E.164, empty unless number_changed
	Note             string
	RecordedAt       time.Time
	// Resolved are the reasons (ReasonOverdue, ...) of the tasks it resolved.
	Resolved []string
}

// Reason is why a tenant is on the visit list.
type Reason struct {
	Code string // ReasonOverdue, ReasonBrokenPromises or ReasonUndeliverable
	// DaysOverdue is, for ReasonOverdue, how many days the oldest unpaid invoice is past due now;
	// 0 when nothing is overdue any more.
	DaysOverdue int
	// BrokenPromises is, for ReasonBrokenPromises, how many promises broke since the tenant's last
	// such visit.
	BrokenPromises int
}

// ListResult is one page of the visit list, with how many of all its rows are still open.
type ListResult struct {
	listing.Result[Row]
	// OpenTotal counts the rows of the whole list (not only this page) with status open.
	OpenTotal int64
}

// Row is one tenant on the visit list.
type Row struct {
	TenantID            uuid.UUID
	TenantName          string
	Units               []billing.FieldUnit
	Reasons             []Reason // of the open tasks, oldest first; empty when done
	Outstanding         int64
	Overdue             int64
	OldestUnpaidDueDate *time.Time
	Status              string
	// LastOutcome is the latest outcome the member recorded on the tenant today, if any.
	LastOutcome *Outcome
	// UnlinkedCashToday is the field cash the member took from the tenant today that no outcome
	// names yet, oldest first, for "Sudah bayar hari ini".
	UnlinkedCashToday []billing.FieldCashPayment
}

// validInput trims and checks an outcome; the phone is normalized by the caller.
func validInput(in Input) (Input, error) {
	in.Note = strings.TrimSpace(in.Note)
	in.NewPhone = strings.TrimSpace(in.NewPhone)
	if in.RequestID == uuid.Nil {
		return in, &InputError{Field: "requestId", Message: "Permintaan tidak valid. Muat ulang halaman lalu coba lagi."}
	}
	if _, ok := resolves[in.Outcome]; !ok {
		return in, &InputError{Field: "outcome", Message: "Hasil kunjungan tidak dikenal."}
	}
	paid, promise, number := in.Outcome == OutcomePaid, in.Outcome == OutcomePromise, in.Outcome == OutcomeNumberChanged
	switch {
	case paid != (in.PaymentID != uuid.Nil):
		return in, &InputError{Field: "paymentId", Message: "Pembayaran tunai wajib dipilih untuk hasil \"sudah bayar\", dan hanya untuk itu."}
	case promise != (in.PromiseVersionID != uuid.Nil):
		return in, &InputError{Field: "promiseVersionId", Message: "Janji bayar wajib dipilih untuk hasil \"janji baru\", dan hanya untuk itu."}
	case number != (in.NewPhone != ""):
		return in, &InputError{Field: "newPhone", Message: "Nomor WA baru wajib diisi untuk hasil \"nomor berubah\", dan hanya untuk itu."}
	case utf8.RuneCountInString(in.Note) > maxNote:
		return in, &InputError{Field: "note", Message: "Catatan paling panjang 500 huruf."}
	}
	return in, nil
}

// clientReason is the reason the client sees for a task reason.
func clientReason(task string) string {
	if task == TaskH7 {
		return ReasonOverdue
	}
	return task
}
