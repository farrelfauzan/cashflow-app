// Package upload keeps staff photos (cash receipt and cash deposit photos for now) in object
// storage, R2 in production (S6-1, decision 18).
//
// A photo goes from the phone straight to storage through a presigned PUT; the database keeps
// only its key. The upload stays pending until the scan job (Checker.Scan, run by the
// upload:scan task) has checked the stored file's real size and type and scanned it for malware.
// Only a clean upload can be read, through a short-lived presigned GET that downloads it as an
// attachment. A rejected or infected file is moved to the quarantine prefix and flagged.
//
// Object keys, all derived from upload.object_key (uploads/<organization>/<32 hex>):
//
//	incoming/<key>    the presigned PUT writes here; the scan moves the file away before reading it
//	files/<key>       the copy the scan checks, and the one a clean upload is read from
//	quarantine/<key>  a rejected or infected file
//
// The scan reads its own copy, so a second PUT with the same URL (valid for PutURLTTL) can never
// replace a file after it was scanned. A PUT that arrives after the scan lands in incoming/ and
// is never read; a lifecycle rule on the bucket should expire incoming/ after a day.
//
// Service is the API side (create, complete, read); Checker is the worker side (scan,
// reconcile). The storage and scanner adapters live in internal/storage/s3store and
// internal/scan/clamd; uploadtest has hand-written fakes of both.
package upload

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// What an upload is for. Each purpose names the record it belongs to, which decides the
// permission: see Service.Create.
const (
	PurposeCashReceipt = "cash_receipt"
	PurposeCashDeposit = "cash_deposit"
)

// An upload's status. Only pending changes, once, to one of the other three.
const (
	StatusPending  = "pending"
	StatusClean    = "clean"
	StatusRejected = "rejected" // not an allowed image, not its declared size, too large, or never arrived
	StatusInfected = "infected"
)

// The limits on what may be uploaded. They are expected to change; keep them here only. The
// migration's upload_clean_type check lists the same types.
const (
	// MaxSize is the largest file accepted: 10 MB.
	MaxSize int64 = 10 << 20
	// PutURLTTL is how long a presigned PUT stays valid.
	PutURLTTL = 10 * time.Minute
	// GetURLTTL is how long a download link stays valid.
	GetURLTTL = 5 * time.Minute
	// StaleAfter is how long a completed upload may stay pending before the reconcile enqueues its
	// scan again.
	StaleAfter = 5 * time.Minute
	// MissingAfter is how long after completeUpload a file that is still not in storage is waited
	// for; after it the upload is rejected.
	MissingAfter = time.Hour
)

// allowedTypes are the accepted image types, as http.DetectContentType names them, with the
// extension a download gets. HEIC (the iPhone default) is not accepted: the web app asks the
// camera for JPEG or converts.
var allowedTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// AllowedTypes returns the accepted content types, sorted.
func AllowedTypes() []string {
	out := make([]string, 0, len(allowedTypes))
	for t := range allowedTypes {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

// AllowedType reports whether contentType is an accepted image type.
func AllowedType(contentType string) bool {
	_, ok := allowedTypes[contentType]
	return ok
}

// Actions an upload is authorized by. Each is in the authz catalog (TestActionsAreInTheCatalog).
const (
	actionCashRecord   = "cash:record"
	actionCashDeposit  = "cash:deposit"
	actionPaymentCheck = "payment:check"
	actionCashRead     = "cash:read"
	// auditScanAction names the scan's audit rows for a rejected or infected file, a platform
	// action with no actor (authz.Service.RecordSystemAction).
	auditScanAction = "upload:scan"
)

var (
	// ErrPurposeUnknown is a purpose other than PurposeCashReceipt and PurposeCashDeposit.
	ErrPurposeUnknown = errors.New("upload: unknown purpose")
	// ErrTypeNotAllowed is a declared content type that is not an accepted image type.
	ErrTypeNotAllowed = errors.New("upload: content type not allowed")
	// ErrTooLarge is a declared size above MaxSize.
	ErrTooLarge = errors.New("upload: file too large")
	// ErrNotFound is an upload that is not in the organization.
	ErrNotFound = errors.New("upload: not found")
	// ErrTenantNotFound is a tenant that is not in the organization.
	ErrTenantNotFound = errors.New("upload: tenant not found")
	// ErrObjectNotFound is what an Objects adapter returns for a key that holds no object.
	ErrObjectNotFound = errors.New("upload: object not found")
)

// InputError is an invalid input other than the ones with their own error, naming the field,
// with a message in Bahasa Indonesia.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return "upload: invalid " + e.Field + ": " + e.Message }

// PresignedPut is where and how the browser PUTs a file.
type PresignedPut struct {
	URL string
	// Headers the PUT must carry exactly (Content-Type, and anything else the adapter signed
	// that a browser can set). Content-Length is signed too, but the browser sets it from the
	// body.
	Headers map[string]string
}

// Presigner makes presigned URLs. The API's storage adapter implements it; making a URL does
// no I/O.
type Presigner interface {
	// PresignPut lets the holder PUT exactly size bytes of contentType at key until ttl passes.
	PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (PresignedPut, error)
	// PresignGet lets the holder download key until ttl passes, as an attachment named filename
	// with contentType.
	PresignGet(ctx context.Context, key string, ttl time.Duration, filename, contentType string) (string, error)
}

// Objects reads and moves stored files. The worker's storage adapter implements it.
type Objects interface {
	// Head returns the size of the object at key, or ErrObjectNotFound.
	Head(ctx context.Context, key string) (int64, error)
	// Open returns the object's content, or ErrObjectNotFound. The caller closes it.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// Move copies the object at src to dst, replacing dst, then deletes src. A missing src is
	// ErrObjectNotFound.
	Move(ctx context.Context, src, dst string) error
}

// Verdict is a malware scanner's answer.
type Verdict struct {
	Clean bool
	// Signature names what was found when not clean, such as "Eicar-Test-Signature".
	Signature string
}

// Scanner scans content for malware. The clamd adapter implements it.
type Scanner interface {
	Scan(ctx context.Context, r io.Reader) (Verdict, error)
}

// Upload is an upload as the API shows it.
type Upload struct {
	ID       uuid.UUID
	Purpose  string
	TenantID uuid.UUID // uuid.Nil for a cash_deposit
	Status   string
	// ContentType is the type the scan found in the content, empty while pending.
	ContentType string
	Size        int64
	UploadedBy  uuid.UUID // staff user id
	CreatedAt   time.Time
	CompletedAt *time.Time
	ScannedAt   *time.Time
}

// newObjectKey makes an upload's key from its organization and 128 bits of fresh randomness;
// nothing in it comes from the client.
func newObjectKey(org uuid.UUID) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (it crashes the program instead)
	return "uploads/" + org.String() + "/" + hex.EncodeToString(b)
}

// IncomingKey is where the presigned PUT writes the upload with key.
func IncomingKey(key string) string { return "incoming/" + key }

// StoredKey is the copy the scan checks and a clean upload is read from.
func StoredKey(key string) string { return "files/" + key }

// QuarantineKey is where a rejected or infected file is moved.
func QuarantineKey(key string) string { return "quarantine/" + key }

// downloadName is the file name a download gets: bukti-<upload id>.<extension of its type>.
func downloadName(id uuid.UUID, contentType string) string {
	return "bukti-" + id.String() + allowedTypes[contentType]
}

// CreateInput starts an upload.
type CreateInput struct {
	Purpose     string
	TenantID    uuid.UUID // required for PurposeCashReceipt, uuid.Nil for PurposeCashDeposit
	ContentType string
	Size        int64
}

// validCreate checks in and normalizes its content type (lower case, no spaces).
func validCreate(in CreateInput) (CreateInput, error) {
	switch in.Purpose {
	case PurposeCashReceipt:
		if in.TenantID == uuid.Nil {
			return in, &InputError{Field: "tenantId", Message: "Pilih penyewa untuk foto bukti ini."}
		}
	case PurposeCashDeposit:
		if in.TenantID != uuid.Nil {
			return in, &InputError{Field: "tenantId", Message: "Foto setoran tidak untuk penyewa tertentu."}
		}
	default:
		return in, ErrPurposeUnknown
	}
	in.ContentType = strings.ToLower(strings.TrimSpace(in.ContentType))
	if !AllowedType(in.ContentType) {
		return in, ErrTypeNotAllowed
	}
	if in.Size <= 0 {
		return in, &InputError{Field: "size", Message: "Ukuran file tidak valid."}
	}
	if in.Size > MaxSize {
		return in, ErrTooLarge
	}
	return in, nil
}
