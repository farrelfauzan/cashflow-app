//go:build integration

package tenantimport_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenantimport"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

// counts is how many records the organization holds, seen by its admin.
type counts struct{ properties, units, tenants, leases int }

func countAll(t *testing.T, env *testenv.Env, admin authz.Principal) counts {
	t.Helper()
	ctx := context.Background()
	props, err := env.Properties.List(ctx, admin)
	require.NoError(t, err)
	var c counts
	c.properties = len(props)
	for _, p := range props {
		c.units += int(p.UnitCount)
	}
	tenants, err := env.Tenants.List(ctx, admin)
	require.NoError(t, err)
	c.tenants = len(tenants)
	for _, tn := range tenants {
		c.leases += len(tn.Leases)
	}
	return c
}

func errorColumns(row tenantimport.Row) []string {
	var out []string
	for _, e := range row.Errors {
		out = append(out, e.Column)
	}
	return out
}

func TestImport(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812811")
	admin := env.Principal(t, o.ID, o.Admin.UserID)

	t.Run("a dry run reports what would happen and writes nothing", func(t *testing.T) {
		auditBefore := len(env.AuditLog(t, o.ID))
		res, err := env.Imports.Run(ctx, admin, fixture(t, "pilot.csv"), true)
		require.NoError(t, err)
		require.True(t, res.DryRun)
		require.False(t, res.Committed)
		require.Equal(t, tenantimport.Summary{
			Rows: 5, PropertiesCreated: 3, UnitsCreated: 5, TenantsCreated: 4, LeasesCreated: 5,
		}, res.Summary, "Rina rents two units but is one tenant")
		for _, row := range res.Rows {
			require.True(t, row.OK(), "line %d: %v", row.Line, row.Errors)
		}
		require.Equal(t, tenantimport.Matched, res.Rows[3].Tenant, "line 5 finds Rina from line 2")
		require.Equal(t, &tenantimport.Values{
			StartDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Rent: 900_000, DueDay: 10,
		}, res.Rows[1].Values, "the row shows the terms it would save: 01/10/2026 read day first, no deposit")
		require.Equal(t, counts{}, countAll(t, env, admin))
		require.Len(t, env.AuditLog(t, o.ID), auditBefore, "the audit rows of a dry run roll back with it")
	})

	t.Run("a dry run lists every bad row and why", func(t *testing.T) {
		res, err := env.Imports.Run(ctx, admin, fixture(t, "bad_rows.csv"), true)
		require.NoError(t, err)
		require.Equal(t, 6, res.Summary.Rows)
		require.Equal(t, 5, res.Summary.RowsWithErrors)
		require.True(t, res.Rows[0].OK())
		require.Equal(t, []string{"phone"}, errorColumns(res.Rows[1]))
		require.Equal(t, []string{"rent"}, errorColumns(res.Rows[2]))
		require.Nil(t, res.Rows[2].Values, "no values when the rent cannot be read")
		require.Equal(t, []string{"due_day"}, errorColumns(res.Rows[3]))
		require.Equal(t, []string{"start_date"}, errorColumns(res.Rows[4]))
		require.Equal(t, []string{"unit"}, errorColumns(res.Rows[5]), "Kamar 1 is already rented to Rina on line 2")
		require.Equal(t, 7, res.Rows[5].Line)
		require.Equal(t, counts{}, countAll(t, env, admin))
	})

	t.Run("a commit with one bad row writes nothing", func(t *testing.T) {
		res, err := env.Imports.Run(ctx, admin, fixture(t, "bad_rows.csv"), false)
		require.NoError(t, err)
		require.False(t, res.Committed)
		require.Equal(t, counts{}, countAll(t, env, admin))
	})

	t.Run("a clean commit saves every row", func(t *testing.T) {
		res, err := env.Imports.Run(ctx, admin, fixture(t, "pilot.csv"), false)
		require.NoError(t, err)
		require.True(t, res.Committed)
		require.Equal(t, counts{properties: 3, units: 5, tenants: 4, leases: 5}, countAll(t, env, admin))

		props, err := env.Properties.List(ctx, admin)
		require.NoError(t, err)
		require.Equal(t, property.KindKontrakan, props[0].Kind, "Kontrakan Mawar sorts first")
		units, err := env.Properties.ListUnits(ctx, admin, props[1].ID)
		require.NoError(t, err)
		require.Equal(t, []string{"Kamar 1", "Kamar 2", "Kamar 11"}, []string{units[0].Label, units[1].Label, units[2].Label})
		require.Equal(t, int64(850_000), *units[0].DefaultRent, "a new unit takes the rent as its usual rent")

		tenants, err := env.Tenants.List(ctx, admin)
		require.NoError(t, err)
		var budi tenant.Tenant
		for _, tn := range tenants {
			if tn.Name == "Budi Santoso" {
				budi = tn
			}
		}
		require.Equal(t, "+6281281005002", budi.Phone)
		require.Len(t, budi.Leases, 1)
		require.Equal(t, int64(900_000), budi.Leases[0].RentAmount)
		require.Zero(t, budi.Leases[0].DepositAmount)
		require.Equal(t, 10, budi.Leases[0].DueDay)

		var actions []string
		for _, a := range env.AuditLog(t, o.ID) {
			actions = append(actions, a.Action)
		}
		require.Contains(t, actions, "lease:manage", "the import's changes are audited like any other")
	})

	t.Run("running the same file again changes nothing", func(t *testing.T) {
		res, err := env.Imports.Run(ctx, admin, fixture(t, "pilot.csv"), false)
		require.NoError(t, err)
		require.True(t, res.Committed)
		require.Equal(t, tenantimport.Summary{Rows: 5, LeasesMatched: 5}, res.Summary)
		require.Equal(t, counts{properties: 3, units: 5, tenants: 4, leases: 5}, countAll(t, env, admin))
	})

	t.Run("a known phone under another name is an error, not a merge", func(t *testing.T) {
		data := []byte("property,unit,tenant_name,phone,rent,due_day,start_date\n" +
			"Kos Melati,Kamar 7,Pak Budi,0812-8100-5002,950.000,10,2026-11-01\n")
		res, err := env.Imports.Run(ctx, admin, data, false)
		require.NoError(t, err)
		require.False(t, res.Committed)
		require.Equal(t, []string{"phone"}, errorColumns(res.Rows[0]))
		require.Contains(t, res.Rows[0].Errors[0].Message, "Budi Santoso")
		require.Empty(t, res.Rows[0].Unit, "the row's unit is rolled back with it")
		require.Equal(t, counts{properties: 3, units: 5, tenants: 4, leases: 5}, countAll(t, env, admin))
	})

	t.Run("existing records are matched, and differences are noted, not applied", func(t *testing.T) {
		data := []byte("property,unit,tenant_name,phone,rent,due_day,start_date,deposit\n" +
			"KOS MELATI,kamar 2,budi santoso,0812-8100-5002,900.000,10,2026-10-01,500.000\n" +
			"Kos Melati,Kamar 12,Budi Santoso,0812-8100-5002,950.000,10,2026-11-01,\n")
		res, err := env.Imports.Run(ctx, admin, data, false)
		require.NoError(t, err)
		require.True(t, res.Committed)
		first, second := res.Rows[0], res.Rows[1]
		require.Equal(t, tenantimport.Matched, first.Property)
		require.Equal(t, tenantimport.Matched, first.Unit)
		require.Equal(t, tenantimport.Matched, first.Tenant, "names match ignoring case")
		require.Equal(t, tenantimport.Matched, first.Lease)
		require.Len(t, first.Notes, 1, "only the deposit differs, and it is noted")
		require.Equal(t, tenantimport.Created, second.Unit)
		require.Equal(t, tenantimport.Created, second.Lease)
		require.Equal(t, counts{properties: 3, units: 6, tenants: 4, leases: 6}, countAll(t, env, admin))
	})

	t.Run("a unit rented to someone else is an error", func(t *testing.T) {
		data := []byte("property,unit,tenant_name,phone,rent,due_day,start_date\n" +
			"Kos Melati,Kamar 1,Penyewa Baru,0812-8100-5099,850.000,5,2026-11-01\n")
		res, err := env.Imports.Run(ctx, admin, data, false)
		require.NoError(t, err)
		require.False(t, res.Committed)
		require.Equal(t, []string{"unit"}, errorColumns(res.Rows[0]))
	})
}

func TestImportPermissions(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812821")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	data := fixture(t, "pilot.csv")

	for name, p := range map[string]authz.Principal{
		"karyawan":   env.Principal(t, o.ID, o.Karyawan.UserID),
		"supervisor": env.Principal(t, o.ID, o.Supervisor.UserID),
	} {
		t.Run(name+" cannot import, even as a dry run", func(t *testing.T) {
			_, err := env.Imports.Run(ctx, p, data, true)
			require.ErrorIs(t, err, authz.ErrForbidden)
		})
	}

	t.Run("an admin limited to one property cannot import", func(t *testing.T) {
		melati, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
		require.NoError(t, err)
		sari, err := env.Members.Invite(ctx, admin, member.Invite{Phone: "0812-8210-0099", Name: "Mbak Sari", Roles: []string{authz.RoleKaryawan}})
		require.NoError(t, err)
		_, err = env.Members.SetRoles(ctx, admin, sari.ID, []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: melati.ID}})
		require.NoError(t, err)
		_, err = env.Imports.Run(ctx, env.Principal(t, o.ID, sari.UserID), data, true)
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("a file without the required columns is refused before any row", func(t *testing.T) {
		_, err := env.Imports.Run(ctx, admin, []byte("nama,kamar\nRina,1\n"), true)
		var fileErr *tenantimport.FileError
		require.ErrorAs(t, err, &fileErr)
	})
}

func TestTemplate(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812841")
	admin := env.Principal(t, o.ID, o.Admin.UserID)

	t.Run("the template's example rows are refused, and for nothing else", func(t *testing.T) {
		data, err := env.Imports.Template(ctx, admin)
		require.NoError(t, err)
		res, err := env.Imports.Run(ctx, admin, data, false)
		require.NoError(t, err)
		require.False(t, res.Committed)
		require.Equal(t, 2, res.Summary.RowsWithErrors)
		for _, row := range res.Rows {
			require.Equal(t, []tenantimport.RowError{{Column: "phone", Message: "Ini baris contoh. Hapus baris ini sebelum mengimpor."}}, row.Errors)
		}
		require.Equal(t, counts{}, countAll(t, env, admin))
	})

	t.Run("the template filled in with real numbers imports in a dry run without errors", func(t *testing.T) {
		data, err := env.Imports.Template(ctx, admin)
		require.NoError(t, err)
		data = bytes.ReplaceAll(data, []byte("0812-0000-0001"), []byte("0813-8400-6001"))
		data = bytes.ReplaceAll(data, []byte("0812-0000-0002"), []byte("0813-8400-6002"))
		res, err := env.Imports.Run(ctx, admin, data, true)
		require.NoError(t, err)
		require.Zero(t, res.Summary.RowsWithErrors, "%+v", res.Rows)
		require.Equal(t, 2, res.Summary.LeasesCreated)
	})

	t.Run("only those who may import may download it, and a refusal is audited", func(t *testing.T) {
		karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)
		_, err := env.Imports.Template(ctx, karyawan)
		require.ErrorIs(t, err, authz.ErrForbidden)
		audit := env.AuditLog(t, o.ID)
		require.Equal(t, "deny", audit[0].Decision)
		require.Equal(t, "tenant:manage", audit[0].Action)
	})
}
