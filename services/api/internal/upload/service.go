package upload

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// enqueueTimeout bounds the enqueue after a completed upload is committed.
const enqueueTimeout = 5 * time.Second

// ScanQueue enqueues an upload's scan. uploadtask.Client implements it on asynq. An upload whose
// scan is already queued counts as enqueued.
type ScanQueue interface {
	EnqueueScan(ctx context.Context, org, id uuid.UUID) error
}

// ServiceDeps is what the API side needs.
type ServiceDeps struct {
	Store     *store.Store
	Authz     *authz.Service
	Clock     clock.Clock
	Presigner Presigner
	Queue     ScanQueue
	Log       *slog.Logger
}

// Service starts, completes and reads uploads for staff members.
type Service struct {
	store   *store.Store
	authz   *authz.Service
	clock   clock.Clock
	presign Presigner
	queue   ScanQueue
	log     *slog.Logger
}

// NewService returns a Service.
func NewService(d ServiceDeps) *Service {
	return &Service{store: d.Store, authz: d.Authz, clock: d.Clock, presign: d.Presigner, queue: d.Queue, log: d.Log}
}

// Created is a started upload and where to PUT its file.
type Created struct {
	Upload    Upload
	Put       PresignedPut
	ExpiresAt time.Time
}

// Read is an upload and, only when it is clean, a download link.
type Read struct {
	Upload
	DownloadURL       string // empty unless clean
	DownloadExpiresAt time.Time
}

// Create starts an upload: it records it pending and answers a presigned PUT for exactly
// in.Size bytes of in.ContentType. The permission is the action of the record it is for, in
// the same transaction: cash:record on the tenant for a cash_receipt, through every lease the
// tenant has or had (in the lease's property, owned by its collector, so a karyawan's "self"
// applies), and cash:deposit on the caller's own upload for a cash_deposit. Both are audited.
func (s *Service) Create(ctx context.Context, p authz.Principal, in CreateInput) (Created, error) {
	in, err := validCreate(in)
	if err != nil {
		return Created{}, err
	}
	org, id, now := p.OrganizationID, uuid.New(), s.clock.Now()
	var row dbgen.Upload
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if in.TenantID != uuid.Nil {
			if _, err := q.GetTenant(ctx, dbgen.GetTenantParams{OrganizationID: org, ID: in.TenantID}); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrTenantNotFound
				}
				return fmt.Errorf("loading tenant: %w", err)
			}
		}
		reqs, err := recordRequests(ctx, q, org, in.Purpose, in.TenantID, id, p.UserID)
		if err != nil {
			return err
		}
		if err := s.authorizeAll(ctx, tx, p, reqs); err != nil {
			return err
		}
		row, err = q.InsertUpload(ctx, dbgen.InsertUploadParams{
			ID: id, OrganizationID: org, Purpose: in.Purpose, TenantID: nullUUID(in.TenantID),
			ObjectKey: newObjectKey(org), DeclaredSize: in.Size, UploadedByStaffUserID: p.UserID, CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("recording upload: %w", err)
		}
		return nil
	})
	if err != nil {
		return Created{}, err
	}
	// Presigning does no I/O. Should it fail, the row stays pending and is never completed, so
	// nothing can read it.
	put, err := s.presign.PresignPut(ctx, IncomingKey(row.ObjectKey), in.ContentType, in.Size, PutURLTTL)
	if err != nil {
		return Created{}, fmt.Errorf("presigning upload %s: %w", id, err)
	}
	return Created{Upload: fromRow(row), Put: put, ExpiresAt: now.Add(PutURLTTL)}, nil
}

// Complete records that the client's PUT finished and queues the scan. Only the member who
// started the upload may complete it, and only while they still have the permission Create
// checked (checked again, and audited, in the same transaction). A second call changes nothing
// and queues the scan again while the upload is pending, which is harmless. The scan is queued
// after the commit; the reconcile covers a lost enqueue.
func (s *Service) Complete(ctx context.Context, p authz.Principal, id uuid.UUID) (Upload, error) {
	org := p.OrganizationID
	var row dbgen.Upload
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		var err error
		if row, err = getUpload(ctx, q, org, id); err != nil {
			return err
		}
		reqs, err := recordRequests(ctx, q, org, row.Purpose, row.TenantID.UUID, row.ID, row.UploadedByStaffUserID)
		if err != nil {
			return err
		}
		if row.UploadedByStaffUserID != p.UserID {
			return s.authz.Deny(ctx, p, reqs[0])
		}
		if err := s.authorizeAll(ctx, tx, p, reqs); err != nil {
			return err
		}
		if _, err := q.MarkUploadCompleted(ctx, dbgen.MarkUploadCompletedParams{
			OrganizationID: org, ID: id, CompletedAt: s.clock.Now(),
		}); err != nil {
			return fmt.Errorf("completing upload: %w", err)
		}
		row, err = getUpload(ctx, q, org, id)
		return err
	})
	if err != nil {
		return Upload{}, err
	}
	if row.Status == StatusPending {
		s.enqueue(ctx, org, id)
	}
	return fromRow(row), nil
}

// enqueue queues an upload's scan once its completion is committed. It does not wait on the
// request's cancellation, since the upload is already completed; a failure is logged and the
// reconcile enqueues the scan later.
func (s *Service) enqueue(ctx context.Context, org, id uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), enqueueTimeout)
	defer cancel()
	if err := s.queue.EnqueueScan(ctx, org, id); err != nil {
		s.log.ErrorContext(ctx, "enqueueing an upload's scan; the reconcile will enqueue it",
			"organization", org, "upload", id, "error", err)
	}
}

// Get returns an upload and, only when it is clean, a download link valid for GetURLTTL that
// downloads the file as an attachment. It is allowed with payment:check (the admin checking it),
// cash:read (the Supervisor, who reads everything) or the action that started the upload, on the
// upload's record (see Create): through every lease of a receipt's tenant, and for a deposit
// photo on the upload itself (payment:check and cash:read then need the whole organization). It
// is a read, so only a denial is audited.
func (s *Service) Get(ctx context.Context, p authz.Principal, id uuid.UUID) (Read, error) {
	org := p.OrganizationID
	var row dbgen.Upload
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		var err error
		if row, err = getUpload(ctx, q, org, id); err != nil {
			return err
		}
		reqs, err := recordRequests(ctx, q, org, row.Purpose, row.TenantID.UUID, row.ID, row.UploadedByStaffUserID)
		if err != nil {
			return err
		}
		if allAllowed(p, withAction(reqs, actionPaymentCheck)) || allAllowed(p, withAction(reqs, actionCashRead)) ||
			allAllowed(p, reqs) {
			return nil
		}
		return s.authz.Deny(ctx, p, reqs[0])
	})
	if err != nil {
		return Read{}, err
	}
	out := Read{Upload: fromRow(row)}
	if row.Status != StatusClean || row.ContentType == nil {
		return out, nil
	}
	now := s.clock.Now()
	url, err := s.presign.PresignGet(ctx, StoredKey(row.ObjectKey), GetURLTTL, downloadName(row.ID, *row.ContentType), *row.ContentType)
	if err != nil {
		return Read{}, fmt.Errorf("presigning download of upload %s: %w", id, err)
	}
	out.DownloadURL, out.DownloadExpiresAt = url, now.Add(GetURLTTL)
	return out, nil
}

// recordRequests returns the permission checks of an upload's record, one per way a member
// reaches it, all of which must pass. A cash_receipt is checked as cash:record on its tenant
// through each lease the tenant has or had (the lease's property, owned by its collector); a
// tenant with no lease only by an organization-wide grant. A cash_deposit is checked as
// cash:deposit on the upload, owned by its uploader.
func recordRequests(ctx context.Context, q *dbgen.Queries, org uuid.UUID, purpose string, tenantID, uploadID, uploader uuid.UUID) ([]authz.Request, error) {
	switch purpose {
	case PurposeCashReceipt:
		base := authz.Request{Action: actionCashRecord, Resource: authz.Resource{Type: "tenant", ID: tenantID.String()}}
		leases, err := q.ListTenantLeaseScopes(ctx, dbgen.ListTenantLeaseScopesParams{OrganizationID: org, TenantID: tenantID})
		if err != nil {
			return nil, fmt.Errorf("loading the tenant's leases: %w", err)
		}
		if len(leases) == 0 {
			return []authz.Request{base}, nil
		}
		out := make([]authz.Request, len(leases))
		for i, l := range leases {
			out[i] = base
			out[i].Resource.PropertyID = l.PropertyID
			out[i].Resource.OwnerID = l.CollectorStaffUserID.UUID
		}
		return out, nil
	case PurposeCashDeposit:
		return []authz.Request{{Action: actionCashDeposit, Resource: authz.Resource{
			Type: "upload", ID: uploadID.String(), OwnerID: uploader,
		}}}, nil
	}
	return nil, fmt.Errorf("upload %s has unknown purpose %q", uploadID, purpose)
}

// authorizeAll allows every one of reqs, auditing as authz.Service.Authorize does.
func (s *Service) authorizeAll(ctx context.Context, tx pgx.Tx, p authz.Principal, reqs []authz.Request) error {
	for _, r := range reqs {
		if err := s.authz.Authorize(ctx, tx, p, r); err != nil {
			return err
		}
	}
	return nil
}

// allAllowed reports whether p may do every one of reqs, without auditing.
func allAllowed(p authz.Principal, reqs []authz.Request) bool {
	for _, r := range reqs {
		if !authz.Allowed(p, r) {
			return false
		}
	}
	return len(reqs) > 0
}

// withAction returns reqs with their action replaced.
func withAction(reqs []authz.Request, action string) []authz.Request {
	out := make([]authz.Request, len(reqs))
	for i, r := range reqs {
		out[i] = r
		out[i].Action = action
	}
	return out
}

func getUpload(ctx context.Context, q *dbgen.Queries, org, id uuid.UUID) (dbgen.Upload, error) {
	row, err := q.GetUpload(ctx, dbgen.GetUploadParams{OrganizationID: org, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Upload{}, ErrNotFound
	}
	if err != nil {
		return dbgen.Upload{}, fmt.Errorf("loading upload: %w", err)
	}
	return row, nil
}

func fromRow(r dbgen.Upload) Upload {
	out := Upload{
		ID: r.ID, Purpose: r.Purpose, TenantID: r.TenantID.UUID, Status: r.Status, Size: r.DeclaredSize,
		UploadedBy: r.UploadedByStaffUserID, CreatedAt: r.CreatedAt, CompletedAt: r.CompletedAt, ScannedAt: r.ScannedAt,
	}
	if r.ContentType != nil {
		out.ContentType = *r.ContentType
	}
	return out
}

func nullUUID(id uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil}
}
