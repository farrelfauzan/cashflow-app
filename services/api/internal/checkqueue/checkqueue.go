// Package checkqueue is Antrean cek, the admin's check queue (slice 6, S6-6): what the system
// cannot settle on its own, read from its sources (the check_queue_item view), and the
// resolutions that take an item off it (check_resolution, append-only).
//
// The kinds of slice 6: a tenant's transfer proof (S6-5), a deposit whose counted cash differs from
// its payments (S6-3), an infected upload (S6-1), a QRIS payment AutoGoPay reported at another
// amount (decision 39.5) and a new number a karyawan brought back from a visit (S6-4). A proof is
// matched (it becomes a transfer payment, FIFO and credit as usual), a new number is applied to
// the tenant, and any item may be dismissed with a note ("bukan pembayaran sewa", the deposit gap
// closed, handled). Each item is answered once.
//
// The queue is the organization's: reading it needs payment:check or cash:read (the Supervisor)
// for the whole organization, and answering it payment:check (audited), plus payment:record for a
// match and tenant:manage for a new number.
package checkqueue

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Kinds of item.
const (
	KindTransferProof      = "transfer_proof"
	KindDepositGap         = "deposit_gap"
	KindUploadInfected     = "upload_infected"
	KindQrisAmountMismatch = "qris_amount_mismatch"
	KindPhoneChanged       = "phone_changed"
)

// Kinds lists every kind, in the order the client shows the counts.
var Kinds = []string{KindTransferProof, KindDepositGap, KindUploadInfected, KindQrisAmountMismatch, KindPhoneChanged}

// ValidKind reports whether kind is one of Kinds.
func ValidKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Outcomes of a resolution.
const (
	OutcomeMatched   = "matched"
	OutcomeApplied   = "applied"
	OutcomeDismissed = "dismissed"
)

const maxNote = 500

// Upload statuses the queue reads (upload.Status*); the upload package is not imported for them.
const (
	uploadPending = "pending"
	uploadClean   = "clean"
)

var (
	// ErrNotFound means the organization has no such item of that kind.
	ErrNotFound = errors.New("checkqueue: item not found")
	// ErrResolved means the item was already answered.
	ErrResolved = errors.New("checkqueue: item already resolved")
	// ErrUploadNotReady means a proof's photo is still being scanned, so nobody has seen it yet.
	ErrUploadNotReady = errors.New("checkqueue: the photo is still being scanned")
	// ErrUploadRefused means a proof's photo was refused by the scan (rejected or infected).
	ErrUploadRefused = errors.New("checkqueue: the photo was refused by the scan")
	// ErrRequestIDReused means a match's requestId already recorded a payment for something else.
	ErrRequestIDReused = errors.New("checkqueue: request id used for another payment")
)

// InputError is invalid input on one field, with a message for the user.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return "checkqueue: invalid " + e.Field }

// Page is a page of the queue.
type Page struct {
	Items []Item
	Total int
	// Counts is how many items of each kind wait, whatever kind was asked for; every kind is
	// present.
	Counts map[string]int
	// Waiting is how many items wait in all.
	Waiting int
}

// Item is one waiting item: its kind, its source row's id and when it arrived, and exactly one of
// the kind's details.
type Item struct {
	Kind      string
	ID        uuid.UUID
	CreatedAt time.Time

	TransferProof      *TransferProof
	DepositGap         *DepositGap
	UploadInfected     *UploadInfected
	QrisAmountMismatch *QrisAmountMismatch
	PhoneChanged       *PhoneChanged
}

// TenantRef is a tenant with the units of their leases, active ones first.
type TenantRef struct {
	ID    uuid.UUID
	Name  string
	Units []Unit
}

// Unit is a unit a tenant rents or rented.
type Unit struct {
	Label        string
	PropertyName string
	Active       bool
}

// StaffRef is a member by name.
type StaffRef struct {
	ID   uuid.UUID
	Name string
}

// TransferProof is a tenant's photo of a bank transfer, sent from a payment link (S6-5) or by a
// member who got it from the tenant (decision 49).
type TransferProof struct {
	Tenant TenantRef
	// InvoiceID is the invoice of the link it was sent from; uuid.Nil when a member sent it.
	InvoiceID uuid.UUID
	// SentBy is the member who sent it; nil when the tenant sent it from a link.
	SentBy       *StaffRef
	UploadID     uuid.UUID
	UploadStatus string // pending (being scanned) or clean
}

// DepositGap is a confirmed deposit whose counted cash differs from its payments (S6-3).
type DepositGap struct {
	Depositor    StaffRef
	Expected     int64
	Received     int64
	Gap          int64 // Received minus Expected: below zero is a shortfall
	PaymentCount int
	ConfirmedBy  StaffRef
	ConfirmedAt  time.Time
}

// UploadInfected is an upload the scan found infected (S6-1).
type UploadInfected struct {
	UploadID   uuid.UUID
	Purpose    string
	Tenant     *TenantRef // a receipt's or a proof's tenant
	UploadedBy *StaffRef  // the member, absent for a tenant's proof
	ScannedAt  *time.Time
}

// QrisAmountMismatch is a QRIS payment AutoGoPay reported at another amount than its QR
// (decision 39.5).
type QrisAmountMismatch struct {
	AttemptID     uuid.UUID
	Tenant        TenantRef
	TransactionID string
	QRAmount      int64
	// RecordedAmount is what the payment was recorded at.
	RecordedAmount *int64
	// ReportedAmount is what AutoGoPay reported, when it is known: nil when reconcile recorded
	// the QR's amount first and the report is only in the server log.
	ReportedAmount *int64
}

// PhoneChanged is a new number a karyawan brought back from a visit (S6-4).
type PhoneChanged struct {
	Tenant       TenantRef
	CurrentPhone string
	NewPhone     string
	RecordedBy   StaffRef
	Note         string
}

// Resolution is an answered item.
type Resolution struct {
	ID        uuid.UUID
	Kind      string
	ItemID    uuid.UUID
	Outcome   string
	PaymentID uuid.UUID // uuid.Nil unless matched
	Note      string
	By        uuid.UUID
	CreatedAt time.Time
}

// MatchInput is the transfer payment a proof becomes.
type MatchInput struct {
	RequestID uuid.UUID
	// TenantID is whose payment it is; uuid.Nil means the proof's tenant.
	TenantID   uuid.UUID
	Amount     int64
	ReceivedOn time.Time
	Note       string
}

// validNote trims a note and checks it; required says whether it may be empty.
func validNote(note string, required bool) (string, error) {
	note = strings.TrimSpace(note)
	switch {
	case required && note == "":
		return "", &InputError{Field: "note", Message: "Tulis alasannya."}
	case utf8.RuneCountInString(note) > maxNote:
		return "", &InputError{Field: "note", Message: "Catatan paling panjang 500 huruf."}
	}
	return note, nil
}
