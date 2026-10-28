//go:build integration

package tenant_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func requireField(t *testing.T, err error, field string) {
	t.Helper()
	var inputErr *tenant.InputError
	require.ErrorAs(t, err, &inputErr)
	require.Equal(t, field, inputErr.Field)
}

func tenantNames(ts []tenant.Tenant) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}

// world is an organization with two properties, three units and the principals that act on
// them.
type world struct {
	org                         testenv.Org
	admin, supervisor, karyawan authz.Principal
	karyawanMemberID            uuid.UUID
	melati, mawar               property.Property
	kamar1, kamar2, rumahA      property.Unit
}

func newWorld(t *testing.T, env *testenv.Env, prefix string) world {
	t.Helper()
	ctx := context.Background()
	w := world{org: env.CreateOrg(t, "Griya Sejahtera "+prefix, prefix)}
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	w.supervisor = env.Principal(t, w.org.ID, w.org.Supervisor.UserID)
	w.karyawan = env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	w.karyawanMemberID = w.karyawan.MembershipID

	var err error
	w.melati, err = env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	w.mawar, err = env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kontrakan Mawar", Kind: property.KindKontrakan})
	require.NoError(t, err)
	unit := func(p property.Property, label string) property.Unit {
		u, err := env.Properties.CreateUnit(ctx, w.admin, p.ID, property.UnitInput{Label: label})
		require.NoError(t, err)
		return u
	}
	w.kamar1, w.kamar2, w.rumahA = unit(w.melati, "Kamar 1"), unit(w.melati, "Kamar 2"), unit(w.mawar, "Rumah A")
	return w
}

func TestTenantsAndLeases(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // the clock reads 2 November 2026, 09:00 in Jakarta
	w := newWorld(t, env, "0812711")

	var rina tenant.Tenant
	t.Run("an admin adds a tenant", func(t *testing.T) {
		var err error
		rina, err = env.Tenants.Create(ctx, w.admin, tenant.Input{Name: " Rina Wulandari ", Phone: "0812-7110-5001", Note: "Mahasiswi UGM"})
		require.NoError(t, err)
		require.Equal(t, "Rina Wulandari", rina.Name)
		require.Equal(t, "+6281271105001", rina.Phone)
		require.Empty(t, rina.Leases)
	})

	t.Run("the same number in any common format is the same tenant", func(t *testing.T) {
		for _, raw := range []string{"+62 812 7110 5001", "6281271105001", "81271105001", "0812.7110.5001", "(0812) 7110-5001"} {
			_, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Rina lagi", Phone: raw})
			require.ErrorIs(t, err, tenant.ErrPhoneTaken, raw)
		}
	})

	var lease1 tenant.Lease
	t.Run("an admin rents Kamar 1 to Rina, collected by the karyawan", func(t *testing.T) {
		var err error
		lease1, err = env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: rina.ID, UnitID: w.kamar1.ID, Terms: tenant.Terms{
			StartDate: day(2026, 10, 1), RentAmount: 850_000, DepositAmount: 850_000, CollectorMemberID: w.karyawanMemberID,
		}})
		require.NoError(t, err)
		require.Equal(t, tenant.StatusActive, lease1.Status)
		require.Equal(t, "Kamar 1", lease1.UnitLabel)
		require.Equal(t, "Kos Melati", lease1.PropertyName)
		require.Equal(t, w.org.Karyawan.UserID, lease1.CollectorUserID)
		require.Equal(t, w.org.Karyawan.Name, lease1.CollectorName)
		require.Equal(t, day(2026, 10, 1), lease1.StartDate)
	})

	t.Run("a unit has one active lease", func(t *testing.T) {
		budi, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Budi Santoso", Phone: "0812-7110-5002"})
		require.NoError(t, err)
		_, err = env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: budi.ID, UnitID: w.kamar1.ID, Terms: tenant.Terms{
			StartDate: day(2026, 11, 1), RentAmount: 900_000,
		}})
		require.ErrorIs(t, err, tenant.ErrUnitOccupied)
	})

	t.Run("a tenant can rent two units", func(t *testing.T) {
		_, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: rina.ID, UnitID: w.rumahA.ID, Terms: tenant.Terms{
			StartDate: day(2026, 11, 1), RentAmount: 15_000_000,
		}})
		require.NoError(t, err)
		got, err := env.Tenants.Get(ctx, w.admin, rina.ID)
		require.NoError(t, err)
		require.Len(t, got.Leases, 2)
	})

	t.Run("bad lease input names the field", func(t *testing.T) {
		good := tenant.Terms{StartDate: day(2026, 11, 1), RentAmount: 500_000}
		cases := map[string]tenant.NewLease{
			"tenantId":          {TenantID: uuid.New(), UnitID: w.kamar2.ID, Terms: good},
			"unitId":            {TenantID: rina.ID, UnitID: uuid.New(), Terms: good},
			"startDate":         {TenantID: rina.ID, UnitID: w.kamar2.ID, Terms: tenant.Terms{RentAmount: 500_000}},
			"rentAmount":        {TenantID: rina.ID, UnitID: w.kamar2.ID, Terms: tenant.Terms{StartDate: good.StartDate}},
			"depositAmount":     {TenantID: rina.ID, UnitID: w.kamar2.ID, Terms: tenant.Terms{StartDate: good.StartDate, RentAmount: 1, DepositAmount: -1}},
			"collectorMemberId": {TenantID: rina.ID, UnitID: w.kamar2.ID, Terms: tenant.Terms{StartDate: good.StartDate, RentAmount: 1, CollectorMemberID: uuid.New()}},
		}
		for field, in := range cases {
			_, err := env.Tenants.CreateLease(ctx, w.admin, in)
			requireField(t, err, field)
		}
		_, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "X", Phone: "12345"})
		requireField(t, err, "phone")
		_, err = env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "", Phone: "081271109999"})
		requireField(t, err, "name")
	})

	t.Run("an admin changes the rent, and the change is audited", func(t *testing.T) {
		updated, err := env.Tenants.UpdateLease(ctx, w.admin, lease1.ID, tenant.Terms{
			StartDate: day(2026, 10, 1), RentAmount: 900_000, DepositAmount: 850_000, CollectorMemberID: w.karyawanMemberID,
		})
		require.NoError(t, err)
		require.Equal(t, int64(900_000), updated.RentAmount)

		audit := env.AuditLog(t, w.org.ID)
		require.Equal(t, "allow", audit[0].Decision)
		require.Equal(t, "lease:manage", audit[0].Action)
		require.Equal(t, "property/"+w.melati.ID.String()+"/lease/"+lease1.ID.String(), audit[0].Resource)
	})

	t.Run("an admin changes a tenant's number", func(t *testing.T) {
		updated, err := env.Tenants.Update(ctx, w.admin, rina.ID, tenant.Input{Name: "Rina Wulandari", Phone: "+62 812-7110-5009"})
		require.NoError(t, err)
		require.Equal(t, "+6281271105009", updated.Phone)
		require.Len(t, updated.Leases, 2)

		_, err = env.Tenants.Update(ctx, w.admin, rina.ID, tenant.Input{Name: "Rina", Phone: "0812-7110-5002"})
		require.ErrorIs(t, err, tenant.ErrPhoneTaken, "Budi has that number")
	})

	var booking tenant.Lease
	t.Run("move-out ends the lease on a date and frees the unit", func(t *testing.T) {
		_, err := env.Tenants.MoveOut(ctx, w.admin, lease1.ID, day(2026, 11, 3))
		requireField(t, err, "endDate") // tomorrow in Jakarta
		_, err = env.Tenants.MoveOut(ctx, w.admin, lease1.ID, day(2026, 9, 30))
		requireField(t, err, "endDate") // before the start

		ended, err := env.Tenants.MoveOut(ctx, w.admin, lease1.ID, day(2026, 11, 2))
		require.NoError(t, err)
		require.Equal(t, tenant.StatusEnded, ended.Status)
		require.Equal(t, day(2026, 11, 2), *ended.EndDate)

		_, err = env.Tenants.MoveOut(ctx, w.admin, lease1.ID, day(2026, 11, 2))
		require.ErrorIs(t, err, tenant.ErrLeaseEnded)
		_, err = env.Tenants.UpdateLease(ctx, w.admin, lease1.ID, tenant.Terms{StartDate: day(2026, 10, 1), RentAmount: 1})
		require.ErrorIs(t, err, tenant.ErrLeaseEnded)

		tenants, err := env.Tenants.List(ctx, w.admin)
		require.NoError(t, err)
		var budi tenant.Tenant
		for _, tn := range tenants {
			if tn.Name == "Budi Santoso" {
				budi = tn
			}
		}
		booking, err = env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: budi.ID, UnitID: w.kamar1.ID, Terms: tenant.Terms{
			StartDate: day(2026, 11, 3), RentAmount: 900_000,
		}})
		require.NoError(t, err, "the unit is free again, and a lease may start tomorrow")
	})

	t.Run("a booking that has not started is cancelled by ending it on its start date", func(t *testing.T) {
		_, err := env.Tenants.MoveOut(ctx, w.admin, booking.ID, day(2026, 11, 2))
		requireField(t, err, "endDate") // today, but the lease starts tomorrow
		_, err = env.Tenants.MoveOut(ctx, w.admin, booking.ID, day(2026, 11, 4))
		requireField(t, err, "endDate")

		cancelled, err := env.Tenants.MoveOut(ctx, w.admin, booking.ID, day(2026, 11, 3))
		require.NoError(t, err)
		require.Equal(t, tenant.StatusEnded, cancelled.Status)
		require.Equal(t, cancelled.StartDate, *cancelled.EndDate)
		_, err = env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: booking.TenantID, UnitID: w.kamar1.ID, Terms: tenant.Terms{
			StartDate: day(2026, 11, 2), RentAmount: 900_000,
		}})
		require.NoError(t, err, "the unit is free again")
	})

	t.Run("the supervisor reads every tenant but never writes", func(t *testing.T) {
		tenants, err := env.Tenants.List(ctx, w.supervisor)
		require.NoError(t, err)
		require.Equal(t, []string{"Budi Santoso", "Rina Wulandari"}, tenantNames(tenants))
		_, err = env.Tenants.Create(ctx, w.supervisor, tenant.Input{Name: "X", Phone: "081271109998"})
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = env.Tenants.MoveOut(ctx, w.supervisor, lease1.ID, day(2026, 11, 2))
		require.ErrorIs(t, err, authz.ErrForbidden)
	})
}

func TestKaryawanSeesOnlyTheirTenants(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "0812721")

	newTenant := func(name, phone string) tenant.Tenant {
		tn, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: phone})
		require.NoError(t, err)
		return tn
	}
	rent := func(tn tenant.Tenant, u property.Unit, collector uuid.UUID) tenant.Lease {
		l, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: tn.ID, UnitID: u.ID, Terms: tenant.Terms{
			StartDate: day(2026, 10, 1), RentAmount: 750_000, CollectorMemberID: collector,
		}})
		require.NoError(t, err)
		return l
	}

	sari, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-7210-0099", Name: "Mbak Sari", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)

	rina := newTenant("Rina Wulandari", "0812-7210-5001") // collected by our karyawan
	budi := newTenant("Budi Santoso", "0812-7210-5002")   // collected by Sari
	citra := newTenant("Citra Lestari", "0812-7210-5003") // no collector
	dewi := newTenant("Dewi Anggraini", "0812-7210-5004") // two leases, one each
	_ = newTenant("Eko Prasetyo", "0812-7210-5005")       // no lease at all
	rinaLease := rent(rina, w.kamar1, w.karyawanMemberID)
	rent(budi, w.kamar2, sari.ID)
	rent(citra, w.rumahA, uuid.Nil)
	unit3, err := env.Properties.CreateUnit(ctx, w.admin, w.mawar.ID, property.UnitInput{Label: "Rumah B"})
	require.NoError(t, err)
	unit4, err := env.Properties.CreateUnit(ctx, w.admin, w.mawar.ID, property.UnitInput{Label: "Rumah C"})
	require.NoError(t, err)
	dewiMine := rent(dewi, unit3, w.karyawanMemberID)
	rent(dewi, unit4, sari.ID)

	t.Run("the list holds only their tenants and only their leases", func(t *testing.T) {
		tenants, err := env.Tenants.List(ctx, w.karyawan)
		require.NoError(t, err)
		require.Equal(t, []string{"Dewi Anggraini", "Rina Wulandari"}, tenantNames(tenants))
		require.Len(t, tenants[0].Leases, 1)
		require.Equal(t, dewiMine.ID, tenants[0].Leases[0].ID)
	})

	t.Run("an admin sees every tenant, with or without a lease", func(t *testing.T) {
		tenants, err := env.Tenants.List(ctx, w.admin)
		require.NoError(t, err)
		require.Len(t, tenants, 5)
	})

	t.Run("someone else's tenant is forbidden, and the attempt is audited", func(t *testing.T) {
		_, err := env.Tenants.Get(ctx, w.karyawan, budi.ID)
		require.ErrorIs(t, err, authz.ErrForbidden)
		audit := env.AuditLog(t, w.org.ID)
		require.Equal(t, "deny", audit[0].Decision)
		require.Equal(t, "tenant:read", audit[0].Action)

		got, err := env.Tenants.Get(ctx, w.karyawan, rina.ID)
		require.NoError(t, err)
		require.Len(t, got.Leases, 1)
	})

	t.Run("reassigning the collector moves the tenant to the other karyawan", func(t *testing.T) {
		_, err := env.Tenants.UpdateLease(ctx, w.admin, dewiMine.ID, tenant.Terms{
			StartDate: dewiMine.StartDate, RentAmount: dewiMine.RentAmount, CollectorMemberID: sari.ID,
		})
		require.NoError(t, err)
		tenants, err := env.Tenants.List(ctx, w.karyawan)
		require.NoError(t, err)
		require.Equal(t, []string{"Rina Wulandari"}, tenantNames(tenants))
	})

	t.Run("a karyawan changes no tenant or lease", func(t *testing.T) {
		_, err := env.Tenants.Create(ctx, w.karyawan, tenant.Input{Name: "X", Phone: "081272109990"})
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = env.Tenants.Update(ctx, w.karyawan, rina.ID, tenant.Input{Name: "Rina", Phone: "0812-7210-5001"})
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("after move-out the former collector no longer sees the tenant", func(t *testing.T) {
		_, err := env.Tenants.MoveOut(ctx, w.admin, rinaLease.ID, day(2026, 11, 1))
		require.NoError(t, err)
		tenants, err := env.Tenants.List(ctx, w.karyawan)
		require.NoError(t, err)
		require.Empty(t, tenants)
		_, err = env.Tenants.Get(ctx, w.karyawan, rina.ID)
		require.ErrorIs(t, err, authz.ErrForbidden)

		got, err := env.Tenants.Get(ctx, w.admin, rina.ID)
		require.NoError(t, err)
		require.Len(t, got.Leases, 1, "an organization-wide admin still sees the tenant and the ended lease")
	})
}

func TestAdminLimitedToOnePropertyAndTenants(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "0812731")

	sari, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-7310-0099", Name: "Mbak Sari", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)
	_, err = env.Members.SetRoles(ctx, w.admin, sari.ID, []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: w.melati.ID}})
	require.NoError(t, err)
	limited := env.Principal(t, w.org.ID, sari.UserID)

	rina, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Rina", Phone: "0812-7310-5001"})
	require.NoError(t, err)
	dewi, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Dewi", Phone: "0812-7310-5002"})
	require.NoError(t, err)
	terms := tenant.Terms{StartDate: day(2026, 10, 1), RentAmount: 800_000}
	rinaKamar1, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: rina.ID, UnitID: w.kamar1.ID, Terms: terms})
	require.NoError(t, err)
	_, err = env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: dewi.ID, UnitID: w.kamar2.ID, Terms: terms})
	require.NoError(t, err)
	_, err = env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: dewi.ID, UnitID: w.rumahA.ID, Terms: terms})
	require.NoError(t, err)

	t.Run("she sees the tenants with a lease in Kos Melati, and only those leases", func(t *testing.T) {
		tenants, err := env.Tenants.List(ctx, limited)
		require.NoError(t, err)
		require.Equal(t, []string{"Dewi", "Rina"}, tenantNames(tenants))
		require.Len(t, tenants[0].Leases, 1, "Dewi's lease in Kontrakan Mawar stays hidden")
	})

	t.Run("she changes a tenant whose active leases are all hers, not one who also rents elsewhere", func(t *testing.T) {
		_, err := env.Tenants.Update(ctx, limited, rina.ID, tenant.Input{Name: "Rina W.", Phone: "0812-7310-5001"})
		require.NoError(t, err)
		_, err = env.Tenants.Update(ctx, limited, dewi.ID, tenant.Input{Name: "Dewi A.", Phone: "0812-7310-5002"})
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	var rinaKamar3 tenant.Lease
	t.Run("she rents units in Kos Melati only, and cannot add tenants", func(t *testing.T) {
		kamar3, err := env.Properties.CreateUnit(ctx, limited, w.melati.ID, property.UnitInput{Label: "Kamar 3"})
		require.NoError(t, err)
		rinaKamar3, err = env.Tenants.CreateLease(ctx, limited, tenant.NewLease{TenantID: rina.ID, UnitID: kamar3.ID, Terms: terms})
		require.NoError(t, err)
		rumahB, err := env.Properties.CreateUnit(ctx, w.admin, w.mawar.ID, property.UnitInput{Label: "Rumah B"})
		require.NoError(t, err)
		_, err = env.Tenants.CreateLease(ctx, limited, tenant.NewLease{TenantID: rina.ID, UnitID: rumahB.ID, Terms: terms})
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = env.Tenants.Create(ctx, limited, tenant.Input{Name: "Baru", Phone: "0812-7310-5999"})
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("she sees a tenant only while one of their leases in Kos Melati is active", func(t *testing.T) {
		_, err := env.Tenants.MoveOut(ctx, limited, rinaKamar1.ID, day(2026, 11, 1))
		require.NoError(t, err)
		got, err := env.Tenants.Get(ctx, limited, rina.ID)
		require.NoError(t, err)
		require.Len(t, got.Leases, 1, "the ended Kamar 1 lease is hidden, Kamar 3 still shows")
		require.Equal(t, rinaKamar3.ID, got.Leases[0].ID)

		_, err = env.Tenants.MoveOut(ctx, limited, rinaKamar3.ID, day(2026, 11, 1))
		require.NoError(t, err)
		_, err = env.Tenants.Get(ctx, limited, rina.ID)
		require.ErrorIs(t, err, authz.ErrForbidden)
		tenants, err := env.Tenants.List(ctx, limited)
		require.NoError(t, err)
		require.Equal(t, []string{"Dewi"}, tenantNames(tenants))
	})
}

func TestTenantsStayInsideTheirOrganization(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	a := newWorld(t, env, "0812741")
	b := newWorld(t, env, "0812742")

	tenantB, err := env.Tenants.Create(ctx, b.admin, tenant.Input{Name: "Rina", Phone: "0812-7420-5001"})
	require.NoError(t, err)
	leaseB, err := env.Tenants.CreateLease(ctx, b.admin, tenant.NewLease{TenantID: tenantB.ID, UnitID: b.kamar1.ID, Terms: tenant.Terms{
		StartDate: day(2026, 10, 1), RentAmount: 800_000,
	}})
	require.NoError(t, err)

	t.Run("the same phone can be a tenant in two organizations", func(t *testing.T) {
		_, err := env.Tenants.Create(ctx, a.admin, tenant.Input{Name: "Rina", Phone: "0812-7420-5001"})
		require.NoError(t, err)
	})

	t.Run("B's records do not exist for A, even with the right ids", func(t *testing.T) {
		_, err := env.Tenants.Get(ctx, a.admin, tenantB.ID)
		require.ErrorIs(t, err, tenant.ErrNotFound)
		_, err = env.Tenants.UpdateLease(ctx, a.admin, leaseB.ID, tenant.Terms{StartDate: day(2026, 10, 1), RentAmount: 1})
		require.ErrorIs(t, err, tenant.ErrNotFound)
		_, err = env.Tenants.MoveOut(ctx, a.admin, leaseB.ID, day(2026, 11, 1))
		require.ErrorIs(t, err, tenant.ErrNotFound)
		_, err = env.Tenants.CreateLease(ctx, a.admin, tenant.NewLease{TenantID: tenantB.ID, UnitID: a.kamar2.ID, Terms: tenant.Terms{
			StartDate: day(2026, 10, 1), RentAmount: 1,
		}})
		requireField(t, err, "tenantId")
		_, err = env.Tenants.CreateLease(ctx, a.admin, tenant.NewLease{TenantID: tenantB.ID, UnitID: b.kamar2.ID, Terms: tenant.Terms{
			StartDate: day(2026, 10, 1), RentAmount: 1, CollectorMemberID: b.karyawanMemberID,
		}})
		requireField(t, err, "tenantId")
		tenants, err := env.Tenants.List(ctx, a.admin)
		require.NoError(t, err)
		require.Len(t, tenants, 1)
		require.NotEqual(t, tenantB.ID, tenants[0].ID)
	})

	t.Run("the database refuses a lease that points into another organization", func(t *testing.T) {
		tenantA, err := env.Tenants.Create(ctx, a.admin, tenant.Input{Name: "Budi", Phone: "0812-7410-5002"})
		require.NoError(t, err)
		cases := map[string]dbgen.CreateLeaseParams{
			"unit of B":   {UnitID: b.kamar2.ID, TenantID: tenantA.ID},
			"tenant of B": {UnitID: a.kamar2.ID, TenantID: tenantB.ID},
			"collector of B": {UnitID: a.kamar2.ID, TenantID: tenantA.ID,
				CollectorStaffUserID: uuid.NullUUID{UUID: b.org.Karyawan.UserID, Valid: true}},
		}
		for name, params := range cases {
			params.ID, params.OrganizationID = uuid.New(), a.org.ID
			params.StartDate, params.RentAmount = day(2026, 10, 1), 1
			params.EnteredOn, params.BilledFrom = day(2026, 11, 2), day(2026, 10, 1)
			err := env.Store.WithOrgTx(ctx, a.org.ID, func(tx pgx.Tx) error {
				return dbgen.New(tx).CreateLease(ctx, params)
			})
			var pgErr *pgconn.PgError
			require.True(t, errors.As(err, &pgErr), "%s: expected a Postgres error, got %v", name, err)
			require.Equal(t, "23503", pgErr.Code, "%s: expected a foreign key violation", name)
		}
	})
}

// TestChangeWaitingOnMoveOutSeesTheEndedLease makes the race between two admins deterministic:
// the test moves the tenant out in a transaction it keeps open, starts the second change, waits
// until Postgres reports that change blocked on the lease row, and only then commits.
func TestChangeWaitingOnMoveOutSeesTheEndedLease(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "0812751")
	movedOut := day(2026, 11, 1)

	cases := []struct {
		name   string
		unit   property.Unit
		phone  string
		change func(lease tenant.Lease) error
	}{
		{
			name: "a rent change", unit: w.kamar1, phone: "0812-7510-5001",
			change: func(lease tenant.Lease) error {
				_, err := env.Tenants.UpdateLease(ctx, w.admin, lease.ID, tenant.Terms{
					StartDate: lease.StartDate, RentAmount: 1_250_000, DepositAmount: 1_000_000,
				})
				return err
			},
		},
		{
			name: "a second move-out", unit: w.kamar2, phone: "0812-7510-5002",
			change: func(lease tenant.Lease) error {
				_, err := env.Tenants.MoveOut(ctx, w.admin, lease.ID, day(2026, 11, 2))
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tn, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Siti Rahmawati", Phone: tc.phone})
			require.NoError(t, err)
			lease, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: tn.ID, UnitID: tc.unit.ID, Terms: tenant.Terms{
				StartDate: day(2026, 10, 1), RentAmount: 850_000, DepositAmount: 850_000,
			}})
			require.NoError(t, err)
			allowsBefore := countAllowed(env.AuditLog(t, w.org.ID), "lease:manage")

			done := make(chan error, 1)
			err = env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
				q := dbgen.New(tx)
				if _, err := q.LockLease(ctx, dbgen.LockLeaseParams{OrganizationID: w.org.ID, ID: lease.ID}); err != nil {
					return err
				}
				n, err := q.EndLease(ctx, dbgen.EndLeaseParams{OrganizationID: w.org.ID, ID: lease.ID, EndDate: &movedOut})
				if err != nil {
					return err
				}
				require.Equal(t, int64(1), n)

				go func() { done <- tc.change(lease) }()
				testenv.WaitUntilBlockedOn(ctx, t, tx)
				return nil // commit: the waiting change now reads the ended lease
			})
			require.NoError(t, err)

			select {
			case err = <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("the waiting change never returned")
			}
			require.ErrorIs(t, err, tenant.ErrLeaseEnded)

			got, err := env.Tenants.Get(ctx, w.admin, tn.ID)
			require.NoError(t, err)
			require.Len(t, got.Leases, 1)
			l := got.Leases[0]
			require.Equal(t, tenant.StatusEnded, l.Status)
			require.Equal(t, movedOut, *l.EndDate, "the first move-out's date stands")
			require.Equal(t, int64(850_000), l.RentAmount)
			require.Equal(t, int64(850_000), l.DepositAmount)
			require.Equal(t, allowsBefore, countAllowed(env.AuditLog(t, w.org.ID), "lease:manage"),
				"no allowed change is audited for a change that did not happen")
		})
	}
}

func countAllowed(rows []dbgen.AuditLog, action string) int {
	n := 0
	for _, r := range rows {
		if r.Decision == "allow" && r.Action == action {
			n++
		}
	}
	return n
}

// leaseFixture is a tenant plus a way to get a free unit for each lease a test creates.
type leaseFixture struct {
	env   *testenv.Env
	w     world
	tn    tenant.Tenant
	units int
}

func newLeaseFixture(t *testing.T, env *testenv.Env, w world, phone string) *leaseFixture {
	t.Helper()
	tn, err := env.Tenants.Create(context.Background(), w.admin, tenant.Input{Name: "Siti Rahmawati", Phone: phone})
	require.NoError(t, err)
	return &leaseFixture{env: env, w: w, tn: tn}
}

// unit adds a new unit to Kos Melati.
func (f *leaseFixture) unit(t *testing.T) property.Unit {
	t.Helper()
	f.units++
	u, err := f.env.Properties.CreateUnit(context.Background(), f.w.admin, f.w.melati.ID,
		property.UnitInput{Label: "Kamar " + string(rune('A'+f.units))})
	require.NoError(t, err)
	return u
}

// create rents a new unit to the tenant on the given terms.
func (f *leaseFixture) create(t *testing.T, terms tenant.Terms) (tenant.Lease, error) {
	t.Helper()
	if terms.RentAmount == 0 {
		terms.RentAmount = 850_000
	}
	return f.env.Tenants.CreateLease(context.Background(), f.w.admin, tenant.NewLease{
		TenantID: f.tn.ID, UnitID: f.unit(t).ID, Terms: terms,
	})
}

func TestLeaseEnteredOnIsTheJakartaDay(t *testing.T) {
	env := testenv.New(t)
	w := newWorld(t, env, "0812791")
	f := newLeaseFixture(t, env, w, "0812-7910-5001")

	// 00:30 on 3 November in Jakarta is still 17:30 on 2 November in UTC.
	env.Clock.Set(time.Date(2026, 11, 3, 0, 30, 0, 0, clock.Jakarta))
	lease, err := f.create(t, tenant.Terms{StartDate: day(2026, 10, 1)})
	require.NoError(t, err)
	require.Equal(t, day(2026, 11, 3), lease.EnteredOn)

	got, err := env.Tenants.Get(context.Background(), w.admin, f.tn.ID)
	require.NoError(t, err)
	require.Len(t, got.Leases, 1)
	require.Equal(t, day(2026, 11, 3), got.Leases[0].EnteredOn, "the stored day, read back")
}

func TestCreateLeaseBilledFrom(t *testing.T) {
	env := testenv.New(t) // 2 November 2026
	w := newWorld(t, env, "0812792")
	f := newLeaseFixture(t, env, w, "0812-7920-5001")

	tests := []struct {
		name           string
		start          time.Time
		billedFrom     time.Time // zero: left out
		want           time.Time
		wantErrorField string
	}{
		{"left out bills from the start date", day(2026, 9, 15), time.Time{}, day(2026, 9, 15), ""},
		{"a late lease may skip to a past period", day(2026, 9, 15), day(2026, 10, 15), day(2026, 10, 15), ""},
		{"a late lease may skip to the current period", day(2026, 9, 15), day(2026, 11, 15), day(2026, 11, 15), ""},
		{"a late lease cannot skip past the current period", day(2026, 9, 15), day(2026, 12, 15), time.Time{}, "billedFrom"},
		{"not an anniversary of the start", day(2026, 9, 15), day(2026, 10, 20), time.Time{}, "billedFrom"},
		{"before the start date", day(2026, 9, 15), day(2026, 8, 15), time.Time{}, "billedFrom"},
		{"a booking may bill from its start", day(2026, 12, 1), day(2026, 12, 1), day(2026, 12, 1), ""},
		{"a booking may skip its first period", day(2026, 12, 1), day(2027, 1, 1), day(2027, 1, 1), ""},
		{"a booking cannot skip two periods", day(2026, 12, 1), day(2027, 2, 1), time.Time{}, "billedFrom"},
		{"a month-end start may bill from the clamped day", day(2026, 8, 31), day(2026, 11, 30), day(2026, 11, 30), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lease, err := f.create(t, tenant.Terms{StartDate: tc.start, BilledFrom: tc.billedFrom})
			if tc.wantErrorField != "" {
				requireField(t, err, tc.wantErrorField)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, lease.BilledFrom)
			require.Equal(t, day(2026, 11, 2), lease.EnteredOn)
			require.Equal(t, tc.start, lease.StartDate)
		})
	}

	t.Run("a refused lease is not saved", func(t *testing.T) {
		got, err := env.Tenants.Get(context.Background(), w.admin, f.tn.ID)
		require.NoError(t, err)
		require.Len(t, got.Leases, 6, "only the six allowed cases above")
	})
}

func TestUpdateLeaseBilledFrom(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // 2 November 2026
	w := newWorld(t, env, "0812793")
	f := newLeaseFixture(t, env, w, "0812-7930-5001")

	lease, err := f.create(t, tenant.Terms{StartDate: day(2026, 9, 15), BilledFrom: day(2026, 11, 15)})
	require.NoError(t, err)
	require.Equal(t, day(2026, 11, 15), lease.BilledFrom)
	terms := func(billedFrom time.Time) tenant.Terms {
		return tenant.Terms{StartDate: day(2026, 9, 15), RentAmount: 900_000, BilledFrom: billedFrom}
	}

	t.Run("terms without billedFrom keep the stored one", func(t *testing.T) {
		// An admin fixing only the rent must not undo the months skipped at entry.
		updated, err := env.Tenants.UpdateLease(ctx, w.admin, lease.ID, terms(time.Time{}))
		require.NoError(t, err)
		require.Equal(t, day(2026, 11, 15), updated.BilledFrom)
		require.Equal(t, int64(900_000), updated.RentAmount)
		require.Equal(t, day(2026, 11, 2), updated.EnteredOn, "entered_on never changes")
	})

	t.Run("sending the start date bills from it again", func(t *testing.T) {
		updated, err := env.Tenants.UpdateLease(ctx, w.admin, lease.ID, terms(day(2026, 9, 15)))
		require.NoError(t, err)
		require.Equal(t, day(2026, 9, 15), updated.BilledFrom)
	})

	t.Run("a new start date that no longer fits the kept value is refused on billedFrom", func(t *testing.T) {
		_, err := env.Tenants.UpdateLease(ctx, w.admin, lease.ID, terms(day(2026, 11, 15)))
		require.NoError(t, err)
		moved := tenant.Terms{StartDate: day(2026, 9, 20), RentAmount: 900_000}
		_, err = env.Tenants.UpdateLease(ctx, w.admin, lease.ID, moved)
		requireField(t, err, "billedFrom")
		got, err := env.Tenants.Get(ctx, w.admin, f.tn.ID)
		require.NoError(t, err)
		require.Equal(t, day(2026, 9, 15), got.Leases[0].StartDate, "nothing changed")
		require.Equal(t, day(2026, 11, 15), got.Leases[0].BilledFrom)

		moved.BilledFrom = day(2026, 10, 20)
		updated, err := env.Tenants.UpdateLease(ctx, w.admin, lease.ID, moved)
		require.NoError(t, err)
		require.Equal(t, day(2026, 10, 20), updated.BilledFrom)
		updated, err = env.Tenants.UpdateLease(ctx, w.admin, lease.ID, terms(day(2026, 9, 15)))
		require.NoError(t, err)
		require.Equal(t, day(2026, 9, 15), updated.StartDate)
	})

	t.Run("a later anniversary within the bound is saved", func(t *testing.T) {
		updated, err := env.Tenants.UpdateLease(ctx, w.admin, lease.ID, terms(day(2026, 10, 15)))
		require.NoError(t, err)
		require.Equal(t, day(2026, 10, 15), updated.BilledFrom)
	})

	t.Run("a date that is not an anniversary is refused and nothing changes", func(t *testing.T) {
		_, err := env.Tenants.UpdateLease(ctx, w.admin, lease.ID, terms(day(2026, 10, 20)))
		requireField(t, err, "billedFrom")
		got, err := env.Tenants.Get(ctx, w.admin, f.tn.ID)
		require.NoError(t, err)
		require.Equal(t, day(2026, 10, 15), got.Leases[0].BilledFrom)
	})

	t.Run("the bound uses the stored entered_on, not today", func(t *testing.T) {
		// A month later the current period is 15 Dec, but the lease was entered on 2 Nov, so
		// the latest allowed day stays 15 Nov.
		env.Clock.Set(time.Date(2026, 12, 3, 9, 0, 0, 0, clock.Jakarta))
		t.Cleanup(func() { env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta)) })

		_, err := env.Tenants.UpdateLease(ctx, w.admin, lease.ID, terms(day(2026, 12, 15)))
		requireField(t, err, "billedFrom")
		updated, err := env.Tenants.UpdateLease(ctx, w.admin, lease.ID, terms(day(2026, 11, 15)))
		require.NoError(t, err)
		require.Equal(t, day(2026, 11, 15), updated.BilledFrom)
		require.Equal(t, day(2026, 11, 2), updated.EnteredOn)
	})
}

func TestLeasePlannedEnd(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // 2 November 2026
	w := newWorld(t, env, "0812794")
	f := newLeaseFixture(t, env, w, "0812-7940-5001")

	lease, err := f.create(t, tenant.Terms{StartDate: day(2026, 10, 1), CollectorMemberID: w.karyawanMemberID})
	require.NoError(t, err)
	require.Nil(t, lease.PlannedEndDate, "a new lease has none")
	booking, err := f.create(t, tenant.Terms{StartDate: day(2026, 12, 10)})
	require.NoError(t, err)

	setDate := func(l tenant.Lease, d time.Time) (tenant.Lease, error) {
		return env.Tenants.SetPlannedEnd(ctx, w.admin, l.ID, &d)
	}
	plannedEnd := func(t *testing.T, id uuid.UUID) *time.Time {
		t.Helper()
		got, err := env.Tenants.Get(ctx, w.admin, f.tn.ID)
		require.NoError(t, err)
		for _, l := range got.Leases {
			if l.ID == id {
				return l.PlannedEndDate
			}
		}
		t.Fatalf("lease %s not found", id)
		return nil
	}

	t.Run("an admin sets it, and it is audited", func(t *testing.T) {
		updated, err := setDate(lease, day(2026, 11, 14))
		require.NoError(t, err)
		require.Equal(t, day(2026, 11, 14), *updated.PlannedEndDate)
		require.Equal(t, tenant.StatusActive, updated.Status)
		require.Equal(t, day(2026, 11, 14), *plannedEnd(t, lease.ID), "read back with the tenant")

		audit := env.AuditLog(t, w.org.ID)
		require.Equal(t, "allow", audit[0].Decision)
		require.Equal(t, "lease:manage", audit[0].Action)
		require.Equal(t, "property/"+w.melati.ID.String()+"/lease/"+lease.ID.String(), audit[0].Resource)
	})

	t.Run("a time of day is dropped", func(t *testing.T) {
		updated, err := setDate(lease, time.Date(2026, 11, 20, 17, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		require.Equal(t, day(2026, 11, 20), *updated.PlannedEndDate)
	})

	t.Run("it can change, and today is allowed", func(t *testing.T) {
		updated, err := setDate(lease, day(2026, 11, 30))
		require.NoError(t, err)
		require.Equal(t, day(2026, 11, 30), *updated.PlannedEndDate)
		updated, err = setDate(lease, day(2026, 11, 2))
		require.NoError(t, err)
		require.Equal(t, day(2026, 11, 2), *updated.PlannedEndDate)
		_, err = setDate(lease, day(2026, 11, 30))
		require.NoError(t, err)
	})

	t.Run("a date before today is refused with its field, and the stored date stands", func(t *testing.T) {
		_, err := setDate(lease, day(2026, 11, 1))
		requireField(t, err, "plannedEndDate")
		require.Equal(t, day(2026, 11, 30), *plannedEnd(t, lease.ID))
	})

	t.Run("a date before the start date is refused with its field", func(t *testing.T) {
		_, err := setDate(booking, day(2026, 12, 5)) // after today, before the booking starts
		requireField(t, err, "plannedEndDate")
		require.Nil(t, plannedEnd(t, booking.ID))

		updated, err := setDate(booking, day(2026, 12, 10))
		require.NoError(t, err, "the start date itself is allowed")
		require.Equal(t, day(2026, 12, 10), *updated.PlannedEndDate)
	})

	t.Run("a karyawan and the supervisor are denied, and the date stands", func(t *testing.T) {
		for name, p := range map[string]authz.Principal{"karyawan": w.karyawan, "supervisor": w.supervisor} {
			d := day(2026, 11, 20)
			_, err := env.Tenants.SetPlannedEnd(ctx, p, lease.ID, &d)
			require.ErrorIs(t, err, authz.ErrForbidden, name)
			_, err = env.Tenants.SetPlannedEnd(ctx, p, lease.ID, nil)
			require.ErrorIs(t, err, authz.ErrForbidden, name+" cannot clear it either")
		}
		require.Equal(t, day(2026, 11, 30), *plannedEnd(t, lease.ID))
		audit := env.AuditLog(t, w.org.ID)
		require.Equal(t, "deny", audit[0].Decision)
		require.Equal(t, "lease:manage", audit[0].Action)
	})

	t.Run("an unknown lease is not found", func(t *testing.T) {
		_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, uuid.New(), nil)
		require.ErrorIs(t, err, tenant.ErrNotFound)
	})

	t.Run("nil clears it", func(t *testing.T) {
		updated, err := env.Tenants.SetPlannedEnd(ctx, w.admin, lease.ID, nil)
		require.NoError(t, err)
		require.Nil(t, updated.PlannedEndDate)
		require.Nil(t, plannedEnd(t, lease.ID))

		updated, err = env.Tenants.SetPlannedEnd(ctx, w.admin, lease.ID, nil)
		require.NoError(t, err, "clearing a lease with none is fine")
		require.Nil(t, updated.PlannedEndDate)
	})

	t.Run("move-out clears it", func(t *testing.T) {
		_, err := setDate(lease, day(2026, 11, 14))
		require.NoError(t, err)
		ended, err := env.Tenants.MoveOut(ctx, w.admin, lease.ID, day(2026, 11, 2))
		require.NoError(t, err)
		require.Nil(t, ended.PlannedEndDate)
		require.Nil(t, plannedEnd(t, lease.ID))
	})

	t.Run("an ended lease refuses it", func(t *testing.T) {
		_, err := setDate(lease, day(2026, 11, 30))
		require.ErrorIs(t, err, tenant.ErrLeaseEnded)
		_, err = env.Tenants.SetPlannedEnd(ctx, w.admin, lease.ID, nil)
		require.ErrorIs(t, err, tenant.ErrLeaseEnded)
		require.Nil(t, plannedEnd(t, lease.ID))
	})
}

func TestUpdateLeaseStartDateAgainstPlannedEnd(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // 2 November 2026
	w := newWorld(t, env, "0812795")
	f := newLeaseFixture(t, env, w, "0812-7950-5001")

	lease, err := f.create(t, tenant.Terms{StartDate: day(2026, 10, 1)})
	require.NoError(t, err)
	planned := day(2026, 11, 14)
	_, err = env.Tenants.SetPlannedEnd(ctx, w.admin, lease.ID, &planned)
	require.NoError(t, err)
	move := func(start time.Time) (tenant.Lease, error) {
		// A moved start needs a billing start that fits it; the stored one no longer does.
		return env.Tenants.UpdateLease(ctx, w.admin, lease.ID, tenant.Terms{StartDate: start, RentAmount: 850_000, BilledFrom: start})
	}

	t.Run("a start date after the planned end is refused with its field, and the lease is unchanged", func(t *testing.T) {
		_, err := move(day(2026, 11, 20))
		requireField(t, err, "startDate")
		got, err := env.Tenants.Get(ctx, w.admin, f.tn.ID)
		require.NoError(t, err)
		require.Equal(t, day(2026, 10, 1), got.Leases[0].StartDate)
		require.Equal(t, planned, *got.Leases[0].PlannedEndDate)
	})

	t.Run("a start date on the planned end is allowed", func(t *testing.T) {
		updated, err := move(planned)
		require.NoError(t, err)
		require.Equal(t, planned, updated.StartDate)
		require.Equal(t, planned, *updated.PlannedEndDate)
	})
}
