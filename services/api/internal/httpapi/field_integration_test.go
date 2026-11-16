//go:build integration

package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// requireError checks an error answer's code and field ("" for none).
func requireError(t *testing.T, body []byte, code, field string) {
	t.Helper()
	e := decode[apigen.Error](t, body).Error
	require.Equal(t, code, e.Code, string(body))
	got := ""
	if e.Field != nil {
		got = *e.Field
	}
	require.Equal(t, field, got, string(body))
}

// TestFieldOverHTTP covers the karyawan's field app: recordCashPayment, listFieldToday,
// getMyFieldCash and makeFieldPromise. The clock is 14 Nov 2026, 09:00 in Asia/Jakarta; every
// lease is monthly from today, so each first invoice is due today.
func TestFieldOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 14, 9, 0, 0, 0, clock.Jakarta))
	srv := newLinkServer(t, env, &lockedBuffer{})
	a := newLinkOrg(t, env, "Griya Sejahtera", "0812991")
	b := newLinkOrg(t, env, "Kos Bulanan Sejahtera", "0812992")
	wati, _ := a.rent(t, "Wati Susanti", a.karyawan.MembershipID)
	bambang, _ := a.rent(t, "Bambang Wijaya", uuid.Nil)
	dewi, _ := b.rent(t, "Dewi Lestari", b.karyawan.MembershipID)

	admin, karyawan, karyawanB := newClient(t, srv.URL), newClient(t, srv.URL), newClient(t, srv.URL)
	admin.signIn(env, a.o.Admin.Phone)
	karyawan.signIn(env, a.o.Karyawan.Phone)
	karyawanB.signIn(env, b.o.Karyawan.Phone)
	orgA := "/api/orgs/" + a.o.ID.String()
	var cashPaymentID uuid.UUID
	cashPath := func(tenantID uuid.UUID) string { return orgA + "/tenants/" + tenantID.String() + "/cash-payments" }

	t.Run("recordCashPayment answers the payment, what it paid and the receipt queued", func(t *testing.T) {
		in := apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 300_000, Note: ptrTo("Dititip ke anaknya"),
			PhotoUploadId: env.CashPhoto(t, a.o.ID, a.o.Karyawan.UserID, wati)}
		resp, body := karyawan.do(http.MethodPost, cashPath(wati), in)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		res := decode[apigen.CashPaymentResult](t, body)
		cashPaymentID = res.Payment.Id
		require.Equal(t, apigen.ReceiptPending, res.Receipt.Status)
		require.Equal(t, apigen.PaymentMethodCash, res.Payment.Method)
		require.EqualValues(t, 300_000, res.Payment.Amount)
		require.Equal(t, "Dititip ke anaknya", res.Payment.Note)
		require.Equal(t, date(2026, 11, 14), res.Payment.ReceivedOn)
		require.Len(t, res.Paid, 1)
		require.EqualValues(t, 400_000, res.Paid[0].Outstanding)
		require.EqualValues(t, 400_000, res.Outstanding)

		resp, body = karyawan.do(http.MethodPost, cashPath(wati), in)
		require.Equal(t, http.StatusOK, resp.StatusCode, "a retry records nothing: %s", body)
		require.Equal(t, res.Payment.Id, decode[apigen.CashPaymentResult](t, body).Payment.Id)

		in.Amount = 1
		resp, body = karyawan.do(http.MethodPost, cashPath(wati), in)
		require.Equal(t, http.StatusConflict, resp.StatusCode, string(body))
		requireError(t, body, "request_id_reused", "requestId")

		in.Amount = 300_000
		in.PhotoUploadId = env.CashPhoto(t, a.o.ID, a.o.Karyawan.UserID, wati)
		resp, body = karyawan.do(http.MethodPost, cashPath(wati), in)
		require.Equal(t, http.StatusConflict, resp.StatusCode, "the same cash with another photo: %s", body)
		requireError(t, body, "request_id_reused", "requestId")
	})

	t.Run("recordCashPayment refuses what the karyawan may not do or did not mean", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodPost, cashPath(bambang), apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 1, PhotoUploadId: uuid.New()})
		require.Equal(t, http.StatusForbidden, resp.StatusCode, "not their tenant: %s", body)
		resp, body = admin.do(http.MethodPost, cashPath(wati), apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 1, PhotoUploadId: uuid.New()})
		require.Equal(t, http.StatusForbidden, resp.StatusCode, "admins use recordPayment: %s", body)
		resp, body = karyawan.do(http.MethodPost, cashPath(dewi), apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 1, PhotoUploadId: uuid.New()})
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "another organization's tenant: %s", body)
		resp, body = karyawan.do(http.MethodPost, cashPath(uuid.New()), apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 1, PhotoUploadId: uuid.New()})
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		resp, body = karyawan.do(http.MethodPost, orgA+"/tenants/nope/cash-payments", apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 1, PhotoUploadId: uuid.New()})
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		resp, body = karyawan.do(http.MethodPost, cashPath(wati), apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 0, PhotoUploadId: uuid.New()})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
		requireError(t, body, "invalid_input", "amount")
		resp, body = karyawan.do(http.MethodPost, cashPath(wati), map[string]any{"requestId": uuid.New(), "amount": 1, "method": "cash"})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "an unknown field: %s", body)
		requireError(t, body, "invalid_json", "")
		resp, body = karyawanB.do(http.MethodPost, cashPath(wati), apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 1, PhotoUploadId: uuid.New()})
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "not a member of a, which is not shown to exist: %s", body)
	})

	t.Run("listFieldToday lists the karyawan's own tenants with the cash taken today", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodGet, orgA+"/field/today", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		list := decode[apigen.FieldTodayList](t, body)
		require.Equal(t, 1, list.Total)
		require.Equal(t, 1, list.Page)
		require.Equal(t, 20, list.PageSize)
		require.Len(t, list.Items, 1)
		it := list.Items[0]
		require.Equal(t, wati, it.TenantId)
		require.Equal(t, "Wati Susanti", it.TenantName)
		require.Equal(t, apigen.FieldDueReasonDueToday, it.Reason)
		require.Nil(t, it.DaysOverdue)
		require.Nil(t, it.DaysUntilDue)
		require.Equal(t, ptrTo(date(2026, 11, 14)), it.OldestUnpaidDueDate)
		require.EqualValues(t, 400_000, it.Outstanding)
		require.Zero(t, it.Overdue)
		require.EqualValues(t, 300_000, it.CashToday.Amount)
		require.Equal(t, 1, it.CashToday.Count)
		require.Len(t, it.CashToday.Payments, 1)
		require.Equal(t, cashPaymentID, it.CashToday.Payments[0].PaymentId)
		require.EqualValues(t, 300_000, it.CashToday.Payments[0].Amount)
		require.False(t, it.CashToday.Payments[0].RecordedAt.IsZero())
		require.Equal(t, []apigen.FieldUnit{{UnitLabel: "Kamar 1", PropertyName: "Kos Griya Sejahtera", PropertyAddress: ""}}, it.Units)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(body, &raw))
		item := raw["items"].([]any)[0].(map[string]any)
		require.NotContains(t, item, "daysOverdue", "only for overdue")
		require.Contains(t, item["units"].([]any)[0], "propertyAddress", "always there, empty when unknown")

		env.Clock.Set(time.Date(2026, 11, 16, 9, 0, 0, 0, clock.Jakarta))
		resp, body = karyawan.do(http.MethodGet, orgA+"/field/today", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		it = decode[apigen.FieldTodayList](t, body).Items[0]
		require.Equal(t, apigen.FieldDueReasonOverdue, it.Reason)
		require.Equal(t, ptrTo(2), it.DaysOverdue)
		require.EqualValues(t, 400_000, it.Overdue)
		require.Equal(t, apigen.FieldCashToday{Payments: []apigen.FieldCashPayment{}}, it.CashToday)
		require.Contains(t, string(body), `"payments":[]`, "an empty list, not null")
		env.Clock.Set(time.Date(2026, 11, 14, 9, 0, 0, 0, clock.Jakarta))

		resp, body = admin.do(http.MethodGet, orgA+"/field/today", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Zero(t, decode[apigen.FieldTodayList](t, body).Total, "the admin collects from nobody")

		resp, body = karyawan.do(http.MethodGet, orgA+"/field/today?pageSize=101", nil)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
		requireError(t, body, "invalid_input", "pageSize")
	})

	t.Run("getMyFieldCash is the caller's own cash", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodGet, orgA+"/field/cash", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, apigen.CashTotal{Amount: 300_000, Count: 1}, decode[apigen.CashTotal](t, body))
		resp, body = karyawanB.do(http.MethodGet, "/api/orgs/"+b.o.ID.String()+"/field/cash", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, apigen.CashTotal{}, decode[apigen.CashTotal](t, body))
		resp, body = admin.do(http.MethodGet, orgA+"/field/cash", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
		requireError(t, body, "forbidden", "")
	})

	t.Run("makeFieldPromise records source karyawan", func(t *testing.T) {
		promised := date(2026, 11, 20)
		path := orgA + "/field/tenants/" + wati.String() + "/promises"
		resp, body := karyawan.do(http.MethodPost, path, apigen.MakeTenantPromiseRequest{
			Kind: apigen.SpecificDate, PromisedDate: &promised, Note: ptrTo("Gajian tanggal 20"),
		})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		p := decode[apigen.TenantPromise](t, body)
		require.Len(t, p.Versions, 1)
		v := p.Versions[0]
		require.Equal(t, apigen.PromiseFromKaryawan, v.Source)
		require.NotNil(t, v.CreatedBy)
		require.Equal(t, a.karyawan.MembershipID, v.CreatedBy.MemberId)

		resp, body = karyawan.do(http.MethodPost, orgA+"/field/tenants/"+bambang.String()+"/promises", apigen.MakeTenantPromiseRequest{
			Kind: apigen.SpecificDate, PromisedDate: &promised,
		})
		require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
		resp, body = karyawan.do(http.MethodPost, path, apigen.MakeTenantPromiseRequest{Kind: apigen.SpecificDate})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))

		resp, body = karyawan.do(http.MethodPost, orgA+"/tenants/"+wati.String()+"/promises", apigen.MakeTenantPromiseRequest{
			Kind: apigen.SpecificDate, PromisedDate: &promised,
		})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		latest := decode[apigen.TenantPromise](t, body).Versions
		require.Equal(t, apigen.PromiseFromAdmin, latest[len(latest)-1].Source, "makeTenantPromise is unchanged")
	})

	t.Run("refusals are audited", func(t *testing.T) {
		denied := 0
		for _, row := range env.AuditLog(t, a.o.ID) {
			if row.Decision == "deny" && (row.Action == authz.ActionCashRecord || row.Action == authz.ActionCashDeposit) {
				denied++
			}
		}
		require.Equal(t, 3, denied, "bambang's cash, the admin's cash and the admin's cash on hand")
	})
}

// TestCashPhotoOverHTTP covers the photo of field cash (decision 50): recordCashPayment takes it
// as photoUploadId and refuses cash without a usable one, and attachCashPaymentPhoto retakes a
// photo the scan refused.
func TestCashPhotoOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 14, 9, 0, 0, 0, clock.Jakarta))
	srv := newLinkServer(t, env, &lockedBuffer{})
	a := newLinkOrg(t, env, "Griya Foto Kas", "0812993")
	wati, _ := a.rent(t, "Wati Susanti", a.karyawan.MembershipID)

	storage := uploadtest.NewStorage()
	uploads := upload.NewService(upload.ServiceDeps{
		Store: env.Store, Authz: env.Authz, Clock: env.Clock, Presigner: storage, Queue: &uploadtest.Queue{}, Log: env.Log,
	})
	checker := upload.NewChecker(upload.CheckerDeps{
		Store: env.Store, Objects: storage, Scanner: &uploadtest.Scanner{}, Audit: env.Authz, Clock: env.Clock, Log: env.Log,
	})
	clean := uploadtest.Image("image/jpeg", 1_024)
	infected := append(uploadtest.Image("image/jpeg", 1_024), uploadtest.EICAR...)
	receipt := func(t *testing.T, body []byte) uuid.UUID {
		t.Helper()
		c, err := uploads.Create(t.Context(), a.karyawan, upload.CreateInput{
			Purpose: upload.PurposeCashReceipt, TenantID: wati, ContentType: "image/jpeg", Size: int64(len(body)),
		})
		require.NoError(t, err)
		u, err := url.Parse(c.Put.URL)
		require.NoError(t, err)
		storage.Put(strings.TrimPrefix(u.Path, "/"), body)
		_, err = uploads.Complete(t.Context(), a.karyawan, c.Upload.ID)
		require.NoError(t, err)
		return c.Upload.ID
	}
	scan := func(t *testing.T, id uuid.UUID, status string) {
		t.Helper()
		res, err := checker.Scan(t.Context(), a.o.ID, id)
		require.NoError(t, err)
		require.Equal(t, status, res.Status)
	}

	karyawan := newClient(t, srv.URL)
	karyawan.signIn(env, a.o.Karyawan.Phone)
	orgA := "/api/orgs/" + a.o.ID.String()
	cashPath := orgA + "/tenants/" + wati.String() + "/cash-payments"
	photoPath := func(paymentID string) string { return orgA + "/cash-payments/" + paymentID + "/photo" }

	first := receipt(t, clean)
	resp, body := karyawan.do(http.MethodPost, cashPath, apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 300_000, PhotoUploadId: first})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	recorded := decode[apigen.CashPaymentResult](t, body)
	require.Equal(t, apigen.ReceiptPending, recorded.Receipt.Status, "the receipt does not wait for the scan")
	require.Equal(t, apigen.ReceiptPhoto{UploadId: &first, Status: apigen.PhotoPending}, recorded.Photo)

	t.Run("the field list shows the photo recorded with the cash", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodGet, orgA+"/field/today", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		payments := decode[apigen.FieldTodayList](t, body).Items[0].CashToday.Payments
		require.Len(t, payments, 1)
		require.Equal(t, apigen.ReceiptPhoto{UploadId: &first, Status: apigen.PhotoPending}, payments[0].Photo)
	})

	t.Run("cash without a usable photo is refused on photoUploadId", func(t *testing.T) {
		rejected := receipt(t, []byte("not a photo"))
		scan(t, rejected, upload.StatusRejected)
		for _, tc := range []struct {
			name   string
			body   any
			status int
			code   string
		}{
			{name: "no photo", body: map[string]any{"requestId": uuid.New(), "amount": 1},
				status: http.StatusBadRequest, code: "invalid_input"},
			{name: "an unknown upload", body: apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 1, PhotoUploadId: uuid.New()},
				status: http.StatusNotFound, code: "not_found"},
			{name: "another payment's photo", body: apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 1, PhotoUploadId: first},
				status: http.StatusBadRequest, code: "receipt_photo_upload_invalid"},
			{name: "a photo the scan refused", body: apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 1, PhotoUploadId: rejected},
				status: http.StatusConflict, code: "receipt_photo_upload_refused"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := karyawan.do(http.MethodPost, cashPath, tc.body)
				require.Equal(t, tc.status, resp.StatusCode, string(body))
				requireError(t, body, tc.code, "photoUploadId")
			})
		}
		resp, body := karyawan.do(http.MethodPost, cashPath, map[string]any{"requestId": uuid.New(), "amount": 1, "photoUploadId": "nope"})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
		requireError(t, body, "invalid_json", "")

		resp, body = karyawan.do(http.MethodGet, orgA+"/field/today", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Len(t, decode[apigen.FieldTodayList](t, body).Items[0].CashToday.Payments, 1, "nothing was recorded")
	})

	t.Run("a photo the scan did not refuse is not retaken", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodPut, photoPath(recorded.Payment.Id.String()), apigen.AttachCashPaymentPhotoInput{UploadId: receipt(t, clean)})
		require.Equal(t, http.StatusConflict, resp.StatusCode, string(body))
		requireError(t, body, "receipt_photo_attached", "")
	})

	t.Run("a photo the scan refused is retaken, and again changes nothing", func(t *testing.T) {
		bad := receipt(t, infected)
		resp, body := karyawan.do(http.MethodPost, cashPath, apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 100_000, PhotoUploadId: bad})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		paid := decode[apigen.CashPaymentResult](t, body)
		scan(t, bad, upload.StatusInfected)

		retake := receipt(t, clean)
		for _, name := range []string{"first", "again"} {
			resp, body := karyawan.do(http.MethodPut, photoPath(paid.Payment.Id.String()), apigen.AttachCashPaymentPhotoInput{UploadId: retake})
			require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", name, body)
			require.Equal(t, apigen.ReceiptPhoto{UploadId: &retake, Status: apigen.PhotoPending}, decode[apigen.ReceiptPhoto](t, body), name)
		}
	})

	t.Run("attach refusals answer their code", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			path   string
			body   any
			status int
			code   string
			field  string
		}{
			{name: "an unknown payment", path: photoPath(uuid.NewString()), body: apigen.AttachCashPaymentPhotoInput{UploadId: first},
				status: http.StatusNotFound, code: "not_found"},
			{name: "a payment id that is not a uuid", path: photoPath("nope"), body: apigen.AttachCashPaymentPhotoInput{UploadId: first},
				status: http.StatusNotFound, code: "not_found"},
			{name: "an unknown field", path: photoPath(recorded.Payment.Id.String()), body: map[string]any{"uploadId": first, "status": "clean"},
				status: http.StatusBadRequest, code: "invalid_json"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := karyawan.do(http.MethodPut, tc.path, tc.body)
				require.Equal(t, tc.status, resp.StatusCode, string(body))
				requireError(t, body, tc.code, tc.field)
			})
		}
	})
}
