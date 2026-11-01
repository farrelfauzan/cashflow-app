//go:build integration

package billing_test

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// rentIn creates a unit with a label in a property, a tenant and a monthly lease starting on
// start, saved today (the fake clock), as the admin.
func (w *world) rentIn(t *testing.T, prop property.Property, label, name string, start time.Time, price int64) rental {
	t.Helper()
	ctx := context.Background()
	w.n++
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, prop.ID, property.UnitInput{Label: label, Terms: testenv.Monthly(price)})
	require.NoError(t, err)
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	l, err := w.env.Tenants.CreateLease(ctx, w.admin, newLeaseIn(tn.ID, unit.ID, start))
	require.NoError(t, err)
	return rental{renter: renter{tenantID: tn.ID, leaseID: l.ID}, unitID: unit.ID}
}

// allInvoices reads every page of Penagihan for iq.
func allInvoices(t *testing.T, env *testenv.Env, p authz.Principal, iq billing.InvoiceQuery) []billing.InvoiceListItem {
	t.Helper()
	items, err := listing.All(func(page listing.Page) (listing.Result[billing.InvoiceListItem], error) {
		iq.Page = page
		res, err := env.Billing.ListInvoices(context.Background(), p, iq)
		return res.Result, err
	})
	require.NoError(t, err)
	return items
}

func tenantNames(items []billing.InvoiceListItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.TenantName
	}
	slices.Sort(out)
	return out
}

func arrearsNames(rows []billing.TenantArrears) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.TenantName
	}
	return out
}

func TestDashboardAndInvoiceList(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	// 10 Oct: Fajar starts in the kos; his October invoice will be the oldest debt.
	setNow(env, 2026, 10, 10, 9, 0)
	w := newWorld(t, env, "0812961")
	mawar, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kontrakan Mawar", Kind: property.KindKontrakan})
	require.NoError(t, err)
	fajar := w.rentIn(t, w.kos, "Kamar 10", "Fajar Nugroho", day(2026, 10, 10), 700_000)
	w.generate(t)

	// 28 Oct: Dewi is entered late, started 15 Sep: September due 28 Oct, October due 4 Nov.
	setNow(env, 2026, 10, 28, 9, 0)
	dewi := w.rentIn(t, w.kos, "Kamar 11", "Dewi Lestari", day(2026, 9, 15), 650_000)
	w.generate(t)

	// 1 Nov: five November invoices, all due today.
	setNow(env, 2026, 11, 1, 8, 0)
	rina := w.rentIn(t, w.kos, "Kamar 1", "Rina Wulandari", day(2026, 11, 1), 700_000)
	bambang := w.rentIn(t, w.kos, "Kamar 2", "Bambang Wijaya", day(2026, 11, 1), 700_000)
	w.rentIn(t, w.kos, "Kamar 3", "Citra Ayu", day(2026, 11, 1), 900_000)
	dodi := w.rentIn(t, mawar, "Rumah A", "Dodi Saputra", day(2026, 11, 1), 1_500_000)
	eko := w.rentIn(t, w.kos, "Kamar 4", "Eko Prasetyo", day(2026, 11, 1), 700_000)
	w.generate(t)
	paidRina := w.record(t, cash(rina.renter, 750_000, day(2026, 11, 1)))
	w.record(t, cash(bambang.renter, 300_000, day(2026, 11, 1)))
	w.correct(t, billing.CorrectionInput{RequestID: uuid.New(), PaymentID: paidRina.Payment.ID, AmountDelta: -50_000, Reason: "salah hitung kembalian"})
	ekoInv := w.invoices(t, eko.renter)[0]
	w.correct(t, billing.CorrectionInput{RequestID: uuid.New(), InvoiceID: ekoInv.InvoiceID, AmountDelta: -700_000, Reason: "kamar batal disewa"})

	nov := billing.InvoiceQuery{Period: "2026-11"}

	t.Run("overdue days around midnight in Jakarta", func(t *testing.T) {
		setNow(env, 2026, 11, 1, 23, 59)
		items := allInvoices(t, env, w.admin, nov)
		for _, it := range items {
			require.Zero(t, it.OverdueDays, "%s is due today, not debt yet", it.TenantName)
		}
		require.Equal(t, []string{"Dewi Lestari", "Fajar Nugroho"},
			tenantNames(allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: "2026-11", Status: billing.InvoiceStatusOverdue})),
			"only the older debts: November is due today")

		setNow(env, 2026, 11, 2, 0, 1)
		for _, it := range allInvoices(t, env, w.admin, nov) {
			want := 0
			if it.Outstanding > 0 {
				want = 1
			}
			require.Equal(t, want, it.OverdueDays, it.TenantName)
		}
		d, err := env.Billing.Dashboard(ctx, w.admin, "")
		require.NoError(t, err)
		last := d.Arrears[len(d.Arrears)-1]
		require.Equal(t, day(2026, 11, 1), last.OldestUnpaidDueDate)
		require.Equal(t, 1, last.AgeDays)
	})

	setNow(env, 2026, 11, 2, 9, 0)

	t.Run("the dashboard's month equals the sum of the month's invoices", func(t *testing.T) {
		d, err := env.Billing.Dashboard(ctx, w.admin, "")
		require.NoError(t, err)
		require.Equal(t, day(2026, 11, 1), d.Period, "this month in Jakarta by default")
		items := allInvoices(t, env, w.admin, nov)
		var billed, paid, outstanding int64
		for _, it := range items {
			billed += it.Billed
			paid += it.Paid
			outstanding += it.Outstanding
		}
		require.Len(t, items, 5)
		require.Equal(t, 5, d.InvoiceCount, "the cancelled one included")
		require.Equal(t, billed, d.Billed)
		require.Equal(t, paid, d.Collected)
		require.Equal(t, outstanding, d.Outstanding)
		require.Equal(t, d.Billed, d.Collected+d.Outstanding)
		require.EqualValues(t, 3_800_000, d.Billed)
		require.EqualValues(t, 1_000_000, d.Collected)
		require.EqualValues(t, 2_800_000, d.Outstanding)
		require.Equal(t, 3, d.TenantsOutstanding, "Bambang, Citra and Dodi")

		require.EqualValues(t, 400_000+900_000+1_500_000+700_000+650_000, d.Overdue, "any month")
		require.Equal(t, 5, d.TenantsInArrears)
		require.Equal(t, []string{"Fajar Nugroho", "Dewi Lestari", "Dodi Saputra", "Citra Ayu", "Bambang Wijaya"}, arrearsNames(d.Arrears),
			"the oldest debt first, then the most owed")
		require.Equal(t, billing.TenantArrears{
			TenantID: fajar.tenantID, TenantName: "Fajar Nugroho", Units: []billing.UnitRef{{PropertyName: "Kos Melati", UnitLabel: "Kamar 10"}},
			Overdue: 700_000, OldestUnpaidDueDate: day(2026, 10, 10), AgeDays: 23,
		}, d.Arrears[0])
		require.Equal(t, day(2026, 10, 28), d.Arrears[1].OldestUnpaidDueDate)
		require.EqualValues(t, 650_000, d.Arrears[1].Overdue, "October is due 4 Nov, not debt yet")
		require.Equal(t, 5, d.Arrears[1].AgeDays)
		require.Equal(t, []billing.UnitRef{{PropertyName: "Kontrakan Mawar", UnitLabel: "Rumah A"}}, d.Arrears[2].Units)

		require.Len(t, d.RecentCorrections, 2)
		require.Equal(t, "kamar batal disewa", d.RecentCorrections[0].Reason, "newest first")
		require.Equal(t, eko.tenantID, d.RecentCorrections[0].TenantID)
		require.Equal(t, day(2026, 11, 1), *d.RecentCorrections[0].Period)
		require.Equal(t, &billing.UnitRef{PropertyName: "Kos Melati", UnitLabel: "Kamar 4"}, d.RecentCorrections[0].Unit)
		require.Nil(t, d.RecentCorrections[0].ReceivedOn)
		require.Equal(t, "salah hitung kembalian", d.RecentCorrections[1].Reason)
		require.Equal(t, "Rina Wulandari", d.RecentCorrections[1].TenantName)
		require.Equal(t, day(2026, 11, 1), *d.RecentCorrections[1].ReceivedOn)
		require.Nil(t, d.RecentCorrections[1].Period)
		require.Nil(t, d.RecentCorrections[1].Unit)
		require.Equal(t, w.admin.MembershipID, d.RecentCorrections[1].CreatedBy.MemberID)
	})

	t.Run("an invoice belongs to the month of its period, not its due date", func(t *testing.T) {
		sep := allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: "2026-09"})
		require.Len(t, sep, 1)
		require.Equal(t, dewi.tenantID, sep[0].TenantID)
		require.Equal(t, day(2026, 9, 15), sep[0].Period)
		require.Equal(t, day(2026, 10, 28), sep[0].DueDate)
		require.Equal(t, 5, sep[0].OverdueDays)

		oct := allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: "2026-10"})
		require.Equal(t, []string{"Dewi Lestari", "Fajar Nugroho"}, tenantNames(oct))
		d, err := env.Billing.Dashboard(ctx, w.admin, "2026-09")
		require.NoError(t, err)
		require.EqualValues(t, 650_000, d.Billed)
		require.Equal(t, 1, d.InvoiceCount)
		require.Equal(t, 5, d.TenantsInArrears, "arrears are not tied to the month")
	})

	t.Run("filters", func(t *testing.T) {
		tests := []struct {
			name string
			iq   billing.InvoiceQuery
			want []string
		}{
			{"every invoice", nov, []string{"Bambang Wijaya", "Citra Ayu", "Dodi Saputra", "Eko Prasetyo", "Rina Wulandari"}},
			{"open", billing.InvoiceQuery{Period: "2026-11", Status: "open"}, []string{"Citra Ayu", "Dodi Saputra"}},
			{"partial", billing.InvoiceQuery{Period: "2026-11", Status: "partial"}, []string{"Bambang Wijaya"}},
			{"paid", billing.InvoiceQuery{Period: "2026-11", Status: "paid"}, []string{"Rina Wulandari"}},
			{"cancelled", billing.InvoiceQuery{Period: "2026-11", Status: "cancelled"}, []string{"Eko Prasetyo"}},
			{"unpaid", billing.InvoiceQuery{Period: "2026-11", Status: "unpaid"}, []string{"Bambang Wijaya", "Citra Ayu", "Dodi Saputra"}},
			{"overdue, of any month: Dewi's September and Fajar's October too, not Dewi's October due 4 Nov",
				billing.InvoiceQuery{Period: "2026-11", Status: "overdue"},
				[]string{"Bambang Wijaya", "Citra Ayu", "Dewi Lestari", "Dodi Saputra", "Fajar Nugroho"}},
			{"overdue ignores the month asked for", billing.InvoiceQuery{Period: "2026-10", Status: "overdue"},
				[]string{"Bambang Wijaya", "Citra Ayu", "Dewi Lestari", "Dodi Saputra", "Fajar Nugroho"}},
			{"property", billing.InvoiceQuery{Period: "2026-11", PropertyID: mawar.ID}, []string{"Dodi Saputra"}},
			{"q on the tenant's name", billing.InvoiceQuery{Period: "2026-11", Search: listing.NewSearch("citra")}, []string{"Citra Ayu"}},
			{"q on the unit label", billing.InvoiceQuery{Period: "2026-11", Search: listing.NewSearch("kamar 4")}, []string{"Eko Prasetyo"}},
			{"q on the property name", billing.InvoiceQuery{Period: "2026-11", Search: listing.NewSearch("MAWAR")}, []string{"Dodi Saputra"}},
			{"q and status together", billing.InvoiceQuery{Period: "2026-11", Status: "paid", Search: listing.NewSearch("kos")}, []string{"Rina Wulandari"}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				require.Equal(t, tc.want, tenantNames(allInvoices(t, env, w.admin, tc.iq)))
			})
		}
	})

	t.Run("each item has its lines and balances", func(t *testing.T) {
		items := allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: "2026-11", Status: "partial"})
		require.Len(t, items, 1)
		b := items[0]
		require.Equal(t, bambang.leaseID, b.LeaseID)
		require.Equal(t, w.kos.ID, b.PropertyID)
		require.Equal(t, "Kos Melati", b.PropertyName)
		require.Equal(t, "Kamar 2", b.UnitLabel)
		require.EqualValues(t, 700_000, b.Amount)
		require.EqualValues(t, 700_000, b.Billed)
		require.EqualValues(t, 300_000, b.Paid)
		require.EqualValues(t, 400_000, b.Outstanding)
		require.Equal(t, 1, b.OverdueDays)
		require.False(t, b.PastPlannedEnd)
		require.Len(t, b.Lines, 1)
		require.Equal(t, "rent", b.Lines[0].Kind)
		require.EqualValues(t, 700_000, b.Lines[0].Amount)
	})

	t.Run("sorts, with the id breaking ties", func(t *testing.T) {
		byID := func(a, b billing.InvoiceListItem) int { return bytes.Compare(a.ID[:], b.ID[:]) }
		tests := []struct {
			sort string
			cmp  func(a, b billing.InvoiceListItem) int
		}{
			{"", func(a, b billing.InvoiceListItem) int { return cmp.Or(a.DueDate.Compare(b.DueDate), byID(a, b)) }},
			{"dueDate", func(a, b billing.InvoiceListItem) int { return cmp.Or(a.DueDate.Compare(b.DueDate), byID(a, b)) }},
			{"-dueDate", func(a, b billing.InvoiceListItem) int { return cmp.Or(b.DueDate.Compare(a.DueDate), byID(a, b)) }},
			{"tenantName", func(a, b billing.InvoiceListItem) int {
				return cmp.Or(cmp.Compare(a.TenantName, b.TenantName), byID(a, b))
			}},
			{"-outstanding", func(a, b billing.InvoiceListItem) int {
				return cmp.Or(cmp.Compare(b.Outstanding, a.Outstanding), byID(a, b))
			}},
		}
		for _, period := range []string{"2026-10", "2026-11"} {
			for _, tc := range tests {
				t.Run(period+" "+tc.sort, func(t *testing.T) {
					got := allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: period, Sort: tc.sort})
					require.True(t, slices.IsSortedFunc(got, tc.cmp), "%v", tenantNames(got))
				})
			}
		}
		got := allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: "2026-11", Sort: "-outstanding"})
		require.Equal(t, "Dodi Saputra", got[0].TenantName)
		require.Equal(t, "Citra Ayu", got[1].TenantName)
	})

	t.Run("pages carry the total of every matching row", func(t *testing.T) {
		all := allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: "2026-11", Sort: "tenantName"})
		var seen []billing.InvoiceListItem
		for n := 1; n <= 4; n++ {
			res, err := env.Billing.ListInvoices(ctx, w.admin, billing.InvoiceQuery{Period: "2026-11", Sort: "tenantName", Page: listing.Page{Number: n, Size: 2}})
			require.NoError(t, err)
			require.EqualValues(t, 5, res.Total)
			seen = append(seen, res.Items...)
			if n == 4 {
				require.Empty(t, res.Items, "a page past the last is empty")
			}
		}
		require.Equal(t, all, seen)
		res, err := env.Billing.ListInvoices(ctx, w.admin, billing.InvoiceQuery{Period: "2026-11", Status: "unpaid", Page: listing.Page{Number: 1, Size: 1}})
		require.NoError(t, err)
		require.EqualValues(t, 3, res.Total)
		require.Len(t, res.Items, 1)
	})

	t.Run("the summary ignores status, q and page; the oldest debt is of any month", func(t *testing.T) {
		base, err := env.Billing.ListInvoices(ctx, w.admin, nov)
		require.NoError(t, err)
		require.Equal(t, billing.InvoiceSummary{
			Billed: 3_800_000, Paid: 1_000_000, Outstanding: 2_800_000, TenantsOutstanding: 3,
			OldestOverdue: &billing.OldestOverdue{
				TenantID: fajar.tenantID, TenantName: "Fajar Nugroho", PropertyName: "Kos Melati", UnitLabel: "Kamar 10",
				OldestUnpaidDueDate: day(2026, 10, 10), AgeDays: 23,
			},
		}, base.Summary)
		filtered, err := env.Billing.ListInvoices(ctx, w.admin, billing.InvoiceQuery{
			Period: "2026-11", Status: "paid", Search: listing.NewSearch("rina"), Page: listing.Page{Number: 3, Size: 1},
		})
		require.NoError(t, err)
		require.Equal(t, base.Summary, filtered.Summary)

		inMawar, err := env.Billing.ListInvoices(ctx, w.admin, billing.InvoiceQuery{Period: "2026-11", PropertyID: mawar.ID})
		require.NoError(t, err)
		require.Equal(t, billing.InvoiceSummary{
			Billed: 1_500_000, Outstanding: 1_500_000, TenantsOutstanding: 1,
			OldestOverdue: &billing.OldestOverdue{
				TenantID: dodi.tenantID, TenantName: "Dodi Saputra", PropertyName: "Kontrakan Mawar", UnitLabel: "Rumah A",
				OldestUnpaidDueDate: day(2026, 11, 1), AgeDays: 1,
			},
		}, inMawar.Summary)

		future, err := env.Billing.ListInvoices(ctx, w.admin, billing.InvoiceQuery{Period: "2027-01", PropertyID: mawar.ID})
		require.NoError(t, err)
		require.Empty(t, future.Items)
		require.Zero(t, future.Total)
		require.NotNil(t, future.Summary.OldestOverdue, "the oldest debt is not tied to the month")
	})

	t.Run("bad input names its field", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			iq    billing.InvoiceQuery
			field string
		}{
			{"period", billing.InvoiceQuery{Period: "2026-13"}, "period"},
			{"period without a dash", billing.InvoiceQuery{Period: "202611"}, "period"},
			{"period with a day", billing.InvoiceQuery{Period: "2026-11-01"}, "period"},
			{"status", billing.InvoiceQuery{Status: "lunas"}, "status"},
			{"sort", billing.InvoiceQuery{Sort: "amount"}, "sort"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := env.Billing.ListInvoices(ctx, w.admin, tc.iq)
				requireInput(t, err, tc.field)
			})
		}
		_, err := env.Billing.Dashboard(ctx, w.admin, "Nov 2026")
		requireInput(t, err, "period")
	})

	t.Run("an admin limited to the kos sees, and counts, only the kos", func(t *testing.T) {
		sari, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-9610-0099", Name: "Mbak Sari",
			Roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: w.kos.ID}}})
		require.NoError(t, err)
		limited := env.Principal(t, w.org.ID, sari.UserID)

		items := allInvoices(t, env, limited, nov)
		require.Equal(t, []string{"Bambang Wijaya", "Citra Ayu", "Eko Prasetyo", "Rina Wulandari"}, tenantNames(items))
		res, err := env.Billing.ListInvoices(ctx, limited, nov)
		require.NoError(t, err)
		require.EqualValues(t, 4, res.Total)
		require.EqualValues(t, 2_300_000, res.Summary.Billed)
		require.EqualValues(t, 1_300_000, res.Summary.Outstanding)
		require.Equal(t, 2, res.Summary.TenantsOutstanding)
		inMawar, err := env.Billing.ListInvoices(ctx, limited, billing.InvoiceQuery{Period: "2026-11", PropertyID: mawar.ID})
		require.NoError(t, err)
		require.Empty(t, inMawar.Items)
		require.Zero(t, inMawar.Summary.Billed)
		require.Nil(t, inMawar.Summary.OldestOverdue)

		d, err := env.Billing.Dashboard(ctx, limited, "")
		require.NoError(t, err)
		require.EqualValues(t, 2_300_000, d.Billed)
		require.EqualValues(t, 1_000_000, d.Collected)
		require.Equal(t, 4, d.InvoiceCount)
		require.EqualValues(t, 400_000+900_000+700_000+650_000, d.Overdue)
		require.Equal(t, 4, d.TenantsInArrears)
		require.NotContains(t, arrearsNames(d.Arrears), "Dodi Saputra")
		require.Len(t, d.RecentCorrections, 2, "both corrections are of kos tenants")
	})

	t.Run("an explicit Deny on one property wins over an Allow on everything", func(t *testing.T) {
		dina, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-9610-0098", Name: "Dina Kasir",
			Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
		require.NoError(t, err)
		env.GrantCustomRole(t, w.org.ID, dina.ID, "Pembaca kecuali Melati", `{"Version":"2026-10-25","Statement":[`+
			`{"Effect":"Allow","Action":["invoice:read","report:read"],"Resource":["*"]},`+
			`{"Effect":"Deny","Action":["invoice:read","report:read"],"Resource":["property/`+w.kos.ID.String()+`/*"]}]}`)
		reader := env.Principal(t, w.org.ID, dina.UserID)

		require.Equal(t, []string{"Dodi Saputra"}, tenantNames(allInvoices(t, env, reader, nov)))
		require.Empty(t, allInvoices(t, env, reader, billing.InvoiceQuery{Period: "2026-10"}))
		res, err := env.Billing.ListInvoices(ctx, reader, nov)
		require.NoError(t, err)
		require.EqualValues(t, 1_500_000, res.Summary.Billed)
		require.Equal(t, dodi.tenantID, res.Summary.OldestOverdue.TenantID)

		d, err := env.Billing.Dashboard(ctx, reader, "")
		require.NoError(t, err)
		require.EqualValues(t, 1_500_000, d.Billed)
		require.Equal(t, 1, d.InvoiceCount)
		require.EqualValues(t, 1_500_000, d.Overdue)
		require.Equal(t, []string{"Dodi Saputra"}, arrearsNames(d.Arrears))
		require.Empty(t, d.RecentCorrections, "neither correction is outside the denied property")
	})

	t.Run("a member without the action is denied and audited", func(t *testing.T) {
		karyawan := env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
		before := w.audited(t, authz.ActionReportRead, "deny")
		_, err := env.Billing.Dashboard(ctx, karyawan, "")
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, before+1, w.audited(t, authz.ActionReportRead, "deny"))

		// The karyawan reads only the invoices of tenants assigned to them: none here.
		res, err := env.Billing.ListInvoices(ctx, karyawan, nov)
		require.NoError(t, err)
		require.Empty(t, res.Items)
		require.Zero(t, res.Summary.Billed)

		nina, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-9610-0097", Name: "Nina Tamu",
			Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
		require.NoError(t, err)
		env.GrantCustomRole(t, w.org.ID, nina.ID, "Tanpa tagihan", `{"Version":"2026-10-25","Statement":[`+
			`{"Effect":"Deny","Action":["invoice:read"],"Resource":["*"]}]}`)
		before = w.audited(t, authz.ActionInvoiceRead, "deny")
		_, err = env.Billing.ListInvoices(ctx, env.Principal(t, w.org.ID, nina.UserID), nov)
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, before+1, w.audited(t, authz.ActionInvoiceRead, "deny"))
	})

	t.Run("another organization's invoices never appear", func(t *testing.T) {
		other := newWorld(t, env, "0812962")
		lina := other.rentIn(t, other.kos, "Kamar 1", "Lina Marlina", day(2026, 11, 2), 800_000)
		other.generate(t)
		other.correct(t, billing.CorrectionInput{RequestID: uuid.New(), InvoiceID: other.invoices(t, lina.renter)[0].InvoiceID, AmountDelta: -100_000, Reason: "diskon"})

		items := allInvoices(t, env, w.admin, nov)
		require.Len(t, items, 5)
		for _, it := range items {
			require.NotEqual(t, lina.tenantID, it.TenantID)
		}
		d, err := env.Billing.Dashboard(ctx, w.admin, "")
		require.NoError(t, err)
		require.EqualValues(t, 3_800_000, d.Billed)
		require.Len(t, d.RecentCorrections, 2)

		mine := allInvoices(t, env, other.admin, nov)
		require.Equal(t, []string{"Lina Marlina"}, tenantNames(mine))
	})
}

func TestDashboardCorrectionsAndPlannedEnd(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 10, 28, 9, 0)
	w := newWorld(t, env, "0812963")
	sri := w.rentIn(t, w.kos, "Kamar 1", "Sri Mulyani", day(2026, 10, 28), 700_000)
	w.generate(t)
	setNow(env, 2026, 11, 21, 9, 0)
	w.generate(t)
	invs := w.invoices(t, sri.renter)
	require.Len(t, invs, 2)

	// Ten member corrections of October, then the system's cancellation of November.
	for i := range 10 {
		w.correct(t, billing.CorrectionInput{RequestID: uuid.New(), InvoiceID: invs[0].InvoiceID, AmountDelta: -1_000, Reason: fmt.Sprintf("potongan %d", i+1)})
	}
	end := day(2026, 11, 27)
	_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, sri.leaseID, &end)
	require.NoError(t, err)

	d, err := env.Billing.Dashboard(ctx, w.admin, "")
	require.NoError(t, err)
	require.Len(t, d.RecentCorrections, 10, "at most ten")
	require.Equal(t, "sewa tidak diperpanjang", d.RecentCorrections[0].Reason, "the system's correction, newest first")
	require.Equal(t, day(2026, 11, 28), *d.RecentCorrections[0].Period)
	require.EqualValues(t, -700_000, d.RecentCorrections[0].AmountDelta)
	require.Equal(t, "potongan 10", d.RecentCorrections[1].Reason)
	require.Equal(t, "potongan 2", d.RecentCorrections[9].Reason)

	nov, err := env.Billing.ListInvoices(ctx, w.admin, billing.InvoiceQuery{Period: "2026-11", Status: "cancelled"})
	require.NoError(t, err)
	require.Len(t, nov.Items, 1)
	require.Zero(t, nov.Items[0].Billed)

	setNow(env, 2026, 11, 27, 23, 0)
	oct := allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: "2026-10"})
	require.Len(t, oct, 1)
	require.False(t, oct[0].PastPlannedEnd, "the planned end is today")

	setNow(env, 2026, 11, 28, 8, 0)
	oct = allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: "2026-10"})
	require.True(t, oct[0].PastPlannedEnd, "still active after its planned end")
	require.Equal(t, 31, oct[0].OverdueDays)

	_, err = env.Tenants.MoveOut(ctx, w.admin, sri.leaseID, day(2026, 11, 27))
	require.NoError(t, err)
	oct = allInvoices(t, env, w.admin, billing.InvoiceQuery{Period: "2026-10"})
	require.False(t, oct[0].PastPlannedEnd, "the move-out is recorded")
}
