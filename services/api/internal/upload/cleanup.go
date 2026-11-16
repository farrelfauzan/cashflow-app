package upload

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// cleanupBatch bounds how many unused photos one listing returns; a run lists again until a
// listing comes back short.
const cleanupBatch = 200

// Cleaner deletes the files of cash receipt photos that back no payment (upload:cleanup). Since
// decision 50 the karyawan uploads the photo before recording the cash, so a photo of cash that
// was never recorded stays in storage with nothing pointing at it.
type Cleaner struct {
	store   *store.Store
	objects ObjectDeleter
	clock   clock.Clock
	log     *slog.Logger
}

// NewCleaner returns a Cleaner.
func NewCleaner(st *store.Store, objects ObjectDeleter, clk clock.Clock, log *slog.Logger) *Cleaner {
	return &Cleaner{store: st, objects: objects, clock: clk, log: log}
}

// Clean deletes the stored file of every clean cash_receipt upload of org made more than
// UnusedAfter ago that backs no payment, and returns how many it deleted. The upload row stays;
// upload_deletion records that its file is gone, so no payment can link it afterwards. Only clean
// uploads are covered: a rejected or infected file stays in quarantine, and a file never
// completed stays in incoming/ for the bucket's lifecycle rule.
//
// Each upload is handled in its own transaction under its row lock, the one linking a photo to a
// payment takes too, so a photo is either linked or deleted, never both: the deletion is recorded
// only when no payment links the upload, and the file is deleted before that commits. Should the
// commit fail after the delete, the next run records it again (deleting a missing file changes
// nothing). A failure for one upload does not stop the others; they come back joined.
func (c *Cleaner) Clean(ctx context.Context, org uuid.UUID) (int, error) {
	before := c.clock.Now().Add(-UnusedAfter)
	n := 0
	var errs []error
	failed := map[uuid.UUID]bool{}
	for {
		var rows []dbgen.ListUnusedCashReceiptsRow
		err := c.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			var err error
			rows, err = dbgen.New(tx).ListUnusedCashReceipts(ctx, dbgen.ListUnusedCashReceiptsParams{
				OrganizationID: org, Before: before, RowLimit: cleanupBatch,
			})
			return err
		})
		if err != nil {
			return n, errors.Join(append(errs, fmt.Errorf("listing unused cash receipt photos: %w", err))...)
		}
		progress := false
		for _, r := range rows {
			if failed[r.ID] {
				continue
			}
			deleted, err := c.deleteOne(ctx, org, r.ID, r.ObjectKey)
			if err != nil {
				failed[r.ID] = true
				errs = append(errs, err)
				continue
			}
			progress = true
			if deleted {
				n++
			}
		}
		// A short listing was the last; a listing with only failed uploads would come back the same.
		if len(rows) < cleanupBatch || !progress {
			return n, errors.Join(errs...)
		}
	}
}

// deleteOne deletes one upload's file unless a payment linked it meanwhile, and reports whether
// it did.
func (c *Cleaner) deleteOne(ctx context.Context, org, id uuid.UUID, key string) (bool, error) {
	deleted := false
	err := c.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.LockUpload(ctx, dbgen.LockUploadParams{OrganizationID: org, ID: id}); err != nil {
			return fmt.Errorf("locking upload: %w", err)
		}
		rows, err := q.InsertUploadDeletion(ctx, dbgen.InsertUploadDeletionParams{
			UploadID: id, OrganizationID: org, DeletedAt: c.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("recording the deletion: %w", err)
		}
		if rows == 0 {
			return nil // linked to a payment, or deleted, while it waited for the lock
		}
		if err := c.objects.Delete(ctx, StoredKey(key)); err != nil {
			return fmt.Errorf("deleting the file: %w", err)
		}
		deleted = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("upload %s: %w", id, err)
	}
	return deleted, nil
}
