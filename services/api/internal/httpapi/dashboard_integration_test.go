//go:build integration

package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// TestDashboardOverHTTP covers getDashboard and listInvoices end to end.
func TestDashboardOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 1, 9, 0, 0, 0, clock.Jakarta))
	o := env.CreateOrg(t, "Griya Sejahtera", "0812971")

	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing,
		CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	admin := newClient(t, srv.URL)
	admin.signIn(env, o.Admin.Phone)
	supervisor := newClient(t, srv.URL)
	supervisor.signIn(env, o.Supervisor.Phone)
	karyawan := newClient(t, srv.URL)
	karyawan.signIn(env, o.Karyawan.Phone)

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

	t.Run("an organization with no invoice answers zeros and empty lists", func(t *testing.T) {
		resp, body := supervisor.do(http.MethodGet, orgPath+"/dashboard", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"arrears":[]`)
		require.Contains(t, string(body), `"recentCorrections":[]`)
		d := decode[apigen.Dashboard](t, body)
		require.Equal(t, "2026-11", d.Period, "this month in Jakarta")
		require.Zero(t, d.Billed)

		resp, body = admin.do(http.MethodGet, orgPath+"/invoices", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"items":[]`)
		require.NotContains(t, string(body), "oldestOverdue", "absent when nothing is overdue")
		l := decode[apigen.InvoiceList](t, body)
		require.Equal(t, 1, l.Page)
		require.Equal(t, 20, l.PageSize)
		require.Zero(t, l.Total)
	})

	resp, body := admin.do(http.MethodPost, orgPath+"/properties", apigen.PropertyInput{Name: "Kos Melati", Kind: apigen.Kos})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	melati := decode[apigen.Property](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/properties/"+melati.Id.String()+"/units", apigen.UnitInput{Label: "Kamar 7", Terms: monthly(750_000)})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	kamar := decode[apigen.Unit](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/tenants", apigen.TenantInput{Name: "Wulan Sari", Phone: "0812 9718 5001"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	wulan := decode[apigen.Tenant](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/leases", map[string]any{
		"tenantId": wulan.Id, "unitId": kamar.Id, "termMonths": 1, "startDate": "2026-11-01",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	_, err := env.Billing.GenerateOrg(context.Background(), o.ID)
	require.NoError(t, err)
	resp, body = admin.do(http.MethodPost, orgPath+"/payments", map[string]any{
		"requestId": uuid.New(), "tenantId": wulan.Id, "amount": 250_000, "method": "cash", "receivedOn": "2026-11-01",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	paid := decode[apigen.PaymentResult](t, body)
	resp, body = admin.do(http.MethodPost, orgPath+"/corrections", map[string]any{
		"requestId": uuid.New(), "invoiceId": paid.Paid[0].InvoiceId, "amountDelta": -50_000, "reason": "lampu kamar rusak",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	env.Clock.Set(time.Date(2026, 11, 3, 9, 0, 0, 0, clock.Jakarta))

	t.Run("getDashboard", func(t *testing.T) {
		resp, body := supervisor.do(http.MethodGet, orgPath+"/dashboard?period=2026-11", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		d := decode[apigen.Dashboard](t, body)
		require.Equal(t, apigen.Dashboard{
			Period: "2026-11", Billed: 700_000, Collected: 250_000, Outstanding: 450_000, InvoiceCount: 1, TenantsOutstanding: 1,
			Overdue: 450_000, TenantsInArrears: 1,
			Arrears: []apigen.TenantArrears{{
				TenantId: wulan.Id, TenantName: "Wulan Sari", Units: []apigen.UnitRef{{PropertyName: "Kos Melati", UnitLabel: "Kamar 7"}},
				Overdue: 450_000, OldestUnpaidDueDate: date(2026, 11, 1), AgeDays: 2,
			}},
			RecentCorrections: d.RecentCorrections,
		}, d)
		require.Len(t, d.RecentCorrections, 1)
		c := d.RecentCorrections[0]
		require.Equal(t, wulan.Id, c.TenantId)
		require.Equal(t, "Wulan Sari", c.TenantName)
		require.Equal(t, paid.Paid[0].InvoiceId, *c.InvoiceId)
		require.Equal(t, "2026-11-01", c.Period.String())
		require.Equal(t, &apigen.UnitRef{PropertyName: "Kos Melati", UnitLabel: "Kamar 7"}, c.Unit)
		require.Nil(t, c.PaymentId)
		require.Nil(t, c.ReceivedOn)
		require.NotContains(t, string(body), "paymentId")
		require.EqualValues(t, -50_000, c.AmountDelta)
		require.Equal(t, "lampu kamar rusak", c.Reason)
		require.Equal(t, o.Admin.Name, c.CreatedBy.Name)
	})

	t.Run("listInvoices", func(t *testing.T) {
		resp, body := admin.do(http.MethodGet, orgPath+"/invoices?status=overdue&sort=-outstanding&q=wulan&propertyId="+melati.Id.String()+"&page=1&pageSize=5", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		l := decode[apigen.InvoiceList](t, body)
		require.Equal(t, 1, l.Page)
		require.Equal(t, 5, l.PageSize)
		require.Equal(t, 1, l.Total)
		require.Len(t, l.Items, 1)
		it := l.Items[0]
		require.Equal(t, apigen.InvoiceListItem{
			Id: paid.Paid[0].InvoiceId, Kind: apigen.Regular, TenantId: wulan.Id, TenantName: "Wulan Sari", LeaseId: it.LeaseId,
			PropertyId: melati.Id, PropertyName: "Kos Melati", UnitLabel: "Kamar 7",
			Period: date(2026, 11, 1), PeriodEnd: date(2026, 11, 30), DueDate: date(2026, 11, 1), IssuedOn: date(2026, 11, 1),
			Amount: 750_000, Billed: 700_000, Paid: 250_000, Outstanding: 450_000, Status: apigen.InvoiceStatusPartial,
			OverdueDays: 2, PastPlannedEnd: false, Lines: it.Lines,
		}, it)
		require.Len(t, it.Lines, 1)
		require.Equal(t, apigen.InvoiceLineKind("rent"), it.Lines[0].Kind)
		require.EqualValues(t, 750_000, it.Lines[0].Amount)
		require.Equal(t, apigen.InvoiceListSummary{
			Billed: 700_000, Paid: 250_000, Outstanding: 450_000, TenantsOutstanding: 1,
			OldestOverdue: &apigen.OldestOverdue{
				TenantId: wulan.Id, TenantName: "Wulan Sari", PropertyName: "Kos Melati", UnitLabel: "Kamar 7",
				OldestUnpaidDueDate: date(2026, 11, 1), AgeDays: 2,
			},
		}, l.Summary)

		resp, body = admin.do(http.MethodGet, orgPath+"/invoices?period=2026-10", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		l = decode[apigen.InvoiceList](t, body)
		require.Empty(t, l.Items)
		require.Contains(t, string(body), `"items":[]`)
		require.NotNil(t, l.Summary.OldestOverdue, "the oldest debt is of any month")
	})

	t.Run("bad parameters answer 400 on their field", func(t *testing.T) {
		tests := []struct {
			name, path, field string
		}{
			{"dashboard period", "/dashboard?period=2026-13", "period"},
			{"dashboard period text", "/dashboard?period=november", "period"},
			{"period", "/invoices?period=2026-1", "period"},
			{"status", "/invoices?status=lunas", "status"},
			{"sort", "/invoices?sort=amount", "sort"},
			{"page", "/invoices?page=0", "page"},
			{"pageSize", "/invoices?pageSize=101", "pageSize"},
			{"propertyId", "/invoices?propertyId=kos", "propertyId"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := admin.do(http.MethodGet, orgPath+tc.path, nil)
				requireError(t, resp, body, http.StatusBadRequest, "invalid_input", tc.field)
			})
		}
	})

	t.Run("a member without report:read gets 403", func(t *testing.T) {
		resp, body := karyawan.do(http.MethodGet, orgPath+"/dashboard", nil)
		requireError(t, resp, body, http.StatusForbidden, "forbidden", "")
		resp, body = karyawan.do(http.MethodGet, orgPath+"/invoices", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Zero(t, decode[apigen.InvoiceList](t, body).Total, "only the invoices of tenants assigned to them")
	})
}

func date(y int, m time.Month, d int) openapi_types.Date {
	return openapi_types.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}
