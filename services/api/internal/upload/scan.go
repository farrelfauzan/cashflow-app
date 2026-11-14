package upload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// sniffLen is how much of a file http.DetectContentType reads.
const sniffLen = 512

// staleLimit bounds how many stale uploads one reconcile run enqueues per organization; the next
// run takes the rest.
const staleLimit = 500

// Why a file was rejected, for the log. Never shown to anyone outside the server.
const (
	reasonTooLarge     = "too_large"
	reasonSizeMismatch = "size_mismatch"
	reasonNotImage     = "not_image"
	reasonMissing      = "missing"
	reasonQuarantined  = "quarantined_earlier"
)

// Auditor writes an audit row for the platform's own action. authz.Service implements it.
type Auditor interface {
	RecordSystemAction(ctx context.Context, tx pgx.Tx, org uuid.UUID, action string, res authz.Resource) error
}

// CheckerDeps is what the worker side needs.
type CheckerDeps struct {
	Store   *store.Store
	Objects Objects
	Scanner Scanner
	Audit   Auditor
	Clock   clock.Clock
	Log     *slog.Logger
}

// Checker scans uploads (the upload:scan task) and finds the ones whose scan was lost (the
// reconcile).
type Checker struct {
	store   *store.Store
	objects Objects
	scanner Scanner
	audit   Auditor
	clock   clock.Clock
	log     *slog.Logger
}

// NewChecker returns a Checker.
func NewChecker(d CheckerDeps) *Checker {
	return &Checker{store: d.Store, objects: d.Objects, scanner: d.Scanner, audit: d.Audit, clock: d.Clock, log: d.Log}
}

// Result is what one scan run did.
type Result struct {
	// Status is the upload's status after the run.
	Status string
	// Changed is true when this run decided the status.
	Changed bool
	// Waiting is true when the upload is still pending because its file is not in storage yet,
	// or it was never completed.
	Waiting bool
}

// Scan checks one completed upload and decides its status, once: the stored file must be its
// declared size and at most MaxSize, the scanner must find nothing, and its content must be an
// accepted image type (sniffed, never the name or the declared type). A rejected or infected file
// is moved to quarantine and audited. A finished upload is left as it is, so a rerun changes
// nothing. A file not in storage yet is waited for until MissingAfter has passed since the upload
// was completed, then the upload is rejected.
//
// The whole run holds the upload's row lock (storage and the scanner are called inside the
// transaction), so two runs of one upload take turns and the second finds it decided: one run
// can never take a later PUT from incoming/ while another is scanning the earlier one. It holds
// a database connection for at most the task's timeout, like a QRIS call. An error from storage,
// the scanner or the database is returned for the queue to retry; nothing is decided then.
func (c *Checker) Scan(ctx context.Context, org, id uuid.UUID) (Result, error) {
	var out Result
	var f finding
	err := c.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.LockUpload(ctx, dbgen.LockUploadParams{OrganizationID: org, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("locking upload: %w", err)
		}
		if row.Status != StatusPending {
			out = Result{Status: row.Status}
			return nil
		}
		if row.CompletedAt == nil {
			// Only completeUpload and the reconcile (which lists completed uploads) enqueue a scan.
			out = Result{Status: row.Status, Waiting: true}
			return nil
		}
		if f, err = inspect(ctx, c.objects, c.scanner, row.ObjectKey, row.DeclaredSize); err != nil {
			return fmt.Errorf("scanning upload %s: %w", id, err)
		}
		if f.status == "" {
			if c.clock.Now().Sub(*row.CompletedAt) < MissingAfter {
				out = Result{Status: StatusPending, Waiting: true}
				return nil
			}
			f = finding{status: StatusRejected, reason: reasonMissing}
		}
		out, err = c.finish(ctx, tx, org, id, f)
		return err
	})
	if err != nil {
		return Result{}, err
	}
	switch {
	case out.Waiting:
		c.log.WarnContext(ctx, "upload not scanned: it was never completed or its file is not in storage yet",
			"organization", org, "upload", id)
	case out.Changed && f.status != StatusClean:
		// The signature names the malware, not the file's content; the key is left out.
		c.log.WarnContext(ctx, "upload not clean; its file, if any, is quarantined", "organization", org, "upload", id,
			"status", f.status, "reason", f.reason, "signature", f.signature)
	}
	return out, nil
}

// finish writes the verdict and, for a file that is not clean, its audit row, in tx.
func (c *Checker) finish(ctx context.Context, tx pgx.Tx, org, id uuid.UUID, f finding) (Result, error) {
	q := dbgen.New(tx)
	var ct *string
	if f.contentType != "" {
		ct = &f.contentType
	}
	n, err := q.FinishUploadScan(ctx, dbgen.FinishUploadScanParams{
		OrganizationID: org, ID: id, Status: f.status, ContentType: ct, ScannedAt: c.clock.Now(),
	})
	if err != nil {
		return Result{}, fmt.Errorf("recording the scan of upload %s: %w", id, err)
	}
	if n == 0 {
		// Cannot happen under the row lock; the guard on pending keeps it right regardless.
		row, err := getUpload(ctx, q, org, id)
		return Result{Status: row.Status}, err
	}
	if f.status != StatusClean {
		if err := c.audit.RecordSystemAction(ctx, tx, org, auditScanAction, authz.Resource{Type: "upload_" + f.status, ID: id.String()}); err != nil {
			return Result{}, err
		}
	}
	return Result{Status: f.status, Changed: true}, nil
}

// Stale returns the organization's uploads that were completed more than StaleAfter ago and
// are still pending, oldest first: their scan was lost, failed every retry, or waits for a
// file. Uploads never completed are not listed: their PUT URL expires after PutURLTTL and they
// stay pending, never readable, with any file left in incoming/ for the bucket's lifecycle rule
// to expire.
func (c *Checker) Stale(ctx context.Context, org uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := c.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		ids, err = dbgen.New(tx).ListStaleUploads(ctx, dbgen.ListStaleUploadsParams{
			OrganizationID: org, Before: c.clock.Now().Add(-StaleAfter), RowLimit: staleLimit,
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("listing stale uploads: %w", err)
	}
	return ids, nil
}

// finding is what inspect decided about a file. An empty status means the file is not in
// storage.
type finding struct {
	status      string
	contentType string // the sniffed type, when the content was read
	reason      string // why it was rejected
	signature   string // what the scanner found
}

// inspect checks the upload with key and moves a bad file to quarantine. It reads its own copy
// (StoredKey), moving the file there from IncomingKey first, so a later PUT cannot change what
// was scanned. It is safe to run again after any failure: a file already in quarantine is
// checked there and stays there, and one already moved to StoredKey is read from there.
func inspect(ctx context.Context, objs Objects, sc Scanner, key string, declared int64) (finding, error) {
	src, size, quarantined, err := locate(ctx, objs, key)
	if err != nil || src == "" {
		return finding{}, err
	}
	bad := func(f finding) (finding, error) {
		if !quarantined {
			if err := objs.Move(ctx, src, QuarantineKey(key)); err != nil {
				return finding{}, fmt.Errorf("moving the file to quarantine: %w", err)
			}
		}
		return f, nil
	}
	if size > MaxSize {
		return bad(finding{status: StatusRejected, reason: reasonTooLarge})
	}
	if size != declared {
		return bad(finding{status: StatusRejected, reason: reasonSizeMismatch})
	}

	body, err := objs.Open(ctx, src)
	if err != nil {
		return finding{}, fmt.Errorf("opening the file: %w", err)
	}
	defer func() { _ = body.Close() }() // read only; a close error changes nothing
	head := make([]byte, sniffLen)
	n, err := io.ReadFull(body, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return finding{}, fmt.Errorf("reading the file: %w", err)
	}
	head = head[:n]
	ct := Sniff(head)
	// Every file is scanned, an image or not, so malware is reported as infected (the stronger
	// flag) even when it is not an image, such as the EICAR test file sent as a .jpg.
	v, err := sc.Scan(ctx, io.MultiReader(bytes.NewReader(head), io.LimitReader(body, MaxSize-int64(n))))
	if err != nil {
		return finding{}, fmt.Errorf("scanning the file: %w", err)
	}
	if !v.Clean {
		return bad(finding{status: StatusInfected, contentType: ct, signature: v.Signature})
	}
	if !AllowedType(ct) {
		return bad(finding{status: StatusRejected, contentType: ct, reason: reasonNotImage})
	}
	if quarantined {
		// An earlier run quarantined it and stopped before recording why; a file once quarantined
		// is never served, whatever the scanner says now.
		return finding{status: StatusRejected, contentType: ct, reason: reasonQuarantined}, nil
	}
	return finding{status: StatusClean, contentType: ct}, nil
}

// locate finds the upload's file: in quarantine (an earlier run moved it), at StoredKey, or at
// IncomingKey, which it moves to StoredKey first. It returns the key to read, its size and
// whether it is in quarantine, or an empty key when there is no file.
func locate(ctx context.Context, objs Objects, key string) (src string, size int64, quarantined bool, err error) {
	for _, k := range []string{QuarantineKey(key), StoredKey(key)} {
		size, err := objs.Head(ctx, k)
		if err == nil {
			return k, size, k == QuarantineKey(key), nil
		}
		if !errors.Is(err, ErrObjectNotFound) {
			return "", 0, false, fmt.Errorf("looking for the file: %w", err)
		}
	}
	err = objs.Move(ctx, IncomingKey(key), StoredKey(key))
	if errors.Is(err, ErrObjectNotFound) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("taking the uploaded file: %w", err)
	}
	size, err = objs.Head(ctx, StoredKey(key))
	if err != nil {
		return "", 0, false, fmt.Errorf("looking for the file: %w", err)
	}
	return StoredKey(key), size, false, nil
}

// Sniff returns the content type found in the first bytes of a file (at most 512 are read), as
// http.DetectContentType names it, without parameters.
func Sniff(head []byte) string {
	ct := http.DetectContentType(head)
	if i := bytes.IndexByte([]byte(ct), ';'); i >= 0 {
		ct = ct[:i]
	}
	return ct
}
