//go:build integration

package billing_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// receiptQueue is a fake billing.ReceiptQueue that records the payments it was given.
type receiptQueue struct {
	mu  sync.Mutex
	ids []uuid.UUID
}

func (q *receiptQueue) EnqueueReceipt(_ context.Context, _, paymentID uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids = append(q.ids, paymentID)
	return nil
}

func (q *receiptQueue) enqueued() []uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]uuid.UUID(nil), q.ids...)
}

// fieldWorld is a world whose billing service enqueues receipts, with the org's karyawan and a
// second karyawan, Dina.
type fieldWorld struct {
	*world
	billing  *billing.Service
	queue    *receiptQueue
	karyawan authz.Principal
	dina     authz.Principal
}

func newFieldWorld(t *testing.T, env *testenv.Env, prefix, dinaPhone string) fieldWorld {
	t.Helper()
	w := fieldWorld{world: newWorld(t, env, prefix), queue: &receiptQueue{}}
	w.billing = billing.NewService(env.Store, env.Authz, env.Clock, env.Log, env.Promises, billing.SendReceipts(w.queue))
	w.karyawan = env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	dina, err := env.Members.Invite(context.Background(), w.admin, member.Invite{Phone: dinaPhone, Name: "Dina Kasir",
		Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)
	w.dina = env.Principal(t, w.org.ID, dina.UserID)
	return w
}

// collected is a tenant renting a new unit in the world's kos from start, at Rp700.000 a month,
// collected by the member collector (uuid.Nil for none). The lease is saved now (the fake clock).
func (w fieldWorld) collected(t *testing.T, name string, start time.Time, collector uuid.UUID) rental {
	t.Helper()
	return meterWorld{world: w.world}.rentCollected(t, w.kos, name, start, collector)
}

// cashIn is cash of amount from r recorded by p, with a fresh requestId and a fresh photo by p
// (decision 50).
func (w *world) cashIn(t *testing.T, p authz.Principal, r rental, amount int64) billing.CashInput {
	t.Helper()
	return billing.CashInput{
		RequestID: uuid.New(), TenantID: r.tenantID, Amount: amount,
		PhotoUploadID: w.env.CashPhoto(t, w.org.ID, p.UserID, r.tenantID),
	}
}

// receiptRows are the organization's receipts by payment.
func (w fieldWorld) receiptRows(t *testing.T) map[uuid.UUID]dbgen.PaymentReceipt {
	t.Helper()
	out := map[uuid.UUID]dbgen.PaymentReceipt{}
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		rows, err := tx.Query(ctx, "SELECT payment_id, organization_id, tenant_id, status, sent_at, created_at FROM payment_receipt")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r dbgen.PaymentReceipt
			if err := rows.Scan(&r.PaymentID, &r.OrganizationID, &r.TenantID, &r.Status, &r.SentAt, &r.CreatedAt); err != nil {
				return err
			}
			out[r.PaymentID] = r
		}
		return rows.Err()
	})
	return out
}

// photos makes receipt photo uploads for a world through the upload service and its scan, with
// the fake bucket and scanner.
type photos struct {
	w       fieldWorld
	storage *uploadtest.Storage
	uploads *upload.Service
	checker *upload.Checker
}

func newPhotos(w fieldWorld) photos {
	env := w.env
	storage := uploadtest.NewStorage()
	return photos{w: w, storage: storage,
		uploads: upload.NewService(upload.ServiceDeps{
			Store: env.Store, Authz: env.Authz, Clock: env.Clock, Presigner: storage, Queue: &uploadtest.Queue{}, Log: env.Log,
		}),
		checker: upload.NewChecker(upload.CheckerDeps{
			Store: env.Store, Objects: storage, Scanner: &uploadtest.Scanner{}, Audit: env.Authz, Clock: env.Clock, Log: env.Log,
		}),
	}
}

// Bodies the fake scanner judges: a JPEG is clean, text is rejected, EICAR is infected.
var (
	cleanPhoto    = uploadtest.Image("image/jpeg", 2_048)
	notAPhoto     = uploadtest.Image("text/plain", 64)
	infectedPhoto = []byte(uploadtest.EICAR)
)

// started starts an upload of body for purpose as p (tenantID uuid.Nil for a cash_deposit)
// and puts the file where its presigned PUT points, without completing it.
func (ph photos) started(t *testing.T, p authz.Principal, purpose string, tenantID uuid.UUID, body []byte) uuid.UUID {
	t.Helper()
	c, err := ph.uploads.Create(context.Background(), p, upload.CreateInput{
		Purpose: purpose, TenantID: tenantID, ContentType: "image/jpeg", Size: int64(len(body)),
	})
	require.NoError(t, err)
	u, err := url.Parse(c.Put.URL)
	require.NoError(t, err)
	ph.storage.Put(strings.TrimPrefix(u.Path, "/"), body)
	return c.Upload.ID
}

// receipt is a completed, not yet scanned, cash_receipt upload of body for r made by p.
func (ph photos) receipt(t *testing.T, p authz.Principal, r rental, body []byte) uuid.UUID {
	t.Helper()
	id := ph.started(t, p, upload.PurposeCashReceipt, r.tenantID, body)
	_, err := ph.uploads.Complete(context.Background(), p, id)
	require.NoError(t, err)
	return id
}

// scan runs the upload's scan and checks it ended in status.
func (ph photos) scan(t *testing.T, id uuid.UUID, status string) {
	t.Helper()
	res, err := ph.checker.Scan(context.Background(), ph.w.org.ID, id)
	require.NoError(t, err)
	require.Equal(t, status, res.Status)
}

// scanned is a receipt upload of body for r made by p, completed and scanned to status.
func (ph photos) scanned(t *testing.T, p authz.Principal, r rental, body []byte, status string) uuid.UUID {
	t.Helper()
	id := ph.receipt(t, p, r, body)
	ph.scan(t, id, status)
	return id
}

// insertedReceipt writes a completed cash_receipt upload row for tenantID made by uploader
// straight to the database: an upload the service would not let that member start.
func (ph photos) insertedReceipt(t *testing.T, uploader, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	w, id := ph.w, uuid.New()
	w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.InsertUpload(ctx, dbgen.InsertUploadParams{
			ID: id, OrganizationID: w.org.ID, Purpose: upload.PurposeCashReceipt,
			TenantID:     uuid.NullUUID{UUID: tenantID, Valid: true},
			ObjectKey:    "uploads/" + w.org.ID.String() + "/" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			DeclaredSize: 2_048, UploadedByStaffUserID: uploader, CreatedAt: w.env.Clock.Now(),
		}); err != nil {
			return err
		}
		_, err := q.MarkUploadCompleted(ctx, dbgen.MarkUploadCompletedParams{CompletedAt: w.env.Clock.Now(), OrganizationID: w.org.ID, ID: id})
		return err
	})
	return id
}

// paid is a cash payment of Rp10.000 from r recorded by the world's karyawan before photos were
// required (decision 50), so with no photo ("foto belum ada"): its rows written as
// RecordCashPayment wrote them then, the payment allocated and its receipt pending.
func (w fieldWorld) paid(t *testing.T, r rental) uuid.UUID {
	t.Helper()
	id, ref := uuid.New(), uuid.NewString()
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
		if err := billing.LockTenant(ctx, q, w.org.ID, r.tenantID); err != nil {
			return err
		}
		if _, err := q.InsertPaymentIfNew(ctx, dbgen.InsertPaymentIfNewParams{
			ID: id, OrganizationID: w.org.ID, TenantID: r.tenantID, Amount: 10_000, Method: billing.MethodCash,
			ReceivedAt: w.env.Clock.Now(), Source: billing.SourceManual, SourceRef: &ref,
			RecordedByStaffUserID: uuid.NullUUID{UUID: w.karyawan.UserID, Valid: true},
		}); err != nil {
			return err
		}
		if _, err := billing.Allocate(ctx, tx, w.org.ID, r.tenantID); err != nil {
			return err
		}
		return q.InsertPaymentReceipt(ctx, dbgen.InsertPaymentReceiptParams{
			PaymentID: id, OrganizationID: w.org.ID, TenantID: r.tenantID, CreatedAt: w.env.Clock.Now(),
		})
	})
	return id
}

func TestAttachReceiptPhoto(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 14, 9, 0)
	w := newFieldWorld(t, env, "0812867", "0812-8670-0099")
	wati := w.collected(t, "Wati Susanti", day(2026, 11, 14), w.karyawan.MembershipID)
	bambang := w.collected(t, "Bambang Wijaya", day(2026, 11, 14), w.karyawan.MembershipID)
	hadi := w.collected(t, "Hadi Saputra", day(2026, 11, 14), w.dina.MembershipID)
	w.generate(t)
	ph := newPhotos(w)
	supervisor := env.Principal(t, w.org.ID, w.org.Supervisor.UserID)

	attach := func(p authz.Principal, paymentID, uploadID uuid.UUID) (billing.ReceiptPhoto, bool, error) {
		return w.billing.AttachReceiptPhoto(ctx, p, paymentID, uploadID)
	}
	pay1 := w.paid(t, wati)
	first := ph.receipt(t, w.karyawan, wati, cleanPhoto)

	t.Run("a completed upload still being scanned attaches as pending", func(t *testing.T) {
		links := w.count(t, "payment_receipt_photo")
		allowed := w.audited(t, authz.ActionCashRecord, "allow")
		photo, changed, err := attach(w.karyawan, pay1, first)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, billing.ReceiptPhoto{UploadID: first, Status: upload.StatusPending}, photo)
		require.Equal(t, links+1, w.count(t, "payment_receipt_photo"))
		require.Equal(t, allowed+1, w.audited(t, authz.ActionCashRecord, "allow"), "cash:record through the tenant's one lease")

		var row struct {
			payment, tenant, by uuid.UUID
			at                  time.Time
		}
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			return tx.QueryRow(ctx, "SELECT payment_id, tenant_id, attached_by_staff_user_id, attached_at FROM payment_receipt_photo WHERE upload_id = $1", first).
				Scan(&row.payment, &row.tenant, &row.by, &row.at)
		})
		require.Equal(t, pay1, row.payment)
		require.Equal(t, wati.tenantID, row.tenant)
		require.Equal(t, w.org.Karyawan.UserID, row.by)
		require.True(t, row.at.Equal(env.Clock.Now()))
	})

	t.Run("the same upload again changes nothing and answers the photo now", func(t *testing.T) {
		links := w.count(t, "payment_receipt_photo")
		allowed := w.audited(t, authz.ActionCashRecord, "allow")
		photo, changed, err := attach(w.karyawan, pay1, first)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, billing.ReceiptPhoto{UploadID: first, Status: upload.StatusPending}, photo)

		ph.scan(t, first, upload.StatusClean)
		photo, changed, err = attach(w.karyawan, pay1, first)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, billing.ReceiptPhoto{UploadID: first, Status: upload.StatusClean}, photo)
		require.Equal(t, links, w.count(t, "payment_receipt_photo"))
		require.Equal(t, allowed, w.audited(t, authz.ActionCashRecord, "allow"), "a repeat audits nothing")
	})

	t.Run("a photo the scan did not refuse cannot be swapped", func(t *testing.T) {
		pay := w.paid(t, wati)
		pending := ph.receipt(t, w.karyawan, wati, cleanPhoto)
		_, _, err := attach(w.karyawan, pay, pending)
		require.NoError(t, err)
		links := w.count(t, "payment_receipt_photo")

		for _, tc := range []struct {
			name   string
			before func(t *testing.T)
		}{
			{name: "while the photo is pending", before: func(*testing.T) {}},
			{name: "once the photo is clean", before: func(t *testing.T) { ph.scan(t, pending, upload.StatusClean) }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tc.before(t)
				other := ph.receipt(t, w.karyawan, wati, cleanPhoto)
				_, _, err := attach(w.karyawan, pay, other)
				require.ErrorIs(t, err, billing.ErrReceiptPhotoAttached)
				require.Equal(t, links, w.count(t, "payment_receipt_photo"))
			})
		}
	})

	t.Run("a photo the scan refused can be retaken, and the new one is the photo", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			body   []byte
			status string
		}{
			{name: "rejected", body: notAPhoto, status: upload.StatusRejected},
			{name: "infected", body: infectedPhoto, status: upload.StatusInfected},
		} {
			t.Run(tc.name, func(t *testing.T) {
				pay := w.paid(t, wati)
				bad := ph.receipt(t, w.karyawan, wati, tc.body)
				photo, _, err := attach(w.karyawan, pay, bad)
				require.NoError(t, err)
				require.Equal(t, upload.StatusPending, photo.Status, "the scan decides later")
				ph.scan(t, bad, tc.status)

				retake := ph.receipt(t, w.karyawan, wati, cleanPhoto)
				photo, changed, err := attach(w.karyawan, pay, retake)
				require.NoError(t, err)
				require.True(t, changed)
				require.Equal(t, billing.ReceiptPhoto{UploadID: retake, Status: upload.StatusPending}, photo)

				photo, changed, err = attach(w.karyawan, pay, bad)
				require.NoError(t, err, "the refused upload is already linked: a repeat")
				require.False(t, changed)
				require.Equal(t, billing.ReceiptPhoto{UploadID: retake, Status: upload.StatusPending}, photo, "the latest link is the photo")

				_, _, err = attach(w.karyawan, pay, ph.receipt(t, w.karyawan, wati, cleanPhoto))
				require.ErrorIs(t, err, billing.ErrReceiptPhotoAttached, "the retake is not refused")
			})
		}
	})

	t.Run("an upload the scan refused is refused", func(t *testing.T) {
		pay := w.paid(t, wati)
		for _, tc := range []struct {
			name   string
			body   []byte
			status string
		}{
			{name: "rejected", body: notAPhoto, status: upload.StatusRejected},
			{name: "infected", body: infectedPhoto, status: upload.StatusInfected},
		} {
			t.Run(tc.name, func(t *testing.T) {
				id := ph.scanned(t, w.karyawan, wati, tc.body, tc.status)
				links := w.count(t, "payment_receipt_photo")
				_, _, err := attach(w.karyawan, pay, id)
				require.ErrorIs(t, err, billing.ErrReceiptPhotoRefused)
				require.Equal(t, links, w.count(t, "payment_receipt_photo"))
			})
		}
	})

	t.Run("an upload that cannot be this payment's photo is invalid", func(t *testing.T) {
		pay := w.paid(t, wati)
		for _, tc := range []struct {
			name   string
			upload func(t *testing.T) uuid.UUID
		}{
			{name: "a receipt for another tenant", upload: func(t *testing.T) uuid.UUID {
				return ph.receipt(t, w.karyawan, bambang, cleanPhoto)
			}},
			{name: "an upload never completed", upload: func(t *testing.T) uuid.UUID {
				return ph.started(t, w.karyawan, upload.PurposeCashReceipt, wati.tenantID, cleanPhoto)
			}},
			{name: "a cash deposit photo", upload: func(t *testing.T) uuid.UUID {
				id := ph.started(t, w.karyawan, upload.PurposeCashDeposit, uuid.Nil, cleanPhoto)
				_, err := ph.uploads.Complete(ctx, w.karyawan, id)
				require.NoError(t, err)
				return id
			}},
			{name: "a receipt another member made", upload: func(t *testing.T) uuid.UUID {
				return ph.insertedReceipt(t, w.dina.UserID, wati.tenantID)
			}},
			{name: "another payment's photo", upload: func(*testing.T) uuid.UUID { return first }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				id := tc.upload(t)
				links := w.count(t, "payment_receipt_photo")
				_, _, err := attach(w.karyawan, pay, id)
				require.ErrorIs(t, err, billing.ErrReceiptPhotoInvalid)
				require.Equal(t, links, w.count(t, "payment_receipt_photo"))
			})
		}
		photo, changed, err := attach(w.karyawan, pay, ph.receipt(t, w.karyawan, wati, cleanPhoto))
		require.NoError(t, err, "none of them was linked")
		require.True(t, changed)
		require.Equal(t, upload.StatusPending, photo.Status)
	})

	t.Run("only the member who recorded the cash attaches its photo, and refusals are audited", func(t *testing.T) {
		pay := w.paid(t, wati)
		mine := ph.receipt(t, w.karyawan, wati, cleanPhoto)
		dinas := ph.receipt(t, w.dina, hadi, cleanPhoto)
		links := w.count(t, "payment_receipt_photo")
		denied := w.audited(t, authz.ActionCashRecord, "deny")
		for _, tc := range []struct {
			name     string
			p        authz.Principal
			uploadID uuid.UUID
		}{
			{name: "another karyawan", p: w.dina, uploadID: dinas},
			{name: "another karyawan with the recorder's upload", p: w.dina, uploadID: mine},
			{name: "an admin", p: w.admin, uploadID: mine},
			{name: "the Supervisor", p: supervisor, uploadID: mine},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := attach(tc.p, pay, tc.uploadID)
				require.ErrorIs(t, err, authz.ErrForbidden)
			})
		}
		require.Equal(t, denied+4, w.audited(t, authz.ActionCashRecord, "deny"))
		require.Equal(t, links, w.count(t, "payment_receipt_photo"))
	})

	t.Run("what is not a field cash payment, or not an upload, is not found", func(t *testing.T) {
		pay := w.paid(t, wati)
		byAdmin := w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: wati.tenantID, Amount: 5_000,
			Method: billing.MethodCash, ReceivedOn: day(2026, 11, 14)})
		up := ph.receipt(t, w.karyawan, wati, cleanPhoto)
		for _, tc := range []struct {
			name      string
			paymentID uuid.UUID
			uploadID  uuid.UUID
			field     string
		}{
			{name: "an unknown payment", paymentID: uuid.New(), uploadID: up, field: ""},
			{name: "cash an admin recorded (no receipt)", paymentID: byAdmin.Payment.ID, uploadID: up, field: ""},
			{name: "an unknown upload", paymentID: pay, uploadID: uuid.New(), field: "uploadId"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := attach(w.karyawan, tc.paymentID, tc.uploadID)
				requireNotFound(t, err, tc.field)
			})
		}
	})

	t.Run("a repeated cash payment answers its photo now", func(t *testing.T) {
		up := ph.receipt(t, w.karyawan, wati, cleanPhoto)
		in := billing.CashInput{RequestID: uuid.New(), TenantID: wati.tenantID, Amount: 15_000, PhotoUploadID: up}
		res, err := w.billing.RecordCashPayment(ctx, w.karyawan, in)
		require.NoError(t, err)
		require.Equal(t, billing.ReceiptPhoto{UploadID: up, Status: upload.StatusPending}, res.Photo)
		ph.scan(t, up, upload.StatusClean)
		again, err := w.billing.RecordCashPayment(ctx, w.karyawan, in)
		require.NoError(t, err)
		require.False(t, again.Created)
		require.Equal(t, billing.ReceiptPhoto{UploadID: up, Status: upload.StatusClean}, again.Photo)
	})

	t.Run("a payment recorded with its photo takes no other until the scan refuses it", func(t *testing.T) {
		up := ph.receipt(t, w.karyawan, wati, infectedPhoto)
		res, err := w.billing.RecordCashPayment(ctx, w.karyawan,
			billing.CashInput{RequestID: uuid.New(), TenantID: wati.tenantID, Amount: 15_000, PhotoUploadID: up})
		require.NoError(t, err)
		_, _, err = attach(w.karyawan, res.Payment.ID, ph.receipt(t, w.karyawan, wati, cleanPhoto))
		require.ErrorIs(t, err, billing.ErrReceiptPhotoAttached, "pending: the scan decides first")

		ph.scan(t, up, upload.StatusInfected)
		retake := ph.receipt(t, w.karyawan, wati, cleanPhoto)
		photo, changed, err := attach(w.karyawan, res.Payment.ID, retake)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, billing.ReceiptPhoto{UploadID: retake, Status: upload.StatusPending}, photo)
	})

	t.Run("the field list carries each cash payment's photo", func(t *testing.T) {
		pay := w.paid(t, bambang)
		photoOf := func(t *testing.T) billing.ReceiptPhoto {
			t.Helper()
			items, err := listing.All(func(page listing.Page) (listing.Result[billing.FieldTenant], error) {
				return w.billing.FieldToday(ctx, w.karyawan, page)
			})
			require.NoError(t, err)
			for _, it := range items {
				for _, p := range it.CashToday.Payments {
					if p.PaymentID == pay {
						return p.Photo
					}
				}
			}
			require.FailNow(t, "the payment is not on the field list")
			return billing.ReceiptPhoto{}
		}
		require.Equal(t, billing.ReceiptPhoto{Status: billing.PhotoMissing}, photoOf(t))
		up := ph.receipt(t, w.karyawan, bambang, cleanPhoto)
		_, _, err := attach(w.karyawan, pay, up)
		require.NoError(t, err)
		require.Equal(t, billing.ReceiptPhoto{UploadID: up, Status: upload.StatusPending}, photoOf(t))
	})
}

func TestRecordCashPayment(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 3, 10, 0)
	w := newFieldWorld(t, env, "0812861", "0812-8610-0099")
	own := w.collected(t, "Wati Susanti", day(2026, 11, 3), w.karyawan.MembershipID)
	dinas := w.collected(t, "Bambang Wijaya", day(2026, 11, 3), w.dina.MembershipID)
	nobodys := w.collected(t, "Dodi Saputra", day(2026, 11, 3), uuid.Nil)
	w.generate(t)
	supervisor := env.Principal(t, w.org.ID, w.org.Supervisor.UserID)

	t.Run("a karyawan records cash only for a tenant they collect from, and refusals are audited", func(t *testing.T) {
		allowed := w.audited(t, authz.ActionCashRecord, "allow")
		denied := w.audited(t, authz.ActionCashRecord, "deny")
		payments := w.count(t, "payment")

		res, err := w.billing.RecordCashPayment(ctx, w.karyawan, w.cashIn(t, w.karyawan, own, 300_000))
		require.NoError(t, err)
		require.True(t, res.Created)
		require.Equal(t, billing.ReceiptPending, res.ReceiptStatus)
		require.Equal(t, billing.MethodCash, res.Payment.Method)
		require.Equal(t, day(2026, 11, 3), res.Payment.ReceivedOn)
		require.NotNil(t, res.Payment.RecordedBy)
		require.Equal(t, w.karyawan.MembershipID, res.Payment.RecordedBy.MemberID)
		require.Len(t, res.Paid, 1)
		require.EqualValues(t, 300_000, res.Paid[0].Amount)
		require.EqualValues(t, 400_000, res.Paid[0].Outstanding)
		require.EqualValues(t, 400_000, res.Outstanding)
		require.Equal(t, allowed+1, w.audited(t, authz.ActionCashRecord, "allow"))

		for name, c := range map[string]struct {
			p authz.Principal
			r rental
		}{
			"another karyawan's tenant": {w.karyawan, dinas},
			"a tenant nobody collects":  {w.karyawan, nobodys},
			"Dina on the karyawan's":    {w.dina, own},
			"the Supervisor":            {supervisor, own},
			"an admin (recordPayment)":  {w.admin, own},
		} {
			_, err := w.billing.RecordCashPayment(ctx, c.p, w.cashIn(t, c.p, c.r, 100_000))
			require.ErrorIs(t, err, authz.ErrForbidden, name)
		}
		require.Equal(t, denied+5, w.audited(t, authz.ActionCashRecord, "deny"))
		require.Equal(t, payments+1, w.count(t, "payment"), "a refused request records nothing")

		var stored dbgen.Payment
		w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			var err error
			stored, err = q.GetPayment(ctx, dbgen.GetPaymentParams{OrganizationID: w.org.ID, ID: res.Payment.ID})
			return err
		})
		require.Equal(t, billing.SourceManual, stored.Source)
		require.True(t, stored.ReceivedAt.Equal(env.Clock.Now()), "cash is received now, not at midnight")
		require.Equal(t, w.org.Karyawan.UserID, stored.RecordedByStaffUserID.UUID)
	})

	t.Run("the receipt is recorded with the payment and enqueued once it commits", func(t *testing.T) {
		before := len(w.queue.enqueued())
		res, err := w.billing.RecordCashPayment(ctx, w.karyawan, w.cashIn(t, w.karyawan, own, 50_000))
		require.NoError(t, err)
		rows := w.receiptRows(t)
		row, ok := rows[res.Payment.ID]
		require.True(t, ok, "the payment's receipt row exists as soon as it commits")
		require.Equal(t, billing.ReceiptPending, row.Status)
		require.Nil(t, row.SentAt)
		require.Equal(t, own.tenantID, row.TenantID)
		require.True(t, row.CreatedAt.Equal(env.Clock.Now()))
		require.Equal(t, append([]uuid.UUID{}, res.Payment.ID), w.queue.enqueued()[before:])

		var cashRows, withoutReceipt int
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			return tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE NOT EXISTS (
				SELECT 1 FROM payment_receipt r WHERE r.payment_id = p.id))
				FROM payment p WHERE p.method = 'cash' AND p.recorded_by_staff_user_id = $1`, w.org.Karyawan.UserID).Scan(&cashRows, &withoutReceipt)
		})
		require.Equal(t, 2, cashRows)
		require.Zero(t, withoutReceipt, "every cash payment the karyawan recorded has its receipt")
	})

	t.Run("an admin's recordPayment of cash sends no receipt", func(t *testing.T) {
		receipts := len(w.receiptRows(t))
		w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: dinas.tenantID, Amount: 10_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 3)})
		require.Len(t, w.receiptRows(t), receipts)
	})

	t.Run("the same requestId records once and answers the receipt's status now", func(t *testing.T) {
		in := w.cashIn(t, w.karyawan, own, 20_000)
		first, err := w.billing.RecordCashPayment(ctx, w.karyawan, in)
		require.NoError(t, err)
		require.True(t, first.Created)
		payments := w.count(t, "payment")
		allowed := w.audited(t, authz.ActionCashRecord, "allow")

		again, err := w.billing.RecordCashPayment(ctx, w.karyawan, in)
		require.NoError(t, err)
		require.False(t, again.Created)
		require.Equal(t, first.Payment.ID, again.Payment.ID)
		require.Equal(t, billing.ReceiptPending, again.ReceiptStatus)
		require.Equal(t, payments, w.count(t, "payment"))
		require.Equal(t, allowed, w.audited(t, authz.ActionCashRecord, "allow"), "a repeat audits nothing")

		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			_, err := tx.Exec(ctx, "UPDATE payment_receipt SET status = 'sent', sent_at = now() WHERE payment_id = $1", first.Payment.ID)
			return err
		})
		setNow(env, 2026, 11, 4, 8, 0)
		later, err := w.billing.RecordCashPayment(ctx, w.karyawan, in)
		require.NoError(t, err, "a retry the next day is still the same cash")
		require.False(t, later.Created)
		require.Equal(t, billing.ReceiptSent, later.ReceiptStatus)
		setNow(env, 2026, 11, 3, 10, 0)

		changed := in
		changed.Amount = 25_000
		_, err = w.billing.RecordCashPayment(ctx, w.karyawan, changed)
		require.ErrorIs(t, err, billing.ErrRequestIDReused)

		otherPhoto := in
		otherPhoto.PhotoUploadID = env.CashPhoto(t, w.org.ID, w.karyawan.UserID, own.tenantID)
		_, err = w.billing.RecordCashPayment(ctx, w.karyawan, otherPhoto)
		require.ErrorIs(t, err, billing.ErrRequestIDReused, "the same cash comes with the same photo")

		viaAdmin := billing.PaymentInput{RequestID: uuid.New(), TenantID: own.tenantID, Amount: 5_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 3)}
		w.record(t, viaAdmin)
		_, err = w.billing.RecordCashPayment(ctx, w.admin, billing.CashInput{RequestID: viaAdmin.RequestID, TenantID: own.tenantID, Amount: 5_000,
			PhotoUploadID: env.CashPhoto(t, w.org.ID, w.admin.UserID, own.tenantID)})
		require.ErrorIs(t, err, authz.ErrForbidden, "the admin has no cash:record")
		require.Equal(t, payments, w.count(t, "payment")-1)
	})

	t.Run("bad input names its field", func(t *testing.T) {
		for field, in := range map[string]billing.CashInput{
			"requestId":     {TenantID: own.tenantID, Amount: 1},
			"amount":        {RequestID: uuid.New(), TenantID: own.tenantID},
			"note":          {RequestID: uuid.New(), TenantID: own.tenantID, Amount: 1, Note: string(make([]rune, 501))},
			"photoUploadId": {RequestID: uuid.New(), TenantID: own.tenantID, Amount: 1},
		} {
			_, err := w.billing.RecordCashPayment(ctx, w.karyawan, in)
			requireInput(t, err, field)
		}
		_, err := w.billing.RecordCashPayment(ctx, w.karyawan, billing.CashInput{RequestID: uuid.New(), TenantID: uuid.New(), Amount: 1, PhotoUploadID: uuid.New()})
		requireNotFound(t, err, "")
	})
}

// TestCashPaysFIFOWithCredit: cash collected in the field pays the oldest invoice first, across
// invoices, and the rest is credit, exactly as recordPayment.
func TestCashPaysFIFOWithCredit(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 10, 28, 10, 0)
	w := newFieldWorld(t, env, "0812862", "0812-8620-0099")
	r := w.collected(t, "Siti Rahayu", day(2026, 10, 28), w.karyawan.MembershipID)
	w.generate(t)
	setNow(env, 2026, 11, 21, 10, 0)
	w.generate(t)
	invs := w.invoices(t, r.renter)
	require.Len(t, invs, 2)

	res, err := w.billing.RecordCashPayment(ctx, w.karyawan, w.cashIn(t, w.karyawan, r, 1_500_000))
	require.NoError(t, err)
	require.Equal(t, []billing.PaidInvoice{
		{InvoiceID: invs[0].InvoiceID, Kind: billing.InvoiceRegular, Period: day(2026, 10, 28), PeriodEnd: day(2026, 11, 27), DueDate: day(2026, 10, 28), Amount: 700_000},
		{InvoiceID: invs[1].InvoiceID, Kind: billing.InvoiceRegular, Period: day(2026, 11, 28), PeriodEnd: day(2026, 12, 27), DueDate: day(2026, 11, 28), Amount: 700_000},
	}, res.Paid)
	require.Zero(t, res.Outstanding)
	require.EqualValues(t, 100_000, res.Credit)
	require.Equal(t, []string{"paid", "paid"}, []string{w.invoices(t, r.renter)[0].Status, w.invoices(t, r.renter)[1].Status})
}

func TestFieldToday(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 3, 9, 0)
	w := newFieldWorld(t, env, "0812863", "0812-8630-0099")
	mine := w.karyawan.MembershipID
	mawar, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kontrakan Mawar", Kind: property.KindKontrakan, Address: "Jl. Mawar No. 4, Bekasi"})
	require.NoError(t, err)

	// Each lease is saved on its start day or before, so its first invoice is due on its start day.
	oldest := w.collected(t, "Agus Oldest", day(2026, 11, 3), mine) // due 3 Nov
	setNow(env, 2026, 11, 5, 9, 0)
	older := meterWorld{world: w.world}.rentCollected(t, mawar, "Budi Older", day(2026, 11, 5), mine) // due 5 Nov
	w.generate(t)
	setNow(env, 2026, 11, 14, 9, 0)
	today := w.collected(t, "Citra Today", day(2026, 11, 14), mine)
	tomorrow := w.collected(t, "Dewi Tomorrow", day(2026, 11, 15), mine)
	inTwo := w.collected(t, "Eka InTwo", day(2026, 11, 16), mine)
	w.collected(t, "Fajar InThree", day(2026, 11, 17), mine)
	paid := w.collected(t, "Gita Paid", day(2026, 11, 14), mine)
	dinas := w.collected(t, "Hadi Dinas", day(2026, 11, 14), w.dina.MembershipID)
	shared := w.collected(t, "Indra Shared", day(2026, 11, 14), mine)
	adminOwn := w.collected(t, "Joko Admins", day(2026, 11, 14), w.admin.MembershipID)
	ahead := w.collected(t, "Kiki Ahead", day(2026, 11, 19), mine)      // due in 5 days, pays part in cash today
	paidCash := w.collected(t, "Lia PaidCash", day(2026, 11, 14), mine) // pays everything in cash today
	adminCash := w.collected(t, "Mira AdminCash", day(2026, 11, 19), mine)
	// Indra also rents a second unit that Dina collects: Mas Dimas may not see all of Indra's
	// money, so Indra is not on his list.
	unit, err := env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{Label: "Kamar 99", Terms: testenv.Monthly(500_000)})
	require.NoError(t, err)
	_, err = env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: shared.tenantID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: day(2026, 11, 14), CollectorMemberID: w.dina.MembershipID}})
	require.NoError(t, err)
	w.generate(t)
	w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: paid.tenantID, Amount: 700_000, Method: billing.MethodTransfer, ReceivedOn: day(2026, 11, 14)})
	// Cash an admin records is the admin's, not the karyawan's.
	w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: adminCash.tenantID, Amount: 100_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 14)})
	// Cash the admin records through recordPayment from a tenant he collects is not field cash: it
	// has no receipt and can take no photo, so it is not on his list.
	w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: adminOwn.tenantID, Amount: 100_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 14)})
	citra1, err := w.billing.RecordCashPayment(ctx, w.karyawan, w.cashIn(t, w.karyawan, today, 200_000))
	require.NoError(t, err)
	citra2, err := w.billing.RecordCashPayment(ctx, w.karyawan, w.cashIn(t, w.karyawan, today, 100_000))
	require.NoError(t, err)
	_, err = w.billing.RecordCashPayment(ctx, w.karyawan, w.cashIn(t, w.karyawan, ahead, 100_000))
	require.NoError(t, err)
	lia, err := w.billing.RecordCashPayment(ctx, w.karyawan, w.cashIn(t, w.karyawan, paidCash, 700_000))
	require.NoError(t, err)
	_, err = w.billing.RecordCashPayment(ctx, w.dina, w.cashIn(t, w.dina, dinas, 700_000))
	require.NoError(t, err)

	all := func(p authz.Principal) []billing.FieldTenant {
		t.Helper()
		items, err := listing.All(func(page listing.Page) (listing.Result[billing.FieldTenant], error) {
			return w.billing.FieldToday(ctx, p, page)
		})
		require.NoError(t, err)
		return items
	}
	names := func(items []billing.FieldTenant) []string {
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = it.TenantName
		}
		return out
	}
	noCash := billing.FieldCash{Payments: []billing.FieldCashPayment{}}
	cashOf := func(payments ...billing.CashResult) billing.FieldCash {
		out := billing.FieldCash{Payments: []billing.FieldCashPayment{}}
		for _, p := range payments {
			out.Amount += p.Payment.Net
			out.Count++
			out.Payments = append(out.Payments, billing.FieldCashPayment{PaymentID: p.Payment.ID, Amount: p.Payment.Net, RecordedAt: p.Payment.CreatedAt,
				Photo: p.Photo})
		}
		return out
	}

	t.Run("due rows oldest first, then the tenants paid in cash today by name", func(t *testing.T) {
		items := all(w.karyawan)
		require.Equal(t, []string{"Agus Oldest", "Budi Older", "Citra Today", "Dewi Tomorrow", "Eka InTwo", "Kiki Ahead", "Lia PaidCash"}, names(items),
			"not due in three days, not fully paid by transfer, not another karyawan's, not a tenant with a lease he does not collect, "+
				"not a tenant whose cash today the admin recorded")
		require.Equal(t, billing.FieldReason{Code: billing.FieldOverdue, Days: 11}, items[0].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldOverdue, Days: 9}, items[1].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueToday}, items[2].Reason, "a part payment in cash keeps the due reason")
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueInDays, Days: 1}, items[3].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueInDays, Days: 2}, items[4].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldPaidToday}, items[5].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldPaidToday}, items[6].Reason)

		require.Equal(t, oldest.tenantID, items[0].TenantID)
		require.Equal(t, ptrTo(day(2026, 11, 3)), items[0].OldestUnpaidDueDate)
		require.EqualValues(t, 700_000, items[0].Outstanding)
		require.EqualValues(t, 700_000, items[0].Overdue)
		require.Equal(t, noCash, items[0].CashToday)
		require.Equal(t, []billing.FieldUnit{{UnitLabel: "Kamar 2", PropertyName: "Kontrakan Mawar", PropertyAddress: "Jl. Mawar No. 4, Bekasi"}}, items[1].Units)
		require.Equal(t, older.tenantID, items[1].TenantID)

		require.Equal(t, today.tenantID, items[2].TenantID)
		require.EqualValues(t, 400_000, items[2].Outstanding)
		require.Zero(t, items[2].Overdue, "due today is not debt yet")
		require.Equal(t, cashOf(citra1, citra2), items[2].CashToday, "each payment, oldest first")
		require.Equal(t, tomorrow.tenantID, items[3].TenantID)
		require.Equal(t, inTwo.tenantID, items[4].TenantID)

		require.Equal(t, ahead.tenantID, items[5].TenantID)
		require.Equal(t, ptrTo(day(2026, 11, 19)), items[5].OldestUnpaidDueDate, "still owes, after the window")
		require.EqualValues(t, 600_000, items[5].Outstanding)
		require.Equal(t, paidCash.tenantID, items[6].TenantID)
		require.Nil(t, items[6].OldestUnpaidDueDate, "owes nothing")
		require.Zero(t, items[6].Outstanding)
		require.Equal(t, cashOf(lia), items[6].CashToday)
	})

	t.Run("pages never overlap and total counts both kinds", func(t *testing.T) {
		var got []string
		for n := 1; n <= 4; n++ {
			res, err := w.billing.FieldToday(ctx, w.karyawan, listing.Page{Number: n, Size: 2})
			require.NoError(t, err)
			require.EqualValues(t, 7, res.Total)
			got = append(got, names(res.Items)...)
		}
		require.Equal(t, []string{"Agus Oldest", "Budi Older", "Citra Today", "Dewi Tomorrow", "Eka InTwo", "Kiki Ahead", "Lia PaidCash"}, got)
	})

	t.Run("each member sees only the tenants they collect, an admin included", func(t *testing.T) {
		dina := all(w.dina)
		require.Equal(t, []string{"Hadi Dinas"}, names(dina), "Indra's other lease is Mas Dimas's")
		require.Equal(t, billing.FieldReason{Code: billing.FieldPaidToday}, dina[0].Reason, "Dina took all of Hadi's rent in cash")
		admin := all(w.admin)
		require.Equal(t, []string{"Joko Admins"}, names(admin), "the admin's cash from Mira does not list her: he does not collect from her")
		require.Equal(t, adminOwn.tenantID, admin[0].TenantID)
		require.Equal(t, billing.FieldCash{Payments: []billing.FieldCashPayment{}}, admin[0].CashToday,
			"cash recorded through recordPayment is not field cash")
		supervisor := env.Principal(t, w.org.ID, w.org.Supervisor.UserID)
		require.Empty(t, all(supervisor), "the Supervisor collects from nobody")
	})

	t.Run("the next day the Jakarta midnight moves every reason by one and drops yesterday's cash", func(t *testing.T) {
		setNow(env, 2026, 11, 15, 0, 0)
		t.Cleanup(func() { setNow(env, 2026, 11, 14, 9, 0) })
		items := all(w.karyawan)
		require.Equal(t, []string{"Agus Oldest", "Budi Older", "Citra Today", "Dewi Tomorrow", "Eka InTwo", "Fajar InThree"}, names(items),
			"Kiki and Lia were on yesterday's list only")
		require.Equal(t, billing.FieldReason{Code: billing.FieldOverdue, Days: 1}, items[2].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueToday}, items[3].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueInDays, Days: 2}, items[5].Reason)
		require.Equal(t, noCash, items[2].CashToday, "yesterday's cash is not today's")
	})

	t.Run("a member without invoice:read is refused and audited", func(t *testing.T) {
		nina, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-8630-0098", Name: "Nina Tamu",
			Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
		require.NoError(t, err)
		env.GrantCustomRole(t, w.org.ID, nina.ID, "Tanpa tagihan", `{"Version":"2026-10-25","Statement":[`+
			`{"Effect":"Deny","Action":["invoice:read"],"Resource":["*"]}]}`)
		before := w.audited(t, authz.ActionInvoiceRead, "deny")
		_, err = w.billing.FieldToday(ctx, env.Principal(t, w.org.ID, nina.UserID), listing.FirstPage)
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, before+1, w.audited(t, authz.ActionInvoiceRead, "deny"))
	})
}

func TestFieldCash(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 14, 9, 0)
	w := newFieldWorld(t, env, "0812864", "0812-8640-0099")
	a := w.collected(t, "Wati Susanti", day(2026, 11, 14), w.karyawan.MembershipID)
	b := w.collected(t, "Bambang Wijaya", day(2026, 11, 14), w.dina.MembershipID)
	w.generate(t)

	cash := func(p authz.Principal) billing.CashTotal {
		t.Helper()
		got, err := w.billing.FieldCash(ctx, p)
		require.NoError(t, err)
		return got
	}
	require.Equal(t, billing.CashTotal{}, cash(w.karyawan))

	first, err := w.billing.RecordCashPayment(ctx, w.karyawan, w.cashIn(t, w.karyawan, a, 300_000))
	require.NoError(t, err)
	setNow(env, 2026, 11, 15, 9, 0)
	_, err = w.billing.RecordCashPayment(ctx, w.karyawan, w.cashIn(t, w.karyawan, a, 200_000))
	require.NoError(t, err)
	_, err = w.billing.RecordCashPayment(ctx, w.dina, w.cashIn(t, w.dina, b, 700_000))
	require.NoError(t, err)
	w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: a.tenantID, Amount: 50_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 15)})

	t.Run("only the caller's own cash, across days", func(t *testing.T) {
		require.Equal(t, billing.CashTotal{Amount: 500_000, Count: 2}, cash(w.karyawan))
		require.Equal(t, billing.CashTotal{Amount: 700_000, Count: 1}, cash(w.dina))
	})

	t.Run("net of a correction to the payment", func(t *testing.T) {
		w.correct(t, billing.CorrectionInput{RequestID: uuid.New(), PaymentID: first.Payment.ID, AmountDelta: ptrTo(int64(-100_000)), Reason: "uang palsu dikembalikan"})
		require.Equal(t, billing.CashTotal{Amount: 400_000, Count: 2}, cash(w.karyawan))
	})

	t.Run("an admin and the Supervisor have no cash of their own to hand over", func(t *testing.T) {
		before := w.audited(t, authz.ActionCashDeposit, "deny")
		_, err := w.billing.FieldCash(ctx, w.admin)
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = w.billing.FieldCash(ctx, env.Principal(t, w.org.ID, w.org.Supervisor.UserID))
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, before+2, w.audited(t, authz.ActionCashDeposit, "deny"))
		allowed := w.audited(t, authz.ActionCashDeposit, "allow")
		cash(w.karyawan)
		require.Equal(t, allowed, w.audited(t, authz.ActionCashDeposit, "allow"), "reading one's cash changes nothing and is not audited")
	})
}

// TestCashCollectionIsolation: payment_receipt and payment_receipt_photo are scoped to their
// organization like every business table; a receipt changes only forward and a photo link never.
func TestCashCollectionIsolation(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 14, 9, 0)
	a := newFieldWorld(t, env, "0812865", "0812-8650-0099")
	b := newFieldWorld(t, env, "0812866", "0812-8660-0099")
	ra := a.collected(t, "Rina Wulandari", day(2026, 11, 14), a.karyawan.MembershipID)
	rb := b.collected(t, "Budi Santoso", day(2026, 11, 14), b.karyawan.MembershipID)
	a.generate(t)
	b.generate(t)
	pha, phb := newPhotos(a), newPhotos(b)
	upA := pha.receipt(t, a.karyawan, ra, cleanPhoto)
	upB := phb.receipt(t, b.karyawan, rb, cleanPhoto)
	pa, err := a.billing.RecordCashPayment(ctx, a.karyawan,
		billing.CashInput{RequestID: uuid.New(), TenantID: ra.tenantID, Amount: 100_000, PhotoUploadID: upA})
	require.NoError(t, err)
	pb, err := b.billing.RecordCashPayment(ctx, b.karyawan,
		billing.CashInput{RequestID: uuid.New(), TenantID: rb.tenantID, Amount: 100_000, PhotoUploadID: upB})
	require.NoError(t, err)

	t.Run("each organization sees only its own receipts", func(t *testing.T) {
		for _, w := range []fieldWorld{a, b} {
			var all, foreign int
			w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				return tx.QueryRow(ctx, "SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM payment_receipt").Scan(&all, &foreign)
			})
			require.Equal(t, 1, all)
			require.Zero(t, foreign)
		}
		a.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			_, err := q.GetPaymentReceiptStatus(ctx, dbgen.GetPaymentReceiptStatusParams{OrganizationID: b.org.ID, PaymentID: pb.Payment.ID})
			require.ErrorIs(t, err, pgx.ErrNoRows)
			return nil
		})
	})

	t.Run("another organization's tenant is not found and its cash not counted", func(t *testing.T) {
		// No upload can name another organization's tenant, so the photo is a's own.
		_, err := a.billing.RecordCashPayment(ctx, a.karyawan, billing.CashInput{RequestID: uuid.New(), TenantID: rb.tenantID, Amount: 1_000,
			PhotoUploadID: pha.receipt(t, a.karyawan, ra, cleanPhoto)})
		requireNotFound(t, err, "")
		items, err := a.billing.FieldToday(ctx, a.karyawan, listing.FirstPage)
		require.NoError(t, err)
		require.Len(t, items.Items, 1)
		require.Equal(t, ra.tenantID, items.Items[0].TenantID)
		got, err := a.billing.FieldCash(ctx, a.karyawan)
		require.NoError(t, err)
		require.Equal(t, billing.CashTotal{Amount: 100_000, Count: 1}, got)
	})

	t.Run("no receipt can be written for another organization", func(t *testing.T) {
		err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			return q.InsertPaymentReceipt(ctx, dbgen.InsertPaymentReceiptParams{
				PaymentID: pb.Payment.ID, OrganizationID: b.org.ID, TenantID: rb.tenantID, CreatedAt: env.Clock.Now(),
			})
		})
		require.Contains(t, requireCode(t, err, "42501").Message, "row-level security")
	})

	t.Run("a receipt only moves forward and is never deleted", func(t *testing.T) {
		exec := func(stmt string) error {
			return a.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				_, err := tx.Exec(ctx, stmt, pa.Payment.ID)
				return err
			})
		}
		requireCode(t, exec("DELETE FROM payment_receipt WHERE payment_id = $1"), "42501")
		requireCode(t, exec("UPDATE payment_receipt SET created_at = now() WHERE payment_id = $1"), "42501")
		require.NoError(t, exec("UPDATE payment_receipt SET status = 'skipped' WHERE payment_id = $1"))
		pgErr := requireCode(t, exec("UPDATE payment_receipt SET status = 'pending' WHERE payment_id = $1"), "42501")
		require.Contains(t, pgErr.Message, "already skipped")
		requireCode(t, exec("UPDATE payment_receipt SET status = 'sent', sent_at = now() WHERE payment_id = $1"), "42501")
	})

	t.Run("the receipt belongs to its payment's own tenant", func(t *testing.T) {
		other := a.collected(t, "Sari Lain", day(2026, 11, 14), uuid.Nil)
		pay := a.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: ra.tenantID, Amount: 1_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 14)})
		err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			return q.InsertPaymentReceipt(ctx, dbgen.InsertPaymentReceiptParams{
				PaymentID: pay.Payment.ID, OrganizationID: a.org.ID, TenantID: other.tenantID, CreatedAt: env.Clock.Now(),
			})
		})
		requireCode(t, err, "23503")
	})

	t.Run("each organization sees only its own photo links", func(t *testing.T) {
		for _, w := range []fieldWorld{a, b} {
			var all, foreign int
			w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				return tx.QueryRow(ctx, "SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM payment_receipt_photo").Scan(&all, &foreign)
			})
			require.Equal(t, 1, all)
			require.Zero(t, foreign)
		}
		a.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			rows, err := q.ListPaymentReceiptPhotoLinks(ctx, dbgen.ListPaymentReceiptPhotoLinksParams{OrganizationID: b.org.ID, PaymentID: pb.Payment.ID})
			require.NoError(t, err)
			require.Empty(t, rows)
			return nil
		})
	})

	t.Run("another organization's payment or upload is not found", func(t *testing.T) {
		_, _, err := a.billing.AttachReceiptPhoto(ctx, a.karyawan, pb.Payment.ID, upA)
		requireNotFound(t, err, "")
		_, _, err = a.billing.AttachReceiptPhoto(ctx, a.karyawan, a.paid(t, ra), phb.receipt(t, b.karyawan, rb, cleanPhoto))
		requireNotFound(t, err, "uploadId")

		payments := a.count(t, "payment")
		_, err = a.billing.RecordCashPayment(ctx, a.karyawan, billing.CashInput{RequestID: uuid.New(), TenantID: ra.tenantID, Amount: 1_000,
			PhotoUploadID: phb.receipt(t, b.karyawan, rb, cleanPhoto)})
		requireNotFound(t, err, "photoUploadId")
		require.Equal(t, payments, a.count(t, "payment"), "nothing recorded")
	})

	t.Run("no photo link can be written for another organization", func(t *testing.T) {
		unlinked := phb.receipt(t, b.karyawan, rb, cleanPhoto)
		err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			_, err := q.InsertPaymentReceiptPhoto(ctx, dbgen.InsertPaymentReceiptPhotoParams{
				UploadID: unlinked, OrganizationID: b.org.ID, PaymentID: pb.Payment.ID, TenantID: rb.tenantID,
				AttachedByStaffUserID: b.org.Karyawan.UserID, AttachedAt: env.Clock.Now(),
			})
			return err
		})
		require.Contains(t, requireCode(t, err, "42501").Message, "row-level security")
	})

	t.Run("a photo link is never changed or deleted", func(t *testing.T) {
		for _, tc := range []struct{ name, stmt string }{
			{name: "update", stmt: "UPDATE payment_receipt_photo SET attached_at = now() WHERE upload_id = $1"},
			{name: "delete", stmt: "DELETE FROM payment_receipt_photo WHERE upload_id = $1"},
			{name: "truncate", stmt: "TRUNCATE payment_receipt_photo"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				err := a.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
					if tc.name == "truncate" {
						_, err := tx.Exec(ctx, tc.stmt)
						return err
					}
					_, err := tx.Exec(ctx, tc.stmt, upA)
					return err
				})
				requireCode(t, err, "42501")
			})
		}
		require.Equal(t, 1, a.count(t, "payment_receipt_photo"))
	})

	t.Run("a photo link names an upload of its payment's own tenant", func(t *testing.T) {
		other := a.collected(t, "Tono Lain", day(2026, 11, 14), a.karyawan.MembershipID)
		otherUpload := pha.receipt(t, a.karyawan, other, cleanPhoto)
		err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			_, err := q.InsertPaymentReceiptPhoto(ctx, dbgen.InsertPaymentReceiptPhotoParams{
				UploadID: otherUpload, OrganizationID: a.org.ID, PaymentID: pa.Payment.ID, TenantID: ra.tenantID,
				AttachedByStaffUserID: a.org.Karyawan.UserID, AttachedAt: env.Clock.Now(),
			})
			return err
		})
		require.Contains(t, requireCode(t, err, "23503").Message, "payment_receipt_photo_upload_same_tenant")
	})
}

// ptrTo is a pointer to v.
func ptrTo[T any](v T) *T { return &v }

// TestCashComesWithItsPhoto: field cash is recorded only with its photo, linked in the payment's
// transaction (decision 50); a photo that cannot be used records nothing.
func TestCashComesWithItsPhoto(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 14, 9, 0)
	w := newFieldWorld(t, env, "0812864", "0812-8640-0099")
	wati := w.collected(t, "Wati Susanti", day(2026, 11, 14), w.karyawan.MembershipID)
	bambang := w.collected(t, "Bambang Wijaya", day(2026, 11, 14), w.karyawan.MembershipID)
	hadi := w.collected(t, "Hadi Saputra", day(2026, 11, 14), w.dina.MembershipID)
	w.generate(t)
	ph := newPhotos(w)
	record := func(p authz.Principal, r rental, photo uuid.UUID) (billing.CashResult, error) {
		return w.billing.RecordCashPayment(ctx, p, billing.CashInput{RequestID: uuid.New(), TenantID: r.tenantID, Amount: 50_000, PhotoUploadID: photo})
	}

	t.Run("a photo still being scanned records the cash and queues the receipt", func(t *testing.T) {
		up := ph.receipt(t, w.karyawan, wati, cleanPhoto)
		res, err := record(w.karyawan, wati, up)
		require.NoError(t, err)
		require.True(t, res.Created)
		require.Equal(t, billing.ReceiptPending, res.ReceiptStatus, "the receipt does not wait for the scan")
		require.Equal(t, billing.ReceiptPhoto{UploadID: up, Status: upload.StatusPending}, res.Photo)
		require.Contains(t, w.queue.enqueued(), res.Payment.ID)

		var row struct {
			payment, tenant, by uuid.UUID
			at                  time.Time
		}
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			return tx.QueryRow(ctx, "SELECT payment_id, tenant_id, attached_by_staff_user_id, attached_at FROM payment_receipt_photo WHERE upload_id = $1", up).
				Scan(&row.payment, &row.tenant, &row.by, &row.at)
		})
		require.Equal(t, res.Payment.ID, row.payment)
		require.Equal(t, wati.tenantID, row.tenant)
		require.Equal(t, w.org.Karyawan.UserID, row.by)
		require.True(t, row.at.Equal(env.Clock.Now()))
	})

	t.Run("a clean photo records the cash", func(t *testing.T) {
		up := ph.scanned(t, w.karyawan, wati, cleanPhoto, upload.StatusClean)
		res, err := record(w.karyawan, wati, up)
		require.NoError(t, err)
		require.Equal(t, billing.ReceiptPhoto{UploadID: up, Status: upload.StatusClean}, res.Photo)
	})

	used := ph.receipt(t, w.karyawan, wati, cleanPhoto)
	_, err := record(w.karyawan, wati, used)
	require.NoError(t, err)

	for _, tc := range []struct {
		name  string
		photo func(t *testing.T) uuid.UUID
		check func(t *testing.T, err error)
	}{
		{name: "no photo", photo: func(*testing.T) uuid.UUID { return uuid.Nil },
			check: func(t *testing.T, err error) { requireInput(t, err, "photoUploadId") }},
		{name: "an unknown upload", photo: func(*testing.T) uuid.UUID { return uuid.New() },
			check: func(t *testing.T, err error) { requireNotFound(t, err, "photoUploadId") }},
		{name: "a receipt for another tenant", photo: func(t *testing.T) uuid.UUID {
			return ph.receipt(t, w.karyawan, bambang, cleanPhoto)
		}, check: requirePhotoField(billing.ErrReceiptPhotoInvalid)},
		{name: "a receipt another member made", photo: func(t *testing.T) uuid.UUID {
			return ph.insertedReceipt(t, w.dina.UserID, wati.tenantID)
		}, check: requirePhotoField(billing.ErrReceiptPhotoInvalid)},
		{name: "an upload never completed", photo: func(t *testing.T) uuid.UUID {
			return ph.started(t, w.karyawan, upload.PurposeCashReceipt, wati.tenantID, cleanPhoto)
		}, check: requirePhotoField(billing.ErrReceiptPhotoInvalid)},
		{name: "a cash deposit photo", photo: func(t *testing.T) uuid.UUID {
			id := ph.started(t, w.karyawan, upload.PurposeCashDeposit, uuid.Nil, cleanPhoto)
			_, err := ph.uploads.Complete(ctx, w.karyawan, id)
			require.NoError(t, err)
			return id
		}, check: requirePhotoField(billing.ErrReceiptPhotoInvalid)},
		{name: "another payment's photo", photo: func(*testing.T) uuid.UUID { return used },
			check: requirePhotoField(billing.ErrReceiptPhotoInvalid)},
		{name: "a photo the scan rejected", photo: func(t *testing.T) uuid.UUID {
			return ph.scanned(t, w.karyawan, wati, notAPhoto, upload.StatusRejected)
		}, check: requirePhotoField(billing.ErrReceiptPhotoRefused)},
		{name: "a photo the scan found infected", photo: func(t *testing.T) uuid.UUID {
			return ph.scanned(t, w.karyawan, wati, infectedPhoto, upload.StatusInfected)
		}, check: requirePhotoField(billing.ErrReceiptPhotoRefused)},
	} {
		t.Run(tc.name+" records nothing", func(t *testing.T) {
			photo := tc.photo(t)
			payments, receipts, links := w.count(t, "payment"), w.count(t, "payment_receipt"), w.count(t, "payment_receipt_photo")
			queued := len(w.queue.enqueued())
			_, err := record(w.karyawan, wati, photo)
			tc.check(t, err)
			require.Equal(t, payments, w.count(t, "payment"))
			require.Equal(t, receipts, w.count(t, "payment_receipt"))
			require.Equal(t, links, w.count(t, "payment_receipt_photo"))
			require.Len(t, w.queue.enqueued(), queued, "no receipt is queued")
		})
	}

	t.Run("another karyawan cannot use the collector's photo for their own tenant", func(t *testing.T) {
		_, err := record(w.dina, hadi, ph.insertedReceipt(t, w.karyawan.UserID, hadi.tenantID))
		requirePhotoField(billing.ErrReceiptPhotoInvalid)(t, err)
	})

	t.Run("a refused cash:record is checked before the photo, and audited", func(t *testing.T) {
		denied := w.audited(t, authz.ActionCashRecord, "deny")
		_, err := record(w.dina, wati, ph.receipt(t, w.karyawan, wati, cleanPhoto))
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, denied+1, w.audited(t, authz.ActionCashRecord, "deny"))
	})

	t.Run("one photo backs one payment, even when two are saved at once", func(t *testing.T) {
		for round := range 5 {
			up := ph.receipt(t, w.karyawan, bambang, cleanPhoto)
			payments := w.count(t, "payment")
			errs := make(chan error, 2)
			var wg sync.WaitGroup
			for range 2 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := record(w.karyawan, bambang, up)
					errs <- err
				}()
			}
			wg.Wait()
			close(errs)
			var ok, invalid int
			for err := range errs {
				switch {
				case err == nil:
					ok++
				case errors.Is(err, billing.ErrReceiptPhotoInvalid):
					invalid++
				default:
					require.NoError(t, err, "round %d", round)
				}
			}
			require.Equal(t, 1, ok, "round %d", round)
			require.Equal(t, 1, invalid, "round %d", round)
			require.Equal(t, payments+1, w.count(t, "payment"), "round %d", round)
		}
	})

	t.Run("a payment recorded before photos were required repeats without one", func(t *testing.T) {
		legacy := w.paid(t, wati)
		var ref string
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			return tx.QueryRow(ctx, "SELECT source_ref FROM payment WHERE id = $1", legacy).Scan(&ref)
		})
		res, err := w.billing.RecordCashPayment(ctx, w.karyawan, billing.CashInput{
			RequestID: uuid.MustParse(ref), TenantID: wati.tenantID, Amount: 10_000,
			PhotoUploadID: ph.receipt(t, w.karyawan, wati, cleanPhoto),
		})
		require.NoError(t, err)
		require.False(t, res.Created)
		require.Equal(t, billing.ReceiptPhoto{Status: billing.PhotoMissing}, res.Photo, "a repeat links nothing")
	})
}

// requirePhotoField checks err is target about the photoUploadId field.
func requirePhotoField(target error) func(t *testing.T, err error) {
	return func(t *testing.T, err error) {
		t.Helper()
		require.ErrorIs(t, err, target)
		var fe *billing.FieldError
		require.ErrorAs(t, err, &fe)
		require.Equal(t, "photoUploadId", fe.Field)
	}
}
