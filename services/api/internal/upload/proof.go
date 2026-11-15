package upload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// DefaultProofsInFlight is how many transfer proofs one API process holds in memory at once.
const DefaultProofsInFlight = 16

// LinkOpener finds and opens the payment link behind a token. paylink.Service implements it.
type LinkOpener interface {
	// Resolve finds the organization and link of token, with no organization scope.
	Resolve(ctx context.Context, token string) (org, linkID uuid.UUID, err error)
	// OpenLink checks inside tx that the link still works and returns its tenant and invoice.
	OpenLink(ctx context.Context, tx pgx.Tx, org, linkID uuid.UUID) (tenantID, invoiceID uuid.UUID, err error)
}

// ProofsDeps is what the payment page's proof upload needs.
type ProofsDeps struct {
	Store   *store.Store
	Links   LinkOpener
	Objects ObjectWriter
	Queue   ScanQueue
	Clock   clock.Clock
	Log     *slog.Logger
	// InFlight bounds how many proofs are held in memory at once; 0 means DefaultProofsInFlight.
	InFlight int
}

// Proofs receives the tenant's transfer proof photos from the payment page (S6-5, decision 46.8).
type Proofs struct {
	store   *store.Store
	links   LinkOpener
	objects ObjectWriter
	queue   ScanQueue
	clock   clock.Clock
	log     *slog.Logger
	slots   chan struct{}
}

// NewProofs returns a Proofs.
func NewProofs(d ProofsDeps) *Proofs {
	n := d.InFlight
	if n <= 0 {
		n = DefaultProofsInFlight
	}
	return &Proofs{
		store: d.Store, links: d.Links, objects: d.Objects, queue: d.Queue, clock: d.Clock, log: d.Log,
		slots: make(chan struct{}, n),
	}
}

// Proof is a transfer proof a tenant sent.
type Proof struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	InvoiceID uuid.UUID // the invoice of the link it was sent from
	UploadID  uuid.UUID
	CreatedAt time.Time
}

// Send stores a transfer proof photo the tenant behind token sent, and queues its scan. The token
// grants it (payment:proof, CLAUDE.md "Not RBAC principals"); there is no principal to authorize.
// A malformed, unknown or revoked token is paylink.ErrLinkNotFound and an expired one
// paylink.ErrLinkExpired, as for the page; both are checked before the file is read, so only the
// holder of a working link can make us read or store anything.
//
// The file is read into memory, at most MaxSize bytes (ErrTooLarge above it, an InputError when
// empty), and must be a JPEG, PNG or WebP by its content (ErrTypeNotAllowed); the scan checks the
// stored copy again, as for staff photos. At most InFlight proofs are held at once; a request
// waits for a free slot until its context ends. The file is stored at its incoming key, then the
// upload (pending, already completed) and the proof are recorded together, and the scan queued
// after the commit (the reconcile covers a lost enqueue). Should recording fail after the file
// was stored, the file stays in incoming/ and is never read, as for an abandoned staff upload.
func (s *Proofs) Send(ctx context.Context, token string, file io.Reader) (Proof, error) {
	org, linkID, err := s.links.Resolve(ctx, token)
	if err != nil {
		return Proof{}, err
	}
	if err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		_, _, err := s.links.OpenLink(ctx, tx, org, linkID)
		return err
	}); err != nil {
		return Proof{}, err
	}

	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return Proof{}, fmt.Errorf("waiting to receive a transfer proof: %w", ctx.Err())
	}
	defer func() { <-s.slots }()

	body, contentType, err := readProof(file)
	if err != nil {
		return Proof{}, err
	}
	id, uploadID, key, now := uuid.New(), uuid.New(), newObjectKey(org), s.clock.Now()
	if err := s.objects.PutObject(ctx, IncomingKey(key), contentType, body); err != nil {
		return Proof{}, fmt.Errorf("storing transfer proof upload %s: %w", uploadID, err)
	}

	var out Proof
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		tenantID, invoiceID, err := s.links.OpenLink(ctx, tx, org, linkID)
		if err != nil {
			return err
		}
		q := dbgen.New(tx)
		if _, err := q.InsertTransferProofUpload(ctx, dbgen.InsertTransferProofUploadParams{
			ID: uploadID, OrganizationID: org, TenantID: tenantID, ObjectKey: key,
			DeclaredSize: int64(len(body)), CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("recording transfer proof upload: %w", err)
		}
		row, err := q.InsertTransferProof(ctx, dbgen.InsertTransferProofParams{
			ID: id, OrganizationID: org, TenantID: tenantID, PaymentLinkID: linkID, InvoiceID: invoiceID,
			UploadID: uploadID, CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("recording transfer proof: %w", err)
		}
		out = Proof{ID: row.ID, TenantID: row.TenantID, InvoiceID: row.InvoiceID, UploadID: row.UploadID, CreatedAt: row.CreatedAt}
		return nil
	})
	if err != nil {
		return Proof{}, err
	}
	enqueueScan(ctx, s.queue, s.log, org, uploadID)
	return out, nil
}

// readProof reads a proof file of at most MaxSize bytes and returns it with its type, sniffed
// from the content.
func readProof(file io.Reader) ([]byte, string, error) {
	body, err := io.ReadAll(io.LimitReader(file, MaxSize+1))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, "", ErrTooLarge
		}
		return nil, "", fmt.Errorf("reading a transfer proof: %w", err)
	}
	switch {
	case len(body) == 0:
		return nil, "", &InputError{Field: "file", Message: "Pilih foto bukti transfer."}
	case int64(len(body)) > MaxSize:
		return nil, "", ErrTooLarge
	}
	contentType := http.DetectContentType(body[:min(len(body), sniffLen)])
	if !AllowedType(contentType) {
		return nil, "", ErrTypeNotAllowed
	}
	return body, contentType, nil
}
