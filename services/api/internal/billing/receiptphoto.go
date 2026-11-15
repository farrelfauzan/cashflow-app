package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

// PhotoMissing is the status of a cash payment's photo before one is attached ("foto belum
// ada"). Once attached, the photo's status is its upload's: upload.StatusPending while it is
// scanned, then clean, rejected or infected.
const PhotoMissing = "missing"

var (
	// ErrReceiptPhotoAttached means the cash payment already has a photo the scan did not refuse.
	ErrReceiptPhotoAttached = errors.New("billing: the cash payment already has a photo")
	// ErrReceiptPhotoInvalid means the upload cannot be the payment's photo: it is not a receipt
	// photo for the payment's tenant made by the caller, it was never completed, or it is
	// already another payment's photo.
	ErrReceiptPhotoInvalid = errors.New("billing: the upload cannot be this payment's photo")
	// ErrReceiptPhotoRefused means the scan refused the upload (rejected or infected): take
	// another photo.
	ErrReceiptPhotoRefused = errors.New("billing: the upload was refused by the scan")
)

// ReceiptPhoto is the photo of a cash payment: its upload, if any, and its status (PhotoMissing
// or the upload's status).
type ReceiptPhoto struct {
	UploadID uuid.UUID // uuid.Nil when Status is PhotoMissing
	Status   string
}

// photoRefused reports whether the scan refused an upload with this status.
func photoRefused(status string) bool {
	return status == upload.StatusRejected || status == upload.StatusInfected
}

// AttachReceiptPhoto attaches upload uploadID as the photo of cash payment paymentID ("foto
// belum ada" until then, decision 46.10). Only the member who recorded the payment attaches its
// photo, with cash:record on the payment's tenant and amount through each of its active leases
// (decision 47), as when they recorded it (audited); anyone else is denied, audited. The upload must be
// a completed cash_receipt upload for the payment's tenant made by the caller and not already
// another payment's photo (ErrReceiptPhotoInvalid), and not refused by the scan
// (ErrReceiptPhotoRefused); a pending one is accepted and its scan decides later.
//
// Links are append-only and a payment's photo is its latest one. A payment that has a photo the
// scan did not refuse takes no other (ErrReceiptPhotoAttached); attaching the same upload again
// changes nothing (its transaction, audit row included, is rolled back) and answers the photo with
// changed false. A payment that is not a field cash
// payment, or not in the organization, is a *NotFoundError with no field; an upload not in the
// organization is one on uploadId.
func (s *Service) AttachReceiptPhoto(ctx context.Context, p authz.Principal, paymentID, uploadID uuid.UUID) (photo ReceiptPhoto, changed bool, err error) {
	org := p.OrganizationID
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		pay, err := q.LockCashPaymentForPhoto(ctx, dbgen.LockCashPaymentForPhotoParams{OrganizationID: org, PaymentID: paymentID})
		if errors.Is(err, pgx.ErrNoRows) {
			return &NotFoundError{}
		}
		if err != nil {
			return fmt.Errorf("locking the cash payment: %w", err)
		}
		leases, err := fieldLeases(ctx, q, org, pay.TenantID, "")
		if err != nil {
			return err
		}
		amount := pay.Amount
		req := authz.Request{Action: authz.ActionCashRecord, Resource: authz.Resource{
			Type: "tenant", ID: pay.TenantID.String(), Amount: &amount,
		}}
		if pay.RecordedByStaffUserID.UUID != p.UserID {
			return s.authz.Deny(ctx, p, req)
		}
		if err := s.authorizeThroughLeases(ctx, tx, p, req, leases); err != nil {
			return err
		}

		links, err := q.ListPaymentReceiptPhotoLinks(ctx, dbgen.ListPaymentReceiptPhotoLinksParams{OrganizationID: org, PaymentID: paymentID})
		if err != nil {
			return fmt.Errorf("listing the cash payment's photos: %w", err)
		}
		for _, l := range links {
			if l.UploadID == uploadID {
				// Rolled back, like a repeated payment, so a repeat leaves no audit row.
				photo = ReceiptPhoto{UploadID: links[0].UploadID, Status: links[0].Status}
				return errRepeat
			}
		}
		if len(links) > 0 && !photoRefused(links[0].Status) {
			return ErrReceiptPhotoAttached
		}

		up, err := q.GetUpload(ctx, dbgen.GetUploadParams{OrganizationID: org, ID: uploadID})
		if errors.Is(err, pgx.ErrNoRows) {
			return &NotFoundError{Field: "uploadId"}
		}
		if err != nil {
			return fmt.Errorf("loading the upload: %w", err)
		}
		if up.Purpose != upload.PurposeCashReceipt || up.TenantID.UUID != pay.TenantID ||
			up.UploadedByStaffUserID != p.UserID || up.CompletedAt == nil {
			return ErrReceiptPhotoInvalid
		}
		if photoRefused(up.Status) {
			return ErrReceiptPhotoRefused
		}
		n, err := q.InsertPaymentReceiptPhoto(ctx, dbgen.InsertPaymentReceiptPhotoParams{
			UploadID: uploadID, OrganizationID: org, PaymentID: paymentID, TenantID: pay.TenantID,
			AttachedByStaffUserID: p.UserID, AttachedAt: s.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("linking the cash payment's photo: %w", err)
		}
		if n == 0 {
			// Another payment's photo: this one was not among this payment's links.
			return ErrReceiptPhotoInvalid
		}
		photo, changed = ReceiptPhoto{UploadID: uploadID, Status: up.Status}, true
		return nil
	})
	if errors.Is(err, errRepeat) {
		return photo, false, nil
	}
	if err != nil {
		return ReceiptPhoto{}, false, err
	}
	return photo, changed, nil
}

// ReceiptPhotosTx returns the photo of each of the given payments inside tx scoped to org; a
// payment with none is PhotoMissing. It checks no permission: the caller has.
func ReceiptPhotosTx(ctx context.Context, tx pgx.Tx, org uuid.UUID, paymentIDs []uuid.UUID) (map[uuid.UUID]ReceiptPhoto, error) {
	return currentReceiptPhotos(ctx, dbgen.New(tx), org, paymentIDs)
}

// currentReceiptPhotos returns the photo of each of the given payments; a payment with none is
// PhotoMissing.
func currentReceiptPhotos(ctx context.Context, q *dbgen.Queries, org uuid.UUID, paymentIDs []uuid.UUID) (map[uuid.UUID]ReceiptPhoto, error) {
	out := make(map[uuid.UUID]ReceiptPhoto, len(paymentIDs))
	for _, id := range paymentIDs {
		out[id] = ReceiptPhoto{Status: PhotoMissing}
	}
	if len(paymentIDs) == 0 {
		return out, nil
	}
	rows, err := q.ListCurrentReceiptPhotos(ctx, dbgen.ListCurrentReceiptPhotosParams{OrganizationID: org, PaymentIds: paymentIDs})
	if err != nil {
		return nil, fmt.Errorf("listing the cash payments' photos: %w", err)
	}
	for _, r := range rows {
		out[r.PaymentID] = ReceiptPhoto{UploadID: r.UploadID, Status: r.Status}
	}
	return out, nil
}
