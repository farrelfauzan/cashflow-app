//go:build integration

package qris_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay/autogopaytest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// suite is one database with one fake AutoGoPay shared by every organization in it. The fake
// numbers its transaction ids from 1, and qris_attempt.transaction_id is unique across
// organizations, so two fakes in one database would hand out the same id. Each organization has
// its own key, and flow.calls keeps only the calls made with it.
type suite struct {
	env  *testenv.Env
	fake *autogopaytest.Fake
	n    int
}

func newSuite(t *testing.T) *suite {
	t.Helper()
	env := testenv.New(t)
	return &suite{env: env, fake: autogopaytest.New(env.Clock)}
}

// flow is one organization with its own AutoGoPay key (empty when it has none) and a QRIS
// service on the suite's fake.
type flow struct {
	*world
	fake *autogopaytest.Fake
	svc  *qris.Service
	key  string
}

var orgNames = []string{"Griya Sejahtera", "Kos Melati Indah", "Kontrakan Pak Haji", "Wisma Cempaka", "Kos Putri Anggrek"}

// newFlow makes an organization; withKey gives it an AutoGoPay key of its own.
func (s *suite) newFlow(t *testing.T, withKey bool) *flow {
	t.Helper()
	s.n++
	name := fmt.Sprintf("%s %d", orgNames[s.n%len(orgNames)], s.n)
	w := newWorld(t, s.env, name, fmt.Sprintf("0813%03d", s.n))
	f := &flow{world: w, fake: s.fake}
	if withKey {
		f.key = fmt.Sprintf("agp-live-key-%03d", s.n)
		s.env.SetAutoGoPayKey(t, w.org.ID, f.key)
	}
	f.svc = qris.NewService(qris.Deps{
		Store: s.env.Store, Links: w.links, Clock: s.env.Clock, Gateway: s.fake,
		Keys: s.env.AutoGoPay, Limiter: s.env.Limiter, Audit: s.env.Authz,
	})
	return f
}

// calls are the fake's calls made with this organization's key, oldest first.
func (f *flow) calls() []autogopaytest.Call {
	var out []autogopaytest.Call
	for _, c := range f.fake.Calls() {
		if c.Key == f.key {
			out = append(out, c)
		}
	}
	return out
}

// callsSince are the calls made with this organization's key after the first n.
func (f *flow) callsSince(n int) []autogopaytest.Call {
	return f.calls()[n:]
}

func (f *flow) generate(amount int64) autogopaytest.Call {
	return autogopaytest.Call{Method: autogopaytest.MethodGenerate, Key: f.key, Amount: amount}
}

func (f *flow) status(txID string) autogopaytest.Call {
	return autogopaytest.Call{Method: autogopaytest.MethodStatus, Key: f.key, TransactionID: txID}
}

func (f *flow) cancel(txID string) autogopaytest.Call {
	return autogopaytest.Call{Method: autogopaytest.MethodCancel, Key: f.key, TransactionID: txID}
}

func (f *flow) makeQR(t *testing.T, r renter, amount int64) qris.PublicView {
	t.Helper()
	v, err := f.svc.MakeQR(context.Background(), r.token, amount)
	require.NoError(t, err)
	return v
}

func (f *flow) check(t *testing.T, r renter) qris.PublicView {
	t.Helper()
	v, err := f.svc.Check(context.Background(), r.token)
	require.NoError(t, err)
	return v
}

// requireSameView compares two answers; times are compared as instants, whatever their zone.
func requireSameView(t *testing.T, want, got qris.PublicView) {
	t.Helper()
	require.Equal(t, want.Status, got.Status)
	require.Equal(t, want.Amount, got.Amount)
	require.True(t, want.ExpiresAt.Equal(got.ExpiresAt), "expires %s, want %s", got.ExpiresAt, want.ExpiresAt)
	require.Equal(t, want.CheckedAt == nil, got.CheckedAt == nil)
	if want.CheckedAt != nil {
		require.True(t, want.CheckedAt.Equal(*got.CheckedAt))
	}
}

func (f *flow) publicStatus(t *testing.T, r renter) qris.PublicView {
	t.Helper()
	v, err := f.svc.PublicStatus(context.Background(), r.token)
	require.NoError(t, err)
	return v
}

func (f *flow) reconcile(t *testing.T) qris.ReconcileResult {
	t.Helper()
	res, err := f.svc.ReconcileOrg(context.Background(), f.org.ID)
	require.NoError(t, err)
	return res
}

// secondLink mints another link to r's invoice, as a reminder would.
func (f *flow) secondLink(t *testing.T, r renter) renter {
	t.Helper()
	link, err := f.links.Mint(context.Background(), f.admin, r.invoiceID)
	require.NoError(t, err)
	r.linkID, r.token = link.ID, strings.TrimPrefix(link.URL, webOrigin+"/t/")
	return r
}

// attemptRow is a qris_attempt row as the app role reads it.
type attemptRow struct {
	ID               uuid.UUID
	TransactionID    string
	Amount           int64
	Status           string
	ExpiresAt        time.Time
	ClosedAt         *time.Time
	CheckRequestedAt *time.Time
	PaidAmount       *int64
	AmountMismatch   bool
}

// attempts reads r's attempts, closed ones first.
func (f *flow) attempts(t *testing.T, r renter) []attemptRow {
	t.Helper()
	var out []attemptRow
	require.NoError(t, f.tx(func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, transaction_id, amount, status, expires_at, closed_at,
			check_requested_at, paid_amount, amount_mismatch
			FROM qris_attempt WHERE organization_id = $1 AND tenant_id = $2
			ORDER BY status = 'pending', created_at, closed_at, id`, f.org.ID, r.tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a attemptRow
			if err := rows.Scan(&a.ID, &a.TransactionID, &a.Amount, &a.Status, &a.ExpiresAt, &a.ClosedAt,
				&a.CheckRequestedAt, &a.PaidAmount, &a.AmountMismatch); err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	}))
	return out
}

// only reads r's one attempt.
func (f *flow) only(t *testing.T, r renter) attemptRow {
	t.Helper()
	all := f.attempts(t, r)
	require.Len(t, all, 1)
	return all[0]
}

func byTransaction(t *testing.T, all []attemptRow, txID string) attemptRow {
	t.Helper()
	for _, a := range all {
		if a.TransactionID == txID {
			return a
		}
	}
	t.Fatalf("no attempt with transaction %s", txID)
	return attemptRow{}
}

func pendingCount(all []attemptRow) int {
	n := 0
	for _, a := range all {
		if a.Status == qris.AttemptPending {
			n++
		}
	}
	return n
}

// paymentRow is a payment row with its source and what its allocations hold.
type paymentRow struct {
	ID        uuid.UUID
	Amount    int64
	Method    string
	Source    string
	SourceRef *string
	Allocated int64
}

// payments reads r's payments, oldest first.
func (f *flow) payments(t *testing.T, r renter) []paymentRow {
	t.Helper()
	var out []paymentRow
	require.NoError(t, f.tx(func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT p.id, p.amount, p.method, p.source, p.source_ref,
			COALESCE((SELECT sum(a.amount) FROM payment_allocation a
				WHERE a.organization_id = p.organization_id AND a.payment_id = p.id), 0)
			FROM payment p WHERE p.organization_id = $1 AND p.tenant_id = $2 ORDER BY p.seq`, f.org.ID, r.tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p paymentRow
			if err := rows.Scan(&p.ID, &p.Amount, &p.Method, &p.Source, &p.SourceRef, &p.Allocated); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	}))
	return out
}

func (f *flow) account(t *testing.T, r renter) billing.Account {
	t.Helper()
	acct, err := f.env.Billing.Account(context.Background(), f.admin, r.tenantID)
	require.NoError(t, err)
	return acct
}

// requireQRISPayment checks that r has exactly one QRIS payment of amount for transaction txID,
// allocated in full to r's invoice, and its audit row.
func (f *flow) requireQRISPayment(t *testing.T, r renter, txID string, amount int64) {
	t.Helper()
	pays := f.payments(t, r)
	require.Len(t, pays, 1)
	p := pays[0]
	require.Equal(t, amount, p.Amount)
	require.Equal(t, billing.MethodQRIS, p.Method)
	require.Equal(t, billing.SourceQRIS, p.Source)
	require.NotNil(t, p.SourceRef)
	require.Equal(t, txID, *p.SourceRef)
	require.Equal(t, amount, p.Allocated, "the payment is allocated in full")

	acct := f.account(t, r)
	require.Len(t, acct.Invoices, 1)
	require.Equal(t, int64(700_000)-amount, acct.Invoices[0].Outstanding)
	require.Equal(t, int64(700_000)-amount, acct.Outstanding)
	require.Len(t, acct.Payments, 1)
	require.Nil(t, acct.Payments[0].RecordedBy, "a gateway payment has no member")

	var audited []dbgen.AuditLog
	for _, row := range f.env.AuditLog(t, f.org.ID) {
		if row.Action == authz.ActionPaymentRecord && row.Resource == "payment/"+p.ID.String() {
			audited = append(audited, row)
		}
	}
	require.Len(t, audited, 1)
	require.Equal(t, "allow", audited[0].Decision)
	require.False(t, audited[0].ActorStaffUserID.Valid, "the system records it, no member")
}

// requireNoPayment checks that r paid nothing and still owes the whole invoice.
func (f *flow) requireNoPayment(t *testing.T, r renter) {
	t.Helper()
	require.Empty(t, f.payments(t, r))
	require.Equal(t, int64(700_000), f.account(t, r).Outstanding)
}

func TestMakeQR(t *testing.T) {
	s := newSuite(t)
	env := s.env
	f := s.newFlow(t, true)

	t.Run("a tenant with no attempt gets a new QR", func(t *testing.T) {
		r := f.rent(t, "Rina Wulandari")
		v := f.makeQR(t, r, 700_000)

		calls := f.calls()
		require.Len(t, calls, 1)
		require.Equal(t, f.generate(700_000), calls[0])
		a := f.only(t, r)
		require.Equal(t, qris.AttemptPending, a.Status)
		require.Equal(t, int64(700_000), a.Amount)
		require.Nil(t, a.ClosedAt)
		want := env.Clock.Now().Truncate(time.Second).Add(autogopaytest.QRValidity)
		require.True(t, a.ExpiresAt.Equal(want), "expires %s, want %s", a.ExpiresAt, want)
		require.Equal(t, qris.StatusPending, v.Status)
		require.Equal(t, int64(700_000), v.Amount)
		require.True(t, v.ExpiresAt.Equal(want))
		require.Nil(t, v.CheckedAt)
	})

	t.Run("the same amount reuses the live QR, through any of the tenant's links", func(t *testing.T) {
		r := f.rent(t, "Budi Santoso")
		first := f.makeQR(t, r, 700_000)
		before := len(f.calls())
		a := f.only(t, r)

		again := f.makeQR(t, r, 700_000)
		requireSameView(t, first, again)
		require.Empty(t, f.callsSince(before), "a reuse calls nothing")
		require.Equal(t, a.ID, f.only(t, r).ID)

		other := f.secondLink(t, r)
		require.NotEqual(t, r.token, other.token)
		viaOther := f.makeQR(t, other, 700_000)
		requireSameView(t, first, viaOther)
		require.Empty(t, f.callsSince(before))
		require.Equal(t, a.ID, f.only(t, r).ID)
	})

	t.Run("the same amount with ReuseMargin left replaces the QR", func(t *testing.T) {
		r := f.rent(t, "Siti Rahayu")
		f.makeQR(t, r, 700_000)
		old := f.only(t, r)
		before := len(f.calls())
		env.Clock.Advance(autogopaytest.QRValidity - qris.ReuseMargin)

		v := f.makeQR(t, r, 700_000)
		require.Equal(t, []autogopaytest.Call{f.cancel(old.TransactionID), f.generate(700_000)}, f.callsSince(before))
		all := f.attempts(t, r)
		require.Len(t, all, 2)
		closed := byTransaction(t, all, old.TransactionID)
		require.Equal(t, qris.AttemptCancelled, closed.Status)
		require.NotNil(t, closed.ClosedAt)
		require.Equal(t, 1, pendingCount(all))
		require.Equal(t, qris.StatusPending, v.Status)
	})

	t.Run("another amount replaces the QR", func(t *testing.T) {
		r := f.rent(t, "Agus Pratama")
		f.makeQR(t, r, 700_000)
		old := f.only(t, r)
		before := len(f.calls())

		v := f.makeQR(t, r, 300_000)
		require.Equal(t, []autogopaytest.Call{f.cancel(old.TransactionID), f.generate(300_000)}, f.callsSince(before))
		all := f.attempts(t, r)
		require.Len(t, all, 2)
		closed := byTransaction(t, all, old.TransactionID)
		require.Equal(t, qris.AttemptCancelled, closed.Status)
		require.NotNil(t, closed.ClosedAt)
		live := all[1]
		require.Equal(t, qris.AttemptPending, live.Status)
		require.Equal(t, int64(300_000), live.Amount)
		require.Equal(t, qris.StatusPending, v.Status)
		require.Equal(t, int64(300_000), v.Amount)
	})

	t.Run("a QR past its expiry is read, not cancelled, then replaced", func(t *testing.T) {
		r := f.rent(t, "Dewi Lestari")
		f.makeQR(t, r, 700_000)
		old := f.only(t, r)
		before := len(f.calls())
		env.Clock.Advance(16 * time.Minute)

		v := f.makeQR(t, r, 700_000)
		require.Equal(t, []autogopaytest.Call{f.status(old.TransactionID), f.generate(700_000)}, f.callsSince(before))
		all := f.attempts(t, r)
		require.Len(t, all, 2)
		closed := byTransaction(t, all, old.TransactionID)
		require.Equal(t, qris.AttemptExpired, closed.Status)
		require.NotNil(t, closed.ClosedAt)
		require.Equal(t, 1, pendingCount(all))
		require.Equal(t, qris.StatusPending, v.Status)
	})

	t.Run("a QR paid meanwhile is recorded and no new QR is made", func(t *testing.T) {
		r := f.rent(t, "Wati Kusuma")
		f.makeQR(t, r, 700_000)
		old := f.only(t, r)
		require.NoError(t, f.fake.Pay(old.TransactionID))
		before := len(f.calls())

		v := f.makeQR(t, r, 300_000)
		require.Equal(t, []autogopaytest.Call{f.cancel(old.TransactionID)}, f.callsSince(before))
		require.Equal(t, qris.StatusPaid, v.Status)
		require.Equal(t, int64(700_000), v.Amount)
		a := f.only(t, r)
		require.Equal(t, qris.AttemptPaid, a.Status)
		require.NotNil(t, a.PaidAmount)
		require.Equal(t, int64(700_000), *a.PaidAmount)
		require.False(t, a.AmountMismatch)
		require.NotNil(t, a.ClosedAt)
		f.requireQRISPayment(t, r, old.TransactionID, 700_000)

		_, err := f.svc.MakeQR(context.Background(), r.token, 100_000)
		require.ErrorIs(t, err, qris.ErrNothingToPay, "the tenant owes nothing now")
	})

	t.Run("generates per tenant are capped, failed ones included", func(t *testing.T) {
		r := f.rent(t, "Joko Susilo")
		ctx := context.Background()
		before := len(f.calls())

		f.fake.FailNext(autogopaytest.MethodGenerate, autogopay.ErrUnavailable)
		_, err := f.svc.MakeQR(ctx, r.token, 100_000)
		require.ErrorIs(t, err, qris.ErrUnavailable) // 1
		// A second between makes keeps the attempts in creation order.
		for _, amount := range []int64{100_000, 100_000, 200_000, 300_000, 400_000} { // 2, reuse, 3, 4, 5
			env.Clock.Advance(time.Second)
			f.makeQR(t, r, amount)
		}
		generates := 0
		for _, c := range f.callsSince(before) {
			if c.Method == autogopaytest.MethodGenerate {
				generates++
			}
		}
		require.Equal(t, qris.MaxGeneratesPerDay, generates)
		live := f.attempts(t, r)
		last := live[len(live)-1]
		require.Equal(t, qris.AttemptPending, last.Status)
		require.Equal(t, int64(400_000), last.Amount)

		mark := len(f.calls())
		env.Clock.Advance(time.Second)
		_, err = f.svc.MakeQR(ctx, r.token, 500_000)
		require.ErrorIs(t, err, qris.ErrGenerateLimit)
		require.Empty(t, f.callsSince(mark), "at the cap, another amount reaches AutoGoPay not at all")
		all := f.attempts(t, r)
		require.Equal(t, qris.AttemptPending, byTransaction(t, all, last.TransactionID).Status, "the live QR stays payable")
		require.Equal(t, 1, pendingCount(all))

		v := f.makeQR(t, r, 400_000)
		require.Equal(t, qris.StatusPending, v.Status, "the same amount still reuses the live QR at the cap")
		require.Empty(t, f.callsSince(mark))
	})

	t.Run("a QR paid after the debt was settled another way becomes credit", func(t *testing.T) {
		r := f.rent(t, "Hendra Gunawan")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		_, err := env.Billing.RecordPayment(context.Background(), f.admin, billing.PaymentInput{
			RequestID: uuid.New(), TenantID: r.tenantID, Amount: 700_000,
			Method: billing.MethodCash, ReceivedOn: tenant.Today(env.Clock),
		})
		require.NoError(t, err)
		require.NoError(t, f.fake.Pay(a.TransactionID))

		require.Equal(t, qris.StatusPaid, f.check(t, r).Status)
		var qrisPays []paymentRow
		for _, p := range f.payments(t, r) {
			if p.Source == billing.SourceQRIS {
				qrisPays = append(qrisPays, p)
			}
		}
		require.Len(t, qrisPays, 1)
		require.Equal(t, int64(700_000), qrisPays[0].Amount)
		require.Equal(t, a.TransactionID, *qrisPays[0].SourceRef)
		require.Zero(t, qrisPays[0].Allocated, "nothing was left to pay")
		acct := f.account(t, r)
		require.Zero(t, acct.Outstanding)
		require.Equal(t, int64(700_000), acct.Credit, "the second payment is credit, not lost and not refunded")
	})

	t.Run("a generate failure makes no attempt", func(t *testing.T) {
		r := f.rent(t, "Sari Indah")
		before := len(f.calls())
		f.fake.FailNext(autogopaytest.MethodGenerate, autogopay.ErrUnavailable)

		_, err := f.svc.MakeQR(context.Background(), r.token, 700_000)
		require.ErrorIs(t, err, qris.ErrUnavailable)
		require.Equal(t, []autogopaytest.Call{f.generate(700_000)}, f.callsSince(before))
		require.Empty(t, f.attempts(t, r))
	})

	t.Run("a generate failure on a replace still commits the old QR's close", func(t *testing.T) {
		r := f.rent(t, "Hendra Gunawan")
		f.makeQR(t, r, 700_000)
		old := f.only(t, r)
		before := len(f.calls())
		f.fake.FailNext(autogopaytest.MethodGenerate, autogopay.ErrUnavailable)

		_, err := f.svc.MakeQR(context.Background(), r.token, 300_000)
		require.ErrorIs(t, err, qris.ErrUnavailable)
		require.Equal(t, []autogopaytest.Call{f.cancel(old.TransactionID), f.generate(300_000)}, f.callsSince(before))
		a := f.only(t, r)
		require.Equal(t, old.ID, a.ID)
		require.Equal(t, qris.AttemptCancelled, a.Status)
		require.NotNil(t, a.ClosedAt)
	})

	t.Run("amounts are checked before AutoGoPay is called", func(t *testing.T) {
		r := f.rent(t, "Rudi Hartono")
		tests := []struct {
			name   string
			amount int64
			want   error
		}{
			{"zero", 0, qris.ErrInvalidAmount},
			{"negative", -700_000, qris.ErrInvalidAmount},
			{"above Rp10 juta", 10_000_001, qris.ErrAmountAboveLimit},
			{"above what the tenant owes", 700_001, qris.ErrAmountAboveOutstanding},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				before := len(f.calls())
				_, err := f.svc.MakeQR(context.Background(), r.token, tc.amount)
				require.ErrorIs(t, err, tc.want)
				require.Empty(t, f.callsSince(before))
				require.Empty(t, f.attempts(t, r))
			})
		}

		t.Run("a tenant who owes nothing", func(t *testing.T) {
			paid := f.rent(t, "Yuni Astuti")
			_, err := env.Billing.RecordPayment(context.Background(), f.admin, billing.PaymentInput{
				RequestID: uuid.New(), TenantID: paid.tenantID, Amount: 700_000,
				Method: billing.MethodCash, ReceivedOn: tenant.Today(env.Clock),
			})
			require.NoError(t, err)
			before := len(f.calls())
			_, err = f.svc.MakeQR(context.Background(), paid.token, 700_000)
			require.ErrorIs(t, err, qris.ErrNothingToPay)
			require.Empty(t, f.callsSince(before))
			require.Empty(t, f.attempts(t, paid))
		})

		t.Run("part of what the tenant owes", func(t *testing.T) {
			before := len(f.calls())
			v := f.makeQR(t, r, 300_000)
			require.Equal(t, []autogopaytest.Call{f.generate(300_000)}, f.callsSince(before))
			require.Equal(t, qris.StatusPending, v.Status)
			require.Equal(t, int64(300_000), f.only(t, r).Amount)
		})
	})

	t.Run("an organization with no AutoGoPay key", func(t *testing.T) {
		nokey := s.newFlow(t, false)
		r := nokey.rent(t, "Lina Marlina")
		before := len(s.fake.Calls())

		_, err := nokey.svc.MakeQR(context.Background(), r.token, 700_000)
		require.ErrorIs(t, err, qris.ErrUnavailable)
		require.Len(t, s.fake.Calls(), before, "no gateway call")
		require.Empty(t, nokey.attempts(t, r))
	})
}

func TestQRISIsolation(t *testing.T) {
	s := newSuite(t)
	a := s.newFlow(t, true)
	b := s.newFlow(t, true)
	require.NotEqual(t, a.key, b.key)
	rina := a.rent(t, "Rina Wulandari")
	dewi := b.rent(t, "Dewi Lestari")

	a.makeQR(t, rina, 700_000)
	b.makeQR(t, dewi, 700_000)
	a.check(t, rina)
	b.check(t, dewi)
	aTx, bTx := a.only(t, rina).TransactionID, b.only(t, dewi).TransactionID

	t.Run("every call uses the organization's own key", func(t *testing.T) {
		for _, c := range s.fake.Calls() {
			switch {
			case c.Method == autogopaytest.MethodGenerate:
				require.Contains(t, []string{a.key, b.key}, c.Key)
			case c.TransactionID == aTx:
				require.Equal(t, a.key, c.Key)
			case c.TransactionID == bTx:
				require.Equal(t, b.key, c.Key)
			default:
				t.Fatalf("unexpected call %+v", c)
			}
		}
		require.Equal(t, []autogopaytest.Call{a.generate(700_000), a.status(aTx)}, a.calls())
		require.Equal(t, []autogopaytest.Call{b.generate(700_000), b.status(bTx)}, b.calls())
	})

	t.Run("one organization's reconcile never touches the other's attempts", func(t *testing.T) {
		s.env.Clock.Advance(16 * time.Minute)
		before := len(s.fake.Calls())

		res := a.reconcile(t)
		require.Equal(t, qris.ReconcileResult{Checked: 1, Closed: 1}, res)
		require.Equal(t, []autogopaytest.Call{a.status(aTx)}, s.fake.Calls()[before:])
		require.Equal(t, qris.AttemptExpired, a.only(t, rina).Status)
		require.Equal(t, qris.AttemptPending, b.only(t, dewi).Status)

		res = b.reconcile(t)
		require.Equal(t, qris.ReconcileResult{Checked: 1, Closed: 1}, res)
		require.Equal(t, qris.AttemptExpired, b.only(t, dewi).Status)
	})
}

func TestQRISBusy(t *testing.T) {
	s := newSuite(t)
	f := s.newFlow(t, true)
	r := f.rent(t, "Rina Wulandari")
	f.makeQR(t, r, 700_000)
	s.env.Clock.Advance(16 * time.Minute) // due for reconcile
	ctx := context.Background()
	before := len(f.calls())

	err := s.env.Store.WithOrgTx(ctx, f.org.ID, func(tx pgx.Tx) error {
		locked, err := dbgen.New(tx).TryLockTenantQris(ctx, r.tenantID)
		require.NoError(t, err)
		require.True(t, locked)

		_, err = f.svc.MakeQR(ctx, r.token, 700_000)
		require.ErrorIs(t, err, qris.ErrBusy)
		_, err = f.svc.Check(ctx, r.token)
		require.ErrorIs(t, err, qris.ErrBusy)
		res, err := f.svc.ReconcileOrg(ctx, f.org.ID)
		require.NoError(t, err)
		require.Equal(t, qris.ReconcileResult{Busy: 1}, res)
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, f.callsSince(before), "nothing reached AutoGoPay while the tenant was locked")
	a := f.only(t, r)
	require.Equal(t, qris.AttemptPending, a.Status)
	require.Nil(t, a.CheckRequestedAt)

	require.Equal(t, qris.ReconcileResult{Checked: 1, Closed: 1}, f.reconcile(t), "the next run takes it")
}

func TestCheck(t *testing.T) {
	s := newSuite(t)
	f := s.newFlow(t, true)
	ctx := context.Background()

	t.Run("no attempt", func(t *testing.T) {
		r := f.rent(t, "Rina Wulandari")
		before := len(f.calls())
		_, err := f.svc.Check(ctx, r.token)
		require.ErrorIs(t, err, qris.ErrNoAttempt)
		require.Empty(t, f.callsSince(before))
	})

	t.Run("a pending QR not paid yet is being checked", func(t *testing.T) {
		r := f.rent(t, "Budi Santoso")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		pending := f.publicStatus(t, r)
		require.Equal(t, qris.StatusPending, pending.Status)
		require.Nil(t, pending.CheckedAt)
		before := len(f.calls())
		firstCheck := s.env.Clock.Now()

		v := f.check(t, r)
		require.Equal(t, qris.StatusChecking, v.Status)
		require.NotNil(t, v.CheckedAt)
		require.True(t, v.CheckedAt.Equal(firstCheck), "checked at %s, want %s", v.CheckedAt, firstCheck)
		require.Equal(t, []autogopaytest.Call{f.status(a.TransactionID)}, f.callsSince(before))
		got := f.only(t, r)
		require.Equal(t, qris.AttemptPending, got.Status)
		require.NotNil(t, got.CheckRequestedAt)
		require.True(t, got.CheckRequestedAt.Equal(firstCheck))
		f.requireNoPayment(t, r)

		read := f.publicStatus(t, r)
		require.Equal(t, qris.StatusChecking, read.Status)
		require.NotNil(t, read.CheckedAt)
		require.True(t, read.CheckedAt.Equal(firstCheck))

		s.env.Clock.Advance(time.Minute)
		again := f.check(t, r)
		require.Equal(t, qris.StatusChecking, again.Status)
		require.NotNil(t, again.CheckedAt)
		require.True(t, again.CheckedAt.Equal(firstCheck), "a second check keeps the first time")
		require.True(t, f.only(t, r).CheckRequestedAt.Equal(firstCheck))
		require.True(t, f.publicStatus(t, r).CheckedAt.Equal(firstCheck))
	})

	t.Run("a QR paid at AutoGoPay records the payment", func(t *testing.T) {
		r := f.rent(t, "Siti Rahayu")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		require.NoError(t, f.fake.Pay(a.TransactionID))
		before := len(f.calls())

		v := f.check(t, r)
		require.Equal(t, qris.StatusPaid, v.Status)
		require.Nil(t, v.CheckedAt)
		read := f.publicStatus(t, r)
		require.Equal(t, qris.StatusPaid, read.Status)
		require.Nil(t, read.CheckedAt)
		require.Equal(t, []autogopaytest.Call{f.status(a.TransactionID)}, f.callsSince(before))
		got := f.only(t, r)
		require.Equal(t, qris.AttemptPaid, got.Status)
		require.Equal(t, int64(700_000), *got.PaidAmount)
		f.requireQRISPayment(t, r, a.TransactionID, 700_000)

		t.Run("a check on the paid attempt calls nothing", func(t *testing.T) {
			mark := len(f.calls())
			again := f.check(t, r)
			require.Equal(t, qris.StatusPaid, again.Status)
			require.Nil(t, again.CheckedAt)
			require.Empty(t, f.callsSince(mark))
			f.requireQRISPayment(t, r, a.TransactionID, 700_000)
		})
	})

	t.Run("a QR expired at AutoGoPay is closed", func(t *testing.T) {
		r := f.rent(t, "Agus Pratama")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		require.NoError(t, f.fake.Expire(a.TransactionID))
		before := len(f.calls())

		v := f.check(t, r)
		require.Equal(t, qris.StatusExpired, v.Status)
		require.Nil(t, v.CheckedAt)
		read := f.publicStatus(t, r)
		require.Equal(t, qris.StatusExpired, read.Status)
		require.Nil(t, read.CheckedAt)
		require.Equal(t, []autogopaytest.Call{f.status(a.TransactionID)}, f.callsSince(before))
		got := f.only(t, r)
		require.Equal(t, qris.AttemptExpired, got.Status)
		require.NotNil(t, got.ClosedAt)
		f.requireNoPayment(t, r)

		t.Run("a check on the closed attempt calls nothing", func(t *testing.T) {
			mark := len(f.calls())
			again := f.check(t, r)
			require.Equal(t, qris.StatusExpired, again.Status)
			require.Nil(t, again.CheckedAt)
			require.Empty(t, f.callsSince(mark))
		})
	})

	t.Run("a status failure answers checking and changes nothing else", func(t *testing.T) {
		r := f.rent(t, "Dewi Lestari")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		f.fake.FailNext(autogopaytest.MethodStatus, autogopay.ErrUnavailable)
		before := len(f.calls())

		v := f.check(t, r)
		require.Equal(t, qris.StatusChecking, v.Status)
		require.Equal(t, []autogopaytest.Call{f.status(a.TransactionID)}, f.callsSince(before))
		got := f.only(t, r)
		require.Equal(t, qris.AttemptPending, got.Status)
		require.Nil(t, got.ClosedAt)
		require.NotNil(t, got.CheckRequestedAt)
		f.requireNoPayment(t, r)
	})

	t.Run("checks past MaxChecksPerWindow skip the status read", func(t *testing.T) {
		r := f.rent(t, "Wati Kusuma")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		before := len(f.calls())
		for range qris.MaxChecksPerWindow {
			require.Equal(t, qris.StatusChecking, f.check(t, r).Status)
		}
		require.Len(t, f.callsSince(before), qris.MaxChecksPerWindow)

		mark := len(f.calls())
		v := f.check(t, r)
		require.Equal(t, qris.StatusChecking, v.Status)
		require.Empty(t, f.callsSince(mark))
		require.Equal(t, qris.AttemptPending, f.only(t, r).Status)
		require.Equal(t, a.ID, f.only(t, r).ID)
	})
}

func TestReconcileOrg(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	t.Run("a QR paid with no webhook is recorded", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Rina Wulandari")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		require.NoError(t, f.fake.Pay(a.TransactionID))
		s.env.Clock.Advance(16 * time.Minute)
		before := len(f.calls())

		require.Equal(t, qris.ReconcileResult{Checked: 1, Paid: 1}, f.reconcile(t))
		require.Equal(t, []autogopaytest.Call{f.status(a.TransactionID)}, f.callsSince(before))
		got := f.only(t, r)
		require.Equal(t, qris.AttemptPaid, got.Status)
		require.Equal(t, int64(700_000), *got.PaidAmount)
		f.requireQRISPayment(t, r, a.TransactionID, 700_000)
	})

	t.Run("a QR expired at AutoGoPay is closed", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Budi Santoso")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		s.env.Clock.Advance(16 * time.Minute)
		before := len(f.calls())

		require.Equal(t, qris.ReconcileResult{Checked: 1, Closed: 1}, f.reconcile(t))
		require.Equal(t, []autogopaytest.Call{f.status(a.TransactionID)}, f.callsSince(before))
		got := f.only(t, r)
		require.Equal(t, qris.AttemptExpired, got.Status)
		require.NotNil(t, got.ClosedAt)
		f.requireNoPayment(t, r)
	})

	t.Run("a live QR nobody checked is left alone", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Siti Rahayu")
		f.makeQR(t, r, 700_000)
		before := len(f.calls())

		require.Equal(t, qris.ReconcileResult{}, f.reconcile(t))
		require.Empty(t, f.callsSince(before))
		require.Equal(t, qris.AttemptPending, f.only(t, r).Status)
	})

	t.Run("a checked live QR still pending at AutoGoPay stays pending", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Agus Pratama")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		require.Equal(t, qris.StatusChecking, f.check(t, r).Status)
		before := len(f.calls())

		require.Equal(t, qris.ReconcileResult{Checked: 1}, f.reconcile(t))
		require.Equal(t, []autogopaytest.Call{f.status(a.TransactionID)}, f.callsSince(before))
		got := f.only(t, r)
		require.Equal(t, qris.AttemptPending, got.Status)
		require.Nil(t, got.ClosedAt)
		f.requireNoPayment(t, r)
	})

	t.Run("a QR past expiry that AutoGoPay does not know is closed as expired", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Dewi Lestari")
		p := f.attempt(r) // a random transaction id the fake never made
		require.NoError(t, f.insert(p))
		s.env.Clock.Advance(16 * time.Minute)
		before := len(f.calls())

		require.Equal(t, qris.ReconcileResult{Checked: 1, Closed: 1}, f.reconcile(t))
		require.Equal(t, []autogopaytest.Call{f.status(p.TransactionID)}, f.callsSince(before))
		got := f.only(t, r)
		require.Equal(t, qris.AttemptExpired, got.Status)
		require.NotNil(t, got.ClosedAt)
	})

	t.Run("a status failure counts as failed and changes nothing", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Wati Kusuma")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		require.NoError(t, f.fake.Pay(a.TransactionID))
		s.env.Clock.Advance(16 * time.Minute)
		f.fake.FailNext(autogopaytest.MethodStatus, autogopay.ErrUnavailable)

		res, err := f.svc.ReconcileOrg(ctx, f.org.ID)
		require.NoError(t, err)
		require.Equal(t, qris.ReconcileResult{Failed: 1}, res)
		got := f.only(t, r)
		require.Equal(t, qris.AttemptPending, got.Status)
		require.Nil(t, got.ClosedAt)
		f.requireNoPayment(t, r)

		require.Equal(t, qris.ReconcileResult{Checked: 1, Paid: 1}, f.reconcile(t), "the next run records it")
		f.requireQRISPayment(t, r, a.TransactionID, 700_000)
	})
}

func TestSettle(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	t.Run("check, reconcile and the webhook's settle record one payment", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Rina Wulandari")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)
		require.NoError(t, f.fake.Pay(a.TransactionID))

		require.Equal(t, qris.StatusPaid, f.check(t, r).Status)
		s.env.Clock.Advance(16 * time.Minute)
		require.Equal(t, qris.ReconcileResult{}, f.reconcile(t), "a paid attempt is not reconciled")
		require.NoError(t, f.svc.SettleForTest(ctx, f.org.ID, a.ID, 700_000))

		f.requireQRISPayment(t, r, a.TransactionID, 700_000)
		var allocated int64
		require.NoError(t, f.tx(func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT COALESCE(sum(amount), 0) FROM payment_allocation
				WHERE organization_id = $1 AND tenant_id = $2`, f.org.ID, r.tenantID).Scan(&allocated)
		}))
		require.Equal(t, int64(700_000), allocated)
		got := f.only(t, r)
		require.Equal(t, int64(700_000), *got.PaidAmount)
		require.False(t, got.AmountMismatch)
	})

	t.Run("a payment at another amount flags the attempt", func(t *testing.T) {
		f := s.newFlow(t, true)
		r := f.rent(t, "Budi Santoso")
		f.makeQR(t, r, 700_000)
		a := f.only(t, r)

		require.NoError(t, f.svc.SettleForTest(ctx, f.org.ID, a.ID, 650_000))
		got := f.only(t, r)
		require.Equal(t, qris.AttemptPaid, got.Status)
		require.Equal(t, int64(700_000), got.Amount)
		require.Equal(t, int64(650_000), *got.PaidAmount)
		require.True(t, got.AmountMismatch)
		f.requireQRISPayment(t, r, a.TransactionID, 650_000)

		require.NoError(t, f.svc.SettleForTest(ctx, f.org.ID, a.ID, 700_000))
		again := f.only(t, r)
		require.Equal(t, int64(650_000), *again.PaidAmount, "a second settle changes nothing")
		require.True(t, again.AmountMismatch)
		f.requireQRISPayment(t, r, a.TransactionID, 650_000)
	})
}

// TestLatestAttemptTie: when a closed attempt and the pending one share created_at, the pending
// one is still the latest. Otherwise the page would show "expired" for a QR that can still be
// paid, and MakeQR would plan a generate that the one-pending index refuses.
func TestLatestAttemptTie(t *testing.T) {
	s := newSuite(t)
	f := s.newFlow(t, true)
	r := f.rent(t, "Rina Wulandari")

	live := f.attempt(r)
	live.ID = uuid.MustParse("00000000-0000-4000-8000-000000000001")
	closed := f.attempt(r)
	closed.ID = uuid.MustParse("ffffffff-ffff-4fff-bfff-ffffffffffff")
	closed.Status = qris.AttemptCancelled
	closedAt := s.env.Clock.Now()
	closed.ClosedAt = &closedAt
	require.True(t, live.CreatedAt.Equal(closed.CreatedAt), "the two attempts share created_at")
	require.NoError(t, f.insert(closed))
	require.NoError(t, f.insert(live))

	require.Equal(t, qris.StatusPending, f.publicStatus(t, r).Status, "the live QR is the one shown")
	before := len(f.calls())
	v, err := f.svc.MakeQR(context.Background(), r.token, live.Amount)
	require.NoError(t, err, "the live QR for the same amount is reused")
	require.Equal(t, qris.StatusPending, v.Status)
	require.Empty(t, f.callsSince(before))
}

// hangingGateway is the fake with a Generate that waits until release is closed, as when
// AutoGoPay hangs. entered says a call is waiting.
type hangingGateway struct {
	*autogopaytest.Fake
	entered chan struct{}
	release chan struct{}
}

func (g *hangingGateway) Generate(ctx context.Context, key secret.Value, amount int64) (autogopay.QRIS, error) {
	g.entered <- struct{}{}
	<-g.release
	return g.Fake.Generate(ctx, key, amount)
}

// A hanging AutoGoPay holds at most MaxConcurrentCalls connections: the next QR request answers
// overloaded at once, a check leaves the read to reconcile, and the payment page, with its bank
// transfer fallback, still loads.
func TestAutoGoPayHangIsBounded(t *testing.T) {
	s := newSuite(t)
	f := s.newFlow(t, true)
	ctx := context.Background()
	gw := &hangingGateway{Fake: s.fake, entered: make(chan struct{}, 1), release: make(chan struct{})}
	bounded := qris.NewService(qris.Deps{
		Store: s.env.Store, Links: f.links, Clock: s.env.Clock, Gateway: gw, Keys: s.env.AutoGoPay,
		Limiter: s.env.Limiter, Audit: s.env.Authz, MaxConcurrentCalls: 1,
	})
	waiting, refused, checked := f.rent(t, "Rina Wulandari"), f.rent(t, "Budi Santoso"), f.rent(t, "Dewi Lestari")
	f.makeQR(t, checked, 700_000) // made before the hang

	done := make(chan error, 1)
	go func() {
		_, err := bounded.MakeQR(ctx, waiting.token, 700_000)
		done <- err
	}()
	<-gw.entered
	before := len(f.calls())

	_, err := bounded.MakeQR(ctx, refused.token, 700_000)
	require.ErrorIs(t, err, qris.ErrOverloaded)
	require.NotErrorIs(t, err, qris.ErrUnavailable, "busy for a moment is not an outage")
	v, err := bounded.Check(ctx, checked.token)
	require.NoError(t, err)
	require.Equal(t, qris.StatusChecking, v.Status, "the check is recorded; reconcile reads it")
	require.Empty(t, f.callsSince(before), "nothing else reached AutoGoPay")
	page, err := f.links.PublicPage(ctx, refused.token)
	require.NoError(t, err, "the page still loads")
	require.Equal(t, int64(700_000), page.Outstanding)

	close(gw.release)
	require.NoError(t, <-done)
	_, err = bounded.MakeQR(ctx, refused.token, 700_000)
	require.NoError(t, err, "once AutoGoPay answers, the place is free again")
}
