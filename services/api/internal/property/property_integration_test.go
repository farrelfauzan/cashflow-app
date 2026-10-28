//go:build integration

package property_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func rent(v int64) *int64 { return &v }

func names(props []property.Property) []string {
	out := make([]string, len(props))
	for i, p := range props {
		out[i] = p.Name
	}
	return out
}

func labels(units []property.Unit) []string {
	out := make([]string, len(units))
	for i, u := range units {
		out[i] = u.Label
	}
	return out
}

func TestPropertiesAndUnits(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812611")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	supervisor := env.Principal(t, o.ID, o.Supervisor.UserID)
	karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)

	var melati property.Property
	t.Run("an admin adds properties", func(t *testing.T) {
		var err error
		melati, err = env.Properties.Create(ctx, admin, property.PropertyInput{
			Name: "  Kos Melati 2 ", Kind: property.KindKos, Address: "Jl. Kaliurang Km 7, Sleman",
		})
		require.NoError(t, err)
		require.Equal(t, "Kos Melati 2", melati.Name)
		_, err = env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati 11", Kind: property.KindKos})
		require.NoError(t, err)
		_, err = env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Ruko Godean", Kind: property.KindRuko})
		require.NoError(t, err)

		props, err := env.Properties.List(ctx, admin)
		require.NoError(t, err)
		require.Equal(t, []string{"Kos Melati 2", "Kos Melati 11", "Ruko Godean"}, names(props), "numbers sort naturally")
	})

	t.Run("a property name is used once per organization, ignoring case", func(t *testing.T) {
		_, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "kos melati 2", Kind: property.KindKos})
		require.ErrorIs(t, err, property.ErrNameTaken)

		other := env.CreateOrg(t, "Kontrakan Mawar", "0812612")
		_, err = env.Properties.Create(ctx, env.Principal(t, other.ID, other.Admin.UserID),
			property.PropertyInput{Name: "Kos Melati 2", Kind: property.KindKos})
		require.NoError(t, err, "another organization may use the same name")
	})

	t.Run("an admin renames a property", func(t *testing.T) {
		updated, err := env.Properties.Update(ctx, admin, melati.ID, property.PropertyInput{
			Name: "Kos Melati Dua", Kind: property.KindKos, Address: melati.Address,
		})
		require.NoError(t, err)
		require.Equal(t, "Kos Melati Dua", updated.Name)
		melati = updated

		_, err = env.Properties.Update(ctx, admin, melati.ID, property.PropertyInput{Name: "Ruko Godean", Kind: property.KindKos})
		require.ErrorIs(t, err, property.ErrNameTaken)
		_, err = env.Properties.Update(ctx, admin, uuid.New(), property.PropertyInput{Name: "X", Kind: property.KindKos})
		require.ErrorIs(t, err, property.ErrNotFound)
	})

	var kamar2 property.Unit
	t.Run("an admin adds units, listed in natural order", func(t *testing.T) {
		for _, label := range []string{"Kamar 11", "Kamar 2", "Kamar 1"} {
			u, err := env.Properties.CreateUnit(ctx, admin, melati.ID, property.UnitInput{Label: label, DefaultRent: rent(850_000)})
			require.NoError(t, err)
			if label == "Kamar 2" {
				kamar2 = u
			}
		}
		units, err := env.Properties.ListUnits(ctx, admin, melati.ID)
		require.NoError(t, err)
		require.Equal(t, []string{"Kamar 1", "Kamar 2", "Kamar 11"}, labels(units))
		require.Equal(t, int64(850_000), *units[0].DefaultRent)

		props, err := env.Properties.List(ctx, admin)
		require.NoError(t, err)
		for _, p := range props {
			if p.ID == melati.ID {
				require.Equal(t, int64(3), p.UnitCount)
			}
		}
	})

	t.Run("a unit label is used once per property, ignoring case", func(t *testing.T) {
		_, err := env.Properties.CreateUnit(ctx, admin, melati.ID, property.UnitInput{Label: "kamar 2"})
		require.ErrorIs(t, err, property.ErrLabelTaken)

		props, err := env.Properties.List(ctx, admin)
		require.NoError(t, err)
		ruko := props[len(props)-1]
		_, err = env.Properties.CreateUnit(ctx, admin, ruko.ID, property.UnitInput{Label: "Kamar 2"})
		require.NoError(t, err, "another property may use the same label")
	})

	t.Run("an admin changes a unit's label and rent", func(t *testing.T) {
		u, err := env.Properties.UpdateUnit(ctx, admin, kamar2.ID, property.UnitInput{Label: "Kamar 2A", DefaultRent: rent(900_000)})
		require.NoError(t, err)
		require.Equal(t, "Kamar 2A", u.Label)
		require.Equal(t, int64(900_000), *u.DefaultRent)
		require.Equal(t, melati.ID, u.PropertyID)

		u, err = env.Properties.UpdateUnit(ctx, admin, kamar2.ID, property.UnitInput{Label: "Kamar 2A"})
		require.NoError(t, err)
		require.Nil(t, u.DefaultRent, "the default rent can be cleared")

		_, err = env.Properties.UpdateUnit(ctx, admin, kamar2.ID, property.UnitInput{Label: "Kamar 1"})
		require.ErrorIs(t, err, property.ErrLabelTaken)
	})

	t.Run("every change is audited", func(t *testing.T) {
		var actions []string
		for _, a := range env.AuditLog(t, o.ID) {
			if a.Decision == "allow" && a.ActorStaffUserID.UUID == o.Admin.UserID {
				actions = append(actions, a.Action+" "+a.Resource)
			}
		}
		require.Contains(t, actions, "property:manage property/"+melati.ID.String())
		require.Contains(t, actions, fmt.Sprintf("unit:manage property/%s/unit/%s", melati.ID, kamar2.ID))
	})

	t.Run("bad input names the field", func(t *testing.T) {
		props := map[string]property.PropertyInput{
			"name":    {Name: "  ", Kind: property.KindKos},
			"kind":    {Name: "Kos Baru", Kind: "apartemen"},
			"address": {Name: "Kos Baru", Kind: property.KindKos, Address: string(make([]rune, 301))},
		}
		for field, in := range props {
			_, err := env.Properties.Create(ctx, admin, in)
			var inputErr *property.InputError
			require.ErrorAs(t, err, &inputErr, field)
			require.Equal(t, field, inputErr.Field)
		}
		units := map[string]property.UnitInput{
			"label":       {Label: ""},
			"defaultRent": {Label: "Kamar 9", DefaultRent: rent(0)},
		}
		for field, in := range units {
			_, err := env.Properties.CreateUnit(ctx, admin, melati.ID, in)
			var inputErr *property.InputError
			require.ErrorAs(t, err, &inputErr, field)
			require.Equal(t, field, inputErr.Field)
		}
	})

	t.Run("the supervisor reads but never writes", func(t *testing.T) {
		props, err := env.Properties.List(ctx, supervisor)
		require.NoError(t, err)
		require.Len(t, props, 3)
		_, err = env.Properties.ListUnits(ctx, supervisor, melati.ID)
		require.NoError(t, err)

		_, err = env.Properties.Create(ctx, supervisor, property.PropertyInput{Name: "Kos Baru", Kind: property.KindKos})
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = env.Properties.UpdateUnit(ctx, supervisor, kamar2.ID, property.UnitInput{Label: "Kamar 2B"})
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("a karyawan cannot list properties, and the attempt is audited", func(t *testing.T) {
		_, err := env.Properties.List(ctx, karyawan)
		require.ErrorIs(t, err, authz.ErrForbidden)
		audit := env.AuditLog(t, o.ID)
		require.Equal(t, "deny", audit[0].Decision)
		require.Equal(t, "property:read", audit[0].Action)
		require.Equal(t, o.Karyawan.UserID, audit[0].ActorStaffUserID.UUID)
	})
}

func TestAdminLimitedToOneProperty(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812621")
	admin := env.Principal(t, o.ID, o.Admin.UserID)

	melati, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	mawar, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kontrakan Mawar", Kind: property.KindKontrakan})
	require.NoError(t, err)
	mawarUnit, err := env.Properties.CreateUnit(ctx, admin, mawar.ID, property.UnitInput{Label: "Rumah A"})
	require.NoError(t, err)

	sari, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-6210-0099", Name: "Mbak Sari", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)

	t.Run("an admin limits Sari's admin role to Kos Melati", func(t *testing.T) {
		m, err := env.Members.SetRoles(ctx, admin, sari.ID, []member.RoleGrant{
			{Role: authz.RoleAdmin, PropertyID: melati.ID},
			{Role: authz.RoleKaryawan},
		})
		require.NoError(t, err)
		require.Len(t, m.Roles, 2)
		require.Equal(t, authz.RoleAdmin, m.Roles[0].Key)
		require.Equal(t, melati.ID, m.Roles[0].PropertyID)
		require.Equal(t, uuid.Nil, m.Roles[1].PropertyID)

		p := env.Principal(t, o.ID, sari.UserID)
		require.Equal(t, []uuid.UUID{melati.ID}, p.Properties)
		require.Equal(t, melati.ID, p.Roles[0].Property)
	})

	limited := env.Principal(t, o.ID, sari.UserID)

	t.Run("she sees only Kos Melati", func(t *testing.T) {
		props, err := env.Properties.List(ctx, limited)
		require.NoError(t, err)
		require.Equal(t, []string{"Kos Melati"}, names(props))
	})

	t.Run("she manages units in Kos Melati", func(t *testing.T) {
		u, err := env.Properties.CreateUnit(ctx, limited, melati.ID, property.UnitInput{Label: "Kamar 1", DefaultRent: rent(750_000)})
		require.NoError(t, err)
		_, err = env.Properties.UpdateUnit(ctx, limited, u.ID, property.UnitInput{Label: "Kamar 01"})
		require.NoError(t, err)
		_, err = env.Properties.Update(ctx, limited, melati.ID, property.PropertyInput{Name: "Kos Melati Baru", Kind: property.KindKos})
		require.NoError(t, err)
	})

	t.Run("she cannot touch Kontrakan Mawar", func(t *testing.T) {
		_, err := env.Properties.ListUnits(ctx, limited, mawar.ID)
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = env.Properties.CreateUnit(ctx, limited, mawar.ID, property.UnitInput{Label: "Rumah B"})
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = env.Properties.UpdateUnit(ctx, limited, mawarUnit.ID, property.UnitInput{Label: "Rumah Z"})
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = env.Properties.Update(ctx, limited, mawar.ID, property.PropertyInput{Name: "X", Kind: property.KindKos})
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("she cannot add properties or manage members", func(t *testing.T) {
		_, err := env.Properties.Create(ctx, limited, property.PropertyInput{Name: "Kos Baru", Kind: property.KindKos})
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = env.Members.List(ctx, limited)
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("a property limit must name a property of this organization", func(t *testing.T) {
		other := env.CreateOrg(t, "Kos Lain", "0812622")
		otherProp, err := env.Properties.Create(ctx, env.Principal(t, other.ID, other.Admin.UserID),
			property.PropertyInput{Name: "Kos Lain", Kind: property.KindKos})
		require.NoError(t, err)

		for name, id := range map[string]uuid.UUID{"unknown": uuid.New(), "another organization": otherProp.ID} {
			_, err := env.Members.SetRoles(ctx, admin, sari.ID, []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: id}})
			var inputErr *member.InputError
			require.ErrorAs(t, err, &inputErr, name)
			require.Equal(t, "roles", inputErr.Field)
		}
	})

	t.Run("a role cannot be both organization-wide and limited", func(t *testing.T) {
		_, err := env.Members.SetRoles(ctx, admin, sari.ID, []member.RoleGrant{
			{Role: authz.RoleAdmin}, {Role: authz.RoleAdmin, PropertyID: melati.ID},
		})
		var inputErr *member.InputError
		require.ErrorAs(t, err, &inputErr)
	})

	t.Run("the same role can be limited to two properties", func(t *testing.T) {
		m, err := env.Members.SetRoles(ctx, admin, sari.ID, []member.RoleGrant{
			{Role: authz.RoleAdmin, PropertyID: melati.ID},
			{Role: authz.RoleAdmin, PropertyID: mawar.ID},
			{Role: authz.RoleAdmin, PropertyID: mawar.ID}, // repeated grants count once
		})
		require.NoError(t, err)
		require.Len(t, m.Roles, 2)

		props, err := env.Properties.List(ctx, env.Principal(t, o.ID, sari.UserID))
		require.NoError(t, err)
		require.Len(t, props, 2)
	})

	t.Run("changing only a property limit still needs role:assign", func(t *testing.T) {
		supervisor := env.Principal(t, o.ID, o.Supervisor.UserID)
		_, err := env.Members.SetRoles(ctx, admin, supervisor.MembershipID, []member.RoleGrant{
			{Role: authz.RoleSupervisor, PropertyID: melati.ID},
		})
		require.ErrorIs(t, err, authz.ErrForbidden, "an admin cannot narrow the supervisor role")
	})
}

func TestPropertiesStayInsideTheirOrganization(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	a := env.CreateOrg(t, "Kos Melati", "0812631")
	b := env.CreateOrg(t, "Kontrakan Mawar", "0812632")
	adminA := env.Principal(t, a.ID, a.Admin.UserID)
	adminB := env.Principal(t, b.ID, b.Admin.UserID)

	propB, err := env.Properties.Create(ctx, adminB, property.PropertyInput{Name: "Mawar", Kind: property.KindKontrakan})
	require.NoError(t, err)
	unitB, err := env.Properties.CreateUnit(ctx, adminB, propB.ID, property.UnitInput{Label: "Rumah 1"})
	require.NoError(t, err)
	_, err = env.Properties.Create(ctx, adminA, property.PropertyInput{Name: "Melati", Kind: property.KindKos})
	require.NoError(t, err)

	t.Run("A's admin sees only A's properties", func(t *testing.T) {
		props, err := env.Properties.List(ctx, adminA)
		require.NoError(t, err)
		require.Equal(t, []string{"Melati"}, names(props))
	})

	t.Run("B's records do not exist for A, even with the right ids", func(t *testing.T) {
		_, err := env.Properties.ListUnits(ctx, adminA, propB.ID)
		require.ErrorIs(t, err, property.ErrNotFound)
		_, err = env.Properties.CreateUnit(ctx, adminA, propB.ID, property.UnitInput{Label: "Rumah 2"})
		require.ErrorIs(t, err, property.ErrNotFound)
		_, err = env.Properties.Update(ctx, adminA, propB.ID, property.PropertyInput{Name: "Diambil", Kind: property.KindKos})
		require.ErrorIs(t, err, property.ErrNotFound)
		_, err = env.Properties.UpdateUnit(ctx, adminA, unitB.ID, property.UnitInput{Label: "Diambil"})
		require.ErrorIs(t, err, property.ErrNotFound)
	})

	t.Run("the database refuses a unit in another organization's property", func(t *testing.T) {
		err := env.Store.WithOrgTx(ctx, a.ID, func(tx pgx.Tx) error {
			_, err := dbgen.New(tx).CreateUnit(ctx, dbgen.CreateUnitParams{
				ID: uuid.New(), OrganizationID: a.ID, PropertyID: propB.ID, Label: "Selundupan",
			})
			return err
		})
		var pgErr *pgconn.PgError
		require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
		require.Equal(t, "23503", pgErr.Code, "expected a foreign key violation")
	})

	t.Run("the database refuses a role limited to another organization's property", func(t *testing.T) {
		err := env.Store.WithOrgTx(ctx, a.ID, func(tx pgx.Tx) error {
			role := authz.RoleKaryawan
			return dbgen.New(tx).CreateBuiltinRoleAssignment(ctx, dbgen.CreateBuiltinRoleAssignmentParams{
				ID: uuid.New(), OrganizationID: a.ID, MembershipID: adminA.MembershipID, BuiltinRole: &role,
				PropertyID: uuid.NullUUID{UUID: propB.ID, Valid: true},
			})
		})
		var pgErr *pgconn.PgError
		require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
		require.Equal(t, "23503", pgErr.Code, "expected a foreign key violation")
	})

	t.Run("no business table shows A any of B's rows", func(t *testing.T) {
		// Every table with an organization_id column, so tables added later are covered too.
		var tables []string
		err := env.Store.WithOrgTx(ctx, a.ID, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `
				SELECT table_name FROM information_schema.columns
				WHERE table_schema = 'public' AND column_name = 'organization_id'
				ORDER BY table_name`)
			if err != nil {
				return err
			}
			tables, err = pgx.CollectRows(rows, pgx.RowTo[string])
			return err
		})
		require.NoError(t, err)
		require.Contains(t, tables, "property")
		require.Contains(t, tables, "unit")

		for _, table := range tables {
			err := env.Store.WithOrgTx(ctx, a.ID, func(tx pgx.Tx) error {
				var n int
				// The table name comes from the catalog above, not from input.
				q := fmt.Sprintf("SELECT count(*) FROM %s WHERE organization_id = $1", pgx.Identifier{table}.Sanitize())
				if err := tx.QueryRow(ctx, q, b.ID).Scan(&n); err != nil {
					return err
				}
				require.Zero(t, n, table)
				return nil
			})
			require.NoError(t, err, table)
		}
	})
}

// readOnlyUnits is a custom role that may read properties and units but no tenants.
const readOnlyUnits = `{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["property:read","unit:read"]}]}`

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func unitsByLabel(units []property.Unit) map[string]property.Unit {
	out := make(map[string]property.Unit, len(units))
	for _, u := range units {
		out[u.Label] = u
	}
	return out
}

func occupiedCount(t *testing.T, props []property.Property, id uuid.UUID) int64 {
	t.Helper()
	for _, p := range props {
		if p.ID == id {
			return p.OccupiedUnitCount
		}
	}
	require.Failf(t, "property not listed", "%s", id)
	return 0
}

func TestUnitOccupancy(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812641")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	supervisor := env.Principal(t, o.ID, o.Supervisor.UserID)
	karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)

	melati, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	mawar, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kontrakan Mawar", Kind: property.KindKontrakan})
	require.NoError(t, err)
	units := map[string]property.Unit{}
	for _, label := range []string{"Kamar 1", "Kamar 2", "Kamar 3", "Kamar 4"} {
		units[label], err = env.Properties.CreateUnit(ctx, admin, melati.ID, property.UnitInput{Label: label})
		require.NoError(t, err)
	}
	rumahA, err := env.Properties.CreateUnit(ctx, admin, mawar.ID, property.UnitInput{Label: "Rumah A"})
	require.NoError(t, err)

	rent := func(name, phone string, unit property.Unit, start time.Time, collector uuid.UUID) tenant.Lease {
		t.Helper()
		tn, err := env.Tenants.Create(ctx, admin, tenant.Input{Name: name, Phone: phone})
		require.NoError(t, err)
		l, err := env.Tenants.CreateLease(ctx, admin, tenant.NewLease{TenantID: tn.ID, UnitID: unit.ID, Terms: tenant.Terms{
			StartDate: start, RentAmount: 850_000, CollectorMemberID: collector,
		}})
		require.NoError(t, err)
		return l
	}
	rina := rent("Rina Wulandari", "0812-6410-0001", units["Kamar 1"], day(2026, 10, 1), karyawan.MembershipID)
	moved := rent("Dewi Lestari", "0812-6410-0003", units["Kamar 3"], day(2026, 9, 1), uuid.Nil)
	_, err = env.Tenants.MoveOut(ctx, admin, moved.ID, day(2026, 10, 31))
	require.NoError(t, err)
	booked := rent("Budi Santoso", "0812-6410-0004", units["Kamar 4"], day(2026, 12, 15), uuid.Nil)
	rent("Siti Aminah", "0812-6410-0005", rumahA, day(2026, 10, 1), uuid.Nil)

	t.Run("an occupied unit shows its tenant, a free or ended one does not", func(t *testing.T) {
		got, err := env.Properties.ListUnits(ctx, admin, melati.ID)
		require.NoError(t, err)
		byLabel := unitsByLabel(got)

		require.True(t, byLabel["Kamar 1"].Occupied)
		require.NotNil(t, byLabel["Kamar 1"].CurrentTenant)
		require.Equal(t, rina.TenantID, byLabel["Kamar 1"].CurrentTenant.ID)
		require.Equal(t, "Rina Wulandari", byLabel["Kamar 1"].CurrentTenant.Name)

		require.False(t, byLabel["Kamar 2"].Occupied, "a free unit")
		require.Nil(t, byLabel["Kamar 2"].CurrentTenant)
		require.False(t, byLabel["Kamar 3"].Occupied, "a unit whose lease ended")
		require.Nil(t, byLabel["Kamar 3"].CurrentTenant)
	})

	t.Run("a booked lease that has not started counts as occupied", func(t *testing.T) {
		got, err := env.Properties.ListUnits(ctx, admin, melati.ID)
		require.NoError(t, err)
		kamar4 := unitsByLabel(got)["Kamar 4"]
		require.True(t, kamar4.Occupied)
		require.NotNil(t, kamar4.CurrentTenant)
		require.Equal(t, booked.TenantID, kamar4.CurrentTenant.ID)
	})

	t.Run("the occupied unit count is right on list and update", func(t *testing.T) {
		props, err := env.Properties.List(ctx, admin)
		require.NoError(t, err)
		require.Equal(t, int64(2), occupiedCount(t, props, melati.ID))
		require.Equal(t, int64(1), occupiedCount(t, props, mawar.ID))

		updated, err := env.Properties.Update(ctx, admin, melati.ID, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
		require.NoError(t, err)
		require.Equal(t, int64(2), updated.OccupiedUnitCount)
		require.Equal(t, int64(4), updated.UnitCount)
	})

	t.Run("a unit that is updated keeps its occupancy", func(t *testing.T) {
		u, err := env.Properties.UpdateUnit(ctx, admin, units["Kamar 1"].ID, property.UnitInput{Label: "Kamar 1", DefaultRent: new(int64(850_000))})
		require.NoError(t, err)
		require.True(t, u.Occupied)
		require.NotNil(t, u.CurrentTenant)
		require.Equal(t, "Rina Wulandari", u.CurrentTenant.Name)

		u, err = env.Properties.UpdateUnit(ctx, admin, units["Kamar 2"].ID, property.UnitInput{Label: "Kamar 2"})
		require.NoError(t, err)
		require.False(t, u.Occupied)
		require.Nil(t, u.CurrentTenant)
	})

	t.Run("a new unit is free", func(t *testing.T) {
		u, err := env.Properties.CreateUnit(ctx, admin, melati.ID, property.UnitInput{Label: "Kamar 5"})
		require.NoError(t, err)
		require.False(t, u.Occupied)
		require.Nil(t, u.CurrentTenant)
	})

	t.Run("the supervisor sees every tenant", func(t *testing.T) {
		got, err := env.Properties.ListUnits(ctx, supervisor, melati.ID)
		require.NoError(t, err)
		require.NotNil(t, unitsByLabel(got)["Kamar 4"].CurrentTenant)
	})

	t.Run("an admin limited to Kos Melati sees its tenants and its count", func(t *testing.T) {
		sari, err := env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-6410-0099", Name: "Mbak Sari", Roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati.ID}},
		})
		require.NoError(t, err)
		limited := env.Principal(t, o.ID, sari.UserID)

		got, err := env.Properties.ListUnits(ctx, limited, melati.ID)
		require.NoError(t, err)
		byLabel := unitsByLabel(got)
		require.NotNil(t, byLabel["Kamar 1"].CurrentTenant)
		require.Equal(t, "Rina Wulandari", byLabel["Kamar 1"].CurrentTenant.Name)
		require.NotNil(t, byLabel["Kamar 4"].CurrentTenant, "a booked lease counts too")

		props, err := env.Properties.List(ctx, limited)
		require.NoError(t, err)
		require.Len(t, props, 1)
		require.Equal(t, int64(2), props[0].OccupiedUnitCount)
	})

	t.Run("a custom role with unit:read but no tenant:read sees occupied only", func(t *testing.T) {
		hendra, err := env.Members.Invite(ctx, admin, member.Invite{
			Phone: "0812-6410-0098", Name: "Pak Hendra", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}},
		})
		require.NoError(t, err)
		// No built-in role: the custom role is all they have.
		err = env.Store.WithOrgTx(ctx, o.ID, func(tx pgx.Tx) error {
			return dbgen.New(tx).DeleteBuiltinRoleAssignments(ctx, dbgen.DeleteBuiltinRoleAssignmentsParams{
				OrganizationID: o.ID, MembershipID: hendra.ID,
			})
		})
		require.NoError(t, err)
		env.GrantCustomRole(t, o.ID, hendra.ID, "Pembaca Unit", readOnlyUnits)
		reader := env.Principal(t, o.ID, hendra.UserID)

		_, err = env.Tenants.List(ctx, reader)
		require.ErrorIs(t, err, authz.ErrForbidden, "the role really cannot read tenants")

		got, err := env.Properties.ListUnits(ctx, reader, melati.ID)
		require.NoError(t, err)
		byLabel := unitsByLabel(got)
		require.True(t, byLabel["Kamar 1"].Occupied)
		require.Nil(t, byLabel["Kamar 1"].CurrentTenant)
		require.True(t, byLabel["Kamar 4"].Occupied)
		require.Nil(t, byLabel["Kamar 4"].CurrentTenant)

		props, err := env.Properties.List(ctx, reader)
		require.NoError(t, err)
		require.Equal(t, int64(2), occupiedCount(t, props, melati.ID))
	})

	t.Run("a karyawan who may read units sees only the tenants they collect from", func(t *testing.T) {
		env.GrantCustomRole(t, o.ID, karyawan.MembershipID, "Pembaca Unit Lapangan", readOnlyUnits)
		got, err := env.Properties.ListUnits(ctx, env.Principal(t, o.ID, o.Karyawan.UserID), melati.ID)
		require.NoError(t, err)
		byLabel := unitsByLabel(got)
		require.NotNil(t, byLabel["Kamar 1"].CurrentTenant, "Rina's lease names this karyawan as collector")
		require.True(t, byLabel["Kamar 4"].Occupied)
		require.Nil(t, byLabel["Kamar 4"].CurrentTenant, "Budi is collected by nobody")
	})

	t.Run("another organization's units and leases never count", func(t *testing.T) {
		other := env.CreateOrg(t, "Kos Lain", "0812642")
		otherAdmin := env.Principal(t, other.ID, other.Admin.UserID)
		prop, err := env.Properties.Create(ctx, otherAdmin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
		require.NoError(t, err)
		for i, label := range []string{"Kamar 1", "Kamar 2", "Kamar 3"} {
			u, err := env.Properties.CreateUnit(ctx, otherAdmin, prop.ID, property.UnitInput{Label: label})
			require.NoError(t, err)
			tn, err := env.Tenants.Create(ctx, otherAdmin, tenant.Input{Name: "Penyewa Lain", Phone: fmt.Sprintf("0812-6420-000%d", i+1)})
			require.NoError(t, err)
			_, err = env.Tenants.CreateLease(ctx, otherAdmin, tenant.NewLease{TenantID: tn.ID, UnitID: u.ID, Terms: tenant.Terms{
				StartDate: day(2026, 10, 1), RentAmount: 700_000,
			}})
			require.NoError(t, err)
		}

		props, err := env.Properties.List(ctx, otherAdmin)
		require.NoError(t, err)
		require.Equal(t, int64(3), occupiedCount(t, props, prop.ID))

		mine, err := env.Properties.List(ctx, admin)
		require.NoError(t, err)
		require.Equal(t, int64(2), occupiedCount(t, mine, melati.ID), "unchanged by the other organization")
		require.Equal(t, int64(1), occupiedCount(t, mine, mawar.ID))
		require.Len(t, mine, 2)
	})
}
