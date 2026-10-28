//go:build integration

package demoseed_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/demoseed"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func deps(env *testenv.Env) demoseed.Deps {
	return demoseed.Deps{
		Store: env.Store, Authz: env.Authz, Orgs: env.Orgs, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants,
	}
}

// tableState is what one organization's scope sees of one table: its row count and, when the
// table has an id column, the sorted ids.
type tableState struct {
	Rows int
	IDs  []string
}

// tables lists the public tables the app role can read, with whether each has an id and an
// organization_id column.
func tables(t *testing.T, ctx context.Context, tx pgx.Tx) map[string][2]bool {
	t.Helper()
	rows, err := tx.Query(ctx, `
		SELECT table_name,
		       bool_or(column_name = 'id'),
		       bool_or(column_name = 'organization_id')
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name <> 'goose_db_version'
		GROUP BY table_name`)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string][2]bool{}
	for rows.Next() {
		var name string
		var hasID, hasOrg bool
		require.NoError(t, rows.Scan(&name, &hasID, &hasOrg))
		out[name] = [2]bool{hasID, hasOrg}
	}
	require.NoError(t, rows.Err())
	require.Contains(t, out, "lease", "the app role should see the business tables")
	return out
}

// snapshot reads every table from inside each organization's scope.
func snapshot(t *testing.T, env *testenv.Env, orgs ...uuid.UUID) map[uuid.UUID]map[string]tableState {
	t.Helper()
	ctx := context.Background()
	out := map[uuid.UUID]map[string]tableState{}
	for _, org := range orgs {
		state := map[string]tableState{}
		err := env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			for name, cols := range tables(t, ctx, tx) {
				ident := pgx.Identifier{name}.Sanitize()
				var s tableState
				require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM "+ident).Scan(&s.Rows))
				if cols[0] {
					rows, err := tx.Query(ctx, "SELECT id::text FROM "+ident+" ORDER BY id")
					require.NoError(t, err)
					ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
					require.NoError(t, err)
					s.IDs = ids
				}
				state[name] = s
			}
			return nil
		})
		require.NoError(t, err)
		out[org] = state
	}
	return out
}

func names(ts []tenant.Tenant) []string {
	out := make([]string, len(ts))
	for i, tn := range ts {
		out[i] = tn.Name
	}
	slices.Sort(out)
	return out
}

func find(t *testing.T, ts []tenant.Tenant, name string) tenant.Tenant {
	t.Helper()
	for _, tn := range ts {
		if tn.Name == name {
			return tn
		}
	}
	t.Fatalf("tenant %s not found", name)
	return tenant.Tenant{}
}

func leaseUnits(tn tenant.Tenant) []string {
	var out []string
	for _, l := range tn.Leases {
		out = append(out, l.PropertyName+"/"+l.UnitLabel)
	}
	slices.Sort(out)
	return out
}

func TestSeedDemo(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // the clock reads 2 November 2026, 09:00 in Jakarta

	first, err := demoseed.Run(ctx, deps(env), env.Clock)
	require.NoError(t, err)
	require.Empty(t, first.Notes)

	a, err := env.Orgs.FindForMember(ctx, "0812-0001-0002", demoseed.OrgGriya)
	require.NoError(t, err)
	b, err := env.Orgs.FindForMember(ctx, "0812-0002-0002", demoseed.OrgAnggrek)
	require.NoError(t, err)
	before := snapshot(t, env, a.OrganizationID, b.OrganizationID)

	t.Run("running it again creates and changes nothing", func(t *testing.T) {
		second, err := demoseed.Run(ctx, deps(env), env.Clock)
		require.NoError(t, err)
		require.Empty(t, second.Notes)
		require.Equal(t, first, second)

		a2, err := env.Orgs.FindForMember(ctx, "0812-0001-0002", demoseed.OrgGriya)
		require.NoError(t, err)
		require.Equal(t, a, a2)
		require.Equal(t, before, snapshot(t, env, a.OrganizationID, b.OrganizationID))
	})

	t.Run("the counts match the demo contents", func(t *testing.T) {
		ga, gb := before[a.OrganizationID], before[b.OrganizationID]
		require.Equal(t, 7, ga["membership"].Rows)
		require.Equal(t, 3, ga["property"].Rows)
		require.Equal(t, 10, ga["unit"].Rows)
		require.Equal(t, 7, ga["tenant"].Rows)
		require.Equal(t, 8, ga["lease"].Rows, "seven running leases and Dewi's ended one")
		require.Equal(t, 5, ga["charge_type"].Rows)
		require.Equal(t, 4, ga["lease_charge"].Rows, "Wi-Fi on two rooms, parkir and kebersihan on Ruko 1")
		require.Equal(t, 4, gb["membership"].Rows)
		require.Equal(t, 1, gb["property"].Rows)
		require.Equal(t, 4, gb["unit"].Rows)
		require.Equal(t, 3, gb["tenant"].Rows)
		require.Equal(t, 3, gb["lease"].Rows)
		require.Zero(t, gb["charge_type"].Rows)
		require.Zero(t, gb["lease_charge"].Rows)
		require.Positive(t, ga["audit_log"].Rows, "the snapshot covers the audit log too")
		require.Equal(t, 7, ga["staff_user"].Rows, "an organization scope sees its own members' users")
	})

	t.Run("the login list has every member with E.164 phones", func(t *testing.T) {
		require.Len(t, first.Logins, 11)
		var disabled []string
		for _, l := range first.Logins {
			require.Regexp(t, `^\+62\d+$`, l.Phone)
			require.NotEmpty(t, l.Roles)
			if l.Disabled {
				disabled = append(disabled, l.Organization+"/"+l.Name)
			}
		}
		require.Equal(t, []string{"Griya Sejahtera/Mas Agus Setiawan"}, disabled)
		limits := map[string][]string{}
		for _, l := range first.Logins {
			if len(l.PropertyLimits) > 0 {
				limits[l.Organization+"/"+l.Name] = l.PropertyLimits
			}
		}
		require.Equal(t, map[string][]string{
			"Griya Sejahtera/Mbak Sari Wulandari": {"Admin: Kos Melati"},
			"Griya Sejahtera/Mas Joko Prasetyo":   {"Admin: Kontrakan Mawar"},
		}, limits)
	})

	adminA := env.Principal(t, a.OrganizationID, a.UserID)
	adminB := env.Principal(t, b.OrganizationID, b.UserID)
	tenantsA, err := env.AllTenants(ctx, adminA)
	require.NoError(t, err)
	tenantsB, err := env.AllTenants(ctx, adminB)
	require.NoError(t, err)

	t.Run("the limited admin sees only Kos Melati's tenants", func(t *testing.T) {
		sari := env.Principal(t, a.OrganizationID, env.UserID(t, "0812-0001-0004"))
		got, err := env.AllTenants(ctx, sari)
		require.NoError(t, err)
		require.Equal(t, []string{"Budi Santoso", "Citra Lestari", "Fajar Nugroho", "Rina Wulandari"}, names(got))
		require.Equal(t, []string{"Kos Melati/Kamar 1"}, leaseUnits(find(t, got, "Rina Wulandari")))
	})

	t.Run("Dimas sees only the tenants he collects from", func(t *testing.T) {
		dimas := env.Principal(t, a.OrganizationID, env.UserID(t, "0812-0001-0003"))
		got, err := env.AllTenants(ctx, dimas)
		require.NoError(t, err)
		require.Equal(t, []string{"Budi Santoso", "Rina Wulandari"}, names(got))
		require.Equal(t, []string{"Kos Melati/Kamar 1"}, leaseUnits(find(t, got, "Rina Wulandari")))
	})

	t.Run("Joko is karyawan everywhere and admin of Kontrakan Mawar", func(t *testing.T) {
		members, err := env.AllMembers(ctx, adminA)
		require.NoError(t, err)
		mawar := propertyID(t, env, adminA, "Kontrakan Mawar")
		var joko member.Member
		for _, m := range members {
			if m.Phone == "+6281200010005" {
				joko = m
			}
		}
		require.Equal(t, "Mas Joko Prasetyo", joko.Name)
		var got []member.RoleRef
		for _, r := range joko.Roles {
			got = append(got, member.RoleRef{Key: r.Key, PropertyID: r.PropertyID})
		}
		require.ElementsMatch(t, []member.RoleRef{
			{Key: authz.RoleKaryawan},
			{Key: authz.RoleAdmin, PropertyID: mawar},
		}, got)

		p := env.Principal(t, a.OrganizationID, joko.UserID)
		seen, err := env.AllTenants(ctx, p)
		require.NoError(t, err)
		require.Equal(t, []string{"Citra Lestari", "Eko Prasetyo", "Rina Wulandari", "Toko Sumber Rejeki (Pak Agus)"}, names(seen))
	})

	t.Run("Hendra belongs to both organizations with different roles", func(t *testing.T) {
		hendra := env.UserID(t, "0812-0009-0001")
		orgs, err := env.Auth.Organizations(ctx, hendra)
		require.NoError(t, err)
		var got []uuid.UUID
		for _, o := range orgs {
			got = append(got, o.ID)
		}
		require.ElementsMatch(t, []uuid.UUID{a.OrganizationID, b.OrganizationID}, got)
		require.Equal(t, []string{authz.RoleAdmin}, roleKeys(env.Principal(t, a.OrganizationID, hendra)))
		require.Equal(t, []string{authz.RoleSupervisor}, roleKeys(env.Principal(t, b.OrganizationID, hendra)))
	})

	t.Run("Agus is disabled and still collects from Eko", func(t *testing.T) {
		members, err := env.AllMembers(ctx, adminA)
		require.NoError(t, err)
		var agus member.Member
		for _, m := range members {
			if m.Phone == "+6281200010006" {
				agus = m
			}
		}
		require.Equal(t, "Mas Agus Setiawan", agus.Name)
		require.Equal(t, member.StatusDisabled, agus.Status)
		require.Equal(t, int64(1), agus.CollectorLeaseCount, "the flag: 1 penyewa belum punya penagih")

		eko := find(t, tenantsA, "Eko Prasetyo")
		require.Len(t, eko.Leases, 1)
		require.Equal(t, agus.ID, eko.Leases[0].CollectorMemberID)
		require.False(t, eko.Leases[0].CollectorActive)
	})

	t.Run("Dewi has moved out and Fajar's booking starts next month", func(t *testing.T) {
		dewi := find(t, tenantsA, "Dewi Anggraini")
		require.Len(t, dewi.Leases, 1)
		require.Equal(t, tenant.StatusEnded, dewi.Leases[0].Status)
		require.NotNil(t, dewi.Leases[0].EndDate)
		require.Equal(t, time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), *dewi.Leases[0].EndDate)

		fajar := find(t, tenantsA, "Fajar Nugroho")
		require.Len(t, fajar.Leases, 1)
		require.Equal(t, tenant.StatusActive, fajar.Leases[0].Status)
		require.Equal(t, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), fajar.Leases[0].StartDate)
	})

	t.Run("Rina has two leases in A and a separate record in B", func(t *testing.T) {
		rinaA, rinaB := find(t, tenantsA, "Rina Wulandari"), find(t, tenantsB, "Rina Wulandari")
		require.Equal(t, rinaA.Phone, rinaB.Phone)
		require.NotEqual(t, rinaA.ID, rinaB.ID)
		require.Equal(t, []string{"Kontrakan Mawar/Rumah B", "Kos Melati/Kamar 1"}, leaseUnits(rinaA))
		require.Equal(t, []string{"Kos Putri Anggrek/Kamar 3"}, leaseUnits(rinaB))
	})

	t.Run("a few leases carry fixed extras", func(t *testing.T) {
		types, err := env.Tenants.ListChargeTypes(ctx, adminA)
		require.NoError(t, err)
		var typeNames []string
		for _, ct := range types {
			require.False(t, ct.Metered, ct.Name)
			typeNames = append(typeNames, ct.Name)
		}
		require.Equal(t, []string{"Air", "Kebersihan", "Listrik", "Parkir", "Wi-Fi"}, typeNames)

		got := map[string][]tenant.Charge{}
		for _, tn := range tenantsA {
			for _, l := range tn.Leases {
				require.NotNil(t, l.Charges, "%s %s", l.PropertyName, l.UnitLabel)
				for _, c := range l.Charges {
					// Ids differ per run; names and amounts are fixed.
					got[tn.Name+"/"+l.UnitLabel] = append(got[tn.Name+"/"+l.UnitLabel], tenant.Charge{Name: c.Name, Amount: c.Amount})
				}
			}
		}
		require.Equal(t, map[string][]tenant.Charge{
			"Rina Wulandari/Kamar 1": {{Name: "Wi-Fi", Amount: 50_000}},
			"Budi Santoso/Kamar 2":   {{Name: "Wi-Fi", Amount: 50_000}},
			"Toko Sumber Rejeki (Pak Agus)/Ruko 1": {
				{Name: "Kebersihan", Amount: 50_000}, {Name: "Parkir", Amount: 100_000},
			},
		}, got)
	})

	t.Run("a lease whose extras were changed keeps them on a rerun", func(t *testing.T) {
		budi := find(t, tenantsA, "Budi Santoso")
		_, err := env.Tenants.SetLeaseCharges(ctx, adminA, budi.Leases[0].ID, []tenant.ChargeInput{
			{ChargeTypeID: budi.Leases[0].Charges[0].ChargeTypeID, Amount: 60_000},
		})
		require.NoError(t, err)
		_, err = demoseed.Run(ctx, deps(env), env.Clock)
		require.NoError(t, err)
		again, err := env.Tenants.Get(ctx, adminA, budi.ID)
		require.NoError(t, err)
		require.Len(t, again.Leases[0].Charges, 1)
		require.Equal(t, int64(60_000), again.Leases[0].Charges[0].Amount, "the seed never overwrites extras a lease already has")
	})

	t.Run("A's admin cannot see B's tenants", func(t *testing.T) {
		for _, tn := range tenantsA {
			for _, l := range tn.Leases {
				require.NotEqual(t, "Kos Putri Anggrek", l.PropertyName)
			}
		}
		require.NotContains(t, names(tenantsA), "Ayu Lestari")
		require.NotContains(t, names(tenantsA), "Nadia Putri")
		for _, tn := range tenantsB {
			_, err := env.Tenants.Get(ctx, adminA, tn.ID)
			require.ErrorIs(t, err, tenant.ErrNotFound, "A read B's %s", tn.Name)
		}
	})

	t.Run("no table shows A's scope a row of B", func(t *testing.T) {
		// The store package's isolation test is internal to that package, so the check is
		// repeated here against the seeded data: every table with organization_id.
		for _, pair := range [][2]uuid.UUID{{a.OrganizationID, b.OrganizationID}, {b.OrganizationID, a.OrganizationID}} {
			scope, other := pair[0], pair[1]
			err := env.Store.WithOrgTx(ctx, scope, func(tx pgx.Tx) error {
				checked := 0
				for name, cols := range tables(t, ctx, tx) {
					if !cols[1] {
						continue
					}
					var n int
					q := "SELECT count(*) FROM " + pgx.Identifier{name}.Sanitize() + " WHERE organization_id = $1"
					require.NoError(t, tx.QueryRow(ctx, q, other).Scan(&n))
					require.Zero(t, n, "table %s shows another organization's rows", name)
					checked++
				}
				require.GreaterOrEqual(t, checked, 6)
				return nil
			})
			require.NoError(t, err)
		}
	})

	// Last: it moves Eko out, which the subtests above rely on.
	t.Run("a lease recreated on a rerun gets no disabled collector", func(t *testing.T) {
		eko := find(t, tenantsA, "Eko Prasetyo")
		_, err := env.Tenants.MoveOut(ctx, adminA, eko.Leases[0].ID, time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		res, err := demoseed.Run(ctx, deps(env), env.Clock)
		require.NoError(t, err)
		require.Len(t, res.Notes, 1)
		require.Contains(t, res.Notes[0], "Kontrakan Mawar Rumah A: collector")
		require.Contains(t, res.Notes[0], "is disabled; lease created without a collector")

		again, err := env.Tenants.Get(ctx, adminA, eko.ID)
		require.NoError(t, err)
		require.Len(t, again.Leases, 2)
		for _, l := range again.Leases {
			if l.Status == tenant.StatusActive {
				require.Equal(t, uuid.Nil, l.CollectorMemberID)
			}
		}
	})
}

func roleKeys(p authz.Principal) []string {
	var out []string
	for _, r := range p.Roles {
		out = append(out, r.Key)
	}
	return out
}

func propertyID(t *testing.T, env *testenv.Env, p authz.Principal, name string) uuid.UUID {
	t.Helper()
	props, err := env.AllProperties(context.Background(), p)
	require.NoError(t, err)
	for _, pr := range props {
		if pr.Name == name {
			return pr.ID
		}
	}
	t.Fatalf("property %s not found", name)
	return uuid.Nil
}
