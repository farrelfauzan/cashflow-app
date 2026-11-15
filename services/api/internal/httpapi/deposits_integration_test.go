//go:build integration

package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/deposit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// TestCashDepositsOverHTTP covers "Setor tunai": the karyawan lists the cash they hold and
// deposits it (listMyUndepositedCash, recordCashDeposit, listMyCashDeposits), and the admin counts
// it (listCashDeposits, getCashDeposit, listCashDepositPayments, confirmCashDeposit). The clock is
// 14 Nov 2026 in Asia/Jakarta.
func TestCashDepositsOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 14, 9, 0, 0, 0, clock.Jakarta))
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing,
		Deposits:     deposit.NewService(env.Store, env.Authz, env.Clock, env.Log),
		CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)
	a := newLinkOrg(t, env, "Griya Setoran", "0812994")
	wati, _ := a.rent(t, "Wati Susanti", a.karyawan.MembershipID)
	bambang, _ := a.rent(t, "Bambang Wijaya", a.karyawan.MembershipID)

	admin, karyawan := newClient(t, srv.URL), newClient(t, srv.URL)
	admin.signIn(env, a.o.Admin.Phone)
	karyawan.signIn(env, a.o.Karyawan.Phone)
	orgA := "/api/orgs/" + a.o.ID.String()
	for _, c := range []struct {
		tenant uuid.UUID
		amount int64
	}{{wati, 300_000}, {bambang, 200_000}} {
		resp, body := karyawan.do(http.MethodPost, orgA+"/tenants/"+c.tenant.String()+"/cash-payments",
			apigen.CashPaymentInput{RequestId: uuid.New(), Amount: c.amount})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		env.Clock.Set(env.Clock.Now().Add(30 * time.Minute))
	}

	t.Run("listMyUndepositedCash lists the cash held, oldest first", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodGet, orgA+"/field/cash/payments?pageSize=1", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		list := decode[apigen.UndepositedCashList](t, body)
		require.Equal(t, 2, list.Total)
		require.Equal(t, 1, list.PageSize)
		require.Len(t, list.Items, 1)
		require.Equal(t, wati, list.Items[0].TenantId)
		require.EqualValues(t, 300_000, list.Items[0].Amount)
		require.Equal(t, []apigen.FieldUnit{{UnitLabel: "Kamar 1", PropertyName: "Kos Griya Setoran"}}, list.Items[0].Units)
		require.Equal(t, apigen.ReceiptPhoto{Status: apigen.PhotoMissing}, list.Items[0].Photo)

		resp, body = admin.do(http.MethodGet, orgA+"/field/cash/payments", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
	})

	in := apigen.CashDepositInput{RequestId: uuid.New(), ExpectedAmount: 500_000, ExpectedCount: 2, Amount: 490_000,
		Note: ptrTo("Kurang Rp10.000")}
	var recorded apigen.CashDeposit

	t.Run("recordCashDeposit answers 201, then 200 for the same request", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodPost, orgA+"/field/cash-deposits", in)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		recorded = decode[apigen.CashDeposit](t, body)
		require.Equal(t, apigen.CashDepositStatusDepositPending, recorded.Status)
		require.Nil(t, recorded.Confirmation)
		require.EqualValues(t, 490_000, recorded.Amount)
		require.EqualValues(t, 500_000, recorded.ExpectedAmount)
		require.Equal(t, 2, recorded.PaymentCount)
		require.Equal(t, a.o.Karyawan.UserID, recorded.StaffUserId)
		require.NotContains(t, string(body), `"confirmation"`, "absent while pending")

		resp, body = karyawan.do(http.MethodPost, orgA+"/field/cash-deposits", in)
		require.Equal(t, http.StatusOK, resp.StatusCode, "a retry records nothing: %s", body)
		require.Equal(t, recorded.Id, decode[apigen.CashDeposit](t, body).Id)

		resp, body = karyawan.do(http.MethodGet, orgA+"/field/cash", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, apigen.CashTotal{}, decode[apigen.CashTotal](t, body), "nothing held any more")

		resp, body = karyawan.do(http.MethodGet, orgA+"/field/cash-deposits", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		mine := decode[apigen.CashDepositList](t, body)
		require.Equal(t, 1, mine.Total)
		require.Equal(t, recorded.Id, mine.Items[0].Id)
	})

	t.Run("recordCashDeposit refuses stale or reused requests with their codes", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodPost, orgA+"/tenants/"+wati.String()+"/cash-payments",
			apigen.CashPaymentInput{RequestId: uuid.New(), Amount: 150_000})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		for _, tc := range []struct {
			name   string
			body   any
			status int
			code   string
			field  string
		}{
			{name: "the cash changed", body: apigen.CashDepositInput{RequestId: uuid.New(), ExpectedAmount: 100_000, ExpectedCount: 1, Amount: 100_000},
				status: http.StatusConflict, code: "deposit_cash_changed"},
			{name: "a requestId used for another deposit", body: apigen.CashDepositInput{RequestId: in.RequestId, ExpectedAmount: 150_000, ExpectedCount: 1, Amount: 150_000},
				status: http.StatusConflict, code: "request_id_reused", field: "requestId"},
			{name: "an unknown photo", body: apigen.CashDepositInput{RequestId: uuid.New(), ExpectedAmount: 150_000, ExpectedCount: 1, Amount: 150_000, PhotoUploadId: ptrTo(uuid.New())},
				status: http.StatusNotFound, code: "not_found", field: "photoUploadId"},
			{name: "no payment expected", body: apigen.CashDepositInput{RequestId: uuid.New(), Amount: 150_000},
				status: http.StatusBadRequest, code: "invalid_input", field: "expectedCount"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := karyawan.do(http.MethodPost, orgA+"/field/cash-deposits", tc.body)
				require.Equal(t, tc.status, resp.StatusCode, string(body))
				requireError(t, body, tc.code, tc.field)
			})
		}
	})

	t.Run("the admin reads and counts the deposit", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgA+"/cash-deposits?status=pending", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		pending := decode[apigen.CashDepositList](t, body)
		require.Equal(t, 1, pending.Total)
		require.Equal(t, recorded.Id, pending.Items[0].Id)

		path := orgA + "/cash-deposits/" + recorded.Id.String()
		resp, body = admin.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, recorded.Id, decode[apigen.CashDeposit](t, body).Id)

		resp, body = admin.do(http.MethodGet, path+"/payments", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		payments := decode[apigen.CashDepositPaymentList](t, body)
		require.Equal(t, 2, payments.Total)
		require.Equal(t, []uuid.UUID{wati, bambang}, []uuid.UUID{payments.Items[0].TenantId, payments.Items[1].TenantId})

		count := apigen.CashDepositConfirmationInput{ReceivedAmount: 490_000, Note: ptrTo("Dihitung di kantor")}
		resp, body = admin.do(http.MethodPost, path+"/confirmation", count)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		d := decode[apigen.CashDeposit](t, body)
		require.Equal(t, apigen.CashDepositStatusDepositGap, d.Status)
		require.NotNil(t, d.Confirmation)
		require.EqualValues(t, -10_000, d.Confirmation.Gap)
		require.Equal(t, a.o.Admin.UserID, d.Confirmation.ConfirmedBy)

		resp, body = admin.do(http.MethodPost, path+"/confirmation", count)
		require.Equal(t, http.StatusOK, resp.StatusCode, "the same count again: %s", body)
		resp, body = admin.do(http.MethodPost, path+"/confirmation", apigen.CashDepositConfirmationInput{ReceivedAmount: 500_000})
		require.Equal(t, http.StatusConflict, resp.StatusCode, string(body))
		requireError(t, body, "deposit_already_confirmed", "")

		resp, body = karyawan.do(http.MethodPost, path+"/confirmation", count)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
		resp, body = admin.do(http.MethodPost, orgA+"/cash-deposits/"+uuid.NewString()+"/confirmation", count)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		requireError(t, body, "not_found", "")
	})

	t.Run("listCashDeposits filters from a whitelist", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgA+"/cash-deposits?status=confirmed&staffUserId="+a.o.Karyawan.UserID.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, 1, decode[apigen.CashDepositList](t, body).Total)

		resp, body = admin.do(http.MethodGet, orgA+"/cash-deposits?status=bogus", nil)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
		requireError(t, body, "invalid_input", "status")

		resp, body = admin.do(http.MethodGet, orgA+"/cash-deposits?staffUserId=nope", nil)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
		requireError(t, body, "invalid_input", "staffUserId")

		resp, body = karyawan.do(http.MethodGet, orgA+"/cash-deposits", nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
	})
}
