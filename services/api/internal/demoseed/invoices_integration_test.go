//go:build integration

package demoseed_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/demoseed"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// moneyTables are the tables generate-invoices writes to.
var moneyTables = []string{"invoice", "invoice_line", "payment", "payment_allocation", "correction"}

// moneyCounts is how many rows each money table holds in one organization.
func moneyCounts(t *testing.T, env *testenv.Env, org uuid.UUID) map[string]int {
	t.Helper()
	ctx := context.Background()
	out := map[string]int{}
	err := env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		for _, name := range moneyTables {
			var n int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{name}.Sanitize()).Scan(&n); err != nil {
				return err
			}
			out[name] = n
		}
		return nil
	})
	require.NoError(t, err)
	return out
}

// account is the money account of the organization's tenant called name.
func account(t *testing.T, env *testenv.Env, p authz.Principal, name string) billing.Account {
	t.Helper()
	ctx := context.Background()
	tenants, err := env.AllTenants(ctx, p)
	require.NoError(t, err)
	acct, err := env.Billing.Account(ctx, p, find(t, tenants, name).ID)
	require.NoError(t, err)
	return acct
}

// demoOrg is a demo organization's Admin and the tenants that pay in full, in part and not at all.
type demoOrg struct {
	admin authz.Principal
	payer [3]string
}

func TestGenerateInvoices(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // the clock reads 2 November 2026, 09:00 in Jakarta
	today := tenant.Today(env.Clock)

	// An organization that is not a demo one, with a lease that generation would bill.
	other := env.CreateOrg(t, "Kos Bukan Demo", "0812777")
	otherAdmin := env.Principal(t, other.ID, other.Admin.UserID)
	prop, err := env.Properties.Create(ctx, otherAdmin, property.PropertyInput{Name: "Kos Lain", Kind: property.KindKos, Address: "Jl. Lain No. 1, Sleman"})
	require.NoError(t, err)
	price := int64(700_000)
	unit, err := env.Properties.CreateUnit(ctx, otherAdmin, prop.ID, property.UnitInput{
		Label: "Kamar 1", Terms: []property.TermInput{{Months: 1, Price: &price}},
	})
	require.NoError(t, err)
	tn, err := env.Tenants.Create(ctx, otherAdmin, tenant.Input{Name: "Penyewa Lain", Phone: "0812777" + "5001"})
	require.NoError(t, err)
	_, err = env.Tenants.CreateLease(ctx, otherAdmin, tenant.NewLease{
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
	})
	require.NoError(t, err)

	gen := demoseed.GenerateDeps{Store: env.Store, Authz: env.Authz, Orgs: env.Orgs, Billing: env.Billing}

	t.Run("it needs the demo organizations to exist", func(t *testing.T) {
		_, err := demoseed.GenerateInvoices(ctx, gen, env.Clock)
		require.ErrorContains(t, err, "run seed-demo first")
		require.Equal(t, 0, moneyCounts(t, env, other.ID)["invoice"])
	})

	_, err = demoseed.Run(ctx, deps(env), env.Clock)
	require.NoError(t, err)
	orgs := map[string]demoOrg{}
	for name, a := range map[string]struct {
		phone string
		payer [3]string
	}{
		demoseed.OrgGriya:   {"0812-0001-0002", [3]string{"Budi Santoso", "Citra Lestari", "Eko Prasetyo"}},
		demoseed.OrgAnggrek: {"0812-0002-0002", [3]string{"Nadia Putri", "Ayu Lestari", "Rina Wulandari"}},
		demoseed.OrgBulanan: {"0812-0009-0027", [3]string{"Andi Pratama", "Bagas Aditya", "Cahyo Wibowo"}},
	} {
		m, err := env.Orgs.FindForMember(ctx, a.phone, name)
		require.NoError(t, err)
		orgs[name] = demoOrg{admin: env.Principal(t, m.OrganizationID, m.UserID), payer: a.payer}
	}

	first, err := demoseed.GenerateInvoices(ctx, gen, env.Clock)
	require.NoError(t, err)
	require.Len(t, first, 3)

	for _, sum := range first {
		a := orgs[sum.Organization]
		t.Run(sum.Organization, func(t *testing.T) {
			require.Positive(t, sum.Invoices, "the seeded leases were billed")
			require.Equal(t, 2, sum.Payments, "a full and a partial payment")
			require.Zero(t, sum.Skipped)

			full := account(t, env, a.admin, a.payer[0])
			require.Zero(t, full.Outstanding, "the full payer owes nothing")
			require.Len(t, full.Payments, 1)
			require.Equal(t, billing.MethodTransfer, full.Payments[0].Method)
			require.NotEmpty(t, full.Invoices)

			partial := account(t, env, a.admin, a.payer[1])
			require.Positive(t, partial.Outstanding)
			require.Len(t, partial.Payments, 1)
			require.Equal(t, billing.MethodCash, partial.Payments[0].Method)
			statuses := map[string]int{}
			for _, inv := range partial.Invoices {
				statuses[inv.Status]++
			}
			require.Equal(t, 1, statuses["partial"], "the oldest invoice is part paid: %v", statuses)

			overdue := account(t, env, a.admin, a.payer[2])
			require.Empty(t, overdue.Payments)
			require.Positive(t, overdue.Overdue)
			dueBefore := 0
			for _, inv := range overdue.Invoices {
				if inv.DueDate.Before(today) && inv.Outstanding > 0 {
					dueBefore++
				}
			}
			require.Positive(t, dueBefore, "an unpaid invoice is due before today")
		})
	}

	t.Run("an organization that is not a demo one gets nothing", func(t *testing.T) {
		counts := moneyCounts(t, env, other.ID)
		for _, name := range moneyTables {
			require.Zero(t, counts[name], name)
		}
		// Its lease is one generation would have billed.
		res, err := env.Billing.GenerateOrg(ctx, other.ID)
		require.NoError(t, err)
		require.Positive(t, res.Invoices)
	})

	t.Run("running seed and generate-invoices again changes no money row", func(t *testing.T) {
		before := map[string]map[string]int{}
		for name, a := range orgs {
			before[name] = moneyCounts(t, env, a.admin.OrganizationID)
		}
		_, err := demoseed.Run(ctx, deps(env), env.Clock)
		require.NoError(t, err)
		second, err := demoseed.GenerateInvoices(ctx, gen, env.Clock)
		require.NoError(t, err)
		for _, sum := range second {
			require.Zero(t, sum.Invoices, sum.Organization)
			require.Zero(t, sum.Payments, sum.Organization)
		}
		for name, a := range orgs {
			require.Equal(t, before[name], moneyCounts(t, env, a.admin.OrganizationID), name)
		}
	})

	t.Run("a later day bills new periods and still records the payments once", func(t *testing.T) {
		before := map[string]map[string]int{}
		for name, a := range orgs {
			before[name] = moneyCounts(t, env, a.admin.OrganizationID)
		}
		env.Clock.Advance(40 * 24 * time.Hour)
		later, err := demoseed.GenerateInvoices(ctx, gen, env.Clock)
		require.NoError(t, err)
		for _, sum := range later {
			require.Positive(t, sum.Invoices, sum.Organization)
			require.Zero(t, sum.Payments, sum.Organization)
		}
		for name, a := range orgs {
			after := moneyCounts(t, env, a.admin.OrganizationID)
			require.Greater(t, after["invoice"], before[name]["invoice"], name)
			require.Equal(t, before[name]["payment"], after["payment"], name)
		}
	})
}
