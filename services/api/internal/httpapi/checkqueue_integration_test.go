//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/checkqueue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/deposit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/visit"
)

// TestCheckQueueOverHTTP covers Antrean cek end to end: listCheckQueue, matchTransferProof,
// applyPhoneChange and dismissCheckItem, each error code, and getCashDeposit's gapClosure. The
// items are put on the queue through the services; the clock is 14 Nov 2026 in Asia/Jakarta.
func TestCheckQueueOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 14, 9, 0, 0, 0, clock.Jakarta))
	a := newLinkOrg(t, env, "Griya Antrean", "0812996")

	links := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin)
	storage, scanQueue := uploadtest.NewStorage(), &uploadtest.Queue{}
	uploads := upload.NewService(upload.ServiceDeps{
		Store: env.Store, Authz: env.Authz, Clock: env.Clock, Presigner: storage, Queue: scanQueue, Log: env.Log,
	})
	checker := upload.NewChecker(upload.CheckerDeps{
		Store: env.Store, Objects: storage, Scanner: &uploadtest.Scanner{}, Audit: env.Authz, Clock: env.Clock, Log: env.Log,
	})
	proofs := upload.NewProofs(upload.ProofsDeps{
		Store: env.Store, Links: links, Objects: storage, Queue: scanQueue, Clock: env.Clock, Log: env.Log,
	})
	deposits := deposit.NewService(env.Store, env.Authz, env.Clock, env.Log)
	visits := visit.NewService(env.Store, env.Billing, env.Clock)
	qrisSvc := qris.NewService(qris.Deps{Store: env.Store, Clock: env.Clock, Audit: env.Authz, Log: env.Log})
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing, PayLinks: links,
		Deposits:     deposits,
		CheckQueue:   checkqueue.NewService(env.Store, env.Authz, env.Billing, env.Tenants, env.Clock),
		CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	rinaID, rinaInvoice := a.rent(t, "Rina Wulandari", a.karyawan.MembershipID)
	budiID, _ := a.rent(t, "Budi Santoso", a.karyawan.MembershipID)
	citra := a.rentWithLink(t, "Citra Lestari")
	_, rinaToken := a.mintLink(t, rinaInvoice)
	tick := func() { env.Clock.Advance(time.Minute) }
	scan := func(t *testing.T, id uuid.UUID, status string) {
		t.Helper()
		res, err := checker.Scan(ctx, a.o.ID, id)
		require.NoError(t, err)
		require.Equal(t, status, res.Status)
	}
	send := func(t *testing.T, body []byte) upload.Proof {
		t.Helper()
		p, err := proofs.Send(ctx, rinaToken, func() (io.Reader, error) { return bytes.NewReader(body), nil })
		require.NoError(t, err)
		return p
	}
	photo := uploadtest.Image("image/jpeg", 3_000)
	cleanProof := func(t *testing.T) upload.Proof {
		t.Helper()
		p := send(t, photo)
		scan(t, p.UploadID, upload.StatusClean)
		return p
	}
	newNumber := func(t *testing.T, tenantID uuid.UUID, raw string) visit.Outcome {
		t.Helper()
		require.NoError(t, env.Store.WithOrgTx(ctx, a.o.ID, func(tx pgx.Tx) error {
			_, err := dbgen.New(tx).OpenVisitTask(ctx, dbgen.OpenVisitTaskParams{
				ID: uuid.New(), OrganizationID: a.o.ID, TenantID: tenantID, Reason: visit.TaskUndeliverable, CreatedAt: env.Clock.Now(),
			})
			return err
		}))
		out, _, err := visits.Record(ctx, a.karyawan, tenantID, visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomeNumberChanged, NewPhone: raw})
		require.NoError(t, err)
		return out
	}

	// One item of every kind, a minute apart: a clean and a pending proof (a rejected one is not
	// on the queue), a deposit short Rp50.000, an infected receipt, a QRIS paid at another amount
	// and two new numbers.
	proof := cleanProof(t)
	tick()
	pending := send(t, photo)
	tick()
	rejected := send(t, photo)
	var key string
	require.NoError(t, env.Store.WithOrgTx(ctx, a.o.ID, func(tx pgx.Tx) error {
		up, err := dbgen.New(tx).GetUpload(ctx, dbgen.GetUploadParams{OrganizationID: a.o.ID, ID: rejected.UploadID})
		key = up.ObjectKey
		return err
	}))
	storage.Put(upload.IncomingKey(key), append([]byte("%PDF-1.7\n"), make([]byte, 3_000-9)...))
	scan(t, rejected.UploadID, upload.StatusRejected)
	tick()
	_, err := env.Billing.RecordCashPayment(ctx, a.karyawan, billing.CashInput{RequestID: uuid.New(), TenantID: budiID, Amount: 700_000,
		PhotoUploadID: env.CashPhoto(t, a.o.ID, a.o.Karyawan.UserID, budiID)})
	require.NoError(t, err)
	gap, _, err := deposits.Record(ctx, a.karyawan, deposit.Input{RequestID: uuid.New(), ExpectedAmount: 700_000, ExpectedCount: 1, Amount: 650_000})
	require.NoError(t, err)
	_, _, err = deposits.Confirm(ctx, a.admin, gap.ID, deposit.ConfirmInput{ReceivedAmount: 650_000, Note: "Kurang Rp50.000"})
	require.NoError(t, err)
	tick()
	infectedBody := append(uploadtest.Image("image/jpeg", 3_000-len(uploadtest.EICAR)), uploadtest.EICAR...)
	receipt, err := uploads.Create(ctx, a.karyawan, upload.CreateInput{
		Purpose: upload.PurposeCashReceipt, TenantID: rinaID, ContentType: "image/jpeg", Size: int64(len(infectedBody)),
	})
	require.NoError(t, err)
	u, err := url.Parse(receipt.Put.URL)
	require.NoError(t, err)
	storage.Put(strings.TrimPrefix(u.Path, "/"), infectedBody)
	_, err = uploads.Complete(ctx, a.karyawan, receipt.Upload.ID)
	require.NoError(t, err)
	scan(t, receipt.Upload.ID, upload.StatusInfected)
	tick()
	attempt := a.insertAttempt(t, citra, attemptState{status: qris.AttemptPending})
	require.NoError(t, qrisSvc.SettlePaid(ctx, a.o.ID, attempt.TransactionID, 650_000))
	tick()
	budiNumber := newNumber(t, budiID, "0812-9969-0001")
	tick()
	budi, err := env.Tenants.Get(ctx, a.admin, budiID)
	require.NoError(t, err)
	takenNumber := newNumber(t, rinaID, budi.Phone)

	admin, karyawan, supervisor := newClient(t, srv.URL), newClient(t, srv.URL), newClient(t, srv.URL)
	admin.signIn(env, a.o.Admin.Phone)
	karyawan.signIn(env, a.o.Karyawan.Phone)
	supervisor.signIn(env, a.o.Supervisor.Phone)
	orgA := "/api/orgs/" + a.o.ID.String()
	queue := orgA + "/check-queue"

	t.Run("listCheckQueue lists every kind oldest first with its counts", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, queue, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		page := decode[apigen.CheckQueuePage](t, body)
		require.Equal(t, apigen.CheckQueueCounts{TransferProof: 2, DepositGap: 1, UploadInfected: 1, QrisAmountMismatch: 1, PhoneChanged: 2}, page.Counts)
		require.Equal(t, 7, page.Waiting)
		require.Equal(t, 7, page.Total)
		require.Equal(t, 1, page.Page)
		require.Equal(t, 20, page.PageSize)
		wantIDs := []uuid.UUID{proof.ID, pending.ID, gap.ID, receipt.Upload.ID, attempt.ID, budiNumber.ID, takenNumber.ID}
		require.Len(t, page.Items, len(wantIDs))
		for i, it := range page.Items {
			require.Equal(t, wantIDs[i], it.Id, "item %d", i)
			present := 0
			for _, p := range []bool{it.TransferProof != nil, it.DepositGap != nil, it.UploadInfected != nil, it.QrisAmountMismatch != nil, it.PhoneChanged != nil} {
				if p {
					present++
				}
			}
			require.Equal(t, 1, present, "item %d carries exactly its kind's object", i)
		}
		require.Equal(t, apigen.CheckQueueKindTransferProof, page.Items[0].Kind)
		require.Equal(t, apigen.CheckTransferProofUploadStatus("clean"), page.Items[0].TransferProof.UploadStatus)
		require.Equal(t, "Rina Wulandari", page.Items[0].TransferProof.Tenant.Name)
		require.Equal(t, []apigen.CheckTenantUnit{{UnitLabel: "Kamar 1", PropertyName: "Kos Griya Antrean", Active: true}}, page.Items[0].TransferProof.Tenant.Units)
		require.Equal(t, apigen.CheckTransferProofUploadStatus("pending"), page.Items[1].TransferProof.UploadStatus)
		require.EqualValues(t, -50_000, page.Items[2].DepositGap.Gap)
		require.Equal(t, a.o.Karyawan.Name, page.Items[2].DepositGap.Depositor.Name)
		require.NotNil(t, page.Items[3].UploadInfected.Tenant, "a receipt names its tenant")
		require.Equal(t, a.o.Karyawan.UserID, page.Items[3].UploadInfected.UploadedBy.StaffUserId)
		require.EqualValues(t, 700_000, page.Items[4].QrisAmountMismatch.QrAmount)
		require.Equal(t, ptrTo(int64(650_000)), page.Items[4].QrisAmountMismatch.ReportedAmount)
		require.Equal(t, "+6281299690001", page.Items[5].PhoneChanged.NewPhone)
		require.Equal(t, budi.Phone, page.Items[5].PhoneChanged.CurrentPhone)
		require.NotContains(t, string(body), rejected.ID.String(), "a refused proof is not on the queue")
	})

	t.Run("listCheckQueue filters by kind and pages", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, queue+"?kind=phone_changed&pageSize=1&page=2", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		page := decode[apigen.CheckQueuePage](t, body)
		require.Equal(t, 2, page.Total)
		require.Equal(t, 7, page.Waiting)
		require.Len(t, page.Items, 1)
		require.Equal(t, takenNumber.ID, page.Items[0].Id)
		require.Equal(t, 2, page.Counts.TransferProof, "counts whatever kind was asked for")
	})

	t.Run("listCheckQueue refuses an unknown kind and members who may not read it", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, queue+"?kind=bukti_lain", nil)
		requireAPIError(t, resp, body, http.StatusBadRequest, "invalid_input", "kind")
		resp, body = karyawan.do(http.MethodGet, queue, nil)
		requireAPIError(t, resp, body, http.StatusForbidden, "forbidden", "")
		resp, body = supervisor.do(http.MethodGet, queue, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	})

	matchPath := func(id uuid.UUID) string { return queue + "/transfer_proof/" + id.String() + "/match" }
	in := apigen.MatchTransferProofRequest{RequestId: uuid.New(), Amount: 700_000, ReceivedOn: date(2026, 11, 14), Note: ptrTo("BCA a.n. Rina")}
	var matched apigen.MatchTransferProofResult

	t.Run("matchTransferProof answers 201, then 200 for the same request", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, matchPath(proof.ID), in)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		matched = decode[apigen.MatchTransferProofResult](t, body)
		require.Equal(t, apigen.CheckResolutionOutcomeMatched, matched.Resolution.Outcome)
		require.Equal(t, apigen.CheckQueueKindTransferProof, matched.Resolution.Kind)
		require.Equal(t, proof.ID, matched.Resolution.ItemId)
		require.NotNil(t, matched.Resolution.PaymentId)
		require.Equal(t, matched.Payment.Payment.Id, *matched.Resolution.PaymentId)
		require.Equal(t, a.o.Admin.UserID, matched.Resolution.ResolvedBy)
		require.EqualValues(t, 700_000, matched.Payment.Payment.Amount)
		require.Zero(t, matched.Payment.Outstanding)

		resp, body = admin.do(http.MethodPost, matchPath(proof.ID), in)
		require.Equal(t, http.StatusOK, resp.StatusCode, "a retry records nothing: %s", body)
		again := decode[apigen.MatchTransferProofResult](t, body)
		require.Equal(t, matched.Resolution.Id, again.Resolution.Id)
		require.Equal(t, matched.Payment.Payment.Id, again.Payment.Payment.Id)
	})

	other := cleanProof(t)
	t.Run("matchTransferProof answers each refusal with its code", func(t *testing.T) {
		tests := []struct {
			name   string
			path   string
			body   any
			status int
			code   string
			field  string
		}{
			{"an answered proof", matchPath(proof.ID), apigen.MatchTransferProofRequest{RequestId: uuid.New(), Amount: 1, ReceivedOn: date(2026, 11, 14)},
				http.StatusConflict, "check_item_resolved", ""},
			{"a photo still being scanned", matchPath(pending.ID), apigen.MatchTransferProofRequest{RequestId: uuid.New(), Amount: 1, ReceivedOn: date(2026, 11, 14)},
				http.StatusConflict, "upload_not_ready", ""},
			{"a refused photo", matchPath(rejected.ID), apigen.MatchTransferProofRequest{RequestId: uuid.New(), Amount: 1, ReceivedOn: date(2026, 11, 14)},
				http.StatusConflict, "upload_refused", ""},
			{"a requestId that matched another proof", matchPath(other.ID), in, http.StatusConflict, "request_id_reused", ""},
			{"a requestId used for another payment", matchPath(other.ID), apigen.MatchTransferProofRequest{RequestId: in.RequestId, Amount: 1, ReceivedOn: date(2026, 11, 14)},
				http.StatusConflict, "request_id_reused", "requestId"},
			{"no amount", matchPath(other.ID), apigen.MatchTransferProofRequest{RequestId: uuid.New(), ReceivedOn: date(2026, 11, 14)},
				http.StatusBadRequest, "invalid_input", "amount"},
			{"an unknown proof", matchPath(uuid.New()), apigen.MatchTransferProofRequest{RequestId: uuid.New(), Amount: 1, ReceivedOn: date(2026, 11, 14)},
				http.StatusNotFound, "not_found", ""},
			{"an id that is no uuid", queue + "/transfer_proof/bukti-1/match", apigen.MatchTransferProofRequest{RequestId: uuid.New(), Amount: 1, ReceivedOn: date(2026, 11, 14)},
				http.StatusNotFound, "not_found", ""},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := admin.do(http.MethodPost, tc.path, tc.body)
				requireAPIError(t, resp, body, tc.status, tc.code, tc.field)
			})
		}
		resp, body := karyawan.do(http.MethodPost, matchPath(other.ID), apigen.MatchTransferProofRequest{RequestId: uuid.New(), Amount: 1, ReceivedOn: date(2026, 11, 14)})
		requireAPIError(t, resp, body, http.StatusForbidden, "forbidden", "")
	})

	t.Run("applyPhoneChange answers 200, then check_item_resolved, and phone_taken for a number in use", func(t *testing.T) {
		path := queue + "/phone_changed/" + budiNumber.ID.String() + "/apply"
		resp, body := admin.do(http.MethodPost, path, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		res := decode[apigen.CheckResolution](t, body)
		require.Equal(t, apigen.CheckResolutionOutcomeApplied, res.Outcome)
		require.Equal(t, budiNumber.ID, res.ItemId)
		require.Nil(t, res.PaymentId)
		got, err := env.Tenants.Get(ctx, a.admin, budiID)
		require.NoError(t, err)
		require.Equal(t, "+6281299690001", got.Phone)

		resp, body = admin.do(http.MethodPost, path, nil)
		requireAPIError(t, resp, body, http.StatusConflict, "check_item_resolved", "")

		// Rina's new number was Budi's old one, which is free now: give it to Citra first.
		_, err = env.Tenants.Update(ctx, a.admin, citra.tenantID, tenant.Input{Name: "Citra Lestari", Phone: budi.Phone})
		require.NoError(t, err)
		resp, body = admin.do(http.MethodPost, queue+"/phone_changed/"+takenNumber.ID.String()+"/apply", nil)
		requireAPIError(t, resp, body, http.StatusConflict, "phone_taken", "phone")
		resp, body = admin.do(http.MethodPost, queue+"/phone_changed/"+uuid.NewString()+"/apply", nil)
		requireAPIError(t, resp, body, http.StatusNotFound, "not_found", "")
	})

	t.Run("dismissCheckItem answers 200 and each refusal with its code", func(t *testing.T) {
		path := queue + "/deposit_gap/" + gap.ID.String() + "/dismiss"
		resp, body := admin.do(http.MethodPost, path, apigen.DismissCheckItemRequest{Note: "  "})
		requireAPIError(t, resp, body, http.StatusBadRequest, "invalid_input", "note")
		resp, body = admin.do(http.MethodPost, queue+"/bukti_lain/"+gap.ID.String()+"/dismiss", apigen.DismissCheckItemRequest{Note: "Ditutup"})
		requireAPIError(t, resp, body, http.StatusNotFound, "not_found", "")
		resp, body = supervisor.do(http.MethodPost, path, apigen.DismissCheckItemRequest{Note: "Ditutup"})
		requireAPIError(t, resp, body, http.StatusForbidden, "forbidden", "")

		resp, body = admin.do(http.MethodPost, path, apigen.DismissCheckItemRequest{Note: "Selisih ditanggung karyawan"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		res := decode[apigen.CheckResolution](t, body)
		require.Equal(t, apigen.CheckResolutionOutcomeDismissed, res.Outcome)
		require.Equal(t, ptrTo("Selisih ditanggung karyawan"), res.Note)

		resp, body = admin.do(http.MethodPost, path, apigen.DismissCheckItemRequest{Note: "Lagi"})
		requireAPIError(t, resp, body, http.StatusConflict, "check_item_resolved", "")
	})

	t.Run("getCashDeposit shows how its gap was closed", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgA+"/cash-deposits/"+gap.ID.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		d := decode[apigen.CashDeposit](t, body)
		require.NotNil(t, d.GapClosure)
		require.Equal(t, "Selisih ditanggung karyawan", d.GapClosure.Note)
		require.Equal(t, apigen.CheckStaff{StaffUserId: a.o.Admin.UserID, Name: a.o.Admin.Name}, d.GapClosure.ClosedBy)
		require.True(t, d.GapClosure.ClosedAt.Equal(env.Clock.Now()))
	})

	t.Run("the queue no longer lists what was answered", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, queue, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		page := decode[apigen.CheckQueuePage](t, body)
		ids := make([]uuid.UUID, len(page.Items))
		for i, it := range page.Items {
			ids[i] = it.Id
		}
		// Some arrived at the same instant of the test clock, where the id breaks the tie, so this
		// checks which items are left; the order is checked in the checkqueue package.
		require.ElementsMatch(t, []uuid.UUID{pending.ID, receipt.Upload.ID, attempt.ID, takenNumber.ID, other.ID}, ids)
	})
}
