//go:build integration

package checkqueue_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/checkqueue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

// staffProof has p send body as a transfer proof for tenantID through upload.Service (decision
// 49) and returns the proof's and the upload's ids. Its photo is still pending.
func (w *world) staffProof(t *testing.T, p authz.Principal, tenantID uuid.UUID, body []byte) (proofID, uploadID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	c, err := w.uploads.Create(ctx, p, upload.CreateInput{
		Purpose: upload.PurposeTransferProof, TenantID: tenantID, ContentType: "image/jpeg", Size: int64(len(body)),
	})
	require.NoError(t, err)
	u, err := url.Parse(c.Put.URL)
	require.NoError(t, err)
	w.storage.Put(strings.TrimPrefix(u.Path, "/"), body)
	_, err = w.uploads.Complete(ctx, p, c.Upload.ID)
	require.NoError(t, err)
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT id FROM transfer_proof WHERE upload_id = $1", c.Upload.ID).Scan(&proofID)
	})
	return proofID, c.Upload.ID
}

// itemOf finds the item with id in the queue as the admin sees it, filtered by kind.
func (w *world) itemOf(t *testing.T, kind string, id uuid.UUID) (checkqueue.Item, bool) {
	t.Helper()
	for _, it := range w.list(t, w.admin, kind, listing.Page{Number: 1, Size: listing.MaxPageSize}).Items {
		if it.ID == id {
			return it, true
		}
	}
	return checkqueue.Item{}, false
}

// TestStaffProofOnTheQueue covers a proof a member sent for the tenant (decision 49) in Antrean
// cek and on the tenant's payment page: who sent it, its scan, Match and Dismiss.
func TestStaffProofOnTheQueue(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(2026, 11, 14, 9, 0))
	w := newWorld(t, env, "Griya Staf Antre", "0812567")
	// Rina has rented since 14 October: two invoices of Rp700.000.
	rina := w.rent(t, "Rina Wulandari", day(2026, 10, 14), w.karyawan.MembershipID)
	invoices := w.account(t, rina.tenantID).Invoices
	require.Len(t, invoices, 2)

	linkProof := w.sendProof(t, rina.tenantID, cleanPhoto)
	env.Clock.Advance(time.Minute)
	byKaryawan, byKaryawanUpload := w.staffProof(t, w.karyawan, rina.tenantID, cleanPhoto)
	env.Clock.Advance(time.Minute)
	byAdmin, byAdminUpload := w.staffProof(t, w.admin, rina.tenantID, cleanPhoto)

	t.Run("a staff proof names who sent it and no invoice; a link proof names its invoice and no sender", func(t *testing.T) {
		tests := []struct {
			name        string
			id          uuid.UUID
			uploadID    uuid.UUID
			wantSentBy  *checkqueue.StaffRef
			wantInvoice uuid.UUID
		}{
			{"sent from the payment link", linkProof.ID, linkProof.UploadID, nil, linkProof.InvoiceID},
			{"sent by the karyawan", byKaryawan, byKaryawanUpload, &checkqueue.StaffRef{ID: w.org.Karyawan.UserID, Name: w.org.Karyawan.Name}, uuid.Nil},
			{"sent by the admin", byAdmin, byAdminUpload, &checkqueue.StaffRef{ID: w.org.Admin.UserID, Name: w.org.Admin.Name}, uuid.Nil},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				it, ok := w.itemOf(t, checkqueue.KindTransferProof, tc.id)
				require.True(t, ok, "on the queue")
				require.Equal(t, checkqueue.KindTransferProof, it.Kind)
				tp := it.TransferProof
				require.NotNil(t, tp)
				require.Equal(t, rina.tenantID, tp.Tenant.ID)
				require.Equal(t, "Rina Wulandari", tp.Tenant.Name)
				require.Equal(t, tc.wantSentBy, tp.SentBy)
				require.Equal(t, tc.wantInvoice, tp.InvoiceID)
				require.Equal(t, tc.uploadID, tp.UploadID)
				require.Equal(t, upload.StatusPending, tp.UploadStatus)
			})
		}
		require.NotEqual(t, uuid.Nil, linkProof.InvoiceID)
	})

	t.Run("the payment page lists a staff proof while pending and once clean", func(t *testing.T) {
		require.Subset(t, w.proofsWaiting(t, rina.tenantID), []uuid.UUID{linkProof.ID, byKaryawan, byAdmin})
		w.scan(t, byKaryawanUpload, upload.StatusClean)
		w.scan(t, byAdminUpload, upload.StatusClean)
		w.scan(t, linkProof.UploadID, upload.StatusClean)
		require.Subset(t, w.proofsWaiting(t, rina.tenantID), []uuid.UUID{linkProof.ID, byKaryawan, byAdmin})
		it, ok := w.itemOf(t, checkqueue.KindTransferProof, byKaryawan)
		require.True(t, ok)
		require.Equal(t, upload.StatusClean, it.TransferProof.UploadStatus)
	})

	t.Run("Match records the payment FIFO as for a link proof, and the proof leaves the queue and the page", func(t *testing.T) {
		before := w.payments(t)
		res, pay, err := w.queue.Match(ctx, w.admin, byKaryawan, w.matchInput(1_000_000, "Transfer BRI, foto dari Mas Dimas"))
		require.NoError(t, err)
		require.True(t, pay.Created)
		require.Equal(t, before+1, w.payments(t))
		require.Equal(t, billing.MethodTransfer, pay.Payment.Method)
		require.Equal(t, int64(1_000_000), pay.Payment.Amount)
		require.Len(t, pay.Paid, 2)
		require.Equal(t, invoices[0].ID, pay.Paid[0].InvoiceID)
		require.Equal(t, int64(700_000), pay.Paid[0].Amount)
		require.Equal(t, invoices[1].ID, pay.Paid[1].InvoiceID)
		require.Equal(t, int64(300_000), pay.Paid[1].Amount)
		require.Equal(t, int64(400_000), pay.Outstanding)
		require.Equal(t, checkqueue.KindTransferProof, res.Kind)
		require.Equal(t, byKaryawan, res.ItemID)
		require.Equal(t, checkqueue.OutcomeMatched, res.Outcome)
		require.Equal(t, pay.Payment.ID, res.PaymentID)

		require.NotContains(t, w.waiting(t), byKaryawan)
		require.NotContains(t, w.proofsWaiting(t, rina.tenantID), byKaryawan)
		require.Equal(t, 1, w.audited(t, w.org.Admin.UserID, authz.ActionPaymentCheck, "transfer_proof/"+byKaryawan.String(), "allow"))

		_, _, err = w.queue.Match(ctx, w.admin, byKaryawan, w.matchInput(1_000_000, ""))
		require.ErrorIs(t, err, checkqueue.ErrResolved)
	})

	t.Run("the karyawan who sent it cannot match it", func(t *testing.T) {
		before := w.payments(t)
		_, _, err := w.queue.Match(ctx, w.karyawan, byAdmin, w.matchInput(400_000, ""))
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, before, w.payments(t))
		require.Contains(t, w.waiting(t), byAdmin)
	})

	t.Run("Dismiss takes a staff proof off the queue and the page", func(t *testing.T) {
		_, err := w.queue.Dismiss(ctx, w.admin, checkqueue.KindTransferProof, byAdmin, "Foto dobel")
		require.NoError(t, err)
		require.NotContains(t, w.waiting(t), byAdmin)
		require.NotContains(t, w.proofsWaiting(t, rina.tenantID), byAdmin)
		require.Contains(t, w.proofsWaiting(t, rina.tenantID), linkProof.ID, "the link proof still waits")
	})

	t.Run("an infected staff proof is an infected upload with its uploader, not a proof", func(t *testing.T) {
		id, uploadID := w.staffProof(t, w.karyawan, rina.tenantID, infectedPhoto)
		w.scan(t, uploadID, upload.StatusInfected)
		_, ok := w.itemOf(t, checkqueue.KindTransferProof, id)
		require.False(t, ok, "not a proof to match")
		require.NotContains(t, w.proofsWaiting(t, rina.tenantID), id)
		it, ok := w.itemOf(t, checkqueue.KindUploadInfected, uploadID)
		require.True(t, ok)
		inf := it.UploadInfected
		require.NotNil(t, inf)
		require.Equal(t, upload.PurposeTransferProof, inf.Purpose)
		require.NotNil(t, inf.Tenant)
		require.Equal(t, rina.tenantID, inf.Tenant.ID)
		require.Equal(t, &checkqueue.StaffRef{ID: w.org.Karyawan.UserID, Name: w.org.Karyawan.Name}, inf.UploadedBy)

		before := w.payments(t)
		_, _, err := w.queue.Match(ctx, w.admin, id, w.matchInput(100_000, ""))
		require.Error(t, err)
		require.Equal(t, before, w.payments(t))
	})

	t.Run("a rejected staff proof is listed nowhere", func(t *testing.T) {
		id, uploadID := w.staffProof(t, w.karyawan, rina.tenantID, cleanPhoto)
		var key string
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT object_key FROM upload WHERE id = $1", uploadID).Scan(&key)
		})
		w.storage.Put(upload.IncomingKey(key), pdfInPlace)
		w.scan(t, uploadID, upload.StatusRejected)
		waiting := w.waiting(t)
		require.NotContains(t, waiting, id)
		require.NotContains(t, waiting, uploadID)
		require.NotContains(t, w.proofsWaiting(t, rina.tenantID), id)
	})
}
