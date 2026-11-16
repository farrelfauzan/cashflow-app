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
	"github.com/jackc/pgx/v5/pgconn"

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
	// Created is false when the call repeated an earlier one's requestId and stored nothing.
	Created bool
}

// proofRequestUnique is the index that keeps one proof per requestId in an organization.
const proofRequestUnique = "transfer_proof_request_id_unique"

// errProofRaced means another call with the same requestId recorded its proof first.
var errProofRaced = errors.New("upload: another call recorded this request's proof")

// Send stores a transfer proof photo the tenant behind token sent, and queues its scan. The token
// grants it (payment:proof, CLAUDE.md "Not RBAC principals"); there is no principal to authorize.
// A malformed, unknown or revoked token is paylink.ErrLinkNotFound and an expired one
// paylink.ErrLinkExpired, as for the page; both are checked before open is called, so only the
// holder of a working link can make us read any of the request body or store anything. open
// returns the file (for HTTP, the multipart part: reading up to it reads the body); its error is
// returned as it is.
//
// The file is read into memory, at most MaxSize bytes (ErrTooLarge above it, an InputError when
// empty), and must be a JPEG, PNG or WebP by its content (ErrTypeNotAllowed); the scan checks the
// stored copy again, as for staff photos. At most InFlight proofs are held at once; a request
// waits for a free slot until its context ends, and a slow sender holds its slot until the
// request's deadline. The file is stored at its incoming key, then the
// upload (pending, already completed) and the proof are recorded together, and the scan queued
// after the commit (the reconcile covers a lost enqueue). Should recording fail after the file
// was stored, the file stays in incoming/ and is never read, as for an abandoned staff upload.
//
// requestID, when not uuid.Nil, makes a retry safe: a call that repeats a requestId already sent
// from the same link answers that proof with Created false, without reading the file or storing
// anything, and one already sent from another link is ErrRequestIDReused. Two calls racing with
// one requestId store one proof; the other's file stays in incoming/, as above.
func (s *Proofs) Send(ctx context.Context, token string, requestID uuid.UUID, open func() (io.Reader, error)) (Proof, error) {
	org, linkID, err := s.links.Resolve(ctx, token)
	if err != nil {
		return Proof{}, err
	}
	var earlier *Proof
	if err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		if _, _, err := s.links.OpenLink(ctx, tx, org, linkID); err != nil {
			return err
		}
		var err error
		earlier, err = sentProof(ctx, dbgen.New(tx), org, linkID, requestID)
		return err
	}); err != nil {
		return Proof{}, err
	}
	if earlier != nil {
		return *earlier, nil
	}

	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return Proof{}, fmt.Errorf("waiting to receive a transfer proof: %w", ctx.Err())
	}
	defer func() { <-s.slots }()

	file, err := open()
	if err != nil {
		return Proof{}, err
	}
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
			UploadID: uploadID, CreatedAt: now, RequestID: uuid.NullUUID{UUID: requestID, Valid: requestID != uuid.Nil},
		})
		if isUniqueViolation(err, proofRequestUnique) {
			return errProofRaced
		}
		if err != nil {
			return fmt.Errorf("recording transfer proof: %w", err)
		}
		out = proofFromRow(row, true)
		return nil
	})
	if errors.Is(err, errProofRaced) {
		return s.raced(ctx, org, linkID, requestID)
	}
	if err != nil {
		return Proof{}, err
	}
	enqueueScan(ctx, s.queue, s.log, org, uploadID)
	return out, nil
}

// raced answers the proof another call with requestID recorded while this one stored its file.
func (s *Proofs) raced(ctx context.Context, org, linkID, requestID uuid.UUID) (Proof, error) {
	var earlier *Proof
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		earlier, err = sentProof(ctx, dbgen.New(tx), org, linkID, requestID)
		return err
	})
	if err != nil {
		return Proof{}, err
	}
	if earlier == nil {
		// The index refused the insert, so a proof with this requestId committed first.
		return Proof{}, fmt.Errorf("transfer proof of request %s not found after its insert was refused", requestID)
	}
	return *earlier, nil
}

// sentProof returns the proof an earlier call sent under requestID from link linkID, nil when
// there is none (or requestID is uuid.Nil), and ErrRequestIDReused when it was sent from another
// link.
func sentProof(ctx context.Context, q *dbgen.Queries, org, linkID, requestID uuid.UUID) (*Proof, error) {
	if requestID == uuid.Nil {
		return nil, nil
	}
	row, err := q.GetTransferProofByRequest(ctx, dbgen.GetTransferProofByRequestParams{OrganizationID: org, RequestID: requestID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading the transfer proof of request %s: %w", requestID, err)
	}
	if !row.PaymentLinkID.Valid || row.PaymentLinkID.UUID != linkID {
		return nil, ErrRequestIDReused
	}
	p := proofFromRow(row, false)
	return &p, nil
}

func proofFromRow(row dbgen.TransferProof, created bool) Proof {
	return Proof{ID: row.ID, TenantID: row.TenantID, InvoiceID: row.InvoiceID.UUID, UploadID: row.UploadID,
		CreatedAt: row.CreatedAt, Created: created}
}

func isUniqueViolation(err error, index string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == index
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
