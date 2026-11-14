//go:build integration

package billing_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// receiptQueue is a fake billing.ReceiptQueue that records the payments it was given.
type receiptQueue struct {
	mu  sync.Mutex
	ids []uuid.UUID
}

func (q *receiptQueue) EnqueueReceipt(_ context.Context, _, paymentID uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids = append(q.ids, paymentID)
	return nil
}

func (q *receiptQueue) enqueued() []uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]uuid.UUID(nil), q.ids...)
}

// fieldWorld is a world whose billing service enqueues receipts, with the org's karyawan and a
// second karyawan, Dina.
type fieldWorld struct {
	*world
	billing  *billing.Service
	queue    *receiptQueue
	karyawan authz.Principal
	dina     authz.Principal
}

func newFieldWorld(t *testing.T, env *testenv.Env, prefix, dinaPhone string) fieldWorld {
	t.Helper()
	w := fieldWorld{world: newWorld(t, env, prefix), queue: &receiptQueue{}}
	w.billing = billing.NewService(env.Store, env.Authz, env.Clock, env.Log, env.Promises, billing.SendReceipts(w.queue))
	w.karyawan = env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	dina, err := env.Members.Invite(context.Background(), w.admin, member.Invite{Phone: dinaPhone, Name: "Dina Kasir",
		Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)
	w.dina = env.Principal(t, w.org.ID, dina.UserID)
	return w
}

// collected is a tenant renting a new unit in the world's kos from start, at Rp700.000 a month,
// collected by the member collector (uuid.Nil for none). The lease is saved now (the fake clock).
func (w fieldWorld) collected(t *testing.T, name string, start time.Time, collector uuid.UUID) rental {
	t.Helper()
	return meterWorld{world: w.world}.rentCollected(t, w.kos, name, start, collector)
}

// cashIn is cash of amount from r with a fresh requestId.
func cashIn(r rental, amount int64) billing.CashInput {
	return billing.CashInput{RequestID: uuid.New(), TenantID: r.tenantID, Amount: amount}
}

// receiptRows are the organization's receipts by payment.
func (w fieldWorld) receiptRows(t *testing.T) map[uuid.UUID]dbgen.PaymentReceipt {
	t.Helper()
	out := map[uuid.UUID]dbgen.PaymentReceipt{}
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		rows, err := tx.Query(ctx, "SELECT payment_id, organization_id, tenant_id, status, sent_at, created_at FROM payment_receipt")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r dbgen.PaymentReceipt
			if err := rows.Scan(&r.PaymentID, &r.OrganizationID, &r.TenantID, &r.Status, &r.SentAt, &r.CreatedAt); err != nil {
				return err
			}
			out[r.PaymentID] = r
		}
		return rows.Err()
	})
	return out
}

func TestRecordCashPayment(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 3, 10, 0)
	w := newFieldWorld(t, env, "0812861", "0812-8610-0099")
	own := w.collected(t, "Wati Susanti", day(2026, 11, 3), w.karyawan.MembershipID)
	dinas := w.collected(t, "Bambang Wijaya", day(2026, 11, 3), w.dina.MembershipID)
	nobodys := w.collected(t, "Dodi Saputra", day(2026, 11, 3), uuid.Nil)
	w.generate(t)
	supervisor := env.Principal(t, w.org.ID, w.org.Supervisor.UserID)

	t.Run("a karyawan records cash only for a tenant they collect from, and refusals are audited", func(t *testing.T) {
		allowed := w.audited(t, authz.ActionCashRecord, "allow")
		denied := w.audited(t, authz.ActionCashRecord, "deny")
		payments := w.count(t, "payment")

		res, err := w.billing.RecordCashPayment(ctx, w.karyawan, cashIn(own, 300_000))
		require.NoError(t, err)
		require.True(t, res.Created)
		require.Equal(t, billing.ReceiptPending, res.ReceiptStatus)
		require.Equal(t, billing.MethodCash, res.Payment.Method)
		require.Equal(t, day(2026, 11, 3), res.Payment.ReceivedOn)
		require.NotNil(t, res.Payment.RecordedBy)
		require.Equal(t, w.karyawan.MembershipID, res.Payment.RecordedBy.MemberID)
		require.Len(t, res.Paid, 1)
		require.EqualValues(t, 300_000, res.Paid[0].Amount)
		require.EqualValues(t, 400_000, res.Paid[0].Outstanding)
		require.EqualValues(t, 400_000, res.Outstanding)
		require.Equal(t, allowed+1, w.audited(t, authz.ActionCashRecord, "allow"))

		for name, c := range map[string]struct {
			p authz.Principal
			r rental
		}{
			"another karyawan's tenant": {w.karyawan, dinas},
			"a tenant nobody collects":  {w.karyawan, nobodys},
			"Dina on the karyawan's":    {w.dina, own},
			"the Supervisor":            {supervisor, own},
			"an admin (recordPayment)":  {w.admin, own},
		} {
			_, err := w.billing.RecordCashPayment(ctx, c.p, cashIn(c.r, 100_000))
			require.ErrorIs(t, err, authz.ErrForbidden, name)
		}
		require.Equal(t, denied+5, w.audited(t, authz.ActionCashRecord, "deny"))
		require.Equal(t, payments+1, w.count(t, "payment"), "a refused request records nothing")

		var stored dbgen.Payment
		w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			var err error
			stored, err = q.GetPayment(ctx, dbgen.GetPaymentParams{OrganizationID: w.org.ID, ID: res.Payment.ID})
			return err
		})
		require.Equal(t, billing.SourceManual, stored.Source)
		require.True(t, stored.ReceivedAt.Equal(env.Clock.Now()), "cash is received now, not at midnight")
		require.Equal(t, w.org.Karyawan.UserID, stored.RecordedByStaffUserID.UUID)
	})

	t.Run("the receipt is recorded with the payment and enqueued once it commits", func(t *testing.T) {
		before := len(w.queue.enqueued())
		res, err := w.billing.RecordCashPayment(ctx, w.karyawan, cashIn(own, 50_000))
		require.NoError(t, err)
		rows := w.receiptRows(t)
		row, ok := rows[res.Payment.ID]
		require.True(t, ok, "the payment's receipt row exists as soon as it commits")
		require.Equal(t, billing.ReceiptPending, row.Status)
		require.Nil(t, row.SentAt)
		require.Equal(t, own.tenantID, row.TenantID)
		require.True(t, row.CreatedAt.Equal(env.Clock.Now()))
		require.Equal(t, append([]uuid.UUID{}, res.Payment.ID), w.queue.enqueued()[before:])

		var cashRows, withoutReceipt int
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			return tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE NOT EXISTS (
				SELECT 1 FROM payment_receipt r WHERE r.payment_id = p.id))
				FROM payment p WHERE p.method = 'cash' AND p.recorded_by_staff_user_id = $1`, w.org.Karyawan.UserID).Scan(&cashRows, &withoutReceipt)
		})
		require.Equal(t, 2, cashRows)
		require.Zero(t, withoutReceipt, "every cash payment the karyawan recorded has its receipt")
	})

	t.Run("an admin's recordPayment of cash sends no receipt", func(t *testing.T) {
		receipts := len(w.receiptRows(t))
		w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: dinas.tenantID, Amount: 10_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 3)})
		require.Len(t, w.receiptRows(t), receipts)
	})

	t.Run("the same requestId records once and answers the receipt's status now", func(t *testing.T) {
		in := cashIn(own, 20_000)
		first, err := w.billing.RecordCashPayment(ctx, w.karyawan, in)
		require.NoError(t, err)
		require.True(t, first.Created)
		payments := w.count(t, "payment")
		allowed := w.audited(t, authz.ActionCashRecord, "allow")

		again, err := w.billing.RecordCashPayment(ctx, w.karyawan, in)
		require.NoError(t, err)
		require.False(t, again.Created)
		require.Equal(t, first.Payment.ID, again.Payment.ID)
		require.Equal(t, billing.ReceiptPending, again.ReceiptStatus)
		require.Equal(t, payments, w.count(t, "payment"))
		require.Equal(t, allowed, w.audited(t, authz.ActionCashRecord, "allow"), "a repeat audits nothing")

		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			_, err := tx.Exec(ctx, "UPDATE payment_receipt SET status = 'sent', sent_at = now() WHERE payment_id = $1", first.Payment.ID)
			return err
		})
		setNow(env, 2026, 11, 4, 8, 0)
		later, err := w.billing.RecordCashPayment(ctx, w.karyawan, in)
		require.NoError(t, err, "a retry the next day is still the same cash")
		require.False(t, later.Created)
		require.Equal(t, billing.ReceiptSent, later.ReceiptStatus)
		setNow(env, 2026, 11, 3, 10, 0)

		changed := in
		changed.Amount = 25_000
		_, err = w.billing.RecordCashPayment(ctx, w.karyawan, changed)
		require.ErrorIs(t, err, billing.ErrRequestIDReused)

		viaAdmin := billing.PaymentInput{RequestID: uuid.New(), TenantID: own.tenantID, Amount: 5_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 3)}
		w.record(t, viaAdmin)
		_, err = w.billing.RecordCashPayment(ctx, w.admin, billing.CashInput{RequestID: viaAdmin.RequestID, TenantID: own.tenantID, Amount: 5_000})
		require.ErrorIs(t, err, authz.ErrForbidden, "the admin has no cash:record")
		require.Equal(t, payments, w.count(t, "payment")-1)
	})

	t.Run("bad input names its field", func(t *testing.T) {
		for field, in := range map[string]billing.CashInput{
			"requestId": {TenantID: own.tenantID, Amount: 1},
			"amount":    {RequestID: uuid.New(), TenantID: own.tenantID},
			"note":      {RequestID: uuid.New(), TenantID: own.tenantID, Amount: 1, Note: string(make([]rune, 501))},
		} {
			_, err := w.billing.RecordCashPayment(ctx, w.karyawan, in)
			requireInput(t, err, field)
		}
		_, err := w.billing.RecordCashPayment(ctx, w.karyawan, billing.CashInput{RequestID: uuid.New(), TenantID: uuid.New(), Amount: 1})
		requireNotFound(t, err, "")
	})
}

// TestCashPaysFIFOWithCredit: cash collected in the field pays the oldest invoice first, across
// invoices, and the rest is credit, exactly as recordPayment.
func TestCashPaysFIFOWithCredit(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 10, 28, 10, 0)
	w := newFieldWorld(t, env, "0812862", "0812-8620-0099")
	r := w.collected(t, "Siti Rahayu", day(2026, 10, 28), w.karyawan.MembershipID)
	w.generate(t)
	setNow(env, 2026, 11, 21, 10, 0)
	w.generate(t)
	invs := w.invoices(t, r.renter)
	require.Len(t, invs, 2)

	res, err := w.billing.RecordCashPayment(ctx, w.karyawan, cashIn(r, 1_500_000))
	require.NoError(t, err)
	require.Equal(t, []billing.PaidInvoice{
		{InvoiceID: invs[0].InvoiceID, Kind: billing.InvoiceRegular, Period: day(2026, 10, 28), PeriodEnd: day(2026, 11, 27), DueDate: day(2026, 10, 28), Amount: 700_000},
		{InvoiceID: invs[1].InvoiceID, Kind: billing.InvoiceRegular, Period: day(2026, 11, 28), PeriodEnd: day(2026, 12, 27), DueDate: day(2026, 11, 28), Amount: 700_000},
	}, res.Paid)
	require.Zero(t, res.Outstanding)
	require.EqualValues(t, 100_000, res.Credit)
	require.Equal(t, []string{"paid", "paid"}, []string{w.invoices(t, r.renter)[0].Status, w.invoices(t, r.renter)[1].Status})
}

func TestFieldToday(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 3, 9, 0)
	w := newFieldWorld(t, env, "0812863", "0812-8630-0099")
	mine := w.karyawan.MembershipID
	mawar, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kontrakan Mawar", Kind: property.KindKontrakan, Address: "Jl. Mawar No. 4, Bekasi"})
	require.NoError(t, err)

	// Each lease is saved on its start day or before, so its first invoice is due on its start day.
	oldest := w.collected(t, "Agus Oldest", day(2026, 11, 3), mine) // due 3 Nov
	setNow(env, 2026, 11, 5, 9, 0)
	older := meterWorld{world: w.world}.rentCollected(t, mawar, "Budi Older", day(2026, 11, 5), mine) // due 5 Nov
	w.generate(t)
	setNow(env, 2026, 11, 14, 9, 0)
	today := w.collected(t, "Citra Today", day(2026, 11, 14), mine)
	tomorrow := w.collected(t, "Dewi Tomorrow", day(2026, 11, 15), mine)
	inTwo := w.collected(t, "Eka InTwo", day(2026, 11, 16), mine)
	w.collected(t, "Fajar InThree", day(2026, 11, 17), mine)
	paid := w.collected(t, "Gita Paid", day(2026, 11, 14), mine)
	dinas := w.collected(t, "Hadi Dinas", day(2026, 11, 14), w.dina.MembershipID)
	shared := w.collected(t, "Indra Shared", day(2026, 11, 14), mine)
	adminOwn := w.collected(t, "Joko Admins", day(2026, 11, 14), w.admin.MembershipID)
	ahead := w.collected(t, "Kiki Ahead", day(2026, 11, 19), mine)      // due in 5 days, pays part in cash today
	paidCash := w.collected(t, "Lia PaidCash", day(2026, 11, 14), mine) // pays everything in cash today
	adminCash := w.collected(t, "Mira AdminCash", day(2026, 11, 19), mine)
	// Indra also rents a second unit that Dina collects: Mas Dimas may not see all of Indra's
	// money, so Indra is not on his list.
	unit, err := env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{Label: "Kamar 99", Terms: testenv.Monthly(500_000)})
	require.NoError(t, err)
	_, err = env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: shared.tenantID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: day(2026, 11, 14), CollectorMemberID: w.dina.MembershipID}})
	require.NoError(t, err)
	w.generate(t)
	w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: paid.tenantID, Amount: 700_000, Method: billing.MethodTransfer, ReceivedOn: day(2026, 11, 14)})
	// Cash an admin records is the admin's, not the karyawan's.
	w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: adminCash.tenantID, Amount: 100_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 14)})
	citra1, err := w.billing.RecordCashPayment(ctx, w.karyawan, cashIn(today, 200_000))
	require.NoError(t, err)
	citra2, err := w.billing.RecordCashPayment(ctx, w.karyawan, cashIn(today, 100_000))
	require.NoError(t, err)
	_, err = w.billing.RecordCashPayment(ctx, w.karyawan, cashIn(ahead, 100_000))
	require.NoError(t, err)
	lia, err := w.billing.RecordCashPayment(ctx, w.karyawan, cashIn(paidCash, 700_000))
	require.NoError(t, err)
	_, err = w.billing.RecordCashPayment(ctx, w.dina, cashIn(dinas, 700_000))
	require.NoError(t, err)

	all := func(p authz.Principal) []billing.FieldTenant {
		t.Helper()
		items, err := listing.All(func(page listing.Page) (listing.Result[billing.FieldTenant], error) {
			return w.billing.FieldToday(ctx, p, page)
		})
		require.NoError(t, err)
		return items
	}
	names := func(items []billing.FieldTenant) []string {
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = it.TenantName
		}
		return out
	}
	noCash := billing.FieldCash{Payments: []billing.FieldCashPayment{}}
	cashOf := func(payments ...billing.CashResult) billing.FieldCash {
		out := billing.FieldCash{Payments: []billing.FieldCashPayment{}}
		for _, p := range payments {
			out.Amount += p.Payment.Net
			out.Count++
			out.Payments = append(out.Payments, billing.FieldCashPayment{PaymentID: p.Payment.ID, Amount: p.Payment.Net, RecordedAt: p.Payment.CreatedAt})
		}
		return out
	}

	t.Run("due rows oldest first, then the tenants paid in cash today by name", func(t *testing.T) {
		items := all(w.karyawan)
		require.Equal(t, []string{"Agus Oldest", "Budi Older", "Citra Today", "Dewi Tomorrow", "Eka InTwo", "Kiki Ahead", "Lia PaidCash"}, names(items),
			"not due in three days, not fully paid by transfer, not another karyawan's, not a tenant with a lease he does not collect, "+
				"not a tenant whose cash today the admin recorded")
		require.Equal(t, billing.FieldReason{Code: billing.FieldOverdue, Days: 11}, items[0].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldOverdue, Days: 9}, items[1].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueToday}, items[2].Reason, "a part payment in cash keeps the due reason")
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueInDays, Days: 1}, items[3].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueInDays, Days: 2}, items[4].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldPaidToday}, items[5].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldPaidToday}, items[6].Reason)

		require.Equal(t, oldest.tenantID, items[0].TenantID)
		require.Equal(t, ptrTo(day(2026, 11, 3)), items[0].OldestUnpaidDueDate)
		require.EqualValues(t, 700_000, items[0].Outstanding)
		require.EqualValues(t, 700_000, items[0].Overdue)
		require.Equal(t, noCash, items[0].CashToday)
		require.Equal(t, []billing.FieldUnit{{UnitLabel: "Kamar 2", PropertyName: "Kontrakan Mawar", PropertyAddress: "Jl. Mawar No. 4, Bekasi"}}, items[1].Units)
		require.Equal(t, older.tenantID, items[1].TenantID)

		require.Equal(t, today.tenantID, items[2].TenantID)
		require.EqualValues(t, 400_000, items[2].Outstanding)
		require.Zero(t, items[2].Overdue, "due today is not debt yet")
		require.Equal(t, cashOf(citra1, citra2), items[2].CashToday, "each payment, oldest first")
		require.Equal(t, tomorrow.tenantID, items[3].TenantID)
		require.Equal(t, inTwo.tenantID, items[4].TenantID)

		require.Equal(t, ahead.tenantID, items[5].TenantID)
		require.Equal(t, ptrTo(day(2026, 11, 19)), items[5].OldestUnpaidDueDate, "still owes, after the window")
		require.EqualValues(t, 600_000, items[5].Outstanding)
		require.Equal(t, paidCash.tenantID, items[6].TenantID)
		require.Nil(t, items[6].OldestUnpaidDueDate, "owes nothing")
		require.Zero(t, items[6].Outstanding)
		require.Equal(t, cashOf(lia), items[6].CashToday)
	})

	t.Run("pages never overlap and total counts both kinds", func(t *testing.T) {
		var got []string
		for n := 1; n <= 4; n++ {
			res, err := w.billing.FieldToday(ctx, w.karyawan, listing.Page{Number: n, Size: 2})
			require.NoError(t, err)
			require.EqualValues(t, 7, res.Total)
			got = append(got, names(res.Items)...)
		}
		require.Equal(t, []string{"Agus Oldest", "Budi Older", "Citra Today", "Dewi Tomorrow", "Eka InTwo", "Kiki Ahead", "Lia PaidCash"}, got)
	})

	t.Run("each member sees only the tenants they collect, an admin included", func(t *testing.T) {
		dina := all(w.dina)
		require.Equal(t, []string{"Hadi Dinas"}, names(dina), "Indra's other lease is Mas Dimas's")
		require.Equal(t, billing.FieldReason{Code: billing.FieldPaidToday}, dina[0].Reason, "Dina took all of Hadi's rent in cash")
		admin := all(w.admin)
		require.Equal(t, []string{"Joko Admins"}, names(admin), "the admin's cash from Mira does not list her: he does not collect from her")
		require.Equal(t, adminOwn.tenantID, admin[0].TenantID)
		supervisor := env.Principal(t, w.org.ID, w.org.Supervisor.UserID)
		require.Empty(t, all(supervisor), "the Supervisor collects from nobody")
	})

	t.Run("the next day the Jakarta midnight moves every reason by one and drops yesterday's cash", func(t *testing.T) {
		setNow(env, 2026, 11, 15, 0, 0)
		t.Cleanup(func() { setNow(env, 2026, 11, 14, 9, 0) })
		items := all(w.karyawan)
		require.Equal(t, []string{"Agus Oldest", "Budi Older", "Citra Today", "Dewi Tomorrow", "Eka InTwo", "Fajar InThree"}, names(items),
			"Kiki and Lia were on yesterday's list only")
		require.Equal(t, billing.FieldReason{Code: billing.FieldOverdue, Days: 1}, items[2].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueToday}, items[3].Reason)
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueInDays, Days: 2}, items[5].Reason)
		require.Equal(t, noCash, items[2].CashToday, "yesterday's cash is not today's")
	})

	t.Run("a member without invoice:read is refused and audited", func(t *testing.T) {
		nina, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-8630-0098", Name: "Nina Tamu",
			Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
		require.NoError(t, err)
		env.GrantCustomRole(t, w.org.ID, nina.ID, "Tanpa tagihan", `{"Version":"2026-10-25","Statement":[`+
			`{"Effect":"Deny","Action":["invoice:read"],"Resource":["*"]}]}`)
		before := w.audited(t, authz.ActionInvoiceRead, "deny")
		_, err = w.billing.FieldToday(ctx, env.Principal(t, w.org.ID, nina.UserID), listing.FirstPage)
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, before+1, w.audited(t, authz.ActionInvoiceRead, "deny"))
	})
}

func TestFieldCash(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 14, 9, 0)
	w := newFieldWorld(t, env, "0812864", "0812-8640-0099")
	a := w.collected(t, "Wati Susanti", day(2026, 11, 14), w.karyawan.MembershipID)
	b := w.collected(t, "Bambang Wijaya", day(2026, 11, 14), w.dina.MembershipID)
	w.generate(t)

	cash := func(p authz.Principal) billing.CashTotal {
		t.Helper()
		got, err := w.billing.FieldCash(ctx, p)
		require.NoError(t, err)
		return got
	}
	require.Equal(t, billing.CashTotal{}, cash(w.karyawan))

	first, err := w.billing.RecordCashPayment(ctx, w.karyawan, cashIn(a, 300_000))
	require.NoError(t, err)
	setNow(env, 2026, 11, 15, 9, 0)
	_, err = w.billing.RecordCashPayment(ctx, w.karyawan, cashIn(a, 200_000))
	require.NoError(t, err)
	_, err = w.billing.RecordCashPayment(ctx, w.dina, cashIn(b, 700_000))
	require.NoError(t, err)
	w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: a.tenantID, Amount: 50_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 15)})

	t.Run("only the caller's own cash, across days", func(t *testing.T) {
		require.Equal(t, billing.CashTotal{Amount: 500_000, Count: 2}, cash(w.karyawan))
		require.Equal(t, billing.CashTotal{Amount: 700_000, Count: 1}, cash(w.dina))
	})

	t.Run("net of a correction to the payment", func(t *testing.T) {
		w.correct(t, billing.CorrectionInput{RequestID: uuid.New(), PaymentID: first.Payment.ID, AmountDelta: ptrTo(int64(-100_000)), Reason: "uang palsu dikembalikan"})
		require.Equal(t, billing.CashTotal{Amount: 400_000, Count: 2}, cash(w.karyawan))
	})

	t.Run("an admin and the Supervisor have no cash of their own to hand over", func(t *testing.T) {
		before := w.audited(t, authz.ActionCashDeposit, "deny")
		_, err := w.billing.FieldCash(ctx, w.admin)
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = w.billing.FieldCash(ctx, env.Principal(t, w.org.ID, w.org.Supervisor.UserID))
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, before+2, w.audited(t, authz.ActionCashDeposit, "deny"))
		allowed := w.audited(t, authz.ActionCashDeposit, "allow")
		cash(w.karyawan)
		require.Equal(t, allowed, w.audited(t, authz.ActionCashDeposit, "allow"), "reading one's cash changes nothing and is not audited")
	})
}

// TestCashCollectionIsolation: payment_receipt is scoped to its organization like every business
// table, and it changes only forward.
func TestCashCollectionIsolation(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 14, 9, 0)
	a := newFieldWorld(t, env, "0812865", "0812-8650-0099")
	b := newFieldWorld(t, env, "0812866", "0812-8660-0099")
	ra := a.collected(t, "Rina Wulandari", day(2026, 11, 14), a.karyawan.MembershipID)
	rb := b.collected(t, "Budi Santoso", day(2026, 11, 14), b.karyawan.MembershipID)
	a.generate(t)
	b.generate(t)
	pa, err := a.billing.RecordCashPayment(ctx, a.karyawan, cashIn(ra, 100_000))
	require.NoError(t, err)
	pb, err := b.billing.RecordCashPayment(ctx, b.karyawan, cashIn(rb, 100_000))
	require.NoError(t, err)

	t.Run("each organization sees only its own receipts", func(t *testing.T) {
		for _, w := range []fieldWorld{a, b} {
			var all, foreign int
			w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				return tx.QueryRow(ctx, "SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM payment_receipt").Scan(&all, &foreign)
			})
			require.Equal(t, 1, all)
			require.Zero(t, foreign)
		}
		a.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			_, err := q.GetPaymentReceiptStatus(ctx, dbgen.GetPaymentReceiptStatusParams{OrganizationID: b.org.ID, PaymentID: pb.Payment.ID})
			require.ErrorIs(t, err, pgx.ErrNoRows)
			return nil
		})
	})

	t.Run("another organization's tenant is not found and its cash not counted", func(t *testing.T) {
		_, err := a.billing.RecordCashPayment(ctx, a.karyawan, cashIn(rb, 1_000))
		requireNotFound(t, err, "")
		items, err := a.billing.FieldToday(ctx, a.karyawan, listing.FirstPage)
		require.NoError(t, err)
		require.Len(t, items.Items, 1)
		require.Equal(t, ra.tenantID, items.Items[0].TenantID)
		got, err := a.billing.FieldCash(ctx, a.karyawan)
		require.NoError(t, err)
		require.Equal(t, billing.CashTotal{Amount: 100_000, Count: 1}, got)
	})

	t.Run("no receipt can be written for another organization", func(t *testing.T) {
		err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			return q.InsertPaymentReceipt(ctx, dbgen.InsertPaymentReceiptParams{
				PaymentID: pb.Payment.ID, OrganizationID: b.org.ID, TenantID: rb.tenantID, CreatedAt: env.Clock.Now(),
			})
		})
		require.Contains(t, requireCode(t, err, "42501").Message, "row-level security")
	})

	t.Run("a receipt only moves forward and is never deleted", func(t *testing.T) {
		exec := func(stmt string) error {
			return a.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				_, err := tx.Exec(ctx, stmt, pa.Payment.ID)
				return err
			})
		}
		requireCode(t, exec("DELETE FROM payment_receipt WHERE payment_id = $1"), "42501")
		requireCode(t, exec("UPDATE payment_receipt SET created_at = now() WHERE payment_id = $1"), "42501")
		require.NoError(t, exec("UPDATE payment_receipt SET status = 'skipped' WHERE payment_id = $1"))
		pgErr := requireCode(t, exec("UPDATE payment_receipt SET status = 'pending' WHERE payment_id = $1"), "42501")
		require.Contains(t, pgErr.Message, "already skipped")
		requireCode(t, exec("UPDATE payment_receipt SET status = 'sent', sent_at = now() WHERE payment_id = $1"), "42501")
	})

	t.Run("the receipt belongs to its payment's own tenant", func(t *testing.T) {
		other := a.collected(t, "Sari Lain", day(2026, 11, 14), uuid.Nil)
		pay := a.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: ra.tenantID, Amount: 1_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 14)})
		err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			return q.InsertPaymentReceipt(ctx, dbgen.InsertPaymentReceiptParams{
				PaymentID: pay.Payment.ID, OrganizationID: a.org.ID, TenantID: other.tenantID, CreatedAt: env.Clock.Now(),
			})
		})
		requireCode(t, err, "23503")
	})
}

// ptrTo is a pointer to v.
func ptrTo[T any](v T) *T { return &v }
