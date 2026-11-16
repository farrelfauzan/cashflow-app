//go:build integration

package demoseed_test

import (
	"context"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/demoseed"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/visit"
)

// generateDeps are the services GenerateInvoices works through, with promise services on any clock
// it asks for.
func generateDeps(env *testenv.Env) demoseed.GenerateDeps {
	return demoseed.GenerateDeps{
		Store: env.Store, Authz: env.Authz, Orgs: env.Orgs, Billing: env.Billing,
		Promises: func(c clock.Clock) (*promise.Service, *promise.Closer) {
			return promise.NewService(promise.Deps{Store: env.Store, Tenants: env.Billing, Clock: c}),
				promise.NewCloser(env.Store, c, slog.New(slog.DiscardHandler))
		},
	}
}

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

	gen := generateDeps(env)

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
		// Nobody pays here; TestGenerateInvoicesKunjungan covers it.
		demoseed.OrgKunjungan: {"0812-0010-0002", [3]string{}},
	} {
		m, err := env.Orgs.FindForMember(ctx, a.phone, name)
		require.NoError(t, err)
		orgs[name] = demoOrg{admin: env.Principal(t, m.OrganizationID, m.UserID), payer: a.payer}
	}

	first, err := demoseed.GenerateInvoices(ctx, gen, env.Clock)
	require.NoError(t, err)
	require.Len(t, first, 4)

	for _, sum := range first {
		a := orgs[sum.Organization]
		t.Run(sum.Organization, func(t *testing.T) {
			require.Positive(t, sum.Invoices, "the seeded leases were billed")
			if sum.Organization == demoseed.OrgKunjungan {
				require.Zero(t, sum.Payments, "nobody pays here")
				require.Equal(t, 3, sum.Promises)
				return
			}
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
			require.Zero(t, sum.Promises, sum.Organization)
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
			require.Zero(t, sum.Promises, sum.Organization)
		}
		for name, a := range orgs {
			after := moneyCounts(t, env, a.admin.OrganizationID)
			require.Greater(t, after["invoice"], before[name]["invoice"], name)
			require.Equal(t, before[name]["payment"], after["payment"], name)
		}
	})
}

// openVisitTasks is the reasons of the open visit tasks of each tenant (by name) in the
// organization.
func openVisitTasks(t *testing.T, env *testenv.Env, org uuid.UUID) map[string][]string {
	t.Helper()
	ctx := context.Background()
	out := map[string][]string{}
	err := env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT t.name, v.reason FROM visit_task v JOIN tenant t ON t.id = v.tenant_id
			WHERE v.resolved_at IS NULL ORDER BY t.name, v.reason`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name, reason string
			if err := rows.Scan(&name, &reason); err != nil {
				return err
			}
			out[name] = append(out[name], reason)
		}
		return rows.Err()
	})
	require.NoError(t, err)
	return out
}

// promiseCount is how many promises the organization has, whatever their status.
func promiseCount(t *testing.T, env *testenv.Env, org uuid.UUID) int {
	t.Helper()
	ctx := context.Background()
	var n int
	err := env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM payment_promise").Scan(&n)
	})
	require.NoError(t, err)
	return n
}

func TestGenerateInvoicesKunjungan(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // the clock reads 2 November 2026, 09:00 in Jakarta
	today := tenant.Today(env.Clock)
	gen := generateDeps(env)

	_, err := demoseed.Run(ctx, deps(env), env.Clock)
	require.NoError(t, err)
	sums, err := demoseed.GenerateInvoices(ctx, gen, env.Clock)
	require.NoError(t, err)
	var sum demoseed.Summary
	for _, s := range sums {
		if s.Organization == demoseed.OrgKunjungan {
			sum = s
		}
	}
	require.Equal(t, 18, sum.Leases)
	require.Positive(t, sum.Invoices)
	require.Zero(t, sum.Payments, "nobody pays")
	require.Equal(t, 3, sum.Promises)

	m, err := env.Orgs.FindForMember(ctx, "0812-0010-0002", demoseed.OrgKunjungan)
	require.NoError(t, err)
	org := m.OrganizationID
	admin := env.Principal(t, org, m.UserID)
	tenants, err := env.AllTenants(ctx, admin)
	require.NoError(t, err)
	require.Len(t, tenants, 18)
	promises := promise.NewService(promise.Deps{Store: env.Store, Tenants: env.Billing, Clock: env.Clock})

	brokenTwice := []string{"Citra Ayu Lestari", "Gilang Permana", "Kiki Amelia"}
	t.Run("every tenant owes overdue invoices and only three have promises", func(t *testing.T) {
		for _, tn := range tenants {
			acct := account(t, env, admin, tn.Name)
			require.Positive(t, acct.Overdue, tn.Name)
			require.Empty(t, acct.Payments, tn.Name)
			var overdue int
			for _, inv := range acct.Invoices {
				if inv.DueDate.Before(today) && inv.Outstanding > 0 {
					overdue++
				}
			}
			require.GreaterOrEqual(t, overdue, 1, tn.Name)

			got, err := promises.List(ctx, admin, tn.ID, listing.FirstPage)
			require.NoError(t, err)
			if !slices.Contains(brokenTwice, tn.Name) {
				require.Zero(t, got.Total, "%s has no promise", tn.Name)
				continue
			}
			require.Equal(t, int64(2), got.Total, tn.Name)
			for _, p := range got.Items {
				require.Equal(t, promise.StatusBroken, p.Status, tn.Name)
				require.Len(t, p.Versions, 1)
				require.Equal(t, promise.SourceAdmin, p.Latest().Source)
				require.NotNil(t, p.ClosedAt)
				require.True(t, p.CreatedAt.Before(env.Clock.Now()), "dated in the past")
				require.True(t, p.ClosedAt.Before(env.Clock.Now()), "closed in the past")
				require.True(t, p.Latest().PromisedDate.Before(today), "its day has ended")
			}
		}
		require.Equal(t, 6, promiseCount(t, env, org))
	})

	t.Run("the three tenants have an open broken_promises visit task and no other task yet", func(t *testing.T) {
		require.Equal(t, map[string][]string{
			"Citra Ayu Lestari": {promise.VisitReasonBrokenPromises},
			"Gilang Permana":    {promise.VisitReasonBrokenPromises},
			"Kiki Amelia":       {promise.VisitReasonBrokenPromises},
		}, openVisitTasks(t, env, org))
	})

	t.Run("running it again adds no promise and no visit task", func(t *testing.T) {
		again, err := demoseed.GenerateInvoices(ctx, gen, env.Clock)
		require.NoError(t, err)
		for _, s := range again {
			require.Zero(t, s.Promises, s.Organization)
		}
		require.Equal(t, 6, promiseCount(t, env, org))
		require.Len(t, openVisitTasks(t, env, org), 3)
	})

	t.Run("a later day gives no tenant new promises", func(t *testing.T) {
		// On a later day the tenants still have their promises, so none is made again.
		env.Clock.Advance(24 * time.Hour)
		later, err := demoseed.GenerateInvoices(ctx, gen, env.Clock)
		require.NoError(t, err)
		for _, s := range later {
			require.Zero(t, s.Promises, s.Organization)
		}
		require.Equal(t, 6, promiseCount(t, env, org))
		env.Clock.Advance(-24 * time.Hour)
	})

	t.Run("the daily jobs open H+7 for everyone and each karyawan sees exactly their tenants", func(t *testing.T) {
		res, err := reminder.NewDecider(env.Store, env.Clock, slog.New(slog.DiscardHandler), nil).DecideOrg(ctx, org)
		require.NoError(t, err)
		require.Equal(t, 18, res.VisitTasks, "an h7 task for each tenant")

		visits := visit.NewService(env.Store, env.Billing, env.Clock)
		wantTenants := map[string][]string{
			"0812-0010-0011": {"Ahmad Fauzi", "Bayu Saputra", "Citra Ayu Lestari", "Danu Prakoso"},
			"0812-0010-0012": {"Eko Nugroho", "Fajar Ramadhan", "Gilang Permana", "Hesti Wulansari"},
			"0812-0010-0013": {"Imam Santoso", "Joko Susilo", "Kiki Amelia", "Lutfi Hakim"},
			"0812-0010-0021": {"Nico Pranata", "Satria Nugraha"},
			"0812-0010-0022": {"Galuh Kartiko", "Zaskia Ardiningsih"},
			"0812-0010-0023": {"Ilham Sasongko", "Tiara Anindita"},
		}
		for number, want := range wantTenants {
			p := env.Principal(t, org, env.UserID(t, number))
			got, err := visits.List(ctx, p, listing.FirstPage)
			require.NoError(t, err)
			require.EqualValues(t, len(want), got.Total, number)
			var names []string
			for _, row := range got.Items {
				names = append(names, row.TenantName)
				require.Equal(t, visit.StatusOpen, row.Status, row.TenantName)
				require.Positive(t, row.Overdue, row.TenantName)
				codes := map[string]visit.Reason{}
				for _, r := range row.Reasons {
					codes[r.Code] = r
				}
				require.Contains(t, codes, visit.ReasonOverdue, row.TenantName)
				require.Positive(t, codes[visit.ReasonOverdue].DaysOverdue, row.TenantName)
				if slices.Contains(brokenTwice, row.TenantName) {
					require.Len(t, codes, 2, row.TenantName)
					require.Equal(t, 2, codes[visit.ReasonBrokenPromises].BrokenPromises, row.TenantName)
				} else {
					require.Len(t, codes, 1, row.TenantName)
				}
			}
			require.ElementsMatch(t, want, names, number)
		}
	})
}
