//go:build integration

package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// addLease lets one more room of lo's kos to an existing tenant from today, collected by collector
// (a member id or uuid.Nil), generates its first invoice and returns that invoice's id.
func (lo *linkOrg) addLease(t *testing.T, tenantID, collector uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	lo.n++
	unit, err := lo.env.Properties.CreateUnit(ctx, lo.admin, lo.kos.ID, property.UnitInput{
		Label: fmt.Sprintf("Kamar %d", lo.n), Terms: testenv.Monthly(500_000),
	})
	require.NoError(t, err)
	lease, err := lo.env.Tenants.CreateLease(ctx, lo.admin, tenant.NewLease{
		TenantID: tenantID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: tenant.Today(lo.env.Clock), CollectorMemberID: collector},
	})
	require.NoError(t, err)
	_, err = lo.env.Billing.GenerateOrg(ctx, lo.o.ID)
	require.NoError(t, err)
	acct, err := lo.env.Billing.Account(ctx, lo.admin, tenantID)
	require.NoError(t, err)
	for _, inv := range acct.Invoices {
		if inv.LeaseID == lease.ID {
			return inv.ID
		}
	}
	t.Fatalf("no invoice for the new lease")
	return uuid.Nil
}

// TestStaffPromisesOverHTTP covers the promise fields of getTenantAccount (Detail penyewa) and
// listInvoices (Penagihan). The clock starts at 2 Nov 2026, 09:00 in Asia/Jakarta; every lease is
// monthly from the day it is made.
func TestStaffPromisesOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	srv := newLinkServer(t, env, &lockedBuffer{})
	caller := &qrisCaller{base: srv.URL}

	a := newLinkOrg(t, env, "Griya Sejahtera", "0812981")
	b := newLinkOrg(t, env, "Kos Bulanan Sejahtera", "0812982")
	rina := a.rentWithLink(t, "Rina Wulandari")
	sari := a.rentWithLink(t, "Sari Puspita")
	budiID, budiKaryawanInvoice := a.rent(t, "Budi Santoso", a.karyawan.MembershipID)
	budiOtherInvoice := a.addLease(t, budiID, uuid.Nil) // not collected by the karyawan
	dewi := b.rentWithLink(t, "Dewi Lestari")

	admin, karyawan, adminB := newClient(t, srv.URL), newClient(t, srv.URL), newClient(t, srv.URL)
	admin.signIn(env, a.o.Admin.Phone)
	karyawan.signIn(env, a.o.Karyawan.Phone)
	adminB.signIn(env, b.o.Admin.Phone)

	account := func(t *testing.T, c *client, org, tenantID uuid.UUID) (apigen.TenantAccount, string) {
		t.Helper()
		resp, body := c.do(http.MethodGet, "/api/orgs/"+org.String()+"/tenants/"+tenantID.String()+"/account", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		return decode[apigen.TenantAccount](t, body), string(body)
	}
	page := func(t *testing.T, token string) apigen.PaymentPage {
		t.Helper()
		resp, body := getPage(t, srv.URL, token, caller.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		return decode[apigen.PaymentPage](t, body)
	}
	// rows lists Penagihan for November with every status, by invoice id.
	rows := func(t *testing.T, c *client, org uuid.UUID, period string) map[uuid.UUID]apigen.InvoiceListItem {
		t.Helper()
		resp, body := c.do(http.MethodGet, "/api/orgs/"+org.String()+"/invoices?pageSize=100&period="+period, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		out := map[uuid.UUID]apigen.InvoiceListItem{}
		for _, it := range decode[apigen.InvoiceList](t, body).Items {
			out[it.Id] = it
		}
		return out
	}
	// summary is Penagihan's November summary as c sees it.
	summary := func(t *testing.T, c *client, org uuid.UUID) apigen.InvoiceListSummary {
		t.Helper()
		resp, body := c.do(http.MethodGet, "/api/orgs/"+org.String()+"/invoices?period=2026-11", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		return decode[apigen.InvoiceList](t, body).Summary
	}
	requireSameOffer := func(t *testing.T, acct apigen.TenantAccount, p apigen.PaymentPage) {
		t.Helper()
		require.Equal(t, p.PromiseDates, acct.PromiseDates, "promiseDates as on the payment page")
		require.Equal(t, p.NextInvoiceDueDate, acct.NextInvoiceDueDate, "nextInvoiceDueDate as on the payment page")
	}

	t.Run("a tenant without a promise", func(t *testing.T) {
		acct, raw := account(t, admin, a.o.ID, rina.tenantID)
		require.Contains(t, raw, `"activePromise":null`, "the field is there, null")
		require.Nil(t, acct.ActivePromise)
		require.Zero(t, acct.BrokenPromises)
		require.Equal(t, &apigen.PromiseDates{Earliest: date(2026, 11, 3), Latest: date(2027, 1, 1)}, acct.PromiseDates)
		require.Equal(t, ptrTo(date(2026, 12, 2)), acct.NextInvoiceDueDate)
		requireSameOffer(t, acct, page(t, rina.token))

		row := rows(t, admin, a.o.ID, "2026-11")[rina.invoiceID]
		require.Nil(t, row.Promise)
		require.Zero(t, row.BrokenPromises)
	})

	t.Run("a promise made on the payment page has no member", func(t *testing.T) {
		resp, body := caller.post(t, rina.token, "/promise", `{"kind":"specific_date","promisedDate":"2026-11-10","payingNow":100000}`)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		acct, _ := account(t, admin, a.o.ID, rina.tenantID)
		got := acct.ActivePromise
		require.NotNil(t, got)
		require.Equal(t, 1, got.Version)
		require.Equal(t, apigen.SpecificDate, got.Kind)
		require.Equal(t, date(2026, 11, 10), got.PromisedDate)
		require.Nil(t, got.PromisedAmount)
		require.Equal(t, int64(100_000), got.PayingNow)
		require.Equal(t, apigen.PromiseFromTenantPage, got.Source)
		require.Nil(t, got.CreatedBy)
		require.Nil(t, got.Note)
		require.True(t, env.Clock.Now().Equal(got.CreatedAt))
		require.Equal(t, []uuid.UUID{rina.invoiceID}, got.CoveredInvoiceIds)
		requireSameOffer(t, acct, page(t, rina.token))
	})

	var rinaPromise apigen.ActivePromise
	t.Run("an admin's edit shows the newest version, its member and note", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, "/api/orgs/"+a.o.ID.String()+"/tenants/"+rina.tenantID.String()+"/promises", map[string]any{
			"kind": "specific_date", "promisedDate": "2026-11-20", "promisedAmount": 300_000, "note": "Janji lewat telepon",
		})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		made := decode[apigen.TenantPromise](t, body)

		acct, _ := account(t, admin, a.o.ID, rina.tenantID)
		got := acct.ActivePromise
		require.NotNil(t, got)
		require.Equal(t, made.Id, got.Id)
		require.Equal(t, 2, got.Version)
		require.Equal(t, date(2026, 11, 20), got.PromisedDate)
		require.Equal(t, ptrTo(int64(300_000)), got.PromisedAmount)
		require.Equal(t, apigen.PromiseFromAdmin, got.Source)
		require.Equal(t, ptrTo("Janji lewat telepon"), got.Note)
		require.NotNil(t, got.CreatedBy)
		require.Equal(t, a.o.Admin.Name, got.CreatedBy.Name)
		requireSameOffer(t, acct, page(t, rina.token))
		rinaPromise = *got
	})

	// A lease added after the promise bills a November invoice the promise does not cover.
	rinaNewer := a.addLease(t, rina.tenantID, uuid.Nil)

	t.Run("Penagihan shows the promise only on the invoices it covers", func(t *testing.T) {
		got := rows(t, admin, a.o.ID, "2026-11")
		covered, newer := got[rina.invoiceID], got[rinaNewer]
		require.NotNil(t, covered.Promise)
		require.Equal(t, rinaPromise, *covered.Promise)
		require.Equal(t, rina.tenantID, newer.TenantId)
		require.Nil(t, newer.Promise, "a newer invoice the promise does not cover")
		require.Nil(t, got[sari.invoiceID].Promise, "a tenant without a promise")

		sum := summary(t, admin, a.o.ID)
		require.Equal(t, 3, sum.TenantsOutstanding, "Rina, Sari and Budi owe")
		require.Equal(t, 1, sum.TenantsWithPromise, "only Rina has a promise")
	})

	t.Run("a karyawan sees only their rows, and only the covered invoices they may see", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, "/api/orgs/"+a.o.ID.String()+"/tenants/"+budiID.String()+"/promises", map[string]any{
			"kind": "with_next_invoice",
		})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		all := rows(t, admin, a.o.ID, "2026-11")
		require.NotNil(t, all[budiOtherInvoice].Promise)
		require.ElementsMatch(t, []uuid.UUID{budiKaryawanInvoice, budiOtherInvoice}, all[budiOtherInvoice].Promise.CoveredInvoiceIds)

		mine := rows(t, karyawan, a.o.ID, "2026-11")
		require.Len(t, mine, 1, "only the invoice of the lease they collect")
		row, ok := mine[budiKaryawanInvoice]
		require.True(t, ok)
		require.NotNil(t, row.Promise)
		require.Equal(t, []uuid.UUID{budiKaryawanInvoice}, row.Promise.CoveredInvoiceIds,
			"the other lease's invoice is not shown to them")

		require.Equal(t, 2, summary(t, admin, a.o.ID).TenantsWithPromise, "Rina and Budi")
		sum := summary(t, karyawan, a.o.ID)
		require.Equal(t, 1, sum.TenantsOutstanding, "the karyawan sees only Budi")
		require.Equal(t, 1, sum.TenantsWithPromise)
	})

	t.Run("another organization's promises never appear", func(t *testing.T) {
		resp, body := caller.post(t, dewi.token, "/promise", `{"kind":"specific_date","promisedDate":"2026-11-15"}`)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		resp, body = admin.do(http.MethodGet, "/api/orgs/"+a.o.ID.String()+"/tenants/"+dewi.tenantID.String()+"/account", nil)
		requireAPIError(t, resp, body, http.StatusNotFound, "not_found", "")
		for _, row := range rows(t, admin, a.o.ID, "2026-11") {
			require.NotEqual(t, dewi.tenantID, row.TenantId)
			if row.Promise != nil {
				require.NotContains(t, row.Promise.CoveredInvoiceIds, dewi.invoiceID)
			}
		}

		acct, _ := account(t, adminB, b.o.ID, dewi.tenantID)
		require.NotNil(t, acct.ActivePromise)
		require.Equal(t, []uuid.UUID{dewi.invoiceID}, acct.ActivePromise.CoveredInvoiceIds)
		bRows := rows(t, adminB, b.o.ID, "2026-11")
		require.Len(t, bRows, 1)
		require.NotNil(t, bRows[dewi.invoiceID].Promise)
		require.Equal(t, acct.ActivePromise, bRows[dewi.invoiceID].Promise)
	})

	t.Run("a promise closed broken is counted", func(t *testing.T) {
		resp, body := admin.do(http.MethodPost, "/api/orgs/"+a.o.ID.String()+"/tenants/"+sari.tenantID.String()+"/promises", map[string]any{
			"kind": "specific_date", "promisedDate": "2026-11-05",
		})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		env.Clock.Set(time.Date(2026, 11, 6, 9, 0, 0, 0, clock.Jakarta))
		_, err := env.Promises.CloseDue(ctx, a.o.ID)
		require.NoError(t, err)

		acct, _ := account(t, admin, a.o.ID, sari.tenantID)
		require.Nil(t, acct.ActivePromise, "the broken promise is no longer active")
		require.Equal(t, 1, acct.BrokenPromises)
		requireSameOffer(t, acct, page(t, sari.token))
		require.NotNil(t, acct.PromiseDates, "a new promise can be made")

		got := rows(t, admin, a.o.ID, "2026-11")
		require.Equal(t, 1, got[sari.invoiceID].BrokenPromises)
		require.Nil(t, got[sari.invoiceID].Promise)
		require.Zero(t, got[rina.invoiceID].BrokenPromises, "the count is the row tenant's")
		require.NotNil(t, got[rina.invoiceID].Promise, "rina's promise, due 20 Nov, is still active")
	})

	t.Run("at the 60-day bound no promise can be made, as on the payment page", func(t *testing.T) {
		// Rina's promise was first made on 2 Nov; on 2 Jan its bound (1 Jan) is before tomorrow.
		env.Clock.Set(time.Date(2027, 1, 2, 9, 0, 0, 0, clock.Jakarta))
		admin := newClient(t, srv.URL) // the 30-day session has ended
		admin.signIn(env, a.o.Admin.Phone)
		acct, raw := account(t, admin, a.o.ID, rina.tenantID)
		require.NotNil(t, acct.ActivePromise)
		require.Nil(t, acct.PromiseDates)
		require.Nil(t, acct.NextInvoiceDueDate)
		require.Contains(t, raw, `"promiseDates":null`)
		requireSameOffer(t, acct, page(t, rina.token))
	})
}
