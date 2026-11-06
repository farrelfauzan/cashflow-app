package promise

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
)

// MaxDaysAhead is how far ahead a promise date may be (decision 41.2): an active promise silences
// the H+3 and H+7 reminders of what it covers, so it cannot run for months.
const MaxDaysAhead = 60

// maxNoteLength is the longest note a member may add to a promise, in letters.
const maxNoteLength = 500

// Kinds of promise ("Bareng tagihan berikutnya" or "Pilih tanggal sendiri").
const (
	KindSpecificDate    = "specific_date"
	KindWithNextInvoice = "with_next_invoice"
)

// Where a promise version was made.
const (
	SourceTenantPage = "tenant_page"
	SourceKaryawan   = "karyawan" // from slice 6 (W20)
	SourceAdmin      = "admin"
)

// Promise statuses as stored in payment_promise.status. Only active is set here; closing a promise
// is slice 4's S4-3.
const (
	StatusActive        = "active"
	StatusKept          = "kept"
	StatusPartiallyKept = "partially_kept"
	StatusBroken        = "broken"
)

var (
	// ErrNothingOwed means the tenant owes nothing, or will owe nothing once what they pay now is
	// paid, so there is nothing to promise.
	ErrNothingOwed = errors.New("promise: nothing owed")
	// ErrDateOutOfRange means the promise date is not from tomorrow to MaxDaysAhead days ahead
	// (Asia/Jakarta); for with_next_invoice, that the next invoice is due after that.
	ErrDateOutOfRange = errors.New("promise: date out of range")
	// ErrNoNextInvoice means with_next_invoice was asked for and none of the tenant's leases will
	// be billed again: every one has ended or reaches its planned end first.
	ErrNoNextInvoice = errors.New("promise: no next invoice")
	// ErrAmountAboveOutstanding means what is paid now, or the promised amount, is more than the
	// tenant owes (the promised amount: more than what is left after paying now).
	ErrAmountAboveOutstanding = errors.New("promise: amount above what the tenant owes")
)

// FieldError is one of the sentinel errors above for the input field that caused it.
type FieldError struct {
	Err   error
	Field string
}

func (e *FieldError) Error() string { return fmt.Sprintf("%v (%s)", e.Err, e.Field) }

func (e *FieldError) Unwrap() error { return e.Err }

// InputError is an input that is wrong whatever the tenant owes. Message is safe to show.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return fmt.Sprintf("promise: %s: %s", e.Field, e.Message) }

// Input is a promise as asked for, from the tenant page or a member. Dates are calendar days, as
// midnight UTC like a date column.
type Input struct {
	Kind string
	// PromisedDate is the day for KindSpecificDate; it must be nil for KindWithNextInvoice, whose
	// date the server sets.
	PromisedDate *time.Time
	// PromisedAmount is the rupiah promised by then; nil means everything the promise covers.
	PromisedAmount *int64
	// PayingNow is what the tenant pays right after saving; it is not part of the promised rest.
	PayingNow int64
	// Note is a member's note; the tenant page has none.
	Note string
}

// checked returns in with its note trimmed, or an *InputError for an input that is wrong whatever
// the tenant owes.
func (in Input) checked() (Input, error) {
	switch in.Kind {
	case KindSpecificDate:
		if in.PromisedDate == nil {
			return in, &InputError{Field: "promisedDate", Message: "Pilih tanggal janji."}
		}
	case KindWithNextInvoice:
		if in.PromisedDate != nil {
			return in, &InputError{Field: "promisedDate", Message: "Janji bareng tagihan berikutnya tidak memakai tanggal pilihan."}
		}
	default:
		return in, &InputError{Field: "kind", Message: "Pilih kapan akan dibayar."}
	}
	if in.PromisedAmount != nil && *in.PromisedAmount < 1 {
		return in, &InputError{Field: "promisedAmount", Message: "Jumlah janji harus lebih dari Rp0, atau kosongkan untuk semua sisa tagihan."}
	}
	if in.PayingNow < 0 {
		return in, &InputError{Field: "payingNow", Message: "Jumlah yang dibayar sekarang tidak boleh kurang dari Rp0."}
	}
	in.Note = strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(in.Note) > maxNoteLength {
		return in, &InputError{Field: "note", Message: "Catatan paling panjang 500 huruf."}
	}
	return in, nil
}

// Window is what a promise made today may say: a date from Earliest to Latest, and NextDue, the
// due date "Bareng tagihan berikutnya" stands for (nil when no lease will bill again). NextDue can
// be after Latest, which makes that kind unavailable.
type Window struct {
	Earliest, Latest time.Time
	NextDue          *time.Time
}

// WindowOn returns the window of a promise made today (Asia/Jakarta, midnight UTC) by a tenant
// with leases: from tomorrow to MaxDaysAhead days ahead, and the earliest due date after today
// among the leases that will still bill then (see billing.NextDueAfter).
func WindowOn(today time.Time, leases []billing.LeaseDates) Window {
	w := Window{Earliest: today.AddDate(0, 0, 1), Latest: today.AddDate(0, 0, MaxDaysAhead)}
	for _, l := range leases {
		due, ok := billing.NextDueAfter(l, today)
		if ok && (w.NextDue == nil || due.Before(*w.NextDue)) {
			w.NextDue = &due
		}
	}
	return w
}

// NextDueInRange is NextDue when a promise may be made for it, else nil.
func (w Window) NextDueInRange() *time.Time {
	if w.NextDue == nil || !w.contains(*w.NextDue) {
		return nil
	}
	return w.NextDue
}

func (w Window) contains(d time.Time) bool { return !d.Before(w.Earliest) && !d.After(w.Latest) }

// Decide returns the promise date for in (already checked), given what the tenant owes now and the
// window, or why no such promise can be made. It checks what is owed first, then the amounts, then
// the date.
func Decide(in Input, outstanding int64, w Window) (time.Time, error) {
	if outstanding <= 0 {
		return time.Time{}, ErrNothingOwed
	}
	if in.PayingNow > outstanding {
		return time.Time{}, &FieldError{Err: ErrAmountAboveOutstanding, Field: "payingNow"}
	}
	rest := outstanding - in.PayingNow
	if rest == 0 {
		return time.Time{}, ErrNothingOwed
	}
	if in.PromisedAmount != nil && *in.PromisedAmount > rest {
		return time.Time{}, &FieldError{Err: ErrAmountAboveOutstanding, Field: "promisedAmount"}
	}
	switch in.Kind {
	case KindWithNextInvoice:
		if w.NextDue == nil {
			return time.Time{}, &FieldError{Err: ErrNoNextInvoice, Field: "kind"}
		}
		if !w.contains(*w.NextDue) {
			return time.Time{}, &FieldError{Err: ErrDateOutOfRange, Field: "promisedDate"}
		}
		return *w.NextDue, nil
	default:
		d := calendarDay(*in.PromisedDate)
		if !w.contains(d) {
			return time.Time{}, &FieldError{Err: ErrDateOutOfRange, Field: "promisedDate"}
		}
		return d, nil
	}
}

// calendarDay is t's calendar date as midnight UTC, like a date column.
func calendarDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
