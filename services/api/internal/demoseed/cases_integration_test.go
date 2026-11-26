//go:build integration

package demoseed_test

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/checkqueue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/demoseed"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/deposit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/visit"
)

// The case organization's staff.
const (
	kasusSupervisor = "0812-0011-0001"
	kasusAdmin      = "0812-0011-0002"
	kasusYoga       = "0812-0011-0011"
	kasusDina       = "0812-0011-0012"
)

// allRows is a page big enough for every list of the case organization.
var allRows = listing.Page{Number: 1, Size: listing.MaxPageSize}

// caseDeps wires SeedCases as cmd/admin does, with files in storage when it is not nil.
func caseDeps(env *testenv.Env, storage *uploadtest.Storage) demoseed.CaseDeps {
	d := demoseed.CaseDeps{
		Store: env.Store, Authz: env.Authz, Orgs: env.Orgs,
		AutoGoPay: autogopay.NewService(env.Store, demoKeyring, env.Authz, env.Clock),
		Log:       slog.New(slog.DiscardHandler),
	}
	if storage != nil {
		d.Files = &demoseed.CaseFiles{Storage: storage, Put: putTo(storage)}
	}
	return d
}

// putTo stores a photo where its presigned PUT points, as the browser would.
func putTo(storage *uploadtest.Storage) func(context.Context, upload.PresignedPut, []byte) error {
	return func(_ context.Context, put upload.PresignedPut, body []byte) error {
		u, err := url.Parse(put.URL)
		if err != nil {
			return err
		}
		storage.Put(strings.TrimPrefix(u.Path, "/"), body)
		return nil
	}
}

// kasus is the seeded case organization and its members.
type kasus struct {
	env                           *testenv.Env
	org                           uuid.UUID
	admin, supervisor, yoga, dina authz.Principal
	tenants                       map[string]uuid.UUID
}

// seedKasus seeds the demo, bills the other demo organizations and seeds the cases.
func seedKasus(t *testing.T, env *testenv.Env, storage *uploadtest.Storage) (kasus, demoseed.CaseSummary) {
	t.Helper()
	ctx := context.Background()
	_, err := demoseed.Run(ctx, deps(env), env.Clock)
	require.NoError(t, err)
	_, err = demoseed.GenerateInvoices(ctx, generateDeps(env), env.Clock)
	require.NoError(t, err)
	sum, err := demoseed.SeedCases(ctx, caseDeps(env, storage), env.Clock)
	require.NoError(t, err)
	require.Equal(t, demoseed.OrgKasus, sum.Organization)

	m, err := env.Orgs.FindForMember(ctx, kasusAdmin, demoseed.OrgKasus)
	require.NoError(t, err)
	k := kasus{env: env, org: m.OrganizationID, tenants: map[string]uuid.UUID{}}
	k.admin = env.Principal(t, k.org, m.UserID)
	k.supervisor = env.Principal(t, k.org, env.UserID(t, kasusSupervisor))
	k.yoga = env.Principal(t, k.org, env.UserID(t, kasusYoga))
	k.dina = env.Principal(t, k.org, env.UserID(t, kasusDina))
	all, err := env.AllTenants(ctx, k.admin)
	require.NoError(t, err)
	for _, tn := range all {
		k.tenants[tn.Name] = tn.ID
	}
	require.Len(t, k.tenants, 24, "one tenant per case")
	return k, sum
}

func (k kasus) id(t *testing.T, name string) uuid.UUID {
	t.Helper()
	id, ok := k.tenants[name]
	require.True(t, ok, "tenant %s", name)
	return id
}

// fieldToday is member p's "Tagihan hari ini" by tenant name.
func (k kasus) fieldToday(t *testing.T, p authz.Principal) map[string]billing.FieldTenant {
	t.Helper()
	res, err := k.env.Billing.FieldToday(context.Background(), p, allRows)
	require.NoError(t, err)
	require.EqualValues(t, len(res.Items), res.Total, "one page holds the whole list")
	out := map[string]billing.FieldTenant{}
	for _, r := range res.Items {
		out[r.TenantName] = r
	}
	return out
}

// visits is member p's "Kunjungan hari ini" by tenant name.
func (k kasus) visits(t *testing.T, p authz.Principal) map[string]visit.Row {
	t.Helper()
	res, err := visit.NewService(k.env.Store, k.env.Billing, k.env.Clock).List(context.Background(), p, allRows)
	require.NoError(t, err)
	require.EqualValues(t, len(res.Items), res.Total, "one page holds the whole list")
	out := map[string]visit.Row{}
	for _, r := range res.Items {
		out[r.TenantName] = r
	}
	return out
}

// reasons is the codes of a visit row's reasons.
func reasons(r visit.Row) []string {
	out := make([]string, len(r.Reasons))
	for i, x := range r.Reasons {
		out[i] = x.Code
	}
	return out
}

func (k kasus) account(t *testing.T, name string) billing.Account {
	t.Helper()
	acct, err := k.env.Billing.Account(context.Background(), k.admin, k.id(t, name))
	require.NoError(t, err)
	return acct
}

// promiseStatuses is the statuses of the tenant's promises, newest first as List orders them.
func (k kasus) promiseStatuses(t *testing.T, name string) []string {
	t.Helper()
	svc := promise.NewService(promise.Deps{Store: k.env.Store, Tenants: k.env.Billing, Clock: k.env.Clock})
	res, err := svc.List(context.Background(), k.admin, k.id(t, name), allRows)
	require.NoError(t, err)
	out := make([]string, len(res.Items))
	for i, p := range res.Items {
		out[i] = p.Status
	}
	return out
}

func (k kasus) checkQueue() *checkqueue.Service {
	return checkqueue.NewService(k.env.Store, k.env.Authz, k.env.Billing, k.env.Tenants, k.env.Clock)
}

func (k kasus) deposits() *deposit.Service {
	return deposit.NewService(k.env.Store, k.env.Authz, k.env.Clock, slog.New(slog.DiscardHandler))
}

// paidBy sums a tenant's payments of the method.
func paidBy(acct billing.Account, method string) int64 {
	var n int64
	for _, p := range acct.Payments {
		if p.Method == method {
			n += p.Amount
		}
	}
	return n
}

// billed is what the tenant's invoices bill, cancelled ones included at what they bill now.
func billed(acct billing.Account) int64 {
	var n int64
	for _, inv := range acct.Invoices {
		n += inv.Billed
	}
	return n
}

// kasusMoneyTables are the tables SeedCases writes money and field rows to.
var kasusMoneyTables = []string{
	"invoice", "invoice_line", "payment", "payment_allocation", "correction",
	"payment_promise", "promise_version", "visit_task", "visit_outcome", "upload", "cash_deposit", "qris_attempt",
}

func tableCounts(t *testing.T, env *testenv.Env, org uuid.UUID, names []string) map[string]int {
	t.Helper()
	ctx := context.Background()
	out := map[string]int{}
	err := env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		for _, name := range names {
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

func TestSeedCases(t *testing.T) {
	ctx := context.Background()
	// testenv's clock is fixed at 2 November 2026, 09:00 in Jakarta: the case organization's dates
	// are relative to it, never to the real today.
	env := testenv.New(t)
	storage := uploadtest.NewStorage()
	k, sum := seedKasus(t, env, storage)
	require.False(t, sum.FilesSkipped)
	require.Positive(t, sum.Invoices)
	require.Positive(t, sum.Payments)
	require.Positive(t, sum.Promises)
	require.Positive(t, sum.VisitTasks)
	require.Equal(t, 3, sum.Outcomes, "Irfan, Joko and Kevin")
	require.Equal(t, 3, sum.Deposits, "exact, short and waiting")
	require.Equal(t, 7, sum.Uploads, "four receipts, the infected file and two proofs")

	yogaToday := k.fieldToday(t, k.yoga)
	dinaToday := k.fieldToday(t, k.dina)
	yogaVisits := k.visits(t, k.yoga)

	t.Run("1 Rizky is due in two days: on Tagihan hari ini, not on Kunjungan", func(t *testing.T) {
		row, ok := yogaToday[demoseed.KasusDueSoon]
		require.True(t, ok)
		require.Equal(t, billing.FieldReason{Code: billing.FieldDueInDays, Days: 2}, row.Reason)
		require.EqualValues(t, 900_000, row.Outstanding, "only the coming invoice")
		require.Zero(t, row.Overdue, "the earlier invoices are paid")
		require.NotContains(t, yogaVisits, demoseed.KasusDueSoon)
	})

	t.Run("2 Salsabila is due in five days and owes nothing overdue: not on Tagihan hari ini", func(t *testing.T) {
		require.NotContains(t, yogaToday, demoseed.KasusNotDue)
		require.NotContains(t, yogaVisits, demoseed.KasusNotDue)
		acct := k.account(t, demoseed.KasusNotDue)
		require.Zero(t, acct.Overdue)
		// What the product does today: Yoga has no list that shows her, so no screen lets him take
		// her cash in advance (T-109 gap). The admin still can, through her account.
	})

	t.Run("3 Taufik paid everything by QRIS: not listed", func(t *testing.T) {
		acct := k.account(t, demoseed.KasusQRISFull)
		require.Zero(t, acct.Outstanding)
		require.Positive(t, paidBy(acct, billing.MethodQRIS))
		require.NotContains(t, yogaToday, demoseed.KasusQRISFull)
		require.NotContains(t, yogaVisits, demoseed.KasusQRISFull)
	})

	t.Run("4 Umar paid half by QRIS: listed with the rest", func(t *testing.T) {
		acct := k.account(t, demoseed.KasusQRISPart)
		qris := paidBy(acct, billing.MethodQRIS)
		require.Positive(t, qris)
		require.Equal(t, billed(acct)-qris, acct.Outstanding)
		require.InDelta(t, billed(acct)/2, qris, 1000, "half, in thousands")
		row, ok := yogaToday[demoseed.KasusQRISPart]
		require.True(t, ok)
		require.Equal(t, billing.FieldOverdue, row.Reason.Code)
		require.Equal(t, acct.Outstanding, row.Outstanding)
	})

	t.Run("5 Vina's credit paid part of her next invoice", func(t *testing.T) {
		acct := k.account(t, demoseed.KasusCredit)
		require.Zero(t, acct.Credit, "all of it applied")
		require.Zero(t, acct.Overdue)
		require.EqualValues(t, 600_000, acct.Outstanding, "Rp900.000 less the Rp300.000 credit")
		last := acct.Invoices[len(acct.Invoices)-1]
		require.EqualValues(t, 900_000, last.Amount)
		require.EqualValues(t, 300_000, last.Paid)
		require.Equal(t, "partial", last.Status)
		row, ok := yogaToday[demoseed.KasusCredit]
		require.True(t, ok)
		require.Equal(t, billing.FieldDueInDays, row.Reason.Code)
		require.EqualValues(t, 600_000, row.Outstanding)
	})

	t.Run("6 Wahyu paid Yoga cash today: listed as paid today", func(t *testing.T) {
		row, ok := yogaToday[demoseed.KasusCashToday]
		require.True(t, ok)
		require.Equal(t, billing.FieldReason{Code: billing.FieldPaidToday}, row.Reason)
		require.Zero(t, row.Outstanding)
		require.Nil(t, row.OldestUnpaidDueDate)
		require.Equal(t, 1, row.CashToday.Count)
		require.Positive(t, row.CashToday.Amount)
		require.Len(t, row.CashToday.Payments, 1)
		require.Equal(t, upload.StatusClean, row.CashToday.Payments[0].Photo.Status)
	})

	t.Run("7 Yusuf paid Dina cash: not on Yoga's list, on Dina's as paid today", func(t *testing.T) {
		require.NotContains(t, yogaToday, demoseed.KasusCashOther)
		row, ok := dinaToday[demoseed.KasusCashOther]
		require.True(t, ok)
		require.Equal(t, billing.FieldPaidToday, row.Reason.Code)
		require.Zero(t, row.Outstanding)
		require.Positive(t, row.CashToday.Amount)
	})

	queue, err := k.checkQueue().List(ctx, k.admin, "", allRows)
	require.NoError(t, err)
	proofs := map[string]checkqueue.TransferProof{}
	for _, it := range queue.Items {
		if it.TransferProof != nil {
			proofs[it.TransferProof.Tenant.Name] = *it.TransferProof
		}
	}

	t.Run("8 Zahra's proof waits in Antrean cek; Yoga still sees her full debt", func(t *testing.T) {
		acct := k.account(t, demoseed.KasusTenantProof)
		require.Empty(t, acct.Payments)
		row, ok := yogaToday[demoseed.KasusTenantProof]
		require.True(t, ok)
		require.Equal(t, billing.FieldOverdue, row.Reason.Code)
		require.Equal(t, acct.Outstanding, row.Outstanding)
		require.Equal(t, billed(acct), row.Outstanding, "the proof pays nothing until the admin matches it")
		// What the product does today: FieldTenant has no field for a proof waiting for the check,
		// so Yoga's card cannot say "bukti transfer sedang dicek" (T-109 gap).

		proof, ok := proofs[demoseed.KasusTenantProof]
		require.True(t, ok)
		require.Nil(t, proof.SentBy, "sent from her payment page")
		require.NotEqual(t, uuid.Nil, proof.InvoiceID)
		require.Equal(t, upload.StatusClean, proof.UploadStatus)
	})

	t.Run("9 Bima's proof, sent by Yoga, waits in Antrean cek", func(t *testing.T) {
		proof, ok := proofs[demoseed.KasusStaffProof]
		require.True(t, ok)
		require.NotNil(t, proof.SentBy)
		require.Equal(t, k.yoga.UserID, proof.SentBy.ID)
		require.Equal(t, uuid.Nil, proof.InvoiceID)
		require.Equal(t, upload.StatusClean, proof.UploadStatus)
		require.Empty(t, k.account(t, demoseed.KasusStaffProof).Payments)
	})

	t.Run("10 open visit tasks: h7, broken promises and undeliverable", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			reason string
		}{
			{demoseed.KasusOverdue, visit.ReasonOverdue},
			{demoseed.KasusBroken, visit.ReasonBrokenPromises},
			{demoseed.KasusUndeliverable, visit.ReasonUndeliverable},
		} {
			t.Run(tc.name, func(t *testing.T) {
				row, ok := yogaVisits[tc.name]
				require.True(t, ok)
				require.Equal(t, visit.StatusOpen, row.Status)
				require.Contains(t, reasons(row), tc.reason)
			})
		}
		require.Equal(t, []string{promise.StatusBroken, promise.StatusBroken}, k.promiseStatuses(t, demoseed.KasusBroken))
	})

	t.Run("11 promises: active, kept and partly kept", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			status string
		}{
			{demoseed.KasusPromiseActive, promise.StatusActive},
			{demoseed.KasusPromiseKept, promise.StatusKept},
			{demoseed.KasusPromisePartly, promise.StatusPartiallyKept},
		} {
			t.Run(tc.name, func(t *testing.T) {
				require.Equal(t, []string{tc.status}, k.promiseStatuses(t, tc.name))
			})
		}
		require.Zero(t, k.account(t, demoseed.KasusPromiseKept).Outstanding, "Fikri paid everything")
		require.EqualValues(t, 300_000, paidBy(k.account(t, demoseed.KasusPromisePartly), billing.MethodTransfer))
	})

	t.Run("12 today's outcomes: Irfan done, Joko not home and still open", func(t *testing.T) {
		irfan, ok := yogaVisits[demoseed.KasusVisitDone]
		require.True(t, ok)
		require.Equal(t, visit.StatusDone, irfan.Status)
		require.NotNil(t, irfan.LastOutcome)
		require.Equal(t, visit.OutcomePromise, irfan.LastOutcome.Outcome)

		joko, ok := yogaVisits[demoseed.KasusVisitNotHome]
		require.True(t, ok)
		require.Equal(t, visit.StatusOpen, joko.Status)
		require.NotNil(t, joko.LastOutcome)
		require.Equal(t, visit.OutcomeNotHome, joko.LastOutcome.Outcome)
	})

	t.Run("13 leases: two at once, an ended one owing, and another karyawan's tenant", func(t *testing.T) {
		laras, err := env.Tenants.Get(ctx, k.admin, k.id(t, demoseed.KasusTwoLeases))
		require.NoError(t, err)
		require.Len(t, laras.Leases, 2)
		for _, l := range laras.Leases {
			require.Equal(t, tenant.StatusActive, l.Status, l.UnitLabel)
		}
		row, ok := yogaToday[demoseed.KasusTwoLeases]
		require.True(t, ok)
		require.Len(t, row.Units, 2, "both of Yoga's units on one card")

		dedi, err := env.Tenants.Get(ctx, k.admin, k.id(t, demoseed.KasusEndedLease))
		require.NoError(t, err)
		statuses := map[string]string{}
		for _, l := range dedi.Leases {
			statuses[l.UnitLabel] = l.Status
		}
		require.Equal(t, map[string]string{"Rumah 2": tenant.StatusEnded, "Kamar 20": tenant.StatusActive}, statuses)
		acct := k.account(t, demoseed.KasusEndedLease)
		require.Positive(t, acct.Overdue, "he moved out owing")
		dediRow, ok := yogaToday[demoseed.KasusEndedLease]
		require.True(t, ok, "Yoga collects his room, so he sees the debt of the ended lease too")
		require.Equal(t, acct.Outstanding, dediRow.Outstanding)
		require.Equal(t, []billing.FieldUnit{{UnitLabel: "Kamar 20", PropertyName: "Kos Aneka Jakal", PropertyAddress: "Jl. Kaliurang Km 9 No. 4, Sleman"}}, dediRow.Units)

		xena := k.id(t, demoseed.KasusOtherKaryawan)
		require.NotContains(t, yogaToday, demoseed.KasusOtherKaryawan)
		require.NotContains(t, yogaVisits, demoseed.KasusOtherKaryawan)
		_, err = env.Billing.FieldPromiseOffer(ctx, k.yoga, xena)
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, _, err = visit.NewService(env.Store, env.Billing, env.Clock).Record(ctx, k.yoga, xena, visit.Input{
			RequestID: uuid.New(), Outcome: visit.OutcomeNotHome,
		})
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Contains(t, dinaToday, demoseed.KasusOtherKaryawan)
		_, err = env.Billing.FieldPromiseOffer(ctx, k.dina, xena)
		require.NoError(t, err)
	})

	t.Run("14 deposits: one waiting, one counted exactly, one short by Rp50.000", func(t *testing.T) {
		res, err := k.deposits().List(ctx, k.admin, deposit.Query{}, allRows)
		require.NoError(t, err)
		require.EqualValues(t, 3, res.Total)
		byStatus := map[string]deposit.Deposit{}
		for _, d := range res.Items {
			byStatus[d.Status] = d
		}
		require.Len(t, byStatus, 3)
		waiting := byStatus[deposit.StatusPending]
		require.Equal(t, k.dina.UserID, waiting.StaffUserID)
		require.Nil(t, waiting.Confirmation)
		exact := byStatus[deposit.StatusMatched]
		require.Equal(t, k.yoga.UserID, exact.StaffUserID)
		require.EqualValues(t, 500_000, exact.Amount)
		require.NotNil(t, exact.Confirmation)
		require.Zero(t, exact.Confirmation.Gap)
		gap := byStatus[deposit.StatusGap]
		require.Equal(t, k.yoga.UserID, gap.StaffUserID)
		require.EqualValues(t, 400_000, gap.Amount)
		require.NotNil(t, gap.Confirmation)
		require.EqualValues(t, 350_000, gap.Confirmation.ReceivedAmount)
		require.EqualValues(t, -50_000, gap.Confirmation.Gap)

		// Yoga's cash is all deposited except Wahyu's, taken after.
		held, err := env.Billing.FieldCash(ctx, k.yoga)
		require.NoError(t, err)
		require.Equal(t, 1, held.Count)
		require.Equal(t, yogaToday[demoseed.KasusCashToday].CashToday.Amount, held.Amount)
	})

	t.Run("15 Antrean cek holds every kind", func(t *testing.T) {
		for _, kind := range checkqueue.Kinds {
			require.Positive(t, queue.Counts[kind], kind)
		}
		require.Equal(t, 2, queue.Counts[checkqueue.KindTransferProof])
		require.Equal(t, queue.Waiting, queue.Total)
		for _, it := range queue.Items {
			switch {
			case it.DepositGap != nil:
				require.EqualValues(t, -50_000, it.DepositGap.Gap)
				require.Equal(t, k.yoga.UserID, it.DepositGap.Depositor.ID)
			case it.UploadInfected != nil:
				require.Equal(t, upload.PurposeCashReceipt, it.UploadInfected.Purpose)
				require.NotNil(t, it.UploadInfected.Tenant)
				require.Equal(t, demoseed.KasusOverdue, it.UploadInfected.Tenant.Name)
			case it.QrisAmountMismatch != nil:
				m := it.QrisAmountMismatch
				require.Equal(t, demoseed.KasusQRISMismatch, m.Tenant.Name)
				require.EqualValues(t, 500_000, m.QRAmount)
				require.NotNil(t, m.ReportedAmount)
				require.EqualValues(t, 450_000, *m.ReportedAmount)
			case it.PhoneChanged != nil:
				require.Equal(t, demoseed.KasusNumberChanged, it.PhoneChanged.Tenant.Name)
				require.Equal(t, "+6285711002233", it.PhoneChanged.NewPhone)
				require.Equal(t, k.yoga.UserID, it.PhoneChanged.RecordedBy.ID)
			}
		}
	})

	t.Run("the supervisor reads what the screens show and cannot answer the queue", func(t *testing.T) {
		got, err := k.checkQueue().List(ctx, k.supervisor, "", allRows)
		require.NoError(t, err)
		require.Equal(t, queue.Counts, got.Counts)
		deps, err := k.deposits().List(ctx, k.supervisor, deposit.Query{}, allRows)
		require.NoError(t, err)
		require.EqualValues(t, 3, deps.Total)
		_, err = env.Billing.Account(ctx, k.supervisor, k.id(t, demoseed.KasusQRISPart))
		require.NoError(t, err)
		// Tagihan hari ini is the member's own field list: the Supervisor collects from nobody.
		field, err := env.Billing.FieldToday(ctx, k.supervisor, allRows)
		require.NoError(t, err)
		require.Zero(t, field.Total)

		var phoneItem uuid.UUID
		for _, it := range queue.Items {
			if it.Kind == checkqueue.KindPhoneChanged {
				phoneItem = it.ID
			}
		}
		_, err = k.checkQueue().Dismiss(ctx, k.supervisor, checkqueue.KindPhoneChanged, phoneItem, "Bukan tugas supervisor")
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = k.checkQueue().Apply(ctx, k.supervisor, phoneItem)
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	t.Run("running it again records nothing", func(t *testing.T) {
		before := tableCounts(t, env, k.org, kasusMoneyTables)
		again, err := demoseed.SeedCases(ctx, caseDeps(env, storage), env.Clock)
		require.NoError(t, err)
		require.Equal(t, demoseed.CaseSummary{Organization: demoseed.OrgKasus}, again)
		require.Equal(t, before, tableCounts(t, env, k.org, kasusMoneyTables))
	})
}

func TestSeedCasesWithoutFiles(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t) // 2 November 2026, 09:00 in Jakarta
	k, sum := seedKasus(t, env, nil)
	require.True(t, sum.FilesSkipped)
	require.Zero(t, sum.Uploads)
	require.Zero(t, sum.Deposits)
	require.Positive(t, sum.Invoices)
	require.Positive(t, sum.Payments)
	require.Equal(t, 3, sum.Outcomes)

	counts := tableCounts(t, env, k.org, []string{"upload", "cash_deposit"})
	require.Zero(t, counts["upload"])
	require.Zero(t, counts["cash_deposit"])

	t.Run("the cases that need no photo are seeded", func(t *testing.T) {
		require.Zero(t, k.account(t, demoseed.KasusQRISFull).Outstanding)
		require.Zero(t, k.account(t, demoseed.KasusCredit).Credit)
		require.Equal(t, []string{promise.StatusActive}, k.promiseStatuses(t, demoseed.KasusPromiseActive))
		visits := k.visits(t, k.yoga)
		require.Contains(t, reasons(visits[demoseed.KasusUndeliverable]), visit.ReasonUndeliverable)
		require.Equal(t, visit.StatusDone, visits[demoseed.KasusVisitDone].Status)
	})

	t.Run("the photo cases are not", func(t *testing.T) {
		require.Zero(t, paidBy(k.account(t, demoseed.KasusCashToday), billing.MethodCash))
		yusuf, ok := k.fieldToday(t, k.dina)[demoseed.KasusCashOther]
		require.True(t, ok)
		require.Equal(t, billing.FieldOverdue, yusuf.Reason.Code, "he still owes without his cash")
		queue, err := k.checkQueue().List(ctx, k.admin, "", allRows)
		require.NoError(t, err)
		require.Zero(t, queue.Counts[checkqueue.KindTransferProof])
		require.Zero(t, queue.Counts[checkqueue.KindDepositGap])
		require.Zero(t, queue.Counts[checkqueue.KindUploadInfected])
		require.Equal(t, 1, queue.Counts[checkqueue.KindQrisAmountMismatch])
		require.Equal(t, 1, queue.Counts[checkqueue.KindPhoneChanged])
	})

	t.Run("running it again records nothing", func(t *testing.T) {
		before := tableCounts(t, env, k.org, kasusMoneyTables)
		again, err := demoseed.SeedCases(ctx, caseDeps(env, nil), env.Clock)
		require.NoError(t, err)
		require.Equal(t, demoseed.CaseSummary{Organization: demoseed.OrgKasus, FilesSkipped: true}, again)
		require.Equal(t, before, tableCounts(t, env, k.org, kasusMoneyTables))
	})
}
