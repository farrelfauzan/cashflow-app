//go:build integration

package member_test

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

func memberNames(ms []member.Member) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}

func memberIDs(ms []member.Member) []uuid.UUID {
	out := make([]uuid.UUID, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

// findMembers lists every matching member on one page and returns their names; the total must
// count exactly the rows returned.
func findMembers(t *testing.T, env *testenv.Env, p authz.Principal, lq member.ListQuery) []string {
	t.Helper()
	lq.Page = listing.Page{Number: 1, Size: listing.MaxPageSize}
	res, err := env.Members.List(context.Background(), p, lq)
	require.NoError(t, err)
	require.Equal(t, int64(len(res.Items)), res.Total, "the total counts exactly the rows returned")
	return memberNames(res.Items)
}

// pagedMemberIDs reads every page at the given size, checking each page against the total and
// that the page past the last is empty with the same total.
func pagedMemberIDs(t *testing.T, env *testenv.Env, p authz.Principal, lq member.ListQuery, size int) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	var total int64 = -1
	for n := 1; ; n++ {
		lq.Page = listing.Page{Number: n, Size: size}
		res, err := env.Members.List(context.Background(), p, lq)
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
		ids = append(ids, memberIDs(res.Items)...)
	}
}

func memberByIDOrder(a, b member.Member) int { return bytes.Compare(a.ID[:], b.ID[:]) }

func TestMemberList(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	// The founders are Bu Ani, Pak Budi and Mas Dimas; they are created in one transaction, so
	// they share a createdAt and only the id orders them.
	o := env.CreateOrg(t, "Griya Sejahtera", "0812771")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	other := env.CreateOrg(t, "Kos Kenanga", "0812772")
	otherAdmin := env.Principal(t, other.ID, other.Admin.UserID)

	// Distinct first letters keep the expected order the same in any collation; the three
	// Siti Rahayu tie on name.
	names := []string{
		"Agus Salim", "Citra Lestari", "Siti Rahayu", "Dewi Anggraini", "Eko Prasetyo", "Fajar Nugroho",
		"Gita Permata", "Hendra Gunawan", "Siti Rahayu", "Indah Sari", "Joko Susilo", "Kartika Putri",
		"Lukman Hakim", "Nur Aini", "Oki Setiawan", "Siti Rahayu", "Rina Wulandari", "Vina Melati",
		"Wahyu Saputra", "Yusuf Maulana",
	}
	invited := map[string]member.Member{}
	for i, n := range names {
		m, err := env.Members.Invite(ctx, admin, member.Invite{
			Phone: fmt.Sprintf("0857-7710-%04d", i+1), Name: n, Roles: grants(authz.RoleKaryawan),
		})
		require.NoError(t, err)
		invited[n] = m
	}
	_, err := env.Members.Invite(ctx, otherAdmin, member.Invite{Phone: "0896-7720-0001", Name: "Slamet Riyadi", Roles: grants(authz.RoleKaryawan)})
	require.NoError(t, err)

	all, err := env.AllMembers(ctx, admin)
	require.NoError(t, err)
	require.Len(t, all, 23)

	t.Run("23 members at 10 a page: two full pages, the rest, then an empty page with the real total", func(t *testing.T) {
		for _, tc := range []struct{ page, want int }{{1, 10}, {2, 10}, {3, 3}, {4, 0}} {
			res, err := env.Members.List(ctx, admin, member.ListQuery{Page: listing.Page{Number: tc.page, Size: 10}})
			require.NoError(t, err)
			require.Len(t, res.Items, tc.want, "page %d", tc.page)
			require.Equal(t, int64(23), res.Total, "page %d", tc.page)
		}
	})

	byName := slices.Clone(all)
	slices.SortFunc(byName, func(a, b member.Member) int { return cmp.Or(cmp.Compare(a.Name, b.Name), memberByIDOrder(a, b)) })
	byNameDesc := slices.Clone(all)
	slices.SortFunc(byNameDesc, func(a, b member.Member) int { return cmp.Or(cmp.Compare(b.Name, a.Name), memberByIDOrder(a, b)) })
	byCreated := slices.Clone(all)
	slices.SortFunc(byCreated, func(a, b member.Member) int { return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), memberByIDOrder(a, b)) })
	byCreatedDesc := slices.Clone(all)
	slices.SortFunc(byCreatedDesc, func(a, b member.Member) int { return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), memberByIDOrder(a, b)) })

	for _, tc := range []struct {
		name, sort string
		want       []member.Member
	}{
		{name: "the default is by name", sort: "", want: byName},
		{name: "name", sort: "name", want: byName},
		{name: "-name, ties still by id", sort: "-name", want: byNameDesc},
		{name: "createdAt, the founders' tie by id", sort: "createdAt", want: byCreated},
		{name: "-createdAt", sort: "-createdAt", want: byCreatedDesc},
	} {
		t.Run("sort "+tc.name+": the pages together hold every member once, in order", func(t *testing.T) {
			for _, size := range []int{1, 9, 10, 100} {
				require.Equal(t, memberIDs(tc.want), pagedMemberIDs(t, env, admin, member.ListQuery{Sort: tc.sort}, size), "page size %d", size)
			}
		})
	}

	t.Run("the founders share a createdAt, so the id decides their order", func(t *testing.T) {
		require.True(t, byCreated[0].CreatedAt.Equal(byCreated[2].CreatedAt))
	})

	t.Run("tied names are ordered by id across a page boundary", func(t *testing.T) {
		var sitis []uuid.UUID
		for _, m := range byName {
			if m.Name == "Siti Rahayu" {
				sitis = append(sitis, m.ID)
			}
		}
		require.Len(t, sitis, 3)
		// By name the Siti Rahayu are rows 18 to 20: page 2 of 9 ends with the first.
		res, err := env.Members.List(ctx, admin, member.ListQuery{Page: listing.Page{Number: 2, Size: 9}})
		require.NoError(t, err)
		require.Equal(t, sitis[0], res.Items[8].ID)
		res, err = env.Members.List(ctx, admin, member.ListQuery{Page: listing.Page{Number: 3, Size: 9}})
		require.NoError(t, err)
		require.Equal(t, sitis[1:], memberIDs(res.Items[:2]))
	})

	t.Run("q", func(t *testing.T) {
		for _, tc := range []struct {
			name, q string
			want    []string
		}{
			{name: "part of a name, ignoring case", q: "RAHAYU", want: []string{"Siti Rahayu", "Siti Rahayu", "Siti Rahayu"}},
			{name: "a founder's name", q: "pak budi", want: []string{o.Admin.Name}},
			{name: "a national number", q: "0857-7710-0001", want: []string{"Agus Salim"}},
			{name: "a number with +62", q: "+62 857 7710 0002", want: []string{"Citra Lestari"}},
			{name: "digits from the middle", q: "812771", want: []string{o.Supervisor.Name, o.Admin.Name, o.Karyawan.Name}},
			{name: "fewer than 3 digits is not a phone search", q: "08", want: []string{}},
			{name: "LIKE characters are literal", q: "_", want: []string{}},
			{name: "another organization's member by name", q: "Slamet", want: []string{}},
			{name: "another organization's member by phone", q: "0896-7720-0001", want: []string{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				require.ElementsMatch(t, tc.want, findMembers(t, env, admin, member.ListQuery{Search: listing.NewSearch(tc.q)}))
			})
		}
		require.Len(t, findMembers(t, env, admin, member.ListQuery{Search: listing.NewSearch("62857")}), 20, "every invited number")
		require.Len(t, findMembers(t, env, admin, member.ListQuery{Search: listing.NewSearch("")}), 23, "empty q is no filter")
	})

	t.Run("status and collectorDisabled", func(t *testing.T) {
		melati, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
		require.NoError(t, err)
		lease := func(label, tenantName, phone string, collector uuid.UUID) tenant.Lease {
			u, err := env.Properties.CreateUnit(ctx, admin, melati.ID, property.UnitInput{Label: label, Terms: testenv.Monthly(850_000)})
			require.NoError(t, err)
			tn, err := env.Tenants.Create(ctx, admin, tenant.Input{Name: tenantName, Phone: phone})
			require.NoError(t, err)
			l, err := env.Tenants.CreateLease(ctx, admin, tenant.NewLease{TenantID: tn.ID, UnitID: u.ID, TermMonths: 1, Terms: tenant.Terms{
				StartDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), CollectorMemberID: collector,
			}})
			require.NoError(t, err)
			return l
		}
		lease("Kamar 1", "Budi Santoso", "0813-7710-0001", invited["Indah Sari"].ID)
		ended := lease("Kamar 2", "Rudi Hartono", "0813-7710-0002", invited["Hendra Gunawan"].ID)
		_, err = env.Tenants.MoveOut(ctx, admin, ended.ID, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		for _, n := range []string{"Indah Sari", "Hendra Gunawan", "Joko Susilo"} {
			_, err := env.Members.SetStatus(ctx, admin, invited[n].ID, member.StatusDisabled)
			require.NoError(t, err)
		}

		for _, tc := range []struct {
			name string
			lq   member.ListQuery
			want int
			only []string
		}{
			{name: "disabled", lq: member.ListQuery{Status: member.StatusDisabled}, only: []string{"Hendra Gunawan", "Indah Sari", "Joko Susilo"}},
			{name: "active", lq: member.ListQuery{Status: member.StatusActive}, want: 20},
			{
				name: "collectorDisabled: disabled and still collector of an active lease, not of an ended one",
				lq:   member.ListQuery{CollectorDisabled: true}, only: []string{"Indah Sari"},
			},
			{name: "collectorDisabled with status active matches nothing", lq: member.ListQuery{CollectorDisabled: true, Status: member.StatusActive}, only: []string{}},
			{name: "status with q", lq: member.ListQuery{Status: member.StatusDisabled, Search: listing.NewSearch("joko")}, only: []string{"Joko Susilo"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got := findMembers(t, env, admin, tc.lq)
				if tc.only != nil {
					require.ElementsMatch(t, tc.only, got)
					return
				}
				require.Len(t, got, tc.want)
			})
		}
		require.Empty(t, findMembers(t, env, otherAdmin, member.ListQuery{CollectorDisabled: true}), "another organization's flags stay there")
	})

	t.Run("an unknown sort or status names the field", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			lq    member.ListQuery
			field string
		}{
			{name: "unknown sort", lq: member.ListQuery{Sort: "phone"}, field: "sort"},
			{name: "unknown status", lq: member.ListQuery{Status: "invited"}, field: "status"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := env.Members.List(ctx, admin, tc.lq)
				var inputErr *member.InputError
				require.ErrorAs(t, err, &inputErr)
				require.Equal(t, tc.field, inputErr.Field)
			})
		}
	})

	t.Run("another organization lists and counts only its own members", func(t *testing.T) {
		res, err := env.Members.List(ctx, otherAdmin, member.ListQuery{})
		require.NoError(t, err)
		require.Equal(t, int64(4), res.Total)
		require.Empty(t, findMembers(t, env, otherAdmin, member.ListQuery{Search: listing.NewSearch("Rahayu")}))
		require.Empty(t, findMembers(t, env, otherAdmin, member.ListQuery{Search: listing.NewSearch("0857-7710")}))
	})
}
