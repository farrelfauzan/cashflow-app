//go:build integration

package property_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestPropertyAndUnitLists(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812781")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	other := env.CreateOrg(t, "Kos Kenanga", "0812782")
	otherAdmin := env.Principal(t, other.ID, other.Admin.UserID)

	// Made out of order, so the natural order is the list's doing.
	var wantNames []string
	props := map[int]property.Property{}
	for _, n := range []int{11, 2, 23, 1, 10, 3, 20, 4, 19, 5, 6, 7, 8, 9, 12, 13, 14, 15, 16, 17, 18, 21, 22} {
		p, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: fmt.Sprintf("Kos Melati %d", n), Kind: property.KindKos})
		require.NoError(t, err)
		props[n] = p
	}
	for n := 1; n <= 23; n++ {
		wantNames = append(wantNames, fmt.Sprintf("Kos Melati %d", n))
	}
	kenanga, err := env.Properties.Create(ctx, otherAdmin, property.PropertyInput{Name: "Kos Kenanga", Kind: property.KindKos})
	require.NoError(t, err)

	melati1 := props[1]
	var wantLabels []string
	for _, n := range []int{12, 2, 1, 11, 3, 10, 4, 5, 6, 7, 8, 9} {
		_, err := env.Properties.CreateUnit(ctx, admin, melati1.ID, property.UnitInput{Label: fmt.Sprintf("Kamar %d", n), DefaultRent: rent(750_000)})
		require.NoError(t, err)
	}
	for n := 1; n <= 12; n++ {
		wantLabels = append(wantLabels, fmt.Sprintf("Kamar %d", n))
	}
	_, err = env.Properties.CreateUnit(ctx, otherAdmin, kenanga.ID, property.UnitInput{Label: "Kamar 1"})
	require.NoError(t, err)

	listProps := func(p authz.Principal, lq property.ListQuery) ([]string, int64) {
		t.Helper()
		res, err := env.Properties.List(ctx, p, lq)
		require.NoError(t, err)
		return names(res.Items), res.Total
	}
	listUnits := func(uq property.UnitQuery) ([]string, int64) {
		t.Helper()
		res, err := env.Properties.ListUnits(ctx, admin, melati1.ID, uq)
		require.NoError(t, err)
		return labels(res.Items), res.Total
	}
	pg := func(n, size int) listing.Page { return listing.Page{Number: n, Size: size} }

	t.Run("properties page in natural order, with the real total past the last page", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			page listing.Page
			want []string
		}{
			{name: "page 1", page: pg(1, 10), want: wantNames[:10]},
			{name: "page 2", page: pg(2, 10), want: wantNames[10:20]},
			{name: "page 3 has the rest", page: pg(3, 10), want: wantNames[20:]},
			{name: "page 4 is past the last", page: pg(4, 10), want: []string{}},
			{name: "the zero page is page 1 at 20", page: listing.Page{}, want: wantNames[:20]},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, total := listProps(admin, property.ListQuery{Page: tc.page})
				require.Equal(t, tc.want, got)
				require.Equal(t, int64(23), total)
			})
		}
	})

	t.Run("sort -name reverses the natural order across pages", func(t *testing.T) {
		var got []string
		for n := 1; n <= 3; n++ {
			page, total := listProps(admin, property.ListQuery{Page: pg(n, 10), Sort: "-name"})
			require.Equal(t, int64(23), total)
			got = append(got, page...)
		}
		want := slices.Clone(wantNames)
		slices.Reverse(want)
		require.Equal(t, want, got)
		got, _ = listProps(admin, property.ListQuery{Page: pg(1, 100), Sort: "name"})
		require.Equal(t, wantNames, got)
	})

	t.Run("properties search by name", func(t *testing.T) {
		for _, tc := range []struct {
			name, q string
			want    []string
		}{
			{name: "empty is no filter", q: "", want: wantNames[:20]},
			{name: "ignoring case", q: "MELATI 2", want: []string{"Kos Melati 2", "Kos Melati 20", "Kos Melati 21", "Kos Melati 22", "Kos Melati 23"}},
			{name: "_ is literal", q: "Melati_2", want: []string{}},
			{name: "another organization's property", q: "Kenanga", want: []string{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, total := listProps(admin, property.ListQuery{Search: listing.NewSearch(tc.q)})
				require.Equal(t, tc.want, got)
				if tc.q != "" {
					require.Equal(t, int64(len(tc.want)), total)
				}
			})
		}
	})

	t.Run("units page in natural order", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			uq   property.UnitQuery
			want []string
		}{
			{name: "page 1", uq: property.UnitQuery{Page: pg(1, 5)}, want: wantLabels[:5]},
			{name: "page 2", uq: property.UnitQuery{Page: pg(2, 5)}, want: wantLabels[5:10]},
			{name: "page 3 has the rest", uq: property.UnitQuery{Page: pg(3, 5)}, want: wantLabels[10:]},
			{name: "page 4 is past the last", uq: property.UnitQuery{Page: pg(4, 5)}, want: []string{}},
			{name: "-label", uq: property.UnitQuery{Page: pg(1, 4), Sort: "-label"}, want: []string{"Kamar 12", "Kamar 11", "Kamar 10", "Kamar 9"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, total := listUnits(tc.uq)
				require.Equal(t, tc.want, got)
				require.Equal(t, int64(12), total)
			})
		}
	})

	t.Run("units search by label", func(t *testing.T) {
		got, total := listUnits(property.UnitQuery{Search: listing.NewSearch("kamar 1")})
		require.Equal(t, []string{"Kamar 1", "Kamar 10", "Kamar 11", "Kamar 12"}, got)
		require.Equal(t, int64(4), total)
		got, total = listUnits(property.UnitQuery{Search: listing.NewSearch("Rumah")})
		require.Empty(t, got)
		require.Zero(t, total)
	})

	t.Run("an unknown sort names the field", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			run  func() error
		}{
			{name: "properties sort by createdAt", run: func() error {
				_, err := env.Properties.List(ctx, admin, property.ListQuery{Sort: "createdAt"})
				return err
			}},
			{name: "properties sort by label", run: func() error {
				_, err := env.Properties.List(ctx, admin, property.ListQuery{Sort: "label"})
				return err
			}},
			{name: "units sort by name", run: func() error {
				_, err := env.Properties.ListUnits(ctx, admin, melati1.ID, property.UnitQuery{Sort: "name"})
				return err
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var inputErr *property.InputError
				require.ErrorAs(t, tc.run(), &inputErr)
				require.Equal(t, "sort", inputErr.Field)
			})
		}
	})

	t.Run("a property-limited admin lists and counts only her property", func(t *testing.T) {
		sari, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-7810-0099", Name: "Mbak Sari", Roles: grantKaryawan()})
		require.NoError(t, err)
		_, err = env.Members.SetRoles(ctx, admin, sari.ID, []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: props[2].ID}})
		require.NoError(t, err)
		limited := env.Principal(t, o.ID, sari.UserID)

		got, total := listProps(limited, property.ListQuery{})
		require.Equal(t, []string{"Kos Melati 2"}, got)
		require.Equal(t, int64(1), total)
		got, total = listProps(limited, property.ListQuery{Search: listing.NewSearch("Melati 1")})
		require.Empty(t, got)
		require.Zero(t, total)
		got, total = listProps(limited, property.ListQuery{Page: pg(2, 1)})
		require.Empty(t, got)
		require.Equal(t, int64(1), total)

		_, err = env.Properties.ListUnits(ctx, limited, melati1.ID, property.UnitQuery{})
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("another organization lists only its own properties and cannot list ours", func(t *testing.T) {
		got, total := listProps(otherAdmin, property.ListQuery{})
		require.Equal(t, []string{"Kos Kenanga"}, got)
		require.Equal(t, int64(1), total)
		got, total = listProps(otherAdmin, property.ListQuery{Search: listing.NewSearch("Melati")})
		require.Empty(t, got)
		require.Zero(t, total)

		_, err := env.Properties.ListUnits(ctx, otherAdmin, melati1.ID, property.UnitQuery{})
		require.ErrorIs(t, err, property.ErrNotFound)
		_, err = env.Properties.ListUnits(ctx, admin, kenanga.ID, property.UnitQuery{})
		require.ErrorIs(t, err, property.ErrNotFound)
	})
}

func grantKaryawan() []member.RoleGrant { return []member.RoleGrant{{Role: authz.RoleKaryawan}} }
