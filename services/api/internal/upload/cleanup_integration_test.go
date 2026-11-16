//go:build integration

package upload_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// newCleaner returns the world's upload:cleanup on its own fake bucket.
func (w *world) newCleaner() *upload.Cleaner {
	return upload.NewCleaner(w.env.Store, w.storage, w.env.Clock, w.env.Log)
}

// deletions is how many upload_deletion rows record upload id.
func (w *world) deletions(t *testing.T, id uuid.UUID) int {
	t.Helper()
	var n int
	err := w.env.Store.WithOrgTx(context.Background(), w.org.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), "SELECT count(*) FROM upload_deletion WHERE upload_id = $1", id).Scan(&n)
	})
	require.NoError(t, err)
	return n
}

// deleteCalls is the fake bucket's delete calls so far.
func (w *world) deleteCalls() []string {
	return slices.DeleteFunc(w.storage.Calls(), func(c string) bool { return !strings.HasPrefix(c, "delete ") })
}

// storedFile reports whether the fake bucket still holds the scanned file of upload id.
func (w *world) storedFile(t *testing.T, id uuid.UUID) bool {
	t.Helper()
	_, ok := w.storage.Object(upload.StoredKey(w.row(t, id).ObjectKey))
	return ok
}

// cashWith records Rp50.000 of cash from Rina by Dimas with photo, and returns the payment.
func (w *world) cashWith(t *testing.T, photo uuid.UUID) uuid.UUID {
	t.Helper()
	res, err := w.env.Billing.RecordCashPayment(context.Background(), w.dimas, billing.CashInput{
		RequestID: uuid.New(), TenantID: w.rina, Amount: 50_000, PhotoUploadID: photo,
	})
	require.NoError(t, err)
	return res.Payment.ID
}

// scannedAs runs the flow for an upload of purpose as Dimas (a receipt for Rina) and returns its
// id once scanned.
func (w *world) scannedAs(t *testing.T, purpose, contentType string, body []byte) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	in := upload.CreateInput{Purpose: purpose, ContentType: contentType, Size: int64(len(body))}
	if purpose == upload.PurposeCashReceipt {
		in.TenantID = w.rina
	}
	c, err := w.uploads.Create(ctx, w.dimas, in)
	require.NoError(t, err)
	w.put(t, c, body)
	_, err = w.uploads.Complete(ctx, w.dimas, c.Upload.ID)
	require.NoError(t, err)
	_, err = w.checker.Scan(ctx, w.org.ID, c.Upload.ID)
	require.NoError(t, err)
	return c.Upload.ID
}

// TestCleanupDeletesUnusedReceiptPhotos: upload:cleanup deletes the file of a clean cash receipt
// photo that backs no payment once it is 30 days old, records the deletion, and leaves every
// other upload alone.
func TestCleanupDeletesUnusedReceiptPhotos(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	w := newProofWorld(t, env, "Griya Bersih", "0812886")
	other := newWorld(t, env, "Kos Bersih Lain", "0812887")
	_, rinaToken := w.mint(t, w.invoice(t, w.rina))
	jpeg := uploadtest.Image("image/jpeg", 4_000)
	cleaner := w.newCleaner()

	// Day 0: the unused photo, and every upload the job must keep.
	unused := w.uploadAndScan(t, jpeg, len(jpeg))
	linked := w.uploadAndScan(t, jpeg, len(jpeg))
	w.cashWith(t, linked)
	rejected := w.scannedAs(t, upload.PurposeCashReceipt, "image/jpeg", uploadtest.Image("text/plain", 300))
	infected := w.scannedAs(t, upload.PurposeCashReceipt, "image/jpeg", []byte(uploadtest.EICAR))
	pendingUp := w.receipt(t, w.dimas, w.rina, len(jpeg))
	w.put(t, pendingUp, jpeg)
	_, err := w.uploads.Complete(ctx, w.dimas, pendingUp.Upload.ID)
	require.NoError(t, err)
	deposit := w.scannedAs(t, upload.PurposeCashDeposit, "image/png", uploadtest.Image("image/png", 900))
	proof := w.send(t, rinaToken, uploadtest.Image("image/jpeg", 1_500))
	_, err = w.checker.Scan(ctx, w.org.ID, proof.UploadID)
	require.NoError(t, err)
	otherUnused := other.uploadAndScan(t, jpeg, len(jpeg))

	// Day 1: a photo that is only 29 days old when the job runs.
	env.Clock.Advance(24 * time.Hour)
	young := w.uploadAndScan(t, jpeg, len(jpeg))

	before, err := w.uploads.Get(ctx, w.dimas, unused)
	require.NoError(t, err)
	require.NotEmpty(t, before.DownloadURL, "a clean photo has a link before the job")

	env.Clock.Advance(upload.UnusedAfter - 24*time.Hour + time.Minute)

	t.Run("a storage failure records nothing, and the next run deletes it", func(t *testing.T) {
		w.storage.Err = uploadtest.ErrDown
		n, err := cleaner.Clean(ctx, w.org.ID)
		w.storage.Err = nil
		require.ErrorIs(t, err, uploadtest.ErrDown)
		require.Zero(t, n)
		require.Zero(t, w.deletions(t, unused), "the deletion rolled back with the failed delete")
		require.True(t, w.storedFile(t, unused))

		calls := len(w.deleteCalls())
		n, err = cleaner.Clean(ctx, w.org.ID)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		require.Equal(t, []string{"delete " + upload.StoredKey(w.row(t, unused).ObjectKey)}, w.deleteCalls()[calls:],
			"only the unused photo's stored file")
		require.False(t, w.storedFile(t, unused))
		require.Equal(t, 1, w.deletions(t, unused))

		var at time.Time
		err = env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT deleted_at FROM upload_deletion WHERE upload_id = $1", unused).Scan(&at)
		})
		require.NoError(t, err)
		require.True(t, at.Equal(env.Clock.Now()), "%s", at)
	})

	t.Run("a second run deletes nothing", func(t *testing.T) {
		calls := len(w.deleteCalls())
		n, err := cleaner.Clean(ctx, w.org.ID)
		require.NoError(t, err)
		require.Zero(t, n)
		require.Len(t, w.deleteCalls(), calls, "no delete call")
		require.Equal(t, 1, w.deletions(t, unused))
	})

	t.Run("a photo whose file was deleted has no download link", func(t *testing.T) {
		got, err := w.uploads.Get(ctx, w.dimas, unused)
		require.NoError(t, err)
		require.Equal(t, upload.StatusClean, got.Status)
		require.Empty(t, got.DownloadURL)
		require.True(t, got.DownloadExpiresAt.IsZero())
	})

	t.Run("every other upload is kept", func(t *testing.T) {
		kept := []struct {
			name string
			id   uuid.UUID
			key  func(objectKey string) string
		}{
			{name: "a photo only 29 days old", id: young, key: upload.StoredKey},
			{name: "a photo linked to a cash payment", id: linked, key: upload.StoredKey},
			{name: "a rejected photo", id: rejected, key: upload.QuarantineKey},
			{name: "an infected photo", id: infected, key: upload.QuarantineKey},
			{name: "a photo still pending", id: pendingUp.Upload.ID, key: upload.IncomingKey},
			{name: "a cash deposit photo", id: deposit, key: upload.StoredKey},
			{name: "a transfer proof", id: proof.UploadID, key: upload.StoredKey},
		}
		deleted := w.deleteCalls()
		for _, tc := range kept {
			t.Run(tc.name, func(t *testing.T) {
				key := tc.key(w.row(t, tc.id).ObjectKey)
				_, ok := w.storage.Object(key)
				require.True(t, ok, "its file %s is still there", key)
				require.NotContains(t, deleted, "delete "+key)
				require.Zero(t, w.deletions(t, tc.id))
			})
		}
	})

	t.Run("one organization's run never touches another's uploads", func(t *testing.T) {
		require.Zero(t, other.deletions(t, otherUnused))
		require.True(t, other.storedFile(t, otherUnused))
		require.Empty(t, other.deleteCalls())

		n, err := other.newCleaner().Clean(ctx, other.org.ID)
		require.NoError(t, err)
		require.Equal(t, 1, n, "its own run deletes it")
		require.False(t, other.storedFile(t, otherUnused))
		require.Equal(t, 1, other.deletions(t, otherUnused))
	})
}

// TestCleanupWaitsForALink: a photo being linked to a payment holds its row lock, so the job
// waits for that transaction and then sees the link: the photo is linked, never deleted.
func TestCleanupWaitsForALink(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	w := newWorld(t, env, "Griya Kunci", "0812888")
	_, err := env.Billing.GenerateOrg(ctx, w.org.ID)
	require.NoError(t, err)
	jpeg := uploadtest.Image("image/jpeg", 4_000)

	// A cash payment recorded with a photo still being scanned; the transaction below links a
	// second photo to it as linkReceiptPhoto does (the service's retake rule is not what this
	// test is about).
	first := w.receipt(t, w.dimas, w.rina, len(jpeg))
	w.put(t, first, jpeg)
	_, err = w.uploads.Complete(ctx, w.dimas, first.Upload.ID)
	require.NoError(t, err)
	payment := w.cashWith(t, first.Upload.ID)
	photo := w.uploadAndScan(t, jpeg, len(jpeg))
	env.Clock.Advance(upload.UnusedAfter + time.Hour)

	type cleaned struct {
		n   int
		err error
	}
	done := make(chan cleaned, 1)
	err = env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		// What linkReceiptPhoto does first.
		if _, err := q.LockUploadShared(ctx, dbgen.LockUploadSharedParams{OrganizationID: w.org.ID, ID: photo}); err != nil {
			return err
		}
		go func() {
			n, err := w.newCleaner().Clean(ctx, w.org.ID)
			done <- cleaned{n, err}
		}()
		testenv.WaitUntilBlockedOn(ctx, t, tx)
		select {
		case r := <-done:
			t.Fatalf("the job finished while the photo was locked: %d, %v", r.n, r.err)
		default:
		}
		_, err := q.InsertPaymentReceiptPhoto(ctx, dbgen.InsertPaymentReceiptPhotoParams{
			UploadID: photo, OrganizationID: w.org.ID, PaymentID: payment, TenantID: w.rina,
			AttachedByStaffUserID: w.dimas.UserID, AttachedAt: env.Clock.Now(),
		})
		return err
	})
	require.NoError(t, err)

	select {
	case r := <-done:
		require.NoError(t, r.err)
		require.Zero(t, r.n)
	case <-time.After(30 * time.Second):
		t.Fatal("the job never finished")
	}
	require.Empty(t, w.deleteCalls())
	require.Zero(t, w.deletions(t, photo))
	require.True(t, w.storedFile(t, photo))
}
