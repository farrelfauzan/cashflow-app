//go:build integration

package tenant_test

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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// find lists every matching tenant on one page and returns their names with the total, which
// must count exactly the rows returned.
func find(t *testing.T, env *testenv.Env, p authz.Principal, lq tenant.ListQuery) []string {
	t.Helper()
	lq.Page = listing.Page{Number: 1, Size: listing.MaxPageSize}
	res, err := env.Tenants.List(context.Background(), p, lq)
	require.NoError(t, err)
	require.Equal(t, int64(len(res.Items)), res.Total, "the total counts exactly the rows returned")
	return tenantNames(res.Items)
}

// search is find with q only.
func search(t *testing.T, env *testenv.Env, p authz.Principal, q string) []string {
	t.Helper()
	return find(t, env, p, tenant.ListQuery{Search: listing.NewSearch(q)})
}

// pagedIDs reads every page of a list at the given size and checks each page against the
// total: full pages, then the rest, then an empty page past the last with the same total.
func pagedIDs(t *testing.T, env *testenv.Env, p authz.Principal, lq tenant.ListQuery, size int) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	var total int64 = -1
	for n := 1; ; n++ {
		lq.Page = listing.Page{Number: n, Size: size}
		res, err := env.Tenants.List(context.Background(), p, lq)
		require.NoError(t, err)
		require.Equal(t, lq.Page, res.Page)
		if total == -1 {
			total = res.Total
		}
		require.Equal(t, total, res.Total, "page %d has the same total", n)
		rest := int(total) - (n-1)*size
		if rest <= 0 {
			require.Empty(t, res.Items, "page %d is past the last", n)
			return ids
		}
		require.Len(t, res.Items, min(size, rest), "page %d", n)
		for _, tn := range res.Items {
			ids = append(ids, tn.ID)
		}
	}
}

func tenantIDs(ts []tenant.Tenant) []uuid.UUID {
	out := make([]uuid.UUID, len(ts))
	for i, tn := range ts {
		out[i] = tn.ID
	}
	return out
}

func byID(a, b tenant.Tenant) int { return bytes.Compare(a.ID[:], b.ID[:]) }

func TestTenantListPages(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "0812751")

	// Distinct first letters keep the expected order the same in any collation; the three
	// Siti Rahayu are a tie that only the id breaks.
	names := []string{
		"Rina Wulandari", "Agus Salim", "Siti Rahayu", "Bambang Wijaya", "Citra Lestari", "Yusuf Maulana",
		"Dewi Anggraini", "Eko Prasetyo", "Siti Rahayu", "Fajar Nugroho", "Gita Permata", "Hendra Gunawan",
		"Indah Sari", "Joko Susilo", "Kartika Putri", "Wahyu Saputra", "Lukman Hakim", "Maya Puspita",
		"Siti Rahayu", "Nur Aini", "Oki Setiawan", "Putri Ayu", "Vina Melati",
	}
	created := make([]tenant.Tenant, len(names))
	for i, n := range names {
		var err error
		created[i], err = env.Tenants.Create(ctx, w.admin, tenant.Input{Name: n, Phone: fmt.Sprintf("0857-7510-%04d", i+1)})
		require.NoError(t, err)
	}

	t.Run("23 tenants at 10 a page: two full pages, the rest, then an empty page with the real total", func(t *testing.T) {
		for _, tc := range []struct {
			page, want int
		}{{1, 10}, {2, 10}, {3, 3}, {4, 0}, {1000, 0}} {
			res, err := env.Tenants.List(ctx, w.admin, tenant.ListQuery{Page: listing.Page{Number: tc.page, Size: 10}})
			require.NoError(t, err)
			require.Len(t, res.Items, tc.want, "page %d", tc.page)
			require.Equal(t, int64(23), res.Total, "page %d", tc.page)
			require.NotNil(t, res.Items)
		}
	})

	t.Run("the zero page is page 1 at the default size", func(t *testing.T) {
		res, err := env.Tenants.List(ctx, w.admin, tenant.ListQuery{})
		require.NoError(t, err)
		require.Equal(t, listing.FirstPage, res.Page)
		require.Len(t, res.Items, listing.DefaultPageSize)
		require.Equal(t, int64(23), res.Total)
	})

	byName := slices.Clone(created)
	slices.SortFunc(byName, func(a, b tenant.Tenant) int { return cmp.Or(cmp.Compare(a.Name, b.Name), byID(a, b)) })
	byNameDesc := slices.Clone(created)
	slices.SortFunc(byNameDesc, func(a, b tenant.Tenant) int { return cmp.Or(cmp.Compare(b.Name, a.Name), byID(a, b)) })
	byCreated := slices.Clone(created)
	slices.SortFunc(byCreated, func(a, b tenant.Tenant) int { return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), byID(a, b)) })
	byCreatedDesc := slices.Clone(created)
	slices.SortFunc(byCreatedDesc, func(a, b tenant.Tenant) int { return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), byID(a, b)) })

	t.Run("tenants made one after another sort by createdAt in the order they were made", func(t *testing.T) {
		require.Equal(t, tenantIDs(created), tenantIDs(byCreated))
	})

	sorts := []struct {
		name, sort string
		want       []tenant.Tenant
	}{
		{name: "the default is by name", sort: "", want: byName},
		{name: "name", sort: "name", want: byName},
		{name: "-name reverses names, ties still by id", sort: "-name", want: byNameDesc},
		{name: "createdAt", sort: "createdAt", want: byCreated},
		{name: "-createdAt", sort: "-createdAt", want: byCreatedDesc},
	}
	for _, tc := range sorts {
		t.Run("sort "+tc.name+": the pages together hold every tenant once, in order", func(t *testing.T) {
			for _, size := range []int{1, 9, 10, 100} {
				got := pagedIDs(t, env, w.admin, tenant.ListQuery{Sort: tc.sort}, size)
				require.Equal(t, tenantIDs(tc.want), got, "page size %d", size)
			}
		})
	}

	t.Run("tied names are ordered by id and split the same way across a page boundary", func(t *testing.T) {
		var sitis []uuid.UUID
		for _, tn := range byName {
			if tn.Name == "Siti Rahayu" {
				sitis = append(sitis, tn.ID)
			}
		}
		require.Len(t, sitis, 3)
		require.True(t, slices.IsSortedFunc(sitis, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) }))

		// At 9 a page the Siti Rahayu are rows 18 to 20: one ends page 2, two start page 3.
		for range 3 {
			p2, err := env.Tenants.List(ctx, w.admin, tenant.ListQuery{Page: listing.Page{Number: 2, Size: 9}})
			require.NoError(t, err)
			p3, err := env.Tenants.List(ctx, w.admin, tenant.ListQuery{Page: listing.Page{Number: 3, Size: 9}})
			require.NoError(t, err)
			require.Equal(t, sitis[0], p2.Items[8].ID)
			require.Equal(t, sitis[1:], tenantIDs(p3.Items[:2]))
		}
	})

	t.Run("an unknown sort or status names the field", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			lq    tenant.ListQuery
			field string
		}{
			{name: "unknown sort", lq: tenant.ListQuery{Sort: "phone"}, field: "sort"},
			{name: "sort is case-sensitive", lq: tenant.ListQuery{Sort: "Name"}, field: "sort"},
			{name: "unknown status", lq: tenant.ListQuery{Status: "late"}, field: "status"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := env.Tenants.List(ctx, w.admin, tc.lq)
				requireField(t, err, tc.field)
			})
		}
	})
}

func TestTenantListSearchAndFilters(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // the clock reads 2 November 2026 in Jakarta
	w := newWorld(t, env, "0812752")

	sari, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-7520-0099", Name: "Mbak Sari", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)

	unit := func(p property.Property, label string) property.Unit {
		u, err := env.Properties.CreateUnit(ctx, w.admin, p.ID, property.UnitInput{Label: label})
		require.NoError(t, err)
		return u
	}
	kamar3, kamar4, kamar5, kamar6 := unit(w.melati, "Kamar 3"), unit(w.melati, "Kamar 4"), unit(w.melati, "Kamar 5"), unit(w.melati, "Kamar 6")
	rumahB := unit(w.mawar, "Rumah B")

	newTenant := func(name, phone string) tenant.Tenant {
		tn, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: phone})
		require.NoError(t, err)
		return tn
	}
	rent := func(tn tenant.Tenant, u property.Unit, start time.Time, collector uuid.UUID) tenant.Lease {
		l, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: tn.ID, UnitID: u.ID, Terms: tenant.Terms{
			StartDate: start, RentAmount: 800_000, CollectorMemberID: collector,
		}})
		require.NoError(t, err)
		return l
	}

	rina := newTenant("Rina Wulandari", "0812-3456-7801")
	budi := newTenant("Budi Santoso", "0857-1000-0002")
	dewi := newTenant("Dewi Anggraini", "0857-1000-0003")
	newTenant("Eko Prasetyo", "0857-1000-0004")
	fajar := newTenant("Fajar Nugroho", "0857-1000-0005")
	gita := newTenant("Gita Permata", "0857-1000-0006")
	hendra := newTenant("Hendra Gunawan", "0857-1000-0007")
	joko := newTenant("Joko Susilo", "0857-1000-0008")
	newTenant("Toko Diskon 50%", "0857-1000-0009")
	newTenant("Toko Diskon 500", "0857-1000-0010")
	newTenant("CV Maju_Jaya", "0857-1000-0011")
	newTenant("CV Maju Jaya", "0857-1000-0012")

	rent(rina, w.kamar1, day(2026, 10, 1), uuid.Nil)
	rent(budi, w.kamar2, day(2026, 10, 1), uuid.Nil)
	rent(dewi, kamar3, day(2026, 10, 1), uuid.Nil)
	rent(dewi, w.rumahA, day(2026, 10, 1), uuid.Nil)
	fajarOld := rent(fajar, kamar4, day(2026, 9, 1), uuid.Nil)
	_, err = env.Tenants.MoveOut(ctx, w.admin, fajarOld.ID, day(2026, 10, 31))
	require.NoError(t, err)
	rent(fajar, rumahB, day(2026, 11, 1), uuid.Nil)
	rent(gita, kamar5, day(2026, 12, 1), uuid.Nil)
	hendraLease := rent(hendra, kamar6, day(2026, 10, 1), sari.ID)
	_, err = env.Tenants.MoveOut(ctx, w.admin, hendraLease.ID, day(2026, 11, 1))
	require.NoError(t, err)
	rent(joko, kamar6, day(2026, 11, 2), sari.ID) // starts today

	everyone := []string{
		"Rina Wulandari", "Budi Santoso", "Dewi Anggraini", "Eko Prasetyo", "Fajar Nugroho", "Gita Permata",
		"Hendra Gunawan", "Joko Susilo", "Toko Diskon 50%", "Toko Diskon 500", "CV Maju_Jaya", "CV Maju Jaya",
	}

	t.Run("q", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			q    string
			want []string
		}{
			{name: "empty is no filter", q: "", want: everyone},
			{name: "blank is no filter", q: "   ", want: everyone},
			{name: "part of a name, ignoring case", q: "wulan", want: []string{"Rina Wulandari"}},
			{name: "a name in capitals", q: "RINA", want: []string{"Rina Wulandari"}},
			{name: "the unit label of a lease", q: "kamar 2", want: []string{"Budi Santoso"}},
			{name: "a label shared by two leases' units", q: "rumah", want: []string{"Dewi Anggraini", "Fajar Nugroho"}},
			{name: "the label of an ended lease", q: "Kamar 4", want: []string{"Fajar Nugroho"}},
			{name: "a national number with a space", q: "0812 3456", want: []string{"Rina Wulandari"}},
			{name: "a national number with a dash", q: "0812-3456", want: []string{"Rina Wulandari"}},
			{name: "a number with +62", q: "+62 812 3456", want: []string{"Rina Wulandari"}},
			{name: "62 and the start of the number", q: "62812", want: []string{"Rina Wulandari"}},
			{name: "digits from the middle", q: "812345", want: []string{"Rina Wulandari"}},
			{name: "the end of a number", q: "1000-0007", want: []string{"Hendra Gunawan"}},
			{name: "fewer than 3 digits is not a phone search", q: "08", want: []string{}},
			{name: "% is a literal percent sign", q: "50%", want: []string{"Toko Diskon 50%"}},
			{name: "_ is a literal underscore", q: "Maju_Jaya", want: []string{"CV Maju_Jaya"}},
			{name: "a lone % matches only a name with one", q: "%", want: []string{"Toko Diskon 50%"}},
			{name: "a backslash is literal", q: `Maju\`, want: []string{}},
			{name: "nothing matches", q: "Slamet", want: []string{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				require.ElementsMatch(t, tc.want, search(t, env, w.admin, tc.q))
			})
		}
	})

	t.Run("filters", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			lq   tenant.ListQuery
			want []string
		}{
			{
				name: "propertyId: every tenant with a lease there, ended ones included",
				lq:   tenant.ListQuery{PropertyID: w.melati.ID},
				want: []string{"Rina Wulandari", "Budi Santoso", "Dewi Anggraini", "Fajar Nugroho", "Gita Permata", "Hendra Gunawan", "Joko Susilo"},
			},
			{
				name: "propertyId: a tenant with leases in two properties shows under both",
				lq:   tenant.ListQuery{PropertyID: w.mawar.ID},
				want: []string{"Dewi Anggraini", "Fajar Nugroho"},
			},
			{name: "propertyId: an unknown property matches nothing", lq: tenant.ListQuery{PropertyID: uuid.New()}, want: []string{}},
			{
				name: "active: a lease that has started, today included, even beside an ended one",
				lq:   tenant.ListQuery{Status: tenant.ListStatusActive},
				want: []string{"Rina Wulandari", "Budi Santoso", "Dewi Anggraini", "Fajar Nugroho", "Joko Susilo"},
			},
			{name: "upcoming: a lease that starts after today", lq: tenant.ListQuery{Status: tenant.ListStatusUpcoming}, want: []string{"Gita Permata"}},
			{name: "ended: only ended leases", lq: tenant.ListQuery{Status: tenant.ListStatusEnded}, want: []string{"Hendra Gunawan"}},
			{
				name: "none: no lease at all",
				lq:   tenant.ListQuery{Status: tenant.ListStatusNone},
				want: []string{"Eko Prasetyo", "Toko Diskon 50%", "Toko Diskon 500", "CV Maju_Jaya", "CV Maju Jaya"},
			},
			{
				name: "filters combine",
				lq:   tenant.ListQuery{PropertyID: w.mawar.ID, Status: tenant.ListStatusActive, Search: listing.NewSearch("fajar")},
				want: []string{"Fajar Nugroho"},
			},
			{name: "collectorDisabled while every collector is active", lq: tenant.ListQuery{CollectorDisabled: true}, want: []string{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				require.ElementsMatch(t, tc.want, find(t, env, w.admin, tc.lq))
			})
		}
	})

	t.Run("collectorDisabled keeps only active leases whose collector is disabled", func(t *testing.T) {
		_, err := env.Members.SetStatus(ctx, w.admin, sari.ID, member.StatusDisabled)
		require.NoError(t, err)
		// Sari collected Hendra's ended lease and collects Joko's running one.
		require.Equal(t, []string{"Joko Susilo"}, find(t, env, w.admin, tenant.ListQuery{CollectorDisabled: true}))
		require.Empty(t, find(t, env, w.admin, tenant.ListQuery{CollectorDisabled: true, PropertyID: w.mawar.ID}))
	})
}

func TestTenantListScope(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // the clock reads 2 November 2026 in Jakarta
	w := newWorld(t, env, "0812761")
	b := newWorld(t, env, "0812762")

	sari, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-7610-0099", Name: "Mbak Sari", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)
	_, err = env.Members.SetRoles(ctx, w.admin, sari.ID, []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: w.melati.ID}})
	require.NoError(t, err)
	limited := env.Principal(t, w.org.ID, sari.UserID) // admin of Kos Melati only
	lina, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-7610-0098", Name: "Mbak Lina", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)

	unit := func(p property.Property, label string) property.Unit {
		u, err := env.Properties.CreateUnit(ctx, w.admin, p.ID, property.UnitInput{Label: label})
		require.NoError(t, err)
		return u
	}
	kamar3, rumahB, rumahC := unit(w.melati, "Kamar 3"), unit(w.mawar, "Rumah B"), unit(w.mawar, "Rumah C")
	newTenant := func(name, phone string) tenant.Tenant {
		tn, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: phone})
		require.NoError(t, err)
		return tn
	}
	rent := func(tn tenant.Tenant, u property.Unit, collector uuid.UUID) tenant.Lease {
		l, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: tn.ID, UnitID: u.ID, Terms: tenant.Terms{
			StartDate: day(2026, 10, 1), RentAmount: 800_000, CollectorMemberID: collector,
		}})
		require.NoError(t, err)
		return l
	}

	rina := newTenant("Rina Wulandari", "0857-7610-0001")
	budi := newTenant("Budi Santoso", "0857-7610-0002")
	dewi := newTenant("Dewi Anggraini", "0857-7610-0003")
	citra := newTenant("Citra Lestari", "0857-7610-0004")
	newTenant("Eko Prasetyo", "0857-7610-0005")
	hendra := newTenant("Hendra Gunawan", "0857-7610-0006")
	rent(rina, w.kamar1, w.karyawanMemberID) // Kos Melati
	rent(budi, w.rumahA, uuid.Nil)           // Kontrakan Mawar only
	dewiMelati := rent(dewi, w.kamar2, uuid.Nil)
	dewiMawar := rent(dewi, rumahB, w.karyawanMemberID)
	rent(citra, rumahC, lina.ID) // Kontrakan Mawar, collected by Lina
	hendraLease := rent(hendra, kamar3, w.karyawanMemberID)
	_, err = env.Tenants.MoveOut(ctx, w.admin, hendraLease.ID, day(2026, 11, 1)) // Kos Melati, ended
	require.NoError(t, err)

	slamet, err := env.Tenants.Create(ctx, b.admin, tenant.Input{Name: "Slamet Riyadi", Phone: "0896-7620-0001"})
	require.NoError(t, err)
	_, err = env.Tenants.CreateLease(ctx, b.admin, tenant.NewLease{TenantID: slamet.ID, UnitID: b.kamar1.ID, Terms: tenant.Terms{
		StartDate: day(2026, 10, 1), RentAmount: 900_000,
	}})
	require.NoError(t, err)

	t.Run("the organization-wide admin sees every tenant, for comparison", func(t *testing.T) {
		require.ElementsMatch(t, []string{"Rina Wulandari", "Budi Santoso", "Dewi Anggraini", "Citra Lestari", "Eko Prasetyo", "Hendra Gunawan"},
			find(t, env, w.admin, tenant.ListQuery{}))
		require.Equal(t, []string{"Dewi Anggraini"}, search(t, env, w.admin, "Rumah B"))
		require.Equal(t, []string{"Hendra Gunawan"}, find(t, env, w.admin, tenant.ListQuery{Status: tenant.ListStatusEnded}))
		require.Equal(t, []string{"Eko Prasetyo"}, find(t, env, w.admin, tenant.ListQuery{Status: tenant.ListStatusNone}))
		require.ElementsMatch(t, []string{"Budi Santoso", "Dewi Anggraini", "Citra Lestari"}, find(t, env, w.admin, tenant.ListQuery{PropertyID: w.mawar.ID}))
	})

	t.Run("a property-limited admin sees and counts only tenants with an active lease in her property", func(t *testing.T) {
		res, err := env.Tenants.List(ctx, limited, tenant.ListQuery{})
		require.NoError(t, err)
		require.Equal(t, int64(2), res.Total)
		require.Equal(t, []string{"Dewi Anggraini", "Rina Wulandari"}, tenantNames(res.Items))
		require.Len(t, res.Items[0].Leases, 1, "Dewi's lease in Kontrakan Mawar stays hidden")
		require.Equal(t, dewiMelati.ID, res.Items[0].Leases[0].ID)

		for n, want := range [][]string{{"Dewi Anggraini"}, {"Rina Wulandari"}, {}} {
			res, err := env.Tenants.List(ctx, limited, tenant.ListQuery{Page: listing.Page{Number: n + 1, Size: 1}})
			require.NoError(t, err)
			require.Equal(t, int64(2), res.Total, "page %d", n+1)
			require.Equal(t, want, tenantNames(res.Items), "page %d", n+1)
		}
	})

	t.Run("no search or filter reveals a tenant outside her property", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			lq   tenant.ListQuery
			want []string
		}{
			{name: "the unit label of a tenant who rents only elsewhere", lq: tenant.ListQuery{Search: listing.NewSearch("Rumah A")}, want: []string{}},
			{name: "the name of a tenant who rents only elsewhere", lq: tenant.ListQuery{Search: listing.NewSearch("Budi")}, want: []string{}},
			{name: "the phone of a tenant who rents only elsewhere", lq: tenant.ListQuery{Search: listing.NewSearch("0857-7610-0002")}, want: []string{}},
			{name: "a tenant without a lease", lq: tenant.ListQuery{Search: listing.NewSearch("Eko")}, want: []string{}},
			{name: "a former tenant of her property", lq: tenant.ListQuery{Search: listing.NewSearch("Hendra")}, want: []string{}},
			{name: "the label of a hidden lease of a visible tenant", lq: tenant.ListQuery{Search: listing.NewSearch("Rumah B")}, want: []string{}},
			{name: "the label of her own lease of that tenant", lq: tenant.ListQuery{Search: listing.NewSearch("Kamar 2")}, want: []string{"Dewi Anggraini"}},
			{name: "the other property", lq: tenant.ListQuery{PropertyID: w.mawar.ID}, want: []string{}},
			{name: "her property", lq: tenant.ListQuery{PropertyID: w.melati.ID}, want: []string{"Dewi Anggraini", "Rina Wulandari"}},
			{name: "status ended", lq: tenant.ListQuery{Status: tenant.ListStatusEnded}, want: []string{}},
			{name: "status none", lq: tenant.ListQuery{Status: tenant.ListStatusNone}, want: []string{}},
			{name: "status upcoming", lq: tenant.ListQuery{Status: tenant.ListStatusUpcoming}, want: []string{}},
			{name: "status active", lq: tenant.ListQuery{Status: tenant.ListStatusActive}, want: []string{"Dewi Anggraini", "Rina Wulandari"}},
			{name: "another organization's tenant by name", lq: tenant.ListQuery{Search: listing.NewSearch("Slamet")}, want: []string{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				require.ElementsMatch(t, tc.want, find(t, env, limited, tc.lq))
			})
		}
	})

	t.Run("a karyawan sees and counts only the tenants whose active lease they collect", func(t *testing.T) {
		res, err := env.Tenants.List(ctx, w.karyawan, tenant.ListQuery{})
		require.NoError(t, err)
		require.Equal(t, int64(2), res.Total)
		require.Equal(t, []string{"Dewi Anggraini", "Rina Wulandari"}, tenantNames(res.Items))
		require.Len(t, res.Items[0].Leases, 1)
		require.Equal(t, dewiMawar.ID, res.Items[0].Leases[0].ID)

		require.Empty(t, search(t, env, w.karyawan, "Kamar 2"), "Dewi's Kamar 2 lease is not theirs")
		require.Equal(t, []string{"Dewi Anggraini"}, search(t, env, w.karyawan, "Rumah B"))
		require.Empty(t, search(t, env, w.karyawan, "Hendra"), "the ended lease no longer shows Hendra")
		require.Empty(t, search(t, env, w.karyawan, "Citra"))
		require.Equal(t, []string{"Rina Wulandari"}, find(t, env, w.karyawan, tenant.ListQuery{PropertyID: w.melati.ID}))
		require.Empty(t, find(t, env, w.karyawan, tenant.ListQuery{Status: tenant.ListStatusEnded}))
	})

	t.Run("collectorDisabled cannot reveal a tenant outside the caller's view", func(t *testing.T) {
		_, err := env.Members.SetStatus(ctx, w.admin, lina.ID, member.StatusDisabled)
		require.NoError(t, err)
		require.Equal(t, []string{"Citra Lestari"}, find(t, env, w.admin, tenant.ListQuery{CollectorDisabled: true}))
		require.Empty(t, find(t, env, limited, tenant.ListQuery{CollectorDisabled: true}))
		require.Empty(t, find(t, env, w.karyawan, tenant.ListQuery{CollectorDisabled: true}))
	})

	t.Run("another organization's tenants are never listed or counted", func(t *testing.T) {
		res, err := env.Tenants.List(ctx, w.admin, tenant.ListQuery{})
		require.NoError(t, err)
		require.Equal(t, int64(6), res.Total)
		for _, q := range []string{"Slamet", "Riyadi", "0896-7620-0001", "62896"} {
			require.Empty(t, search(t, env, w.admin, q), q)
		}
		require.Empty(t, find(t, env, w.admin, tenant.ListQuery{PropertyID: b.melati.ID}))
		require.Equal(t, []string{"Slamet Riyadi"}, search(t, env, b.admin, "Slamet"))
		require.Equal(t, []string{"Slamet Riyadi"}, find(t, env, b.admin, tenant.ListQuery{}))
	})
}
