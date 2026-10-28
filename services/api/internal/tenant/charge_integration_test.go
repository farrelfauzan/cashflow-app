//go:build integration

package tenant_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func chargeNames(cs []tenant.Charge) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func leaseByID(t *testing.T, tn tenant.Tenant, id uuid.UUID) tenant.Lease {
	t.Helper()
	for _, l := range tn.Leases {
		if l.ID == id {
			return l
		}
	}
	t.Fatalf("lease %s not on tenant %s", id, tn.Name)
	return tenant.Lease{}
}

func TestChargeTypesAndLeaseCharges(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // the clock reads 2 November 2026, 09:00 in Jakarta
	w := newWorld(t, env, "0812761")

	rina, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Rina Wulandari", Phone: "0812-7610-5001"})
	require.NoError(t, err)
	lease, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: rina.ID, UnitID: w.kamar1.ID, Terms: tenant.Terms{
		StartDate: day(2026, 10, 1), RentAmount: 850_000, DueDay: 5, CollectorMemberID: w.karyawanMemberID,
	}})
	require.NoError(t, err)

	var wifi, parkir, listrik tenant.ChargeType
	t.Run("an admin adds charge types, and each addition is audited", func(t *testing.T) {
		require.Equal(t, []tenant.Charge{}, lease.Charges, "a new lease has an empty list, never nil")
		var err error
		wifi, err = env.Tenants.CreateChargeType(ctx, w.admin, tenant.ChargeTypeInput{Name: " Wi-Fi "})
		require.NoError(t, err)
		require.Equal(t, "Wi-Fi", wifi.Name)
		require.False(t, wifi.Metered)
		audit := env.AuditLog(t, w.org.ID)
		require.Equal(t, "allow", audit[0].Decision)
		require.Equal(t, "lease:manage", audit[0].Action)
		require.Equal(t, "charge-type/"+wifi.ID.String(), audit[0].Resource)

		parkir, err = env.Tenants.CreateChargeType(ctx, w.admin, tenant.ChargeTypeInput{Name: "Parkir"})
		require.NoError(t, err)
		listrik, err = env.Tenants.CreateChargeType(ctx, w.admin, tenant.ChargeTypeInput{Name: "Listrik", Metered: true})
		require.NoError(t, err)
		require.True(t, listrik.Metered)

		types, err := env.Tenants.ListChargeTypes(ctx, w.admin)
		require.NoError(t, err)
		require.Len(t, types, 3)
		require.Equal(t, []string{"Listrik", "Parkir", "Wi-Fi"}, []string{types[0].Name, types[1].Name, types[2].Name})
	})

	t.Run("a name is taken ignoring case", func(t *testing.T) {
		_, err := env.Tenants.CreateChargeType(ctx, w.admin, tenant.ChargeTypeInput{Name: "wi-fi"})
		require.ErrorIs(t, err, tenant.ErrChargeTypeTaken)
		_, err = env.Tenants.CreateChargeType(ctx, w.admin, tenant.ChargeTypeInput{Name: "  "})
		requireField(t, err, "name")
	})

	t.Run("an admin sets extras on a lease, and the change is audited", func(t *testing.T) {
		got, err := env.Tenants.SetLeaseCharges(ctx, w.admin, lease.ID, []tenant.ChargeInput{
			{ChargeTypeID: wifi.ID, Amount: 50_000}, {ChargeTypeID: parkir.ID, Amount: 100_000},
		})
		require.NoError(t, err)
		require.Equal(t, []tenant.Charge{
			{ChargeTypeID: parkir.ID, Name: "Parkir", Amount: 100_000},
			{ChargeTypeID: wifi.ID, Name: "Wi-Fi", Amount: 50_000},
		}, got.Charges)
		require.Equal(t, int64(850_000), got.RentAmount, "rent is untouched")

		audit := env.AuditLog(t, w.org.ID)
		require.Equal(t, "allow", audit[0].Decision)
		require.Equal(t, "lease:manage", audit[0].Action)
		require.Equal(t, "property/"+w.melati.ID.String()+"/lease/"+lease.ID.String(), audit[0].Resource)
	})

	t.Run("the extras show on the tenant and in the tenant list", func(t *testing.T) {
		tn, err := env.Tenants.Get(ctx, w.admin, rina.ID)
		require.NoError(t, err)
		require.Equal(t, []string{"Parkir", "Wi-Fi"}, chargeNames(leaseByID(t, tn, lease.ID).Charges))

		list, err := env.Tenants.List(ctx, w.karyawan)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, []string{"Parkir", "Wi-Fi"}, chargeNames(list[0].Leases[0].Charges), "the collector reads them too")
	})

	t.Run("setting again replaces the whole set", func(t *testing.T) {
		got, err := env.Tenants.SetLeaseCharges(ctx, w.admin, lease.ID, []tenant.ChargeInput{{ChargeTypeID: wifi.ID, Amount: 75_000}})
		require.NoError(t, err)
		require.Equal(t, []tenant.Charge{{ChargeTypeID: wifi.ID, Name: "Wi-Fi", Amount: 75_000}}, got.Charges)
	})

	t.Run("refused input changes nothing and audits no allowed change", func(t *testing.T) {
		allowsBefore := countAllowed(env.AuditLog(t, w.org.ID), "lease:manage")
		cases := []struct {
			name  string
			in    []tenant.ChargeInput
			field string
		}{
			{"the same type twice", []tenant.ChargeInput{{ChargeTypeID: wifi.ID, Amount: 1}, {ChargeTypeID: wifi.ID, Amount: 2}}, "charges[1].chargeTypeId"},
			{"a metered type", []tenant.ChargeInput{{ChargeTypeID: parkir.ID, Amount: 1}, {ChargeTypeID: listrik.ID, Amount: 1}}, "charges[1].chargeTypeId"},
			{"an unknown type", []tenant.ChargeInput{{ChargeTypeID: uuid.New(), Amount: 1}}, "charges[0].chargeTypeId"},
			{"a free charge", []tenant.ChargeInput{{ChargeTypeID: parkir.ID, Amount: 0}}, "charges[0].amount"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := env.Tenants.SetLeaseCharges(ctx, w.admin, lease.ID, tc.in)
				requireField(t, err, tc.field)
			})
		}
		tn, err := env.Tenants.Get(ctx, w.admin, rina.ID)
		require.NoError(t, err)
		require.Equal(t, []tenant.Charge{{ChargeTypeID: wifi.ID, Name: "Wi-Fi", Amount: 75_000}}, leaseByID(t, tn, lease.ID).Charges)
		require.Equal(t, allowsBefore, countAllowed(env.AuditLog(t, w.org.ID), "lease:manage"))
	})

	t.Run("a karyawan and the supervisor change no extras and add no types", func(t *testing.T) {
		for name, p := range map[string]authz.Principal{"karyawan": w.karyawan, "supervisor": w.supervisor} {
			_, err := env.Tenants.SetLeaseCharges(ctx, p, lease.ID, []tenant.ChargeInput{})
			require.ErrorIs(t, err, authz.ErrForbidden, name)
			_, err = env.Tenants.CreateChargeType(ctx, p, tenant.ChargeTypeInput{Name: "Sampah"})
			require.ErrorIs(t, err, authz.ErrForbidden, name)
			_, err = env.Tenants.ListChargeTypes(ctx, p)
			require.ErrorIs(t, err, authz.ErrForbidden, name)
		}
		audit := env.AuditLog(t, w.org.ID)
		require.Equal(t, "deny", audit[0].Decision, "the denied list is audited")
		require.Equal(t, "charge-type", audit[0].Resource)
	})

	t.Run("an empty set removes every extra", func(t *testing.T) {
		got, err := env.Tenants.SetLeaseCharges(ctx, w.admin, lease.ID, []tenant.ChargeInput{})
		require.NoError(t, err)
		require.Equal(t, []tenant.Charge{}, got.Charges)
	})

	t.Run("an ended lease takes no extras", func(t *testing.T) {
		budi, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Budi Santoso", Phone: "0812-7610-5002"})
		require.NoError(t, err)
		l, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: budi.ID, UnitID: w.kamar2.ID, Terms: tenant.Terms{
			StartDate: day(2026, 10, 1), RentAmount: 900_000, DueDay: 10,
		}})
		require.NoError(t, err)
		_, err = env.Tenants.MoveOut(ctx, w.admin, l.ID, day(2026, 11, 1))
		require.NoError(t, err)
		_, err = env.Tenants.SetLeaseCharges(ctx, w.admin, l.ID, []tenant.ChargeInput{{ChargeTypeID: wifi.ID, Amount: 50_000}})
		require.ErrorIs(t, err, tenant.ErrLeaseEnded)
	})
}

func TestLeaseChargesForAnAdminLimitedToOneProperty(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "0812771")

	sari, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-7710-0099", Name: "Mbak Sari",
		Roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: w.melati.ID}}})
	require.NoError(t, err)
	limited := env.Principal(t, w.org.ID, sari.UserID)

	wifi, err := env.Tenants.CreateChargeType(ctx, w.admin, tenant.ChargeTypeInput{Name: "Wi-Fi"})
	require.NoError(t, err)
	terms := tenant.Terms{StartDate: day(2026, 10, 1), RentAmount: 800_000, DueDay: 1}
	rina, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Rina", Phone: "0812-7710-5001"})
	require.NoError(t, err)
	inMelati, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: rina.ID, UnitID: w.kamar1.ID, Terms: terms})
	require.NoError(t, err)
	inMawar, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: rina.ID, UnitID: w.rumahA.ID, Terms: terms})
	require.NoError(t, err)
	wifiOnly := []tenant.ChargeInput{{ChargeTypeID: wifi.ID, Amount: 50_000}}

	cases := []struct {
		name    string
		do      func() error
		wantErr error
	}{
		{"sets extras on a lease in her property", func() error {
			_, err := env.Tenants.SetLeaseCharges(ctx, limited, inMelati.ID, wifiOnly)
			return err
		}, nil},
		{"not on a lease in another property", func() error {
			_, err := env.Tenants.SetLeaseCharges(ctx, limited, inMawar.ID, wifiOnly)
			return err
		}, authz.ErrForbidden},
		{"cannot add a charge type, which every property shares", func() error {
			_, err := env.Tenants.CreateChargeType(ctx, limited, tenant.ChargeTypeInput{Name: "Parkir"})
			return err
		}, authz.ErrForbidden},
		{"lists the charge types, to pick from for her own leases", func() error {
			_, err := env.Tenants.ListChargeTypes(ctx, limited)
			return err
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.do()
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestLeaseChargesStayInsideTheirOrganization(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	a := newWorld(t, env, "0812781")
	b := newWorld(t, env, "0812782")

	newLease := func(w world, phone string) tenant.Lease {
		tn, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: "Rina", Phone: phone})
		require.NoError(t, err)
		l, err := env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: tn.ID, UnitID: w.kamar1.ID, Terms: tenant.Terms{
			StartDate: day(2026, 10, 1), RentAmount: 800_000, DueDay: 1,
		}})
		require.NoError(t, err)
		return l
	}
	leaseA, leaseB := newLease(a, "0812-7810-5001"), newLease(b, "0812-7820-5001")
	wifiA, err := env.Tenants.CreateChargeType(ctx, a.admin, tenant.ChargeTypeInput{Name: "Wi-Fi"})
	require.NoError(t, err)
	wifiB, err := env.Tenants.CreateChargeType(ctx, b.admin, tenant.ChargeTypeInput{Name: "Wi-Fi"})
	require.NoError(t, err, "the same name in another organization is fine")

	t.Run("A cannot use B's charge type or B's lease", func(t *testing.T) {
		_, err := env.Tenants.SetLeaseCharges(ctx, a.admin, leaseA.ID, []tenant.ChargeInput{{ChargeTypeID: wifiB.ID, Amount: 50_000}})
		requireField(t, err, "charges[0].chargeTypeId")
		_, err = env.Tenants.SetLeaseCharges(ctx, a.admin, leaseB.ID, []tenant.ChargeInput{{ChargeTypeID: wifiA.ID, Amount: 50_000}})
		require.ErrorIs(t, err, tenant.ErrNotFound)
		types, err := env.Tenants.ListChargeTypes(ctx, a.admin)
		require.NoError(t, err)
		require.Len(t, types, 1)
		require.Equal(t, wifiA.ID, types[0].ID)
	})

	t.Run("the database refuses a lease charge that points into another organization", func(t *testing.T) {
		cases := map[string]dbgen.UpsertLeaseChargeParams{
			"charge type of B": {LeaseID: leaseA.ID, ChargeTypeID: wifiB.ID},
			"lease of B":       {LeaseID: leaseB.ID, ChargeTypeID: wifiA.ID},
		}
		for name, params := range cases {
			params.ID, params.OrganizationID, params.Amount = uuid.New(), a.org.ID, 50_000
			err := env.Store.WithOrgTx(ctx, a.org.ID, func(tx pgx.Tx) error {
				return dbgen.New(tx).UpsertLeaseCharge(ctx, params)
			})
			var pgErr *pgconn.PgError
			require.True(t, errors.As(err, &pgErr), "%s: expected a Postgres error, got %v", name, err)
			require.Equal(t, "23503", pgErr.Code, "%s: expected a foreign key violation", name)
		}
	})

	t.Run("the database refuses an amount of Rp0", func(t *testing.T) {
		err := env.Store.WithOrgTx(ctx, a.org.ID, func(tx pgx.Tx) error {
			return dbgen.New(tx).UpsertLeaseCharge(ctx, dbgen.UpsertLeaseChargeParams{
				ID: uuid.New(), OrganizationID: a.org.ID, LeaseID: leaseA.ID, ChargeTypeID: wifiA.ID, Amount: 0,
			})
		})
		var pgErr *pgconn.PgError
		require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
		require.Equal(t, "23514", pgErr.Code, "expected a check violation")
	})
}
