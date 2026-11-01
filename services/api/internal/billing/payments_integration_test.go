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
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// cash is a cash payment input for r received on a day, with a fresh requestId.
func cash(r renter, amount int64, on time.Time) billing.PaymentInput {
	return billing.PaymentInput{RequestID: uuid.New(), TenantID: r.tenantID, Amount: amount, Method: billing.MethodCash, ReceivedOn: on}
}

// record records a payment as the world's admin.
func (w *world) record(t *testing.T, in billing.PaymentInput) billing.PaymentResult {
	t.Helper()
	res, err := w.env.Billing.RecordPayment(context.Background(), w.admin, in)
	require.NoError(t, err)
	return res
}

// correct corrects an invoice or a payment as the world's admin.
func (w *world) correct(t *testing.T, in billing.CorrectionInput) billing.CorrectionResult {
	t.Helper()
	res, err := w.env.Billing.CreateCorrection(context.Background(), w.admin, in)
	require.NoError(t, err)
	return res
}

// count counts the rows of a money table in the world's organization.
func (w *world) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		return tx.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&n)
	})
	return n
}

// audited counts the world's audit rows for one action and decision.
func (w *world) audited(t *testing.T, action, decision string) int {
	t.Helper()
	n := 0
	for _, row := range w.env.AuditLog(t, w.org.ID) {
		if row.Action == action && row.Decision == decision {
			n++
		}
	}
	return n
}

func requireNotFound(t *testing.T, err error, field string) {
	t.Helper()
	var nf *billing.NotFoundError
	require.True(t, errors.As(err, &nf), "expected not found, got %v", err)
	require.Equal(t, field, nf.Field)
}

func requireInput(t *testing.T, err error, field string) {
	t.Helper()
	var ie *billing.InputError
	require.True(t, errors.As(err, &ie), "expected an input error, got %v", err)
	require.Equal(t, field, ie.Field)
}

func TestRecordPayment(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	t.Run("an overpayment becomes credit and pays the next invoice when it is generated", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 10, 0)
		w := newWorld(t, env, "0812901")
		r := w.rent(t, "Siti Rahayu", day(2026, 10, 28), rentOpts{})
		w.generate(t)
		oct := w.invoices(t, r.renter)[0]

		res := w.record(t, cash(r.renter, 1_000_000, day(2026, 10, 28)))
		require.True(t, res.Created)
		require.Equal(t, []billing.PaidInvoice{{
			InvoiceID: oct.InvoiceID, Period: day(2026, 10, 28), PeriodEnd: day(2026, 11, 27), DueDate: day(2026, 10, 28),
			Amount: 700_000, Outstanding: 0,
		}}, res.Paid)
		require.EqualValues(t, 300_000, res.Credit)
		require.Zero(t, res.Outstanding)
		require.Nil(t, res.OldestUnpaidDueDate)
		require.Equal(t, int64(1_000_000), res.Payment.Net)
		require.Equal(t, day(2026, 10, 28), res.Payment.ReceivedOn)
		require.NotNil(t, res.Payment.RecordedBy)
		require.Equal(t, w.admin.MembershipID, res.Payment.RecordedBy.MemberID)
		require.Empty(t, res.Payment.Corrections)

		setNow(env, 2026, 11, 21, 5, 0)
		require.Equal(t, 1, w.generate(t).Invoices)
		invs := w.invoices(t, r.renter)
		require.Equal(t, "partial", invs[1].Status, "the credit paid the new invoice at once")
		require.EqualValues(t, 300_000, invs[1].Paid)
		bal := w.balance(t, r.renter)
		require.Zero(t, bal.Credit)
		require.EqualValues(t, 400_000, bal.Outstanding)
	})

	t.Run("a payment pays the oldest period first across two leases", func(t *testing.T) {
		setNow(env, 2026, 11, 2, 9, 0)
		w := newWorld(t, env, "0812902")
		first := w.rent(t, "Agus Salim", day(2026, 10, 15), rentOpts{})
		second := w.secondLease(t, first.renter, day(2026, 10, 1))
		w.generate(t)
		// Both entered late on 2 Nov: 1 Oct and 1 Nov of the second lease, 15 Oct of the first.
		invs := w.invoices(t, first.renter)
		require.Len(t, invs, 3)
		require.Equal(t, second.leaseID, invs[0].LeaseID, "1 Oct is the oldest period")
		require.Equal(t, first.leaseID, invs[1].LeaseID)

		res := w.record(t, cash(first.renter, 1_000_000, day(2026, 11, 2)))
		require.Len(t, res.Paid, 2)
		require.Equal(t, invs[0].InvoiceID, res.Paid[0].InvoiceID)
		require.EqualValues(t, 700_000, res.Paid[0].Amount)
		require.Zero(t, res.Paid[0].Outstanding)
		require.Equal(t, invs[1].InvoiceID, res.Paid[1].InvoiceID)
		require.EqualValues(t, 300_000, res.Paid[1].Amount)
		require.EqualValues(t, 400_000, res.Paid[1].Outstanding)
		require.EqualValues(t, 1_100_000, res.Outstanding)
		require.Equal(t, day(2026, 11, 9), *res.OldestUnpaidDueDate)
	})

	t.Run("requestId: a repeat records nothing, a different body is refused, two at once record one", func(t *testing.T) {
		setNow(env, 2026, 11, 2, 9, 0)
		w := newWorld(t, env, "0812903")
		r := w.rent(t, "Dewi Lestari", day(2026, 11, 1), rentOpts{})
		w.generate(t)
		in := cash(r.renter, 350_000, day(2026, 11, 2))
		in.Note = "dititip ke Pak RT"

		first := w.record(t, in)
		require.True(t, first.Created)
		auditedBefore := w.audited(t, authz.ActionPaymentRecord, "allow")
		require.Equal(t, 1, auditedBefore)

		again := w.record(t, in)
		require.False(t, again.Created)
		require.Equal(t, first.Payment.ID, again.Payment.ID)
		require.Equal(t, first.Paid, again.Paid)
		require.Equal(t, 1, w.count(t, "payment"))
		require.Equal(t, auditedBefore, w.audited(t, authz.ActionPaymentRecord, "allow"), "a repeat writes no audit row")

		for name, change := range map[string]func(*billing.PaymentInput){
			"amount":     func(in *billing.PaymentInput) { in.Amount = 351_000 },
			"method":     func(in *billing.PaymentInput) { in.Method = billing.MethodTransfer },
			"receivedOn": func(in *billing.PaymentInput) { in.ReceivedOn = day(2026, 11, 1) },
			"note":       func(in *billing.PaymentInput) { in.Note = "" },
		} {
			t.Run("another "+name, func(t *testing.T) {
				other := in
				change(&other)
				_, err := env.Billing.RecordPayment(ctx, w.admin, other)
				require.ErrorIs(t, err, billing.ErrRequestIDReused)
			})
		}
		require.Equal(t, 1, w.count(t, "payment"))

		twice := cash(r.renter, 100_000, day(2026, 11, 2))
		var wg sync.WaitGroup
		results := make([]billing.PaymentResult, 2)
		errs := make([]error, 2)
		for i := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i], errs[i] = env.Billing.RecordPayment(ctx, w.admin, twice)
			}()
		}
		wg.Wait()
		require.NoError(t, errs[0])
		require.NoError(t, errs[1])
		require.Equal(t, results[0].Payment.ID, results[1].Payment.ID)
		require.NotEqual(t, results[0].Created, results[1].Created, "exactly one of them recorded it")
		require.Equal(t, 2, w.count(t, "payment"))
		require.EqualValues(t, 250_000, w.balance(t, r.renter).Outstanding)
	})

	t.Run("receivedOn is checked against today in Jakarta", func(t *testing.T) {
		setNow(env, 2026, 11, 2, 9, 0)
		w := newWorld(t, env, "0812904")
		r := w.rent(t, "Rudi Hartono", day(2026, 11, 1), rentOpts{})

		// 3 Nov 00:30 in Jakarta is still 2 Nov in UTC.
		setNow(env, 2026, 11, 3, 0, 30)
		res := w.record(t, cash(r.renter, 100_000, day(2026, 11, 3)))
		require.Equal(t, day(2026, 11, 3), res.Payment.ReceivedOn)
		_, err := env.Billing.RecordPayment(ctx, w.admin, cash(r.renter, 100_000, day(2026, 11, 4)))
		requireInput(t, err, "receivedOn")

		// 3 Nov 23:59 in Jakarta: today is still 3 Nov.
		setNow(env, 2026, 11, 3, 23, 59)
		w.record(t, cash(r.renter, 100_000, day(2026, 11, 3)))
		_, err = env.Billing.RecordPayment(ctx, w.admin, cash(r.renter, 100_000, day(2026, 11, 4)))
		requireInput(t, err, "receivedOn")
		require.Equal(t, 2, w.count(t, "payment"))
	})
}

func TestCreateCorrection(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()

	t.Run("disputed amount: an invoice corrected down, and what was paid over it is credit", func(t *testing.T) {
		setNow(env, 2026, 11, 2, 9, 0)
		w := newWorld(t, env, "0812911")
		r := w.rent(t, "Siti Rahayu", day(2026, 11, 1), rentOpts{})
		w.generate(t)
		inv := w.invoices(t, r.renter)[0]
		w.record(t, cash(r.renter, 700_000, day(2026, 11, 2)))

		res := w.correct(t, billing.CorrectionInput{
			RequestID: uuid.New(), InvoiceID: inv.InvoiceID, AmountDelta: -200_000, Reason: "  AC rusak, diskon disepakati ",
		})
		require.True(t, res.Created)
		require.Equal(t, r.tenantID, res.TenantID)
		require.Equal(t, "AC rusak, diskon disepakati", res.Correction.Reason)
		require.Equal(t, inv.InvoiceID, res.Correction.InvoiceID)
		require.Equal(t, w.admin.MembershipID, res.Correction.CreatedBy.MemberID)
		require.EqualValues(t, 200_000, res.Credit)
		require.Zero(t, res.Outstanding)

		after := w.invoices(t, r.renter)[0]
		require.EqualValues(t, 500_000, after.Billed)
		require.EqualValues(t, 500_000, after.Paid)
		require.Equal(t, "paid", after.Status)
		require.Equal(t, 1, w.audited(t, authz.ActionCorrectionCreate, "allow"), "audited with the change")
	})

	t.Run("a payment corrected below what it paid takes the money back from its newest invoice", func(t *testing.T) {
		setNow(env, 2026, 10, 28, 9, 0)
		w := newWorld(t, env, "0812912")
		r := w.rent(t, "Budi Santoso", day(2026, 9, 28), rentOpts{}) // entered late: two invoices
		w.generate(t)
		invs := w.invoices(t, r.renter)
		require.Len(t, invs, 2)
		pay := w.record(t, cash(r.renter, 1_000_000, day(2026, 10, 28)))
		require.Len(t, pay.Paid, 2)

		res := w.correct(t, billing.CorrectionInput{
			RequestID: uuid.New(), PaymentID: pay.Payment.ID, AmountDelta: -500_000, Reason: "uang kurang saat dihitung ulang",
		})
		require.EqualValues(t, 900_000, res.Outstanding)
		got := w.invoices(t, r.renter)
		require.EqualValues(t, 500_000, got[0].Paid, "the older invoice keeps what the payment still covers")
		require.EqualValues(t, 200_000, got[0].Outstanding)
		require.Zero(t, got[1].Paid, "the newest invoice gave its money back")
		require.Equal(t, "open", got[1].Status)

		acct, err := env.Billing.Account(ctx, w.admin, r.tenantID)
		require.NoError(t, err)
		require.Len(t, acct.Payments, 1)
		require.EqualValues(t, 500_000, acct.Payments[0].Net)
		require.Equal(t, []billing.PaidAmount{{InvoiceID: invs[0].InvoiceID, Amount: 500_000}}, acct.Payments[0].Allocations)
		require.Len(t, acct.Payments[0].Corrections, 1)
	})

	t.Run("a correction below zero is refused on an invoice and on a payment; nothing is written", func(t *testing.T) {
		setNow(env, 2026, 11, 2, 9, 0)
		w := newWorld(t, env, "0812913")
		r := w.rent(t, "Rina Wulandari", day(2026, 11, 1), rentOpts{})
		w.generate(t)
		inv := w.invoices(t, r.renter)[0]
		pay := w.record(t, cash(r.renter, 300_000, day(2026, 11, 2)))

		tests := []struct {
			name string
			in   billing.CorrectionInput
		}{
			{name: "invoice", in: billing.CorrectionInput{RequestID: uuid.New(), InvoiceID: inv.InvoiceID, AmountDelta: -700_001, Reason: "salah hitung"}},
			{name: "payment", in: billing.CorrectionInput{RequestID: uuid.New(), PaymentID: pay.Payment.ID, AmountDelta: -300_001, Reason: "salah hitung"}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := env.Billing.CreateCorrection(ctx, w.admin, tc.in)
				require.ErrorIs(t, err, billing.ErrCorrectionBelowZero)
			})
		}
		require.Zero(t, w.count(t, "correction"))
		require.Zero(t, w.audited(t, authz.ActionCorrectionCreate, "allow"), "the audit row rolled back with the refusal")
	})

	t.Run("requestId: a repeat of a correction to zero answers the stored one; another body is refused", func(t *testing.T) {
		setNow(env, 2026, 11, 2, 9, 0)
		w := newWorld(t, env, "0812914")
		r := w.rent(t, "Wahyu Pratama", day(2026, 11, 1), rentOpts{})
		w.generate(t)
		inv := w.invoices(t, r.renter)[0]
		in := billing.CorrectionInput{RequestID: uuid.New(), InvoiceID: inv.InvoiceID, AmountDelta: -700_000, Reason: "sewa dibatalkan pemilik"}

		first := w.correct(t, in)
		require.True(t, first.Created)
		again := w.correct(t, in)
		require.False(t, again.Created, "the trigger would refuse a second -700.000, so it must not be inserted")
		require.Equal(t, first.Correction.ID, again.Correction.ID)
		require.Equal(t, 1, w.count(t, "correction"))
		require.Equal(t, "cancelled", w.invoices(t, r.renter)[0].Status)

		other := in
		other.Reason = "alasan lain"
		_, err := env.Billing.CreateCorrection(ctx, w.admin, other)
		require.ErrorIs(t, err, billing.ErrRequestIDReused)
	})

	t.Run("a missing invoice or payment is not found on its field", func(t *testing.T) {
		w := newWorld(t, env, "0812915")
		_, err := env.Billing.CreateCorrection(ctx, w.admin, billing.CorrectionInput{RequestID: uuid.New(), InvoiceID: uuid.New(), AmountDelta: 1, Reason: "x"})
		requireNotFound(t, err, "invoiceId")
		_, err = env.Billing.CreateCorrection(ctx, w.admin, billing.CorrectionInput{RequestID: uuid.New(), PaymentID: uuid.New(), AmountDelta: 1, Reason: "x"})
		requireNotFound(t, err, "paymentId")
	})
}

func TestMoneyAuthorization(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 2, 9, 0)
	w := newWorld(t, env, "0812921")
	mawar, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kontrakan Mawar", Kind: property.KindKontrakan})
	require.NoError(t, err)
	rumah, err := env.Properties.CreateUnit(ctx, w.admin, mawar.ID, property.UnitInput{Label: "Rumah A", Terms: testenv.Monthly(1_500_000)})
	require.NoError(t, err)

	sari, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-9210-0099", Name: "Mbak Sari",
		Roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: w.kos.ID}}})
	require.NoError(t, err)
	limited := env.Principal(t, w.org.ID, sari.UserID)
	supervisor := env.Principal(t, w.org.ID, w.org.Supervisor.UserID)
	karyawan := env.Principal(t, w.org.ID, w.org.Karyawan.UserID)

	onlyKos := w.rent(t, "Siti Rahayu", day(2026, 11, 1), rentOpts{})
	both := w.rent(t, "Agus Salim", day(2026, 11, 1), rentOpts{})
	_, err = env.Tenants.CreateLease(ctx, w.admin, newLeaseIn(both.tenantID, rumah.ID, day(2026, 11, 1)))
	require.NoError(t, err)
	w.generate(t)

	t.Run("an admin limited to the kos records and corrects for a tenant only in the kos", func(t *testing.T) {
		res, err := env.Billing.RecordPayment(ctx, limited, cash(onlyKos.renter, 700_000, day(2026, 11, 2)))
		require.NoError(t, err)
		_, err = env.Billing.CreateCorrection(ctx, limited, billing.CorrectionInput{
			RequestID: uuid.New(), PaymentID: res.Payment.ID, AmountDelta: -50_000, Reason: "uang sobek ditolak",
		})
		require.NoError(t, err)
		_, err = env.Billing.Account(ctx, limited, onlyKos.tenantID)
		require.NoError(t, err)
	})

	t.Run("but not for a tenant who also rents in another property; each denial is audited", func(t *testing.T) {
		deniedBefore := w.audited(t, authz.ActionPaymentRecord, "deny")
		payments := w.count(t, "payment")
		_, err := env.Billing.RecordPayment(ctx, limited, cash(both.renter, 700_000, day(2026, 11, 2)))
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, payments, w.count(t, "payment"))
		require.Equal(t, deniedBefore+1, w.audited(t, authz.ActionPaymentRecord, "deny"))

		_, err = env.Billing.Account(ctx, limited, both.tenantID)
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Positive(t, w.audited(t, authz.ActionInvoiceRead, "deny"))

		// The invoice of the kos lease is in her property, so she may correct it.
		var kosInvoice uuid.UUID
		for _, inv := range w.invoices(t, both.renter) {
			if inv.LeaseID == both.leaseID {
				kosInvoice = inv.InvoiceID
			}
		}
		_, err = env.Billing.CreateCorrection(ctx, limited, billing.CorrectionInput{
			RequestID: uuid.New(), InvoiceID: kosInvoice, AmountDelta: -100_000, Reason: "air mati seminggu",
		})
		require.NoError(t, err)
	})

	t.Run("an ended lease in another property still counts", func(t *testing.T) {
		moved := w.rent(t, "Dian Puspita", day(2026, 11, 1), rentOpts{})
		other, err := env.Properties.CreateUnit(ctx, w.admin, mawar.ID, property.UnitInput{Label: "Rumah B", Terms: testenv.Monthly(1_500_000)})
		require.NoError(t, err)
		l, err := env.Tenants.CreateLease(ctx, w.admin, newLeaseIn(moved.tenantID, other.ID, day(2026, 11, 1)))
		require.NoError(t, err)
		_, err = env.Tenants.MoveOut(ctx, w.admin, l.ID, day(2026, 11, 1))
		require.NoError(t, err)
		_, err = env.Billing.RecordPayment(ctx, limited, cash(moved.renter, 700_000, day(2026, 11, 2)))
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("the admin's allowed payment and correction are audited with the change", func(t *testing.T) {
		before := w.audited(t, authz.ActionPaymentRecord, "allow")
		res := w.record(t, cash(both.renter, 2_200_000, day(2026, 11, 2)))
		require.Equal(t, before+1, w.audited(t, authz.ActionPaymentRecord, "allow"))
		var found bool
		for _, row := range env.AuditLog(t, w.org.ID) {
			if row.Action == authz.ActionPaymentRecord && row.Resource == "tenant/"+both.tenantID.String() {
				found = true
			}
		}
		require.True(t, found, "the audit row names the tenant")

		before = w.audited(t, authz.ActionCorrectionCreate, "allow")
		w.correct(t, billing.CorrectionInput{RequestID: uuid.New(), PaymentID: res.Payment.ID, AmountDelta: 10_000, Reason: "salah ketik"})
		require.Equal(t, before+1, w.audited(t, authz.ActionCorrectionCreate, "allow"))
	})

	t.Run("the Supervisor reads the account; the karyawan may not; neither records", func(t *testing.T) {
		acct, err := env.Billing.Account(ctx, supervisor, both.tenantID)
		require.NoError(t, err)
		require.Len(t, acct.Invoices, 2)
		_, err = env.Billing.Account(ctx, karyawan, both.tenantID)
		require.ErrorIs(t, err, authz.ErrForbidden)
		for _, p := range []authz.Principal{supervisor, karyawan} {
			_, err = env.Billing.RecordPayment(ctx, p, cash(both.renter, 1, day(2026, 11, 2)))
			require.ErrorIs(t, err, authz.ErrForbidden)
		}
	})

	t.Run("a tenant with no lease needs the organization-wide grant", func(t *testing.T) {
		nobody := w.newTenantOnly(t, "Hendra Gunawan")
		_, err := env.Billing.RecordPayment(ctx, limited, cash(nobody, 100_000, day(2026, 11, 2)))
		require.ErrorIs(t, err, authz.ErrForbidden)
		res := w.record(t, cash(nobody, 100_000, day(2026, 11, 2)))
		require.EqualValues(t, 100_000, res.Credit)
		require.Empty(t, res.Paid)
	})
}

func TestMoneyAcrossOrganizations(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 2, 9, 0)
	a := newWorld(t, env, "0812931")
	b := newWorld(t, env, "0812932")
	rb := b.rent(t, "Lina Marlina", day(2026, 11, 1), rentOpts{})
	b.generate(t)
	invB := b.invoices(t, rb.renter)[0]
	payB := b.record(t, cash(rb.renter, 200_000, day(2026, 11, 2)))
	before := b.balance(t, rb.renter)

	_, err := env.Billing.RecordPayment(ctx, a.admin, cash(rb.renter, 500_000, day(2026, 11, 2)))
	requireNotFound(t, err, "tenantId")
	_, err = env.Billing.CreateCorrection(ctx, a.admin, billing.CorrectionInput{RequestID: uuid.New(), InvoiceID: invB.InvoiceID, AmountDelta: -1, Reason: "x"})
	requireNotFound(t, err, "invoiceId")
	_, err = env.Billing.CreateCorrection(ctx, a.admin, billing.CorrectionInput{RequestID: uuid.New(), PaymentID: payB.Payment.ID, AmountDelta: -1, Reason: "x"})
	requireNotFound(t, err, "paymentId")
	_, err = env.Billing.Account(ctx, a.admin, rb.tenantID)
	requireNotFound(t, err, "")

	// Organization A reuses B's requestId for a tenant of its own: separate organizations.
	ra := a.rent(t, "Yusuf Hidayat", day(2026, 11, 1), rentOpts{})
	reused := cash(ra.renter, 200_000, day(2026, 11, 2))
	reused.RequestID = uuid.MustParse(*mustSourceRef(t, b, payB.Payment.ID))
	require.True(t, a.record(t, reused).Created)

	require.Equal(t, before, b.balance(t, rb.renter))
	require.Equal(t, 1, b.count(t, "payment"))
	require.Zero(t, b.count(t, "correction"))
}

// newLeaseIn is a monthly lease of a unit for a tenant, starting on start.
func newLeaseIn(tenantID, unitID uuid.UUID, start time.Time) tenant.NewLease {
	return tenant.NewLease{TenantID: tenantID, UnitID: unitID, TermMonths: 1, Terms: tenant.Terms{StartDate: start}}
}

// newTenantOnly creates a tenant without any lease.
func (w *world) newTenantOnly(t *testing.T, name string) renter {
	t.Helper()
	w.n++
	tn, err := w.env.Tenants.Create(context.Background(), w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	return renter{tenantID: tn.ID}
}

// mustSourceRef reads a payment's source_ref.
func mustSourceRef(t *testing.T, w *world, paymentID uuid.UUID) *string {
	t.Helper()
	var ref *string
	w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
		p, err := q.GetPayment(ctx, dbgen.GetPaymentParams{OrganizationID: w.org.ID, ID: paymentID})
		ref = p.SourceRef
		return err
	})
	require.NotNil(t, ref)
	return ref
}
