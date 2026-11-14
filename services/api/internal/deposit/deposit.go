// Package deposit is "Setor tunai" (slice 6, S6-3, decision 46.3): a karyawan hands the field cash
// they hold to the office, and an admin counts it. A deposit covers every field cash payment the
// member holds at once; the admin's count is recorded once, and a count that differs from the
// payments' total is a "selisih setoran" for the check queue. Tenants' balances never change: their
// payments were recorded when the cash was taken.
package deposit

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// Statuses of a deposit.
const (
	StatusPending = "pending" // not counted yet
	StatusMatched = "matched" // counted, equal to the payments' total
	StatusGap     = "gap"     // counted, different from it
)

// Filters of List by status.
const (
	FilterPending   = "pending"
	FilterConfirmed = "confirmed"
)

const (
	maxNote   = 500
	maxAmount = 1_000_000_000_000
	maxCount  = 1_000_000
)

var (
	// ErrNotFound means no such deposit in the organization.
	ErrNotFound = errors.New("deposit: not found")
	// ErrPhotoNotFound means the photo upload is not in the organization.
	ErrPhotoNotFound = errors.New("deposit: photo upload not found")
	// ErrNothingToDeposit means the member holds no field cash.
	ErrNothingToDeposit = errors.New("deposit: no cash to deposit")
	// ErrCashChanged means the cash the member holds is no longer what they were shown.
	ErrCashChanged = errors.New("deposit: the cash held changed")
	// ErrPhotoInvalid means the upload cannot be the deposit's photo: it is not a completed
	// cash_deposit upload by the member, or it is already another deposit's photo.
	ErrPhotoInvalid = errors.New("deposit: the upload cannot be this deposit's photo")
	// ErrPhotoRefused means the scan refused the photo.
	ErrPhotoRefused = errors.New("deposit: the photo was refused by the scan")
	// ErrRequestIDReused means a requestId already recorded a different deposit.
	ErrRequestIDReused = errors.New("deposit: request id used for another deposit")
	// ErrAlreadyConfirmed means the deposit was counted before with another amount or note.
	ErrAlreadyConfirmed = errors.New("deposit: already confirmed")
	// ErrStatusUnknown means a status filter outside FilterPending and FilterConfirmed.
	ErrStatusUnknown = errors.New("deposit: unknown status filter")
)

// InputError is invalid input on one field, with a message for the user.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return "deposit: invalid " + e.Field }

// Input is a deposit a member records.
type Input struct {
	RequestID uuid.UUID
	// ExpectedAmount and ExpectedCount are the cash the member was shown (billing.FieldCash).
	ExpectedAmount int64
	ExpectedCount  int
	// Amount is what the member hands over.
	Amount        int64
	Note          string
	PhotoUploadID uuid.UUID // uuid.Nil for none
}

// ConfirmInput is an admin's count of a deposit.
type ConfirmInput struct {
	ReceivedAmount int64
	Note           string
}

// Confirmation is a deposit's count.
type Confirmation struct {
	ReceivedAmount int64
	// Gap is ReceivedAmount minus the deposit's ExpectedAmount: negative when money is missing.
	Gap             int64
	Note            string
	ConfirmedBy     uuid.UUID
	ConfirmedByName string
	ConfirmedAt     time.Time
}

// Deposit is a recorded deposit.
type Deposit struct {
	ID             uuid.UUID
	StaffUserID    uuid.UUID
	StaffName      string
	Amount         int64
	ExpectedAmount int64
	PaymentCount   int
	Note           string
	Photo          billing.ReceiptPhoto
	RecordedAt     time.Time
	Status         string
	Confirmation   *Confirmation // nil while pending
}

// Query filters List. Empty fields filter nothing.
type Query struct {
	Status      string // FilterPending, FilterConfirmed or empty
	StaffUserID uuid.UUID
}

// validInput trims and checks a deposit.
func validInput(in Input) (Input, error) {
	in.Note = strings.TrimSpace(in.Note)
	switch {
	case in.RequestID == uuid.Nil:
		return in, &InputError{Field: "requestId", Message: "Permintaan tidak valid. Muat ulang halaman lalu coba lagi."}
	case in.ExpectedAmount < 0 || in.ExpectedAmount > maxAmount:
		return in, &InputError{Field: "expectedAmount", Message: "Total tunai tidak valid. Muat ulang halaman lalu coba lagi."}
	case in.ExpectedCount < 1 || in.ExpectedCount > maxCount:
		return in, &InputError{Field: "expectedCount", Message: "Jumlah tagihan tidak valid. Muat ulang halaman lalu coba lagi."}
	case in.Amount < 0 || in.Amount > maxAmount:
		return in, &InputError{Field: "amount", Message: "Jumlah yang disetor tidak valid."}
	case utf8.RuneCountInString(in.Note) > maxNote:
		return in, &InputError{Field: "note", Message: "Catatan paling panjang 500 huruf."}
	}
	return in, nil
}

// validConfirm trims and checks a count.
func validConfirm(in ConfirmInput) (ConfirmInput, error) {
	in.Note = strings.TrimSpace(in.Note)
	switch {
	case in.ReceivedAmount < 0 || in.ReceivedAmount > maxAmount:
		return in, &InputError{Field: "receivedAmount", Message: "Jumlah yang diterima tidak valid."}
	case utf8.RuneCountInString(in.Note) > maxNote:
		return in, &InputError{Field: "note", Message: "Catatan paling panjang 500 huruf."}
	}
	return in, nil
}

// fromRow turns a listed deposit into a Deposit.
func fromRow(r dbgen.ListCashDepositsRow) Deposit {
	d := Deposit{
		ID: r.ID, StaffUserID: r.StaffUserID, StaffName: r.StaffName, Amount: r.Amount,
		ExpectedAmount: r.ExpectedAmount, PaymentCount: int(r.PaymentCount), Note: deref(r.Note),
		Photo: billing.ReceiptPhoto{Status: billing.PhotoMissing}, RecordedAt: r.CreatedAt, Status: StatusPending,
	}
	if r.PhotoUploadID.Valid && r.PhotoStatus != nil {
		d.Photo = billing.ReceiptPhoto{UploadID: r.PhotoUploadID.UUID, Status: *r.PhotoStatus}
	}
	if r.ReceivedAmount != nil && r.ConfirmedAt != nil {
		c := Confirmation{
			ReceivedAmount: *r.ReceivedAmount, Gap: *r.ReceivedAmount - r.ExpectedAmount, Note: deref(r.ConfirmationNote),
			ConfirmedBy: r.ConfirmedByStaffUserID.UUID, ConfirmedByName: deref(r.ConfirmedByName), ConfirmedAt: *r.ConfirmedAt,
		}
		d.Confirmation = &c
		d.Status = StatusMatched
		if c.Gap != 0 {
			d.Status = StatusGap
		}
	}
	return d
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
