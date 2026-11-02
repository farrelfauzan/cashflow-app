//go:build integration

package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// TestMoneyOverHTTP covers recordPayment, createCorrection and getTenantAccount end to end.
func TestMoneyOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	o := env.CreateOrg(t, "Griya Sejahtera", "0812951")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing,
		CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)
	karyawan := newClient(t, srv.URL)
	karyawan.signIn(env, o.Karyawan.Phone)

	resp, body := admin.do(http.MethodPost, orgPath+"/properties", apigen.PropertyInput{Name: "Kos Melati", Kind: apigen.Kos})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	melati := decode[apigen.Property](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/properties/"+melati.Id.String()+"/units", apigen.UnitInput{Label: "Kamar 1", Terms: monthly(700_000)})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	kamar1 := decode[apigen.Unit](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: "Rina Wulandari", Phone: "0812 9518 5001"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	rina := decode[apigen.Tenant](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: "Bambang Wijaya", Phone: "0812 9518 5002"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	bambang := decode[apigen.Tenant](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
		"tenantId": rina.Id, "unitId": kamar1.Id, "termMonths": 1, "startDate": "2026-11-01",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	_, err := env.Billing.GenerateOrg(context.Background(), o.ID)
	require.NoError(t, err)

	requireError := func(t *testing.T, resp result, body []byte, status int, code, field string) {
		t.Helper()
		require.Equal(t, status, resp.StatusCode, string(body))
		e := decode[apigen.Error](t, body)
		require.Equal(t, code, e.Error.Code)
		require.NotEmpty(t, e.Error.Message)
		if field == "" {
			require.Nil(t, e.Error.Field)
			return
		}
		require.NotNil(t, e.Error.Field)
		require.Equal(t, field, *e.Error.Field)
	}

	payment := map[string]any{
		"requestId": uuid.New(), "tenantId": rina.Id, "amount": 500_000, "method": "cash",
		"receivedOn": "2026-11-02", "note": "dibayar di warung",
	}
	var paid apigen.PaymentResult
	var invoiceID uuid.UUID

	t.Run("recordPayment answers 201 with what it paid, then 200 for the same request", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, orgPath+"/payments", payment)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		paid = decode[apigen.PaymentResult](t, body)
		require.Len(t, paid.Paid, 1)
		invoiceID = paid.Paid[0].InvoiceId
		require.EqualValues(t, 500_000, paid.Paid[0].Amount)
		require.EqualValues(t, 200_000, paid.Paid[0].Outstanding)
		require.Equal(t, "2026-11-01", paid.Paid[0].Period.String())
		require.EqualValues(t, 200_000, paid.Outstanding)
		require.Zero(t, paid.Credit)
		require.Equal(t, "2026-11-09", paid.OldestUnpaidDueDate.String(), "entered a day late: due 7 days after")
		require.Equal(t, apigen.PaymentMethodCash, paid.Payment.Method)
		require.Equal(t, "dibayar di warung", paid.Payment.Note)
		require.Equal(t, "2026-11-02", paid.Payment.ReceivedOn.String())
		require.NotNil(t, paid.Payment.RecordedBy)
		require.Equal(t, o.Admin.Name, paid.Payment.RecordedBy.Name)
		require.Contains(t, string(body), `"corrections":[]`, "an empty list, not null")

		resp, body = admin.do(http.MethodPost, orgPath+"/payments", payment)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, paid.Payment.Id, decode[apigen.PaymentResult](t, body).Payment.Id)
	})

	t.Run("recordPayment refuses bad input", func(t *testing.T) {
		with := func(key string, value any) map[string]any {
			out := map[string]any{}
			for k, v := range payment {
				out[k] = v
			}
			out["requestId"] = uuid.New()
			if value == nil {
				delete(out, key)
			} else {
				out[key] = value
			}
			return out
		}
		reused := with("amount", 600_000)
		reused["requestId"] = payment["requestId"]
		tests := []struct {
			name   string
			client *client
			body   any
			status int
			code   string
			field  string
		}{
			{"the same requestId for another payment", admin, reused, http.StatusConflict, "request_id_reused", "requestId"},
			{"an unknown field", admin, with("discount", 1), http.StatusBadRequest, "invalid_json", ""},
			{"a received date after today", admin, with("receivedOn", "2026-11-03"), http.StatusBadRequest, "invalid_input", "receivedOn"},
			{"a zero amount", admin, with("amount", 0), http.StatusBadRequest, "invalid_input", "amount"},
			{"qris recorded by hand", admin, with("method", "qris"), http.StatusBadRequest, "invalid_input", "method"},
			{"no requestId", admin, with("requestId", nil), http.StatusBadRequest, "invalid_input", "requestId"},
			{"a tenant not in the organization", admin, with("tenantId", uuid.New()), http.StatusNotFound, "not_found", "tenantId"},
			{"a karyawan may not record a payment", karyawan, with("amount", 1), http.StatusForbidden, "forbidden", ""},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := tc.client.do(http.MethodPost, orgPath+"/payments", tc.body)
				requireError(t, resp, body, tc.status, tc.code, tc.field)
			})
		}
	})

	correction := map[string]any{
		"requestId": uuid.New(), "amountDelta": -100_000, "reason": "Air mati seminggu",
	}

	t.Run("createCorrection answers 201, then 200 for the same request", func(t *testing.T) {
		correction["invoiceId"] = invoiceID
		resp, body := admin.do(http.MethodPost, orgPath+"/corrections", correction)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		res := decode[apigen.CorrectionResult](t, body)
		require.Equal(t, rina.Id, res.TenantId)
		require.EqualValues(t, -100_000, res.Correction.AmountDelta)
		require.Equal(t, invoiceID, *res.Correction.InvoiceId)
		require.Nil(t, res.Correction.PaymentId)
		require.Equal(t, o.Admin.Name, res.Correction.CreatedBy.Name)
		require.EqualValues(t, 100_000, res.Outstanding)

		resp, body = admin.do(http.MethodPost, orgPath+"/corrections", correction)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, res.Correction.Id, decode[apigen.CorrectionResult](t, body).Correction.Id)
	})

	t.Run("createCorrection refuses bad input", func(t *testing.T) {
		fresh := func(fields map[string]any) map[string]any {
			out := map[string]any{"requestId": uuid.New(), "amountDelta": -1, "reason": "salah hitung", "invoiceId": invoiceID}
			for k, v := range fields {
				if v == nil {
					delete(out, k)
				} else {
					out[k] = v
				}
			}
			return out
		}
		tests := []struct {
			name   string
			client *client
			body   any
			status int
			code   string
			field  string
		}{
			{"below zero on the invoice", admin, fresh(map[string]any{"amountDelta": -600_001}), http.StatusConflict, "correction_below_zero", "amountDelta"},
			{"below zero on the payment", admin, fresh(map[string]any{"invoiceId": nil, "paymentId": paid.Payment.Id, "amountDelta": -500_001}),
				http.StatusConflict, "correction_below_zero", "amountDelta"},
			{"the same requestId for another correction", admin, fresh(map[string]any{"requestId": correction["requestId"]}),
				http.StatusConflict, "request_id_reused", "requestId"},
			{"both an invoice and a payment", admin, fresh(map[string]any{"paymentId": paid.Payment.Id}), http.StatusBadRequest, "invalid_input", "invoiceId"},
			{"neither", admin, fresh(map[string]any{"invoiceId": nil}), http.StatusBadRequest, "invalid_input", "invoiceId"},
			{"a zero change", admin, fresh(map[string]any{"amountDelta": 0}), http.StatusBadRequest, "invalid_input", "amountDelta"},
			{"a blank reason", admin, fresh(map[string]any{"reason": "  "}), http.StatusBadRequest, "invalid_input", "reason"},
			{"an unknown field", admin, fresh(map[string]any{"note": "x"}), http.StatusBadRequest, "invalid_json", ""},
			{"an invoice not in the organization", admin, fresh(map[string]any{"invoiceId": uuid.New()}), http.StatusNotFound, "not_found", "invoiceId"},
			{"a payment not in the organization", admin, fresh(map[string]any{"invoiceId": nil, "paymentId": uuid.New()}), http.StatusNotFound, "not_found", "paymentId"},
			{"a karyawan may not correct", karyawan, fresh(nil), http.StatusForbidden, "forbidden", ""},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := tc.client.do(http.MethodPost, orgPath+"/corrections", tc.body)
				requireError(t, resp, body, tc.status, tc.code, tc.field)
			})
		}
	})

	t.Run("getTenantAccount answers invoices, payments and totals", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String()+"/account", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		acct := decode[apigen.TenantAccount](t, body)
		require.Equal(t, rina.Id, acct.TenantId)
		require.EqualValues(t, 100_000, acct.Outstanding)
		require.Zero(t, acct.Credit)
		require.Len(t, acct.Invoices, 1)
		inv := acct.Invoices[0]
		require.Equal(t, "Kamar 1", inv.UnitLabel)
		require.Equal(t, "Kos Melati", inv.PropertyName)
		require.EqualValues(t, 700_000, inv.Amount)
		require.EqualValues(t, 600_000, inv.Billed)
		require.EqualValues(t, 500_000, inv.Paid)
		require.Equal(t, apigen.InvoiceStatusPartial, inv.Status)
		require.Len(t, inv.Lines, 1)
		require.Equal(t, apigen.Rent, inv.Lines[0].Kind)
		require.Len(t, inv.Corrections, 1)
		require.Len(t, acct.Payments, 1)
		require.Equal(t, []apigen.PaymentAllocation{{InvoiceId: inv.Id, Amount: 500_000}}, acct.Payments[0].Allocations)
	})

	t.Run("an account with nothing in it has empty lists and no oldest due date", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgPath+"/tenants/"+bambang.Id.String()+"/account", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"invoices":[]`)
		require.Contains(t, string(body), `"payments":[]`)
		require.NotContains(t, string(body), "oldestUnpaidDueDate")
		require.Contains(t, string(body), `"overdue":0`)
	})

	t.Run("getTenantAccount: 404 for an unknown tenant, 403 for a karyawan", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgPath+"/tenants/"+uuid.New().String()+"/account", nil)
		requireError(t, resp, body, http.StatusNotFound, "not_found", "")
		resp, body = admin.do(http.MethodGet, orgPath+"/tenants/not-a-uuid/account", nil)
		requireError(t, resp, body, http.StatusNotFound, "not_found", "")
		resp, body = karyawan.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String()+"/account", nil)
		requireError(t, resp, body, http.StatusForbidden, "forbidden", "")
	})

	t.Run("another organization's tenant is not found and nothing is written", func(t *testing.T) {
		other := env.CreateOrg(t, "Kontrakan Barokah", "0812952")
		outsider := newClient(t, srv.URL)
		outsider.signIn(env, other.Admin.Phone)
		otherPath := "/api/orgs/" + other.ID.String()

		resp, body := outsider.do(http.MethodGet, otherPath+"/tenants/"+rina.Id.String()+"/account", nil)
		requireError(t, resp, body, http.StatusNotFound, "not_found", "")
		in := map[string]any{"requestId": uuid.New(), "tenantId": rina.Id, "amount": 100_000, "method": "transfer", "receivedOn": "2026-11-02"}
		resp, body = outsider.do(http.MethodPost, otherPath+"/payments", in)
		requireError(t, resp, body, http.StatusNotFound, "not_found", "tenantId")
		resp, body = outsider.do(http.MethodPost, otherPath+"/corrections",
			map[string]any{"requestId": uuid.New(), "invoiceId": invoiceID, "amountDelta": -1, "reason": "x"})
		requireError(t, resp, body, http.StatusNotFound, "not_found", "invoiceId")
		resp, body = outsider.do(http.MethodPost, otherPath+"/corrections",
			map[string]any{"requestId": uuid.New(), "paymentId": paid.Payment.Id, "amountDelta": -1, "reason": "x"})
		requireError(t, resp, body, http.StatusNotFound, "not_found", "paymentId")

		resp, body = admin.do(http.MethodGet, orgPath+"/tenants/"+rina.Id.String()+"/account", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		acct := decode[apigen.TenantAccount](t, body)
		require.Len(t, acct.Payments, 1)
		require.Len(t, acct.Invoices[0].Corrections, 1)
	})
}
