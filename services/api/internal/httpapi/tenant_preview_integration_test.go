//go:build integration

package httpapi_test

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// TestTenantPaymentPreviewOverHTTP covers previewTenantPayment: oldest invoice first, an amount
// above what is owed leaves credit, nothing is recorded, invoice:read through the tenant's
// leases, and another organization's tenant is not found.
func TestTenantPaymentPreviewOverHTTP(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	srv := newLinkServer(t, env, io.Discard)
	a := newLinkOrg(t, env, "Griya Pratinjau", "0812641")
	b := newLinkOrg(t, env, "Kos Lain", "0812642")
	// Rina rents since 2 Oct, entered today: October and November, 700.000 each.
	rina, rinaOct := a.rentSince(t, "Rina Wulandari", time.Date(2026, 10, 2, 0, 0, 0, 0, clock.Jakarta))
	dewi, _ := b.rent(t, "Dewi Lestari", uuid.Nil)

	admin, supervisor, karyawan := newClient(t, srv.URL), newClient(t, srv.URL), newClient(t, srv.URL)
	admin.signIn(env, a.o.Admin.Phone)
	supervisor.signIn(env, a.o.Supervisor.Phone)
	karyawan.signIn(env, a.o.Karyawan.Phone)
	path := func(org, tenantID uuid.UUID, amount string) string {
		return "/api/orgs/" + org.String() + "/tenants/" + tenantID.String() + "/payment-preview?amount=" + amount
	}
	preview := func(t *testing.T, c *client, amount string) apigen.TenantPaymentPreview {
		t.Helper()
		resp, body := c.do(http.MethodGet, path(a.o.ID, rina, amount), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		return decode[apigen.TenantPaymentPreview](t, body)
	}

	t.Run("an amount within what is owed pays October first, then November in part", func(t *testing.T) {
		got := preview(t, admin, "1000000")
		require.Equal(t, int64(1_400_000), got.OutstandingBefore)
		require.Equal(t, int64(400_000), got.OutstandingAfter)
		require.Zero(t, got.CreditAdded)
		require.Len(t, got.Items, 2)
		require.Equal(t, rinaOct, got.Items[0].InvoiceId)
		require.True(t, got.Items[0].PaidInFull)
		require.Equal(t, int64(300_000), got.Items[1].Applied)
		require.Equal(t, int64(400_000), got.Items[1].BalanceAfter)
		require.Equal(t, "2026-10-02", got.Items[0].Period.String())
	})

	t.Run("an amount above what is owed pays everything and leaves credit", func(t *testing.T) {
		got := preview(t, admin, "1500000")
		require.Zero(t, got.OutstandingAfter)
		require.Equal(t, int64(100_000), got.CreditAdded)
		require.Len(t, got.Items, 2)
	})

	t.Run("the Supervisor may preview; nothing is recorded", func(t *testing.T) {
		preview(t, supervisor, "700000")
		acct, err := env.Billing.Account(t.Context(), a.admin, rina)
		require.NoError(t, err)
		require.Equal(t, int64(1_400_000), acct.Outstanding)
		require.Empty(t, acct.Payments)
	})

	t.Run("refusals", func(t *testing.T) {
		tests := []struct {
			name   string
			c      *client
			path   string
			status int
			code   string
		}{
			{"no amount", admin, path(a.o.ID, rina, ""), http.StatusBadRequest, "invalid_input"},
			{"zero", admin, path(a.o.ID, rina, "0"), http.StatusBadRequest, "invalid_input"},
			{"not a whole number", admin, path(a.o.ID, rina, "1.5"), http.StatusBadRequest, "invalid_input"},
			{"a karyawan who does not collect from the tenant", karyawan, path(a.o.ID, rina, "1000"), http.StatusForbidden, "forbidden"},
			{"another organization's tenant", admin, path(a.o.ID, dewi, "1000"), http.StatusNotFound, "not_found"},
			{"an unknown tenant", admin, path(a.o.ID, uuid.New(), "1000"), http.StatusNotFound, "not_found"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := tc.c.do(http.MethodGet, tc.path, nil)
				require.Equal(t, tc.status, resp.StatusCode, string(body))
				require.Equal(t, tc.code, decode[apigen.Error](t, body).Error.Code)
			})
		}
	})
}
