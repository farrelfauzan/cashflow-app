//go:build integration

package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/visit"
)

// TestVisitsOverHTTP covers "Kunjungan hari ini" and "Catat hasil kunjungan": listFieldVisits and
// recordVisitOutcome. Rooms are let on 1 Nov 2026, so each first invoice is due then; the visits
// happen on 14 Nov 2026 in Asia/Jakarta.
func TestVisitsOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 1, 9, 0, 0, 0, clock.Jakarta))
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing,
		Visits:       visit.NewService(env.Store, env.Billing, env.Clock),
		CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)
	a := newLinkOrg(t, env, "Griya Kunjungan", "0812995")
	wati, _ := a.rent(t, "Wati Susanti", a.karyawan.MembershipID)
	bambang, _ := a.rent(t, "Bambang Wijaya", a.karyawan.MembershipID)
	citra, _ := a.rent(t, "Citra Lestari", a.karyawan.MembershipID)
	openTask := func(tenantID uuid.UUID, reason string, at time.Time) {
		t.Helper()
		err := env.Store.WithOrgTx(context.Background(), a.o.ID, func(tx pgx.Tx) error {
			_, err := dbgen.New(tx).OpenVisitTask(context.Background(), dbgen.OpenVisitTaskParams{
				ID: uuid.New(), OrganizationID: a.o.ID, TenantID: tenantID, Reason: reason, CreatedAt: at,
			})
			return err
		})
		require.NoError(t, err)
	}
	openTask(wati, visit.TaskH7, time.Date(2026, 11, 8, 6, 0, 0, 0, clock.Jakarta))
	openTask(citra, visit.TaskUndeliverable, time.Date(2026, 11, 9, 6, 0, 0, 0, clock.Jakarta))
	env.Clock.Set(time.Date(2026, 11, 14, 9, 0, 0, 0, clock.Jakarta))

	admin, karyawan := newClient(t, srv.URL), newClient(t, srv.URL)
	admin.signIn(env, a.o.Admin.Phone)
	karyawan.signIn(env, a.o.Karyawan.Phone)
	orgA := "/api/orgs/" + a.o.ID.String()
	outcomePath := func(tenantID uuid.UUID) string {
		return orgA + "/field/tenants/" + tenantID.String() + "/visit-outcomes"
	}

	t.Run("listFieldVisits answers the open visits with their reasons", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodGet, orgA+"/field/visits?pageSize=1", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		list := decode[apigen.FieldVisitList](t, body)
		require.Equal(t, 2, list.Total)
		require.Equal(t, 1, list.Page)
		require.Equal(t, 1, list.PageSize)
		require.Len(t, list.Items, 1)
		row := list.Items[0]
		require.Equal(t, wati, row.TenantId)
		require.Equal(t, "Wati Susanti", row.TenantName)
		require.Equal(t, apigen.VisitOpen, row.Status)
		require.Equal(t, []apigen.VisitReason{{Code: apigen.VisitReasonCodeOverdue, DaysOverdue: ptrTo(13)}}, row.Reasons)
		require.EqualValues(t, 700_000, row.Outstanding)
		require.EqualValues(t, 700_000, row.Overdue)
		require.Equal(t, ptrTo(date(2026, 11, 1)), row.OldestUnpaidDueDate)
		require.Equal(t, []apigen.FieldUnit{{UnitLabel: "Kamar 1", PropertyName: "Kos Griya Kunjungan"}}, row.Units)
		require.Equal(t, []apigen.FieldCashPayment{}, row.UnlinkedCashToday)
		require.Nil(t, row.LastOutcome)
		require.Contains(t, string(body), `"unlinkedCashToday":[]`)
		require.NotContains(t, string(body), `"lastOutcome"`, "absent when nothing was recorded today")

		resp, body = karyawan.do(http.MethodGet, orgA+"/field/visits?page=2&pageSize=1", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		second := decode[apigen.FieldVisitList](t, body).Items
		require.Len(t, second, 1)
		require.Equal(t, citra, second[0].TenantId)
		require.Equal(t, []apigen.VisitReason{{Code: apigen.VisitReasonCodeUndeliverable}}, second[0].Reasons)

		resp, body = admin.do(http.MethodGet, orgA+"/field/visits", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Zero(t, decode[apigen.FieldVisitList](t, body).Total, "the admin collects from nobody")
	})

	t.Run("recordVisitOutcome answers 201, then 200 for the same request", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodPost, orgA+"/tenants/"+wati.String()+"/cash-payments",
			apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 700_000, PhotoUploadId: env.CashPhoto(t, a.o.ID, a.o.Karyawan.UserID, wati)})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		paymentID := decode[apigen.CashPaymentResult](t, body).Payment.Id

		resp, body = karyawan.do(http.MethodGet, orgA+"/field/visits", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		unlinked := decode[apigen.FieldVisitList](t, body).Items[0].UnlinkedCashToday
		require.Len(t, unlinked, 1)
		require.Equal(t, paymentID, unlinked[0].PaymentId)
		require.EqualValues(t, 700_000, unlinked[0].Amount)
		require.Equal(t, apigen.PhotoPending, unlinked[0].Photo.Status, "the photo recorded with the cash")

		in := apigen.VisitOutcomeInput{RequestId: uuid.New(), Outcome: apigen.VisitOutcomeKindPaid, PaymentId: &paymentID, Note: ptrTo("Lunas")}
		resp, body = karyawan.do(http.MethodPost, outcomePath(wati), in)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		got := decode[apigen.VisitOutcome](t, body)
		require.Equal(t, apigen.VisitOutcomeKindPaid, got.Outcome)
		require.Equal(t, wati, got.TenantId)
		require.Equal(t, &paymentID, got.PaymentId)
		require.Nil(t, got.PromiseVersionId)
		require.Nil(t, got.NewPhone)
		require.Equal(t, ptrTo("Lunas"), got.Note)
		require.Equal(t, []apigen.VisitReasonCode{apigen.VisitReasonCodeOverdue}, got.Resolved)

		resp, body = karyawan.do(http.MethodPost, outcomePath(wati), in)
		require.Equal(t, http.StatusOK, resp.StatusCode, "a retry records nothing: %s", body)
		require.Equal(t, got, decode[apigen.VisitOutcome](t, body))

		resp, body = karyawan.do(http.MethodGet, orgA+"/field/visits", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		list := decode[apigen.FieldVisitList](t, body)
		require.Equal(t, 2, list.Total)
		done := list.Items[1]
		require.Equal(t, wati, done.TenantId)
		require.Equal(t, apigen.VisitDone, done.Status)
		require.Equal(t, []apigen.VisitReason{}, done.Reasons)
		require.Nil(t, done.OldestUnpaidDueDate)
		require.NotNil(t, done.LastOutcome)
		require.Equal(t, got.Id, done.LastOutcome.Id)
		require.Equal(t, []apigen.FieldCashPayment{}, done.UnlinkedCashToday)

		in.Note = ptrTo("Lunas November")
		resp, body = karyawan.do(http.MethodPost, outcomePath(wati), in)
		require.Equal(t, http.StatusConflict, resp.StatusCode, string(body))
		requireError(t, body, "request_id_reused", "requestId")
	})

	t.Run("recordVisitOutcome refuses with its codes", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			tenant uuid.UUID
			body   any
			status int
			code   string
			field  string
		}{
			{name: "no open visit", tenant: bambang, body: apigen.VisitOutcomeInput{RequestId: uuid.New(), Outcome: apigen.VisitOutcomeKindNotHome},
				status: http.StatusConflict, code: "visit_not_open"},
			{name: "a payment that cannot back it", tenant: citra,
				body:   apigen.VisitOutcomeInput{RequestId: uuid.New(), Outcome: apigen.VisitOutcomeKindPaid, PaymentId: ptrTo(uuid.New())},
				status: http.StatusBadRequest, code: "visit_payment_invalid", field: "paymentId"},
			{name: "a promise that cannot back it", tenant: citra,
				body:   apigen.VisitOutcomeInput{RequestId: uuid.New(), Outcome: apigen.VisitOutcomeKindPromise, PromiseVersionId: ptrTo(uuid.New())},
				status: http.StatusBadRequest, code: "visit_promise_invalid", field: "promiseVersionId"},
			{name: "not a WhatsApp number", tenant: citra,
				body:   apigen.VisitOutcomeInput{RequestId: uuid.New(), Outcome: apigen.VisitOutcomeKindNumberChanged, NewPhone: ptrTo("021-5551234")},
				status: http.StatusBadRequest, code: "invalid_phone", field: "newPhone"},
			{name: "a missing payment", tenant: citra, body: apigen.VisitOutcomeInput{RequestId: uuid.New(), Outcome: apigen.VisitOutcomeKindPaid},
				status: http.StatusBadRequest, code: "invalid_input", field: "paymentId"},
			{name: "an unknown tenant", tenant: uuid.New(), body: apigen.VisitOutcomeInput{RequestId: uuid.New(), Outcome: apigen.VisitOutcomeKindNotHome},
				status: http.StatusNotFound, code: "not_found"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := karyawan.do(http.MethodPost, outcomePath(tc.tenant), tc.body)
				require.Equal(t, tc.status, resp.StatusCode, string(body))
				requireError(t, body, tc.code, tc.field)
			})
		}

		resp, body := admin.do(http.MethodPost, outcomePath(citra), apigen.VisitOutcomeInput{RequestId: uuid.New(), Outcome: apigen.VisitOutcomeKindNotHome})
		require.Equal(t, http.StatusForbidden, resp.StatusCode, "the admin has no visit:record: %s", body)
	})

	t.Run("number_changed answers the number in E.164", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodPost, outcomePath(citra), apigen.VisitOutcomeInput{
			RequestId: uuid.New(), Outcome: apigen.VisitOutcomeKindNumberChanged, NewPhone: ptrTo("0812-3456-7890"),
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		got := decode[apigen.VisitOutcome](t, body)
		require.Equal(t, ptrTo("+6281234567890"), got.NewPhone)
		require.Equal(t, []apigen.VisitReasonCode{apigen.VisitReasonCodeUndeliverable}, got.Resolved)
	})
}
