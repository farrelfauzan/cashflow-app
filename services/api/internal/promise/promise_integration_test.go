//go:build integration

package promise_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

const webOrigin = "https://app.test"

// listingAll is a page big enough for every promise a test makes.
var listingAll = listing.Page{Number: 1, Size: listing.MaxPageSize}

// promiseTables are the tables of this package.
var promiseTables = []string{"payment_promise", "promise_version", "promise_covered_invoice"}

// world is one organization with a kos, its admin, and the link and promise services.
type world struct {
	env      *testenv.Env
	org      testenv.Org
	admin    authz.Principal
	kos      property.Property
	links    *paylink.Service
	promises *promise.Service
	prefix   string
	n        int
}

// renter is a tenant with one monthly lease of Rp700.000 from the day it was let, its first
// invoice and a link to it.
type renter struct {
	tenantID, invoiceID, linkID uuid.UUID
	token                       string
}

func newWorld(t *testing.T, env *testenv.Env, name, prefix string) *world {
	t.Helper()
	w := &world{env: env, org: env.CreateOrg(t, name, prefix), prefix: prefix,
		links: paylink.NewService(env.Store, env.Authz, env.Clock, webOrigin)}
	w.promises = promise.NewService(promise.Deps{Store: env.Store, Links: w.links, Tenants: env.Billing, Clock: env.Clock})
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	var err error
	w.kos, err = env.Properties.Create(context.Background(), w.admin, property.PropertyInput{Name: "Kos Melati " + prefix, Kind: property.KindKos})
	require.NoError(t, err)
	return w
}

func (w *world) rent(t *testing.T, tenantName string) renter {
	t.Helper()
	ctx := context.Background()
	w.n++
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{
		Label: fmt.Sprintf("Kamar %d", w.n), Terms: testenv.Monthly(700_000),
	})
	require.NoError(t, err)
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: tenantName, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	_, err = w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: tenant.Today(w.env.Clock)},
	})
	require.NoError(t, err)
	_, err = w.env.Billing.GenerateOrg(ctx, w.org.ID)
	require.NoError(t, err)
	acct, err := w.env.Billing.Account(ctx, w.admin, tn.ID)
	require.NoError(t, err)
	require.Len(t, acct.Invoices, 1)
	link, err := w.links.Mint(ctx, w.admin, acct.Invoices[0].ID)
	require.NoError(t, err)
	token := strings.TrimPrefix(link.URL, webOrigin+"/t/")
	return renter{tenantID: tn.ID, invoiceID: acct.Invoices[0].ID, linkID: link.ID, token: token}
}

func (w *world) pay(t *testing.T, r renter, amount int64) {
	t.Helper()
	_, err := w.env.Billing.RecordPayment(context.Background(), w.admin, billing.PaymentInput{
		RequestID: uuid.New(), TenantID: r.tenantID, Amount: amount, Method: billing.MethodCash, ReceivedOn: tenant.Today(w.env.Clock),
	})
	require.NoError(t, err)
}

func (w *world) make(t *testing.T, r renter, in promise.Input) promise.Promise {
	t.Helper()
	p, err := w.promises.MakeFromLink(context.Background(), r.token, in)
	require.NoError(t, err)
	return p
}

func (w *world) list(t *testing.T, r renter) []promise.Promise {
	t.Helper()
	res, err := w.promises.List(context.Background(), w.admin, r.tenantID, listingAll)
	require.NoError(t, err)
	return res.Items
}

func (w *world) inTx(fn func(ctx context.Context, tx pgx.Tx) error) error {
	ctx := context.Background()
	return w.env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error { return fn(ctx, tx) })
}

func requireCode(t *testing.T, err error, code string) *pgconn.PgError {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
	return pgErr
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func ptr[T any](v T) *T { return &v }

func onDay(d time.Time) promise.Input {
	return promise.Input{Kind: promise.KindSpecificDate, PromisedDate: &d}
}

func requireFieldErr(t *testing.T, err, target error, field string) {
	t.Helper()
	require.ErrorIs(t, err, target)
	var fe *promise.FieldError
	require.ErrorAs(t, err, &fe)
	require.Equal(t, field, fe.Field)
}

// TestMakeFromLink: the clock starts on 2 Nov 2026 (Asia/Jakarta); each lease starts that day.
func TestMakeFromLink(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	w := newWorld(t, env, "Griya Sejahtera", "0812951")
	rina := w.rent(t, "Rina Wulandari")
	ctx := context.Background()

	var first promise.Promise
	t.Run("a promise is made and covers the open invoices", func(t *testing.T) {
		first = w.make(t, rina, promise.Input{
			Kind: promise.KindSpecificDate, PromisedDate: ptr(day(2026, 11, 10)), PromisedAmount: ptr(int64(500_000)), PayingNow: 200_000,
		})
		require.Equal(t, promise.StatusActive, first.Status)
		require.Equal(t, rina.tenantID, first.TenantID)
		require.Len(t, first.Versions, 1)
		v := first.Latest()
		require.Equal(t, 1, v.Number)
		require.Equal(t, promise.KindSpecificDate, v.Kind)
		require.Equal(t, day(2026, 11, 10), v.PromisedDate)
		require.Equal(t, ptr(int64(500_000)), v.PromisedAmount)
		require.Equal(t, int64(200_000), v.PayingNow)
		require.Equal(t, int64(700_000), v.OutstandingAt)
		require.Equal(t, promise.SourceTenantPage, v.Source)
		require.Nil(t, v.CreatedBy)
		require.Empty(t, v.Note)
		require.Equal(t, []uuid.UUID{rina.invoiceID}, v.CoveredInvoiceIDs)

		var link uuid.NullUUID
		require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT payment_link_id FROM promise_version WHERE id = $1", v.ID).Scan(&link)
		}))
		require.Equal(t, uuid.NullUUID{UUID: rina.linkID, Valid: true}, link, "the version names the link it was made through")
	})

	t.Run("making it again is an edit: the next version of the same promise, covering what is open now", func(t *testing.T) {
		// The second month's invoice is made on 25 Nov (7 days ahead of 2 Dec). Rina first moves
		// her date, so paying the first invoice next does not keep the promise: version 2 covers
		// both invoices and promised no amount (paying version 1's Rp700.000 target would have
		// closed it as kept).
		env.Clock.Set(time.Date(2026, 11, 26, 9, 0, 0, 0, clock.Jakarta))
		_, err := env.Billing.GenerateOrg(ctx, w.org.ID)
		require.NoError(t, err)
		acct, err := env.Billing.Account(ctx, w.admin, rina.tenantID)
		require.NoError(t, err)
		require.Len(t, acct.Invoices, 2)
		second := acct.Invoices[1].ID
		moved := w.make(t, rina, onDay(day(2026, 11, 30)))
		require.Equal(t, first.ID, moved.ID)
		require.Equal(t, []uuid.UUID{rina.invoiceID, second}, moved.Latest().CoveredInvoiceIDs)
		w.pay(t, rina, 700_000)
		require.Equal(t, promise.StatusActive, w.list(t, rina)[0].Status, "the second invoice is still owed")

		got := w.make(t, rina, promise.Input{Kind: promise.KindWithNextInvoice})
		require.Equal(t, first.ID, got.ID)
		require.Len(t, got.Versions, 3)
		require.Equal(t, first.Versions[0].ID, got.Versions[0].ID, "version 1 is kept as it was")
		v := got.Latest()
		require.Equal(t, 3, v.Number)
		require.Equal(t, promise.KindWithNextInvoice, v.Kind)
		require.Equal(t, day(2026, 12, 2), v.PromisedDate, "the next invoice's due date, set by the server")
		require.Nil(t, v.PromisedAmount)
		require.Equal(t, int64(700_000), v.OutstandingAt)
		require.Equal(t, []uuid.UUID{second}, v.CoveredInvoiceIDs, "the paid invoice is no longer covered")
		require.Equal(t, []uuid.UUID{rina.invoiceID}, got.Versions[0].CoveredInvoiceIDs)

		listed := w.list(t, rina)
		require.Len(t, listed, 1)
		require.Equal(t, got, listed[0])
	})

	t.Run("the payment page shows the active promise and what a new one may say", func(t *testing.T) {
		page, err := w.links.PublicPage(ctx, rina.token)
		require.NoError(t, err)
		require.NotNil(t, page.Promise.Active)
		require.Equal(t, 3, page.Promise.Active.Number)
		require.Equal(t, day(2026, 12, 2), page.Promise.Active.PromisedDate)
		require.NotNil(t, page.Promise.Window)
		require.Equal(t, day(2026, 11, 27), page.Promise.Window.Earliest)
		require.Equal(t, day(2027, 1, 1), page.Promise.Window.Latest, "an edit stays within 60 days of the first version (2 Nov)")
		require.Equal(t, ptr(day(2026, 12, 2)), page.Promise.Window.NextDueInRange())
	})

	// Decision 41: an active promise silences H+3 and H+7 for at most 60 days, so editing it
	// never moves its date past 60 days from its first version.
	t.Run("an edit cannot go past 60 days from the first version", func(t *testing.T) {
		_, err := w.promises.MakeFromLink(ctx, rina.token, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: ptr(day(2027, 1, 2))})
		var fe *promise.FieldError
		require.ErrorAs(t, err, &fe)
		require.ErrorIs(t, err, promise.ErrDateOutOfRange)
		require.Equal(t, "promisedDate", fe.Field)

		got := w.make(t, rina, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: ptr(day(2027, 1, 1))})
		require.Equal(t, 4, got.Latest().Number)
		require.Equal(t, day(2027, 1, 1), got.Latest().PromisedDate)

		// On the bound itself no later day is left: the page offers no dates and an edit is refused.
		// Rina's first link expired 30 days after its invoice was paid, so use a link of the
		// invoice still open.
		acct, err := env.Billing.Account(ctx, w.admin, rina.tenantID)
		require.NoError(t, err)
		var open uuid.UUID
		for _, inv := range acct.Invoices {
			if inv.Outstanding > 0 {
				open = inv.ID
				break
			}
		}
		require.NotEqual(t, uuid.Nil, open)
		link, err := w.links.Mint(ctx, w.admin, open)
		require.NoError(t, err)
		rina.token = strings.TrimPrefix(link.URL, webOrigin+"/t/")
		env.Clock.Set(time.Date(2027, 1, 1, 9, 0, 0, 0, clock.Jakarta))
		page, err := w.links.PublicPage(ctx, rina.token)
		require.NoError(t, err)
		require.NotNil(t, page.Promise.Window)
		require.True(t, page.Promise.Window.Empty())
		_, err = w.promises.MakeFromLink(ctx, rina.token, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: ptr(day(2027, 1, 2))})
		require.ErrorIs(t, err, promise.ErrDateOutOfRange)
	})
}

func TestMakeRefusals(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	w := newWorld(t, env, "Griya Sejahtera", "0812952")
	rina := w.rent(t, "Rina Wulandari")
	ctx := context.Background()

	tests := []struct {
		name  string
		in    promise.Input
		check func(t *testing.T, err error)
	}{
		{
			name: "a date sent for with_next_invoice",
			in:   promise.Input{Kind: promise.KindWithNextInvoice, PromisedDate: ptr(day(2026, 12, 2))},
			check: func(t *testing.T, err error) {
				var ie *promise.InputError
				require.ErrorAs(t, err, &ie)
				require.Equal(t, "promisedDate", ie.Field)
			},
		},
		{
			name:  "today",
			in:    onDay(day(2026, 11, 2)),
			check: func(t *testing.T, err error) { requireFieldErr(t, err, promise.ErrDateOutOfRange, "promisedDate") },
		},
		{
			name:  "61 days ahead",
			in:    onDay(day(2027, 1, 2)),
			check: func(t *testing.T, err error) { requireFieldErr(t, err, promise.ErrDateOutOfRange, "promisedDate") },
		},
		{
			name: "an amount above what is owed",
			in:   promise.Input{Kind: promise.KindSpecificDate, PromisedDate: ptr(day(2026, 11, 10)), PromisedAmount: ptr(int64(700_001))},
			check: func(t *testing.T, err error) {
				requireFieldErr(t, err, promise.ErrAmountAboveOutstanding, "promisedAmount")
			},
		},
		{
			name: "paying more than is owed now",
			in:   promise.Input{Kind: promise.KindSpecificDate, PromisedDate: ptr(day(2026, 11, 10)), PayingNow: 700_001},
			check: func(t *testing.T, err error) {
				requireFieldErr(t, err, promise.ErrAmountAboveOutstanding, "payingNow")
			},
		},
		{
			name:  "paying everything now",
			in:    promise.Input{Kind: promise.KindSpecificDate, PromisedDate: ptr(day(2026, 11, 10)), PayingNow: 700_000},
			check: func(t *testing.T, err error) { require.ErrorIs(t, err, promise.ErrNothingOwed) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := w.promises.MakeFromLink(ctx, rina.token, tc.in)
			tc.check(t, err)
		})
	}
	require.Empty(t, w.list(t, rina), "nothing was saved")

	t.Run("a tenant whose lease reaches its planned end has no next invoice", func(t *testing.T) {
		budi := w.rent(t, "Budi Santoso")
		acct, err := env.Billing.Account(ctx, w.admin, budi.tenantID)
		require.NoError(t, err)
		_, err = env.Tenants.SetPlannedEnd(ctx, w.admin, acct.Invoices[0].LeaseID, ptr(day(2026, 11, 30)))
		require.NoError(t, err)
		_, err = w.promises.MakeFromLink(ctx, budi.token, promise.Input{Kind: promise.KindWithNextInvoice})
		requireFieldErr(t, err, promise.ErrNoNextInvoice, "kind")
		page, err := w.links.PublicPage(ctx, budi.token)
		require.NoError(t, err)
		require.Nil(t, page.Promise.Window.NextDueInRange())
	})

	t.Run("a tenant who owes nothing", func(t *testing.T) {
		w.pay(t, rina, 700_000)
		_, err := w.promises.MakeFromLink(ctx, rina.token, onDay(day(2026, 11, 10)))
		require.ErrorIs(t, err, promise.ErrNothingOwed)
		page, err := w.links.PublicPage(ctx, rina.token)
		require.NoError(t, err)
		require.Nil(t, page.Promise.Active)
		require.Nil(t, page.Promise.Window, "no promise dates when nothing is owed")
	})

	t.Run("a revoked or unknown token answers like the page", func(t *testing.T) {
		dewi := w.rent(t, "Dewi Lestari")
		_, err := w.links.RevokeAll(ctx, w.admin, dewi.invoiceID)
		require.NoError(t, err)
		_, err = w.promises.MakeFromLink(ctx, dewi.token, onDay(day(2026, 11, 10)))
		require.ErrorIs(t, err, paylink.ErrLinkNotFound)
		_, err = w.promises.MakeFromLink(ctx, strings.Repeat("A", 43), onDay(day(2026, 11, 10)))
		require.ErrorIs(t, err, paylink.ErrLinkNotFound)
		_, err = w.promises.MakeFromLink(ctx, "garbage", onDay(day(2026, 11, 10)))
		require.ErrorIs(t, err, paylink.ErrLinkNotFound)
	})
}

// TestConcurrentMakes: two makes for one tenant at once end as one promise with versions 1 and 2,
// never a unique violation. Both wait on the tenant's lock while a third transaction holds it, so
// they really race once it is released.
func TestConcurrentMakes(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	w := newWorld(t, env, "Griya Sejahtera", "0812953")
	rina := w.rent(t, "Rina Wulandari")

	errs := make([]error, 2)
	var wg sync.WaitGroup
	err := w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		if _, err := dbgen.New(tx).LockTenantForAllocation(ctx, dbgen.LockTenantForAllocationParams{OrganizationID: w.org.ID, TenantID: rina.tenantID}); err != nil {
			return err
		}
		for i := range errs {
			wg.Go(func() {
				_, errs[i] = w.promises.MakeFromLink(ctx, rina.token, onDay(day(2026, 11, 10+i)))
			})
		}
		waitForLockWaiters(ctx, t, tx, 2)
		return nil
	})
	require.NoError(t, err)
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	got := w.list(t, rina)
	require.Len(t, got, 1, "one promise")
	require.Len(t, got[0].Versions, 2)
	require.Equal(t, 1, got[0].Versions[0].Number)
	require.Equal(t, 2, got[0].Versions[1].Number)
}

// waitForLockWaiters polls until n sessions wait on a lock. The first make waits on tx's
// transaction, the second on the first's place in the row's queue (a tuple lock). It reads
// pg_locks, which is live; pg_stat_activity is a snapshot taken once per transaction.
func waitForLockWaiters(ctx context.Context, t *testing.T, tx pgx.Tx, n int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var waiting int
		require.NoError(t, tx.QueryRow(ctx, `SELECT count(DISTINCT pid) FROM pg_locks WHERE NOT granted`).Scan(&waiting))
		if waiting >= n {
			return
		}
		require.True(t, time.Now().Before(deadline), "only %d makes waited on the tenant's lock", waiting)
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPromiseRowsAreAppendOnly(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	w := newWorld(t, env, "Griya Sejahtera", "0812954")
	rina := w.rent(t, "Rina Wulandari")
	made := w.make(t, rina, onDay(day(2026, 11, 10)))

	appendOnly := []string{"promise_version", "promise_covered_invoice"}
	statements := map[string]string{
		"update":   "UPDATE %s SET organization_id = organization_id",
		"delete":   "DELETE FROM %s",
		"truncate": "TRUNCATE %s CASCADE",
	}

	t.Run("the app role has no privilege to change them", func(t *testing.T) {
		for _, table := range appendOnly {
			for verb, stmt := range statements {
				t.Run(table+" "+verb, func(t *testing.T) {
					err := w.inTx(func(ctx context.Context, tx pgx.Tx) error {
						_, err := tx.Exec(ctx, fmt.Sprintf(stmt, table))
						return err
					})
					require.Contains(t, requireCode(t, err, "42501").Message, "permission denied")
				})
			}
		}
	})

	t.Run("the owner is stopped by the trigger", func(t *testing.T) {
		conn, err := pgx.Connect(ctx, env.URLs.Owner)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, conn.Close(ctx)) })
		for _, table := range appendOnly {
			for verb, stmt := range statements {
				t.Run(table+" "+verb, func(t *testing.T) {
					_, err := conn.Exec(ctx, fmt.Sprintf(stmt, table))
					require.Contains(t, requireCode(t, err, "42501").Message, "is append-only")
				})
			}
		}
	})

	t.Run("the app role may not delete a promise or change more than its status", func(t *testing.T) {
		for name, stmt := range map[string]string{
			"delete":        "DELETE FROM payment_promise",
			"truncate":      "TRUNCATE payment_promise CASCADE",
			"change tenant": "UPDATE payment_promise SET tenant_id = tenant_id",
		} {
			t.Run(name, func(t *testing.T) {
				err := w.inTx(func(ctx context.Context, tx pgx.Tx) error {
					_, err := tx.Exec(ctx, stmt)
					return err
				})
				require.Contains(t, requireCode(t, err, "42501").Message, "permission denied")
			})
		}
	})

	t.Run("a closed promise never changes again", func(t *testing.T) {
		closeAs := func(status string) error {
			return w.inTx(func(ctx context.Context, tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "UPDATE payment_promise SET status = $2, closed_at = now() WHERE id = $1", made.ID, status)
				return err
			})
		}
		require.NoError(t, closeAs(promise.StatusBroken), "the app role closes an active promise")
		require.Contains(t, requireCode(t, closeAs(promise.StatusKept), "42501").Message, "is closed")

		t.Run("and a new one can be made after it", func(t *testing.T) {
			next := w.make(t, rina, onDay(day(2026, 11, 12)))
			require.NotEqual(t, made.ID, next.ID)
			require.Equal(t, 1, next.Latest().Number)
		})
	})
}

func TestPromiseSchema(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	w := newWorld(t, env, "Griya Sejahtera", "0812955")
	rina := w.rent(t, "Rina Wulandari")
	budi := w.rent(t, "Budi Santoso")
	made := w.make(t, rina, onDay(day(2026, 11, 10)))

	version := func(mod func(p *dbgen.InsertPromiseVersionParams)) dbgen.InsertPromiseVersionParams {
		p := dbgen.InsertPromiseVersionParams{
			ID: uuid.New(), OrganizationID: w.org.ID, PromiseID: made.ID, TenantID: rina.tenantID, Version: 9,
			Kind: promise.KindSpecificDate, PromisedDate: day(2026, 11, 20), PayingNow: 0, OutstandingAt: 700_000,
			Source: promise.SourceTenantPage, PaymentLinkID: uuid.NullUUID{UUID: rina.linkID, Valid: true},
			CreatedAt: env.Clock.Now(),
		}
		mod(&p)
		return p
	}
	tests := []struct {
		name       string
		insert     func(ctx context.Context, q *dbgen.Queries) error
		code       string
		constraint string
	}{
		{
			name: "a second active promise for one tenant",
			insert: func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPaymentPromise(ctx, dbgen.InsertPaymentPromiseParams{ID: uuid.New(), OrganizationID: w.org.ID, TenantID: rina.tenantID, CreatedAt: env.Clock.Now()})
			},
			code: "23505", constraint: "payment_promise_one_active",
		},
		{
			name: "a version number taken",
			insert: func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPromiseVersion(ctx, version(func(p *dbgen.InsertPromiseVersionParams) { p.Version = 1 }))
			},
			code: "23505", constraint: "promise_version_promise_id_version_key",
		},
		{
			name: "a link of another tenant",
			insert: func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPromiseVersion(ctx, version(func(p *dbgen.InsertPromiseVersionParams) { p.PaymentLinkID.UUID = budi.linkID }))
			},
			code: "23503", constraint: "promise_version_link_same_tenant",
		},
		{
			name: "the tenant page with no link",
			insert: func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPromiseVersion(ctx, version(func(p *dbgen.InsertPromiseVersionParams) { p.PaymentLinkID = uuid.NullUUID{} }))
			},
			code: "23514", constraint: "promise_version_link_from_page",
		},
		{
			name: "a note from the tenant page",
			insert: func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPromiseVersion(ctx, version(func(p *dbgen.InsertPromiseVersionParams) { p.Note = ptr("catatan") }))
			},
			code: "23514", constraint: "promise_version_note_from_staff",
		},
		{
			name: "nothing left after paying now",
			insert: func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPromiseVersion(ctx, version(func(p *dbgen.InsertPromiseVersionParams) { p.PayingNow = 700_000 }))
			},
			code: "23514", constraint: "promise_version_something_promised",
		},
		{
			name: "a promised amount above the rest",
			insert: func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPromiseVersion(ctx, version(func(p *dbgen.InsertPromiseVersionParams) {
					p.PayingNow, p.PromisedAmount = 200_000, ptr(int64(500_001))
				}))
			},
			code: "23514", constraint: "promise_version_amount_fits",
		},
		{
			name: "an invoice of another tenant covered",
			insert: func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPromiseCoveredInvoices(ctx, dbgen.InsertPromiseCoveredInvoicesParams{
					OrganizationID: w.org.ID, PromiseVersionID: made.Latest().ID, InvoiceIds: []uuid.UUID{budi.invoiceID}, TenantID: rina.tenantID,
				})
			},
			code: "23503", constraint: "promise_covered_invoice_same_tenant",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := w.inTx(func(ctx context.Context, tx pgx.Tx) error { return tc.insert(ctx, dbgen.New(tx)) })
			pgErr := requireCode(t, err, tc.code)
			require.Equal(t, tc.constraint, pgErr.ConstraintName)
		})
	}
}

// TestPromiseIsolation: each organization sees and writes only its own promise rows, and a
// token of one organization reaches only its own tenant.
func TestPromiseIsolation(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	a := newWorld(t, env, "Griya Sejahtera", "0812956")
	b := newWorld(t, env, "Kos Bulanan Sejahtera", "0812957")
	rina, dewi := a.rent(t, "Rina Wulandari"), b.rent(t, "Dewi Lestari")
	pa := a.make(t, rina, onDay(day(2026, 11, 10)))
	pb := b.make(t, dewi, onDay(day(2026, 11, 11)))
	require.Equal(t, rina.tenantID, pa.TenantID, "a's token reaches a's tenant")
	require.Equal(t, dewi.tenantID, pb.TenantID, "b's token reaches b's tenant")

	t.Run("each organization sees only its own rows, in every table", func(t *testing.T) {
		for _, w := range []*world{a, b} {
			for _, table := range promiseTables {
				var all, foreign int
				require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
					return tx.QueryRow(ctx, fmt.Sprintf(
						"SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM %s", table)).Scan(&all, &foreign)
				}))
				require.Positive(t, all, "%s should have rows of its own organization", table)
				require.Zero(t, foreign, "%s shows another organization's rows", table)
			}
		}
	})

	t.Run("another organization's promise is not found", func(t *testing.T) {
		require.NoError(t, a.inTx(func(ctx context.Context, tx pgx.Tx) error {
			for _, check := range []struct{ table, col string }{
				{"payment_promise", "tenant_id"}, {"promise_version", "tenant_id"}, {"promise_covered_invoice", "tenant_id"},
			} {
				var n int
				require.NoError(t, tx.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s WHERE %s = $1", check.table, check.col), dewi.tenantID).Scan(&n))
				require.Zero(t, n, check.table)
			}
			_, err := dbgen.New(tx).GetActivePromise(ctx, dbgen.GetActivePromiseParams{OrganizationID: b.org.ID, TenantID: dewi.tenantID})
			require.ErrorIs(t, err, pgx.ErrNoRows)
			return nil
		}))
		_, err := a.promises.List(context.Background(), a.admin, dewi.tenantID, listingAll)
		var nf *billing.NotFoundError
		require.ErrorAs(t, err, &nf)
	})

	t.Run("no row can be written for another organization", func(t *testing.T) {
		inserts := map[string]func(ctx context.Context, q *dbgen.Queries) error{
			"payment_promise": func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPaymentPromise(ctx, dbgen.InsertPaymentPromiseParams{ID: uuid.New(), OrganizationID: b.org.ID, TenantID: dewi.tenantID, CreatedAt: env.Clock.Now()})
			},
			"promise_version": func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPromiseVersion(ctx, dbgen.InsertPromiseVersionParams{
					ID: uuid.New(), OrganizationID: b.org.ID, PromiseID: pb.ID, TenantID: dewi.tenantID, Version: 2,
					Kind: promise.KindSpecificDate, PromisedDate: day(2026, 11, 20), OutstandingAt: 700_000,
					Source: promise.SourceTenantPage, PaymentLinkID: uuid.NullUUID{UUID: dewi.linkID, Valid: true}, CreatedAt: env.Clock.Now(),
				})
			},
			"promise_covered_invoice": func(ctx context.Context, q *dbgen.Queries) error {
				return q.InsertPromiseCoveredInvoices(ctx, dbgen.InsertPromiseCoveredInvoicesParams{
					OrganizationID: b.org.ID, PromiseVersionID: pb.Latest().ID, InvoiceIds: []uuid.UUID{uuid.New()}, TenantID: dewi.tenantID,
				})
			},
		}
		for _, table := range promiseTables {
			t.Run(table, func(t *testing.T) {
				err := a.inTx(func(ctx context.Context, tx pgx.Tx) error { return inserts[table](ctx, dbgen.New(tx)) })
				require.Contains(t, requireCode(t, err, "42501").Message, "row-level security")
			})
		}
	})
}
