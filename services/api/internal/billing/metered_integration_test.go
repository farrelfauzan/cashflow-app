//go:build integration

package billing_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// meterWorld is a world with two metered charge types, Listrik and Air.
type meterWorld struct {
	*world
	listrik, air tenant.ChargeType
}

func newMeterWorld(t *testing.T, env *testenv.Env, prefix string) meterWorld {
	t.Helper()
	ctx := context.Background()
	w := meterWorld{world: newWorld(t, env, prefix)}
	var err error
	w.listrik, err = env.Tenants.CreateChargeType(ctx, w.admin, tenant.ChargeTypeInput{Name: "Listrik", Metered: true})
	require.NoError(t, err)
	w.air, err = env.Tenants.CreateChargeType(ctx, w.admin, tenant.ChargeTypeInput{Name: "Air", Metered: true})
	require.NoError(t, err)
	return w
}

// meter records a metered amount as p and fails the test on an error.
func (w meterWorld) meter(t *testing.T, p authz.Principal, leaseID uuid.UUID, ct tenant.ChargeType, month time.Time, amount int64) billing.MeteredCharge {
	t.Helper()
	mc, err := w.env.Billing.RecordMeteredCharge(context.Background(), p, leaseID, ct.ID, month, billing.MeteredInput{Amount: amount})
	require.NoError(t, err)
	return mc
}

// expect sets the metered types a lease expects, as the admin, keeping its fixed extras.
func (w meterWorld) expect(t *testing.T, leaseID uuid.UUID, types ...tenant.ChargeType) tenant.Lease {
	t.Helper()
	ids := make([]uuid.UUID, len(types))
	for i, ct := range types {
		ids[i] = ct.ID
	}
	l, err := w.env.Tenants.SetLeaseCharges(context.Background(), w.admin, leaseID, tenant.LeaseChargesInput{Charges: []tenant.ChargeInput{}, MeteredTypes: &ids})
	require.NoError(t, err)
	return l
}

// rentCollected creates a unit in prop, a tenant and a monthly lease from start, collected by
// the member collector (uuid.Nil for none), as the admin.
func (w meterWorld) rentCollected(t *testing.T, prop property.Property, name string, start time.Time, collector uuid.UUID) rental {
	t.Helper()
	ctx := context.Background()
	w.n++
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, prop.ID, property.UnitInput{Label: fmt.Sprintf("Kamar %d", w.n), Terms: testenv.Monthly(700_000)})
	require.NoError(t, err)
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	l, err := w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: start, CollectorMemberID: collector},
	})
	require.NoError(t, err)
	return rental{renter: renter{tenantID: tn.ID, leaseID: l.ID}, unitID: unit.ID}
}

func (w meterWorld) meteredList(t *testing.T, p authz.Principal, leaseID uuid.UUID, mq billing.MeteredQuery) billing.MeteredList {
	t.Helper()
	res, err := w.env.Billing.ListLeaseMeteredCharges(context.Background(), p, leaseID, mq)
	require.NoError(t, err)
	return res
}

// missing reads every page of the missing-readings list.
func (w meterWorld) missing(t *testing.T, p authz.Principal, mq billing.MissingQuery) []billing.MissingReading {
	t.Helper()
	items, err := listing.All(func(page listing.Page) (listing.Result[billing.MissingReading], error) {
		mq.Page = page
		return w.env.Billing.ListMissingMeterReadings(context.Background(), p, mq)
	})
	require.NoError(t, err)
	return items
}

func missingNames(items []billing.MissingReading) []string {
	out := []string{}
	for _, it := range items {
		var types []string
		for _, m := range it.Missing {
			types = append(types, m.Name)
		}
		out = append(out, fmt.Sprintf("%s %v", it.TenantName, types))
	}
	return out
}

// storedAmount reads a metered amount as stored.
func (w meterWorld) storedAmount(t *testing.T, id uuid.UUID) int64 {
	t.Helper()
	var amount int64
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		return tx.QueryRow(ctx, "SELECT amount FROM metered_charge WHERE id = $1", id).Scan(&amount)
	})
	return amount
}

// meteredLines returns the invoice lines that bill a metered amount, for every invoice of r.
func (w meterWorld) meteredLines(t *testing.T, r renter) []dbgen.InvoiceLine {
	t.Helper()
	var out []dbgen.InvoiceLine
	for _, inv := range w.invoices(t, r) {
		for _, ln := range w.lines(t, inv.InvoiceID) {
			if ln.MeteredChargeID.Valid {
				out = append(out, ln)
			}
		}
	}
	return out
}

func TestMeteredCharges(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	t.Run("record, edit, bill once on the next invoice, then locked", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 9, 0)
		w := newMeterWorld(t, env, "0812841")
		r := w.rent(t, "Rina Wulandari", day(2026, 10, 28), rentOpts{})
		require.Equal(t, 1, w.generate(t).Invoices)

		setNow(env, 2026, 11, 3, 10, 0)
		first, err := env.Billing.RecordMeteredCharge(ctx, w.admin, r.leaseID, w.listrik.ID, day(2026, 10, 1),
			billing.MeteredInput{Amount: 123_000, MeterStart: i64(1200), MeterEnd: i64(1350)})
		require.NoError(t, err)
		require.Equal(t, "Listrik", first.ChargeTypeName)
		require.Equal(t, day(2026, 10, 1), first.UsageMonth)
		require.EqualValues(t, 123_000, first.Amount)
		require.Equal(t, int64(1350), *first.MeterEnd)
		require.Equal(t, w.org.Admin.Name, first.RecordedBy.Name)
		require.Nil(t, first.Billed)

		setNow(env, 2026, 11, 4, 10, 0)
		edited := w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 150_000)
		require.Equal(t, first.ID, edited.ID, "the same month and type is edited, not added")
		require.EqualValues(t, 150_000, edited.Amount)
		require.Nil(t, edited.MeterStart, "the meter numbers are replaced too")
		require.True(t, edited.UpdatedAt.After(first.UpdatedAt))
		require.Equal(t, first.CreatedAt, edited.CreatedAt)

		list := w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{})
		require.EqualValues(t, 1, list.Total)
		require.Equal(t, &billing.NextInvoice{Period: day(2026, 11, 28), IssuedOn: day(2026, 11, 21)}, list.NextInvoice)

		// The dry run the import shows equals what generation inserts, the reading included.
		setNow(env, 2026, 11, 21, 5, 0)
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			n, total, err := billing.PreviewLease(ctx, tx, w.org.ID, r.leaseID, day(2026, 11, 21))
			require.Equal(t, 1, n)
			require.EqualValues(t, 850_000, total)
			return err
		})
		require.Equal(t, 1, w.generate(t).Invoices)
		invs := w.invoices(t, r.renter)
		require.Len(t, invs, 2)
		nov := invs[1]
		require.EqualValues(t, 850_000, nov.Amount, "rent plus Listrik Oktober")
		lines := w.lines(t, nov.InvoiceID)
		require.Len(t, lines, 2)
		require.Equal(t, "Listrik Oktober 2026", lines[1].Label)
		require.Equal(t, "charge", lines[1].Kind)
		require.EqualValues(t, 150_000, lines[1].Amount)
		require.Equal(t, w.listrik.ID, lines[1].ChargeTypeID.UUID)
		require.Equal(t, first.ID, lines[1].MeteredChargeID.UUID)
		require.Len(t, w.lines(t, invs[0].InvoiceID), 1, "the older invoice is untouched")

		require.Zero(t, w.generate(t).Invoices)
		require.Len(t, w.meteredLines(t, r.renter), 1, "a rerun bills nothing twice")

		_, err = env.Billing.RecordMeteredCharge(ctx, w.admin, r.leaseID, w.listrik.ID, day(2026, 10, 1), billing.MeteredInput{Amount: 1})
		require.ErrorIs(t, err, billing.ErrMeteredChargeBilled)
		require.EqualValues(t, 150_000, w.storedAmount(t, first.ID))
		list = w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{})
		require.Equal(t, &billing.BilledRef{InvoiceID: nov.InvoiceID, Period: day(2026, 11, 28)}, list.Items[0].Billed)
		require.Equal(t, &billing.NextInvoice{Period: day(2026, 12, 28), IssuedOn: day(2026, 12, 21)}, list.NextInvoice)

		t.Run("the database refuses to change or delete a billed amount", func(t *testing.T) {
			err := w.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				_, err := tx.Exec(ctx, "UPDATE metered_charge SET amount = 1 WHERE id = $1", first.ID)
				return err
			})
			pgErr := requireCode(t, err, "42501")
			require.Contains(t, pgErr.Message, "is billed")
			err = w.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				_, err := tx.Exec(ctx, "DELETE FROM metered_charge WHERE id = $1", first.ID)
				return err
			})
			requireCode(t, err, "42501") // the app role has no DELETE at all

			// The owner role has every privilege; the trigger still refuses.
			owner, err := pgx.Connect(ctx, env.URLs.Owner)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, owner.Close(ctx)) })
			for _, stmt := range []string{"UPDATE metered_charge SET amount = 1 WHERE id = $1", "DELETE FROM metered_charge WHERE id = $1"} {
				tx, err := owner.Begin(ctx)
				require.NoError(t, err)
				_, err = tx.Exec(ctx, "SELECT set_config('app.org_id', $1, true)", w.org.ID.String())
				require.NoError(t, err)
				_, err = tx.Exec(ctx, stmt, first.ID)
				pgErr := requireCode(t, err, "42501")
				require.Contains(t, pgErr.Message, "is billed", stmt)
				require.NoError(t, tx.Rollback(ctx))
			}
			require.EqualValues(t, 150_000, w.storedAmount(t, first.ID))
		})

		t.Run("an unbilled amount may still be changed directly", func(t *testing.T) {
			air := w.meter(t, w.admin, r.leaseID, w.air, day(2026, 11, 1), 40_000)
			w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				_, err := tx.Exec(ctx, "UPDATE metered_charge SET amount = 45000 WHERE id = $1", air.ID)
				return err
			})
			require.EqualValues(t, 45_000, w.storedAmount(t, air.ID))
		})

		t.Run("a reading recorded after an invoice was made goes on the following one", func(t *testing.T) {
			setNow(env, 2026, 11, 25, 10, 0)
			nov := w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 11, 1), 98_000)
			setNow(env, 2026, 12, 21, 5, 0)
			require.Equal(t, 1, w.generate(t).Invoices)
			invs := w.invoices(t, r.renter)
			dec := invs[len(invs)-1]
			require.Equal(t, day(2026, 12, 28), dec.Period)
			var labels []string
			for _, ln := range w.lines(t, dec.InvoiceID) {
				labels = append(labels, ln.Label)
			}
			require.Equal(t, []string{"Sewa 28 Des 2026 – 27 Jan 2027", "Air November 2026", "Listrik November 2026"}, labels,
				"oldest month first, then by type name")
			require.EqualValues(t, 700_000+45_000+98_000, dec.Amount)
			list := w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{})
			require.Equal(t, day(2026, 11, 1), list.Items[0].UsageMonth, "the newest month first")
			require.Equal(t, "Air", list.Items[0].ChargeTypeName)
			require.Equal(t, nov.ID, list.Items[1].ID)
			require.Equal(t, day(2026, 10, 1), list.Items[2].UsageMonth)
			only := w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{ChargeTypeID: w.listrik.ID, UsageMonth: day(2026, 11, 1)})
			require.EqualValues(t, 1, only.Total)
			require.Equal(t, nov.ID, only.Items[0].ID)
			page2 := w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{Page: listing.Page{Number: 2, Size: 2}})
			require.EqualValues(t, 3, page2.Total)
			require.Len(t, page2.Items, 1)
		})
	})

	t.Run("a late-entered lease bills its readings on the newest of the invoices made at once", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 9, 0)
		w := newMeterWorld(t, env, "0812842")
		// Saving the lease only enqueues generation, so the reading is recorded before its
		// first invoices exist.
		r := w.rent(t, "Dewi Lestari", day(2026, 9, 15), rentOpts{})
		w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 9, 1), 75_000)
		list := w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{})
		require.Equal(t, &billing.NextInvoice{Period: day(2026, 10, 15), IssuedOn: day(2026, 10, 28)}, list.NextInvoice)
		require.Equal(t, 2, w.generate(t).Invoices)
		invs := w.invoices(t, r.renter)
		require.Len(t, w.lines(t, invs[0].InvoiceID), 1)
		lines := w.lines(t, invs[1].InvoiceID)
		require.Len(t, lines, 2)
		require.Equal(t, "Listrik September 2026", lines[1].Label)
	})

	t.Run("refusals", func(t *testing.T) {
		setNow(env, 2026, 11, 3, 10, 0)
		w := newMeterWorld(t, env, "0812843")
		r := w.rent(t, "Bambang Wijaya", day(2026, 10, 10), rentOpts{})
		other := newMeterWorld(t, env, "0812844")
		ro := other.rent(t, "Slamet Riyadi", day(2026, 10, 10), rentOpts{})
		tests := []struct {
			name   string
			lease  uuid.UUID
			ct     uuid.UUID
			month  time.Time
			amount int64
			field  string
			err    error
		}{
			{"a month before the lease started", r.leaseID, w.listrik.ID, day(2026, 9, 1), 1, "usageMonth", nil},
			{"a month after this one", r.leaseID, w.listrik.ID, day(2026, 12, 1), 1, "usageMonth", nil},
			{"a type that is not metered", r.leaseID, w.wifi.ID, day(2026, 10, 1), 1, "chargeTypeId", nil},
			{"a type that does not exist", r.leaseID, uuid.New(), day(2026, 10, 1), 1, "chargeTypeId", nil},
			{"another organization's type", r.leaseID, other.listrik.ID, day(2026, 10, 1), 1, "chargeTypeId", nil},
			{"no amount", r.leaseID, w.listrik.ID, day(2026, 10, 1), 0, "amount", nil},
			{"another organization's lease", ro.leaseID, w.listrik.ID, day(2026, 10, 1), 1, "", billing.ErrNotFound},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := env.Billing.RecordMeteredCharge(ctx, w.admin, tc.lease, tc.ct, tc.month, billing.MeteredInput{Amount: tc.amount})
				if tc.err != nil {
					require.ErrorIs(t, err, tc.err)
					return
				}
				requireInput(t, err, tc.field)
			})
		}
		_, err := env.Billing.ListLeaseMeteredCharges(ctx, w.admin, ro.leaseID, billing.MeteredQuery{})
		require.ErrorIs(t, err, billing.ErrNotFound)

		t.Run("an ended lease takes no amount but stays readable", func(t *testing.T) {
			w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 50_000)
			_, err := env.Tenants.MoveOut(ctx, w.admin, r.leaseID, day(2026, 11, 2))
			require.NoError(t, err)
			_, err = env.Billing.RecordMeteredCharge(ctx, w.admin, r.leaseID, w.listrik.ID, day(2026, 11, 1), billing.MeteredInput{Amount: 1})
			require.ErrorIs(t, err, tenant.ErrLeaseEnded)
			list := w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{})
			require.EqualValues(t, 1, list.Total)
			require.Nil(t, list.NextInvoice, "no invoice comes after the move-out")
		})

		t.Run("nothing changed for the refusals", func(t *testing.T) {
			require.EqualValues(t, 1, w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{}).Total)
		})
	})

	t.Run("a planned end with no period left gives no next invoice", func(t *testing.T) {
		setNow(env, 2026, 11, 3, 10, 0)
		w := newMeterWorld(t, env, "0812845")
		r := w.rent(t, "Citra Ayu", day(2026, 10, 10), rentOpts{})
		w.generate(t)
		_, err := env.Tenants.SetPlannedEnd(ctx, w.admin, r.leaseID, ptr(day(2026, 11, 9)))
		require.NoError(t, err)
		require.Nil(t, w.meteredList(t, w.admin, r.leaseID, billing.MeteredQuery{}).NextInvoice)
	})
}

func TestMeteredChargeAccess(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 3, 10, 0)
	w := newMeterWorld(t, env, "0812851")
	mawar, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kontrakan Mawar", Kind: property.KindKontrakan})
	require.NoError(t, err)
	karyawan := env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	supervisor := env.Principal(t, w.org.ID, w.org.Supervisor.UserID)
	own := w.rentCollected(t, w.kos, "Rina Wulandari", day(2026, 10, 1), karyawan.MembershipID)
	notOwn := w.rentCollected(t, w.kos, "Bambang Wijaya", day(2026, 10, 1), uuid.Nil)
	inMawar := w.rentCollected(t, mawar, "Dodi Saputra", day(2026, 10, 1), uuid.Nil)
	for _, r := range []rental{own, notOwn, inMawar} {
		w.expect(t, r.leaseID, w.listrik)
	}
	oct := day(2026, 10, 1)
	record := func(p authz.Principal, r rental) error {
		_, err := env.Billing.RecordMeteredCharge(ctx, p, r.leaseID, w.listrik.ID, oct, billing.MeteredInput{Amount: 100_000})
		return err
	}
	read := func(p authz.Principal, r rental) error {
		_, err := env.Billing.ListLeaseMeteredCharges(ctx, p, r.leaseID, billing.MeteredQuery{})
		return err
	}

	t.Run("the karyawan records and reads only for the tenants they collect from", func(t *testing.T) {
		require.NoError(t, record(karyawan, own))
		require.ErrorIs(t, record(karyawan, notOwn), authz.ErrForbidden)
		require.NoError(t, read(karyawan, own))
		require.ErrorIs(t, read(karyawan, notOwn), authz.ErrForbidden)
		require.Equal(t, []string{}, missingNames(w.missing(t, karyawan, billing.MissingQuery{})),
			"their tenant's October reading is recorded")
	})

	t.Run("the Supervisor reads but never records, and the refusal is audited", func(t *testing.T) {
		before := w.audited(t, authz.ActionChargeRecord, "deny")
		require.ErrorIs(t, record(supervisor, notOwn), authz.ErrForbidden)
		require.Equal(t, before+1, w.audited(t, authz.ActionChargeRecord, "deny"))
		require.NoError(t, read(supervisor, notOwn))
		require.Equal(t, []string{"Bambang Wijaya [Listrik]", "Dodi Saputra [Listrik]"},
			missingNames(w.missing(t, supervisor, billing.MissingQuery{})))
	})

	t.Run("an admin's amount is audited as allowed", func(t *testing.T) {
		before := w.audited(t, authz.ActionChargeRecord, "allow")
		require.NoError(t, record(w.admin, notOwn))
		require.Equal(t, before+1, w.audited(t, authz.ActionChargeRecord, "allow"))
		row := env.AuditLog(t, w.org.ID)[0]
		require.Equal(t, authz.ActionChargeRecord, row.Action)
		require.Equal(t, "property/"+w.kos.ID.String()+"/lease/"+notOwn.leaseID.String(), row.Resource)
	})

	t.Run("an admin limited to the kos records and sees only the kos", func(t *testing.T) {
		sari, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-8510-0099", Name: "Mbak Sari",
			Roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: w.kos.ID}}})
		require.NoError(t, err)
		limited := env.Principal(t, w.org.ID, sari.UserID)
		require.NoError(t, record(limited, notOwn))
		require.ErrorIs(t, record(limited, inMawar), authz.ErrForbidden)
		require.ErrorIs(t, read(limited, inMawar), authz.ErrForbidden)
		require.Equal(t, []string{}, missingNames(w.missing(t, limited, billing.MissingQuery{})),
			"Dodi in Mawar is missing, but not visible")
	})

	t.Run("an explicit Deny on one property wins over an Allow on everything", func(t *testing.T) {
		dina, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-8510-0098", Name: "Dina Kasir",
			Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
		require.NoError(t, err)
		env.GrantCustomRole(t, w.org.ID, dina.ID, "Meteran kecuali kos", `{"Version":"2026-10-25","Statement":[`+
			`{"Effect":"Allow","Action":["charge:*"],"Resource":["*"]},`+
			`{"Effect":"Deny","Action":["charge:*"],"Resource":["property/`+w.kos.ID.String()+`/*"]}]}`)
		reader := env.Principal(t, w.org.ID, dina.UserID)
		require.ErrorIs(t, record(reader, notOwn), authz.ErrForbidden)
		require.ErrorIs(t, read(reader, notOwn), authz.ErrForbidden)
		require.Equal(t, []string{"Dodi Saputra [Listrik]"}, missingNames(w.missing(t, reader, billing.MissingQuery{})))
		require.NoError(t, record(reader, inMawar))
		require.Equal(t, []string{}, missingNames(w.missing(t, reader, billing.MissingQuery{})))
	})

	t.Run("a member without charge:read is refused the list and audited", func(t *testing.T) {
		nina, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-8510-0097", Name: "Nina Tamu",
			Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
		require.NoError(t, err)
		env.GrantCustomRole(t, w.org.ID, nina.ID, "Tanpa meteran", `{"Version":"2026-10-25","Statement":[`+
			`{"Effect":"Deny","Action":["charge:read"],"Resource":["*"]}]}`)
		before := w.audited(t, authz.ActionChargeRead, "deny")
		_, err = env.Billing.ListMissingMeterReadings(ctx, env.Principal(t, w.org.ID, nina.UserID), billing.MissingQuery{})
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, before+1, w.audited(t, authz.ActionChargeRead, "deny"))
	})
}

func TestMissingMeterReadings(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 3, 10, 0)
	w := newMeterWorld(t, env, "0812861")
	mawar, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kontrakan Mawar", Kind: property.KindKontrakan})
	require.NoError(t, err)
	rina := w.rentCollected(t, w.kos, "Rina Wulandari", day(2026, 10, 1), uuid.Nil)
	budi := w.rentCollected(t, w.kos, "Budi Santoso", day(2026, 10, 1), uuid.Nil)
	citra := w.rentCollected(t, w.kos, "Citra Ayu", day(2026, 11, 1), uuid.Nil)
	dodi := w.rentCollected(t, mawar, "Dodi Saputra", day(2026, 10, 20), uuid.Nil)
	w.generate(t)
	w.expect(t, rina.leaseID, w.listrik, w.air)
	w.expect(t, citra.leaseID, w.listrik)
	w.expect(t, dodi.leaseID, w.air)
	_ = budi // no metered type: never listed

	t.Run("a lease that expects a type with no amount is listed, with the types and its next invoice", func(t *testing.T) {
		items := w.missing(t, w.admin, billing.MissingQuery{})
		require.Equal(t, []string{"Dodi Saputra [Air]", "Rina Wulandari [Air Listrik]"}, missingNames(items),
			"last month by default; Citra started after it; Budi expects nothing")
		require.Equal(t, rina.leaseID, items[1].LeaseID)
		require.Equal(t, rina.tenantID, items[1].TenantID)
		require.Equal(t, "Kos Melati", items[1].PropertyName)
		require.Equal(t, &billing.NextInvoice{Period: day(2026, 12, 1), IssuedOn: day(2026, 11, 24)}, items[1].NextInvoice)
		require.Equal(t, &billing.NextInvoice{Period: day(2026, 11, 20), IssuedOn: day(2026, 11, 13)}, items[0].NextInvoice)
	})

	t.Run("this month lists Citra too", func(t *testing.T) {
		require.Equal(t, []string{"Citra Ayu [Listrik]", "Dodi Saputra [Air]", "Rina Wulandari [Air Listrik]"},
			missingNames(w.missing(t, w.admin, billing.MissingQuery{Month: day(2026, 11, 1)})))
	})

	t.Run("a property filter", func(t *testing.T) {
		require.Equal(t, []string{"Dodi Saputra [Air]"}, missingNames(w.missing(t, w.admin, billing.MissingQuery{PropertyID: mawar.ID})))
	})

	t.Run("an amount recorded takes its type off, and the lease once every type has one", func(t *testing.T) {
		w.meter(t, w.admin, rina.leaseID, w.listrik, day(2026, 10, 1), 120_000)
		require.Equal(t, []string{"Dodi Saputra [Air]", "Rina Wulandari [Air]"}, missingNames(w.missing(t, w.admin, billing.MissingQuery{})))
		w.meter(t, w.admin, rina.leaseID, w.air, day(2026, 10, 1), 40_000)
		require.Equal(t, []string{"Dodi Saputra [Air]"}, missingNames(w.missing(t, w.admin, billing.MissingQuery{})))
	})

	t.Run("a type switched off is not expected any more", func(t *testing.T) {
		w.expect(t, dodi.leaseID)
		res, err := env.Billing.ListMissingMeterReadings(ctx, w.admin, billing.MissingQuery{})
		require.NoError(t, err)
		require.Empty(t, res.Items)
		require.Zero(t, res.Total)
	})

	t.Run("a month after this one is refused", func(t *testing.T) {
		_, err := env.Billing.ListMissingMeterReadings(ctx, w.admin, billing.MissingQuery{Month: day(2026, 12, 1)})
		requireInput(t, err, "month")
	})

	t.Run("pages and total", func(t *testing.T) {
		w.expect(t, dodi.leaseID, w.air, w.listrik)
		res, err := env.Billing.ListMissingMeterReadings(ctx, w.admin, billing.MissingQuery{Month: day(2026, 11, 1), Page: listing.Page{Number: 2, Size: 2}})
		require.NoError(t, err)
		require.EqualValues(t, 3, res.Total)
		require.Len(t, res.Items, 1)
		require.Equal(t, "Rina Wulandari", res.Items[0].TenantName)
	})
}

func TestLeaseMeteredTypes(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 3, 10, 0)
	w := newMeterWorld(t, env, "0812871")
	r := w.rent(t, "Rina Wulandari", day(2026, 10, 1), rentOpts{})
	ids := func(cts ...tenant.ChargeType) *[]uuid.UUID {
		out := make([]uuid.UUID, len(cts))
		for i, ct := range cts {
			out[i] = ct.ID
		}
		return &out
	}
	set := func(in tenant.LeaseChargesInput) (tenant.Lease, error) {
		return env.Tenants.SetLeaseCharges(ctx, w.admin, r.leaseID, in)
	}
	wifi := []tenant.ChargeInput{{ChargeTypeID: w.wifi.ID, Amount: 50_000}}

	t.Run("a new lease expects none", func(t *testing.T) {
		require.Equal(t, []tenant.MeteredType{}, w.lease(t, r.renter).MeteredChargeTypes)
	})

	t.Run("set, by name", func(t *testing.T) {
		l, err := set(tenant.LeaseChargesInput{Charges: wifi, MeteredTypes: ids(w.listrik, w.air)})
		require.NoError(t, err)
		require.Equal(t, []tenant.MeteredType{{ChargeTypeID: w.air.ID, Name: "Air"}, {ChargeTypeID: w.listrik.ID, Name: "Listrik"}}, l.MeteredChargeTypes)
		require.Len(t, l.Charges, 1)
	})

	t.Run("left out, they stay", func(t *testing.T) {
		l, err := set(tenant.LeaseChargesInput{Charges: []tenant.ChargeInput{}})
		require.NoError(t, err)
		require.Len(t, l.MeteredChargeTypes, 2)
		require.Empty(t, l.Charges)
	})

	t.Run("replaced", func(t *testing.T) {
		l, err := set(tenant.LeaseChargesInput{Charges: wifi, MeteredTypes: ids(w.listrik)})
		require.NoError(t, err)
		require.Equal(t, []tenant.MeteredType{{ChargeTypeID: w.listrik.ID, Name: "Listrik"}}, l.MeteredChargeTypes)
		require.Equal(t, l.MeteredChargeTypes, w.lease(t, r.renter).MeteredChargeTypes, "the tenant's lease shows them too")
	})

	t.Run("refusals change nothing", func(t *testing.T) {
		other := newMeterWorld(t, env, "0812872")
		tests := []struct {
			name  string
			ids   *[]uuid.UUID
			field string
		}{
			{"a type that is not metered", ids(w.air, w.wifi), "meteredChargeTypeIds[1]"},
			{"a type that does not exist", &[]uuid.UUID{uuid.New()}, "meteredChargeTypeIds[0]"},
			{"another organization's type", ids(other.listrik), "meteredChargeTypeIds[0]"},
			{"the same type twice", ids(w.air, w.air), "meteredChargeTypeIds[1]"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := set(tenant.LeaseChargesInput{Charges: wifi, MeteredTypes: tc.ids})
				var ie *tenant.InputError
				require.ErrorAs(t, err, &ie)
				require.Equal(t, tc.field, ie.Field)
			})
		}
		require.Equal(t, []tenant.MeteredType{{ChargeTypeID: w.listrik.ID, Name: "Listrik"}}, w.lease(t, r.renter).MeteredChargeTypes)
	})

	t.Run("the Supervisor may not change them", func(t *testing.T) {
		_, err := env.Tenants.SetLeaseCharges(ctx, env.Principal(t, w.org.ID, w.org.Supervisor.UserID), r.leaseID,
			tenant.LeaseChargesInput{Charges: []tenant.ChargeInput{}, MeteredTypes: ids()})
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("an empty list removes them; recorded amounts are still billed", func(t *testing.T) {
		mc := w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 90_000)
		l, err := set(tenant.LeaseChargesInput{Charges: []tenant.ChargeInput{}, MeteredTypes: ids()})
		require.NoError(t, err)
		require.Equal(t, []tenant.MeteredType{}, l.MeteredChargeTypes)
		w.generate(t) // October and November at once (entered late): the amount goes on November
		setNow(env, 2026, 11, 24, 5, 0)
		w.generate(t)
		lines := w.meteredLines(t, r.renter)
		require.Len(t, lines, 1)
		require.Equal(t, mc.ID, lines[0].MeteredChargeID.UUID)
	})
}

// TestMeteredRacesGeneration proves an edit and generation on the same lease never leave a line
// whose amount differs from the stored amount: both lock the lease row first.
func TestMeteredRacesGeneration(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	t.Run("an edit waiting on generation is refused, the line bills what was stored", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newMeterWorld(t, env, "0812881")
		r := w.rent(t, "Rina Wulandari", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		mc := w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 100_000)
		setNow(env, 2026, 11, 10, 9, 0)

		done := make(chan error, 1)
		err := env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
			n, err := env.Billing.GenerateLease(ctx, tx, w.org.ID, r.leaseID, day(2026, 11, 10))
			require.NoError(t, err)
			require.Equal(t, 1, n)
			go func() {
				_, err := env.Billing.RecordMeteredCharge(ctx, w.admin, r.leaseID, w.listrik.ID, day(2026, 10, 1), billing.MeteredInput{Amount: 150_000})
				done <- err
			}()
			testenv.WaitUntilBlockedOn(ctx, t, tx)
			return nil
		})
		require.NoError(t, err)
		require.ErrorIs(t, <-done, billing.ErrMeteredChargeBilled)
		lines := w.meteredLines(t, r.renter)
		require.Len(t, lines, 1)
		require.EqualValues(t, 100_000, lines[0].Amount)
		require.EqualValues(t, 100_000, w.storedAmount(t, mc.ID))
	})

	t.Run("generation waiting on an edit bills the edited amount", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newMeterWorld(t, env, "0812882")
		r := w.rent(t, "Dewi Lestari", day(2026, 10, 15), rentOpts{})
		w.generate(t)
		mc := w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 100_000)
		setNow(env, 2026, 11, 10, 9, 0)

		done := make(chan error, 1)
		// The statements RecordMeteredCharge runs: lock the lease, lock the row, change it.
		err := env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
			q := dbgen.New(tx)
			if _, err := q.LockLeaseForBilling(ctx, dbgen.LockLeaseForBillingParams{OrganizationID: w.org.ID, ID: r.leaseID}); err != nil {
				return err
			}
			if _, err := q.LockMeteredCharge(ctx, dbgen.LockMeteredChargeParams{
				OrganizationID: w.org.ID, LeaseID: r.leaseID, ChargeTypeID: w.listrik.ID, UsageMonth: day(2026, 10, 1),
			}); err != nil {
				return err
			}
			if _, err := q.UpdateMeteredCharge(ctx, dbgen.UpdateMeteredChargeParams{
				OrganizationID: w.org.ID, ID: mc.ID, Amount: 150_000, RecordedByStaffUserID: w.org.Admin.UserID, Now: env.Clock.Now(),
			}); err != nil {
				return err
			}
			go func() {
				_, err := env.Billing.GenerateOrg(ctx, w.org.ID)
				done <- err
			}()
			testenv.WaitUntilBlockedOn(ctx, t, tx)
			return nil
		})
		require.NoError(t, err)
		require.NoError(t, <-done)
		lines := w.meteredLines(t, r.renter)
		require.Len(t, lines, 1)
		require.EqualValues(t, 150_000, lines[0].Amount)
	})

	t.Run("concurrent edits and generation never disagree or deadlock", func(t *testing.T) {
		setNow(env, 2026, 10, 15, 9, 0)
		w := newMeterWorld(t, env, "0812883")
		const rounds = 6
		leases := make([]rental, rounds)
		for i := range leases {
			leases[i] = w.rent(t, fmt.Sprintf("Penyewa %d", i), day(2026, 10, 15), rentOpts{})
		}
		w.generate(t)
		for _, r := range leases {
			w.meter(t, w.admin, r.leaseID, w.listrik, day(2026, 10, 1), 100_000)
		}
		setNow(env, 2026, 11, 10, 9, 0)

		for i, r := range leases {
			var wg sync.WaitGroup
			errs := make(chan error, 3)
			run := func(fn func() error) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					errs <- fn()
				}()
			}
			for _, amount := range []int64{150_000, 175_000} {
				run(func() error {
					_, err := env.Billing.RecordMeteredCharge(ctx, w.admin, r.leaseID, w.listrik.ID, day(2026, 10, 1), billing.MeteredInput{Amount: amount})
					if errors.Is(err, billing.ErrMeteredChargeBilled) {
						return nil
					}
					return err
				})
			}
			run(func() error {
				return env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
					_, err := env.Billing.GenerateLease(ctx, tx, w.org.ID, r.leaseID, day(2026, 11, 10))
					return err
				})
			})
			wg.Wait()
			close(errs)
			for err := range errs {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) {
					require.NotEqual(t, "40P01", pgErr.Code, "round %d deadlocked: %v", i, err)
				}
				require.NoError(t, err, "round %d", i)
			}
			lines := w.meteredLines(t, r.renter)
			require.Len(t, lines, 1, "round %d", i)
			require.Equal(t, w.storedAmount(t, lines[0].MeteredChargeID.UUID), lines[0].Amount, "round %d", i)
		}
	})
}

// TestMeteredIsolation checks the two new tables the way TestMoneyIsolation checks the money
// tables: an organization sees only its own rows and cannot write rows naming another.
func TestMeteredIsolation(t *testing.T) {
	env := testenv.New(t)
	setNow(env, 2026, 11, 3, 10, 0)
	a := newMeterWorld(t, env, "0812891")
	b := newMeterWorld(t, env, "0812892")
	ra := a.rent(t, "Rina Wulandari", day(2026, 10, 1), rentOpts{})
	rb := b.rent(t, "Budi Santoso", day(2026, 10, 1), rentOpts{})
	for _, x := range []struct {
		w meterWorld
		r rental
	}{{a, ra}, {b, rb}} {
		x.w.expect(t, x.r.leaseID, x.w.listrik)
		x.w.meter(t, x.w.admin, x.r.leaseID, x.w.listrik, day(2026, 10, 1), 100_000)
	}
	tables := []string{"lease_metered_type", "metered_charge"}

	t.Run("each organization sees only its own rows", func(t *testing.T) {
		for _, w := range []meterWorld{a, b} {
			for _, table := range tables {
				var all, foreign int
				w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
					return tx.QueryRow(ctx, fmt.Sprintf(
						"SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM %s", table,
					)).Scan(&all, &foreign)
				})
				require.Equal(t, 1, all, table)
				require.Zero(t, foreign, table)
			}
		}
	})

	t.Run("no row can be written for another organization", func(t *testing.T) {
		inserts := map[string]func(ctx context.Context, q *dbgen.Queries) error{
			"lease_metered_type": func(ctx context.Context, q *dbgen.Queries) error {
				return q.AddLeaseMeteredType(ctx, dbgen.AddLeaseMeteredTypeParams{
					ID: uuid.New(), OrganizationID: b.org.ID, LeaseID: rb.leaseID, ChargeTypeID: b.air.ID,
				})
			},
			"metered_charge": func(ctx context.Context, q *dbgen.Queries) error {
				return q.CreateMeteredCharge(ctx, dbgen.CreateMeteredChargeParams{
					ID: uuid.New(), OrganizationID: b.org.ID, LeaseID: rb.leaseID, ChargeTypeID: b.air.ID,
					UsageMonth: day(2026, 10, 1), Amount: 1, RecordedByStaffUserID: b.org.Admin.UserID, Now: env.Clock.Now(),
				})
			},
		}
		for _, table := range tables {
			t.Run(table, func(t *testing.T) {
				err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error { return inserts[table](ctx, q) })
				pgErr := requireCode(t, err, "42501")
				require.Contains(t, pgErr.Message, "row-level security")
			})
		}
	})

	t.Run("a row cannot point into another organization", func(t *testing.T) {
		err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			return q.CreateMeteredCharge(ctx, dbgen.CreateMeteredChargeParams{
				ID: uuid.New(), OrganizationID: a.org.ID, LeaseID: rb.leaseID, ChargeTypeID: a.air.ID,
				UsageMonth: day(2026, 10, 1), Amount: 1, RecordedByStaffUserID: a.org.Admin.UserID, Now: env.Clock.Now(),
			})
		})
		requireCode(t, err, "23503")
	})

	t.Run("generation in one organization bills only its own amounts", func(t *testing.T) {
		setNow(env, 2026, 11, 3, 11, 0)
		a.generate(t)
		require.Len(t, a.meteredLines(t, ra.renter), 1)
		require.Empty(t, b.meteredLines(t, rb.renter))
	})
}
