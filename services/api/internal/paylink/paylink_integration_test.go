//go:build integration

package paylink_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

const webOrigin = "https://app.test"

// world is one organization with its three members, a kos, and the payment link service.
type world struct {
	env                         *testenv.Env
	org                         testenv.Org
	admin, karyawan, supervisor authz.Principal
	kos                         property.Property
	svc                         *paylink.Service
	prefix                      string
	n                           int
}

// rental is a tenant with one monthly lease and its first invoice.
type rental struct {
	tenantID  uuid.UUID
	leaseID   uuid.UUID
	invoiceID uuid.UUID
}

func newWorld(t *testing.T, env *testenv.Env, name, prefix string) *world {
	t.Helper()
	w := &world{env: env, org: env.CreateOrg(t, name, prefix), prefix: prefix,
		svc: paylink.NewService(env.Store, env.Authz, env.Clock, webOrigin)}
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	w.karyawan = env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	w.supervisor = env.Principal(t, w.org.ID, w.org.Supervisor.UserID)
	var err error
	w.kos, err = env.Properties.Create(context.Background(), w.admin, property.PropertyInput{Name: "Kos Melati " + prefix, Kind: property.KindKos})
	require.NoError(t, err)
	return w
}

// rent makes a unit, a tenant and a monthly lease starting today, collected by collector (a
// member id, or uuid.Nil), and generates its first invoice.
func (w *world) rent(t *testing.T, tenantName string, collector uuid.UUID, price int64) rental {
	t.Helper()
	ctx := context.Background()
	w.n++
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{
		Label: fmt.Sprintf("Kamar %d", w.n), Terms: testenv.Monthly(price),
	})
	require.NoError(t, err)
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: tenantName, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	l, err := w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: tenant.Today(w.env.Clock), CollectorMemberID: collector},
	})
	require.NoError(t, err)
	_, err = w.env.Billing.GenerateOrg(ctx, w.org.ID)
	require.NoError(t, err)
	acct, err := w.env.Billing.Account(ctx, w.admin, tn.ID)
	require.NoError(t, err)
	require.Len(t, acct.Invoices, 1)
	return rental{tenantID: tn.ID, leaseID: l.ID, invoiceID: acct.Invoices[0].ID}
}

func (w *world) pay(t *testing.T, r rental, amount int64) {
	t.Helper()
	_, err := w.env.Billing.RecordPayment(context.Background(), w.admin, billing.PaymentInput{
		RequestID: uuid.New(), TenantID: r.tenantID, Amount: amount, Method: billing.MethodCash, ReceivedOn: tenant.Today(w.env.Clock),
	})
	require.NoError(t, err)
}

func (w *world) mint(t *testing.T, p authz.Principal, invoiceID uuid.UUID) string {
	t.Helper()
	link, err := w.svc.Mint(context.Background(), p, invoiceID)
	require.NoError(t, err)
	require.Equal(t, invoiceID, link.InvoiceID)
	token, ok := strings.CutPrefix(link.URL, webOrigin+"/t/")
	require.True(t, ok, link.URL)
	require.Regexp(t, `^[A-Za-z0-9_-]{43}$`, token)
	return token
}

func (w *world) links(t *testing.T) []dbgen.PaymentLink {
	t.Helper()
	ctx := context.Background()
	var out []dbgen.PaymentLink
	require.NoError(t, w.env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT * FROM payment_link ORDER BY created_at, id")
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[dbgen.PaymentLink])
		return err
	}))
	return out
}

func TestMintAndReadThePage(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "Griya Sejahtera", "0812611")
	rina := w.rent(t, "Rina Wulandari", uuid.Nil, 700_000)

	first := w.mint(t, w.admin, rina.invoiceID)
	page, err := w.svc.PublicPage(ctx, first)
	require.NoError(t, err)
	require.Equal(t, "Griya Sejahtera", page.OrganizationName)
	require.Equal(t, "Rina Wulandari", page.TenantName)
	require.Equal(t, rina.invoiceID, page.Invoice.ID)
	require.Equal(t, "Kamar 1", page.Invoice.UnitLabel)
	require.Equal(t, "Kos Melati 0812611", page.Invoice.PropertyName)
	require.Equal(t, billing.InvoiceRegular, page.Invoice.Kind)
	require.EqualValues(t, 700_000, page.Invoice.Amount)
	require.EqualValues(t, 700_000, page.Invoice.Balance)
	require.NotEmpty(t, page.Invoice.Lines)
	require.EqualValues(t, 700_000, page.Outstanding)
	require.False(t, page.Paid)
	require.Len(t, page.OpenInvoices, 1)
	require.Nil(t, page.BankAccount, "no bank account set yet")

	t.Run("each mint is a new link, and every one works", func(t *testing.T) {
		second := w.mint(t, w.admin, rina.invoiceID)
		require.NotEqual(t, first, second)
		for _, token := range []string{first, second} {
			_, err := w.svc.PublicPage(ctx, token)
			require.NoError(t, err)
		}
		links := w.links(t)
		require.Len(t, links, 2)
		for _, l := range links {
			require.Equal(t, rina.tenantID, l.TenantID)
			require.Equal(t, uuid.NullUUID{UUID: w.org.Admin.UserID, Valid: true}, l.CreatedByStaffUserID)
			require.Len(t, l.TokenHash, 32)
		}
	})

	t.Run("the bank account shows once set", func(t *testing.T) {
		_, err := w.svc.SetBankAccount(ctx, w.org.ID, paylink.BankAccount{BankName: "BCA", AccountNumber: "123 456 7890", AccountHolder: "Suhwendi"})
		require.NoError(t, err)
		page, err := w.svc.PublicPage(ctx, first)
		require.NoError(t, err)
		require.Equal(t, &paylink.BankAccount{BankName: "BCA", AccountNumber: "123 456 7890", AccountHolder: "Suhwendi"}, page.BankAccount)
	})

	t.Run("a partial payment lowers what is owed; a full one turns the page paid", func(t *testing.T) {
		w.pay(t, rina, 200_000)
		page, err := w.svc.PublicPage(ctx, first)
		require.NoError(t, err)
		require.EqualValues(t, 500_000, page.Outstanding)
		require.EqualValues(t, 500_000, page.Invoice.Balance)
		require.False(t, page.Paid)

		w.pay(t, rina, 550_000)
		page, err = w.svc.PublicPage(ctx, first)
		require.NoError(t, err)
		require.True(t, page.Paid)
		require.Zero(t, page.Outstanding)
		require.EqualValues(t, 50_000, page.Credit)
		require.Empty(t, page.OpenInvoices)
		require.NotNil(t, page.OpenInvoices, "an empty list, not nil")
		require.Zero(t, page.Invoice.Balance)
	})

	t.Run("minting is audited with the member who did it", func(t *testing.T) {
		var n int
		for _, row := range env.AuditLog(t, w.org.ID) {
			if row.Action == authz.ActionInvoiceShare {
				require.Equal(t, "allow", row.Decision)
				require.Equal(t, w.org.Admin.UserID, row.ActorStaffUserID.UUID)
				require.Contains(t, row.Resource, "invoice/"+rina.invoiceID.String())
				n++
			}
		}
		require.Equal(t, 2, n)
	})
}

func TestRevokeStopsEveryLinkOfTheInvoice(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "Griya Sejahtera", "0812612")
	rina := w.rent(t, "Rina Wulandari", uuid.Nil, 700_000)
	budi := w.rent(t, "Budi Santoso", uuid.Nil, 650_000)
	a, b := w.mint(t, w.admin, rina.invoiceID), w.mint(t, w.admin, rina.invoiceID)
	other := w.mint(t, w.admin, budi.invoiceID)

	n, err := w.svc.RevokeAll(ctx, w.admin, rina.invoiceID)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	for _, token := range []string{a, b} {
		_, err := w.svc.PublicPage(ctx, token)
		require.ErrorIs(t, err, paylink.ErrLinkNotFound)
	}
	_, err = w.svc.PublicPage(ctx, other)
	require.NoError(t, err, "another invoice's link still works")

	n, err = w.svc.RevokeAll(ctx, w.admin, rina.invoiceID)
	require.NoError(t, err)
	require.Zero(t, n, "nothing left to revoke is not an error")

	fresh := w.mint(t, w.admin, rina.invoiceID)
	_, err = w.svc.PublicPage(ctx, fresh)
	require.NoError(t, err, "a link made after revoking works")

	var revokes int
	for _, row := range env.AuditLog(t, w.org.ID) {
		if row.Action == authz.ActionInvoiceShare && strings.HasSuffix(row.Resource, "invoice/"+rina.invoiceID.String()) {
			revokes++
		}
	}
	require.Equal(t, 5, revokes, "two mints, two revokes and one more mint, each audited")
}

// TestLinksExpireThirtyDaysAfterPayment uses the database's own clock for when the invoice was
// settled (allocations and corrections are stamped by now()), so it brackets that moment with the
// wall clock and moves the injected clock well clear of the boundary; the exact boundary is the
// unit test of Expired.
func TestLinksExpireThirtyDaysAfterPayment(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "Griya Sejahtera", "0812613")
	start := env.Clock.Now()

	t.Run("paid", func(t *testing.T) {
		env.Clock.Set(start)
		r := w.rent(t, "Rina Wulandari", uuid.Nil, 700_000)
		token := w.mint(t, w.admin, r.invoiceID)
		before := time.Now()
		w.pay(t, r, 700_000)
		after := time.Now()

		env.Clock.Set(before.Add(29 * 24 * time.Hour).In(clock.Jakarta))
		_, err := w.svc.PublicPage(ctx, token)
		require.NoError(t, err, "29 days after payment the link works")
		w.mint(t, w.admin, r.invoiceID)

		env.Clock.Set(after.Add(30*24*time.Hour + time.Hour).In(clock.Jakarta))
		_, err = w.svc.PublicPage(ctx, token)
		require.ErrorIs(t, err, paylink.ErrLinkExpired)
		_, err = w.svc.Mint(ctx, w.admin, r.invoiceID)
		require.ErrorIs(t, err, paylink.ErrInvoicePastWindow)
	})

	t.Run("cancelled by a correction counts as paid at the correction", func(t *testing.T) {
		env.Clock.Set(start)
		r := w.rent(t, "Budi Santoso", uuid.Nil, 650_000)
		token := w.mint(t, w.admin, r.invoiceID)
		zero := int64(0)
		before := time.Now()
		_, err := env.Billing.CreateCorrection(ctx, w.admin, billing.CorrectionInput{
			RequestID: uuid.New(), InvoiceID: r.invoiceID, NewAmount: &zero, Reason: "sewa tidak diperpanjang",
		})
		require.NoError(t, err)
		after := time.Now()

		env.Clock.Set(before.Add(29 * 24 * time.Hour).In(clock.Jakarta))
		page, err := w.svc.PublicPage(ctx, token)
		require.NoError(t, err)
		require.Zero(t, page.Invoice.Amount, "the cancelled invoice bills nothing")

		env.Clock.Set(after.Add(30*24*time.Hour + time.Hour).In(clock.Jakarta))
		_, err = w.svc.PublicPage(ctx, token)
		require.ErrorIs(t, err, paylink.ErrLinkExpired)
	})

	t.Run("an open invoice's link never expires", func(t *testing.T) {
		env.Clock.Set(start)
		r := w.rent(t, "Dewi Lestari", uuid.Nil, 600_000)
		token := w.mint(t, w.admin, r.invoiceID)
		env.Clock.Set(start.AddDate(1, 0, 0))
		_, err := w.svc.PublicPage(ctx, token)
		require.NoError(t, err)
	})
}

func TestSharingNeedsInvoiceShare(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	w := newWorld(t, env, "Griya Sejahtera", "0812614")
	assigned := w.rent(t, "Rina Wulandari", w.karyawan.MembershipID, 700_000)
	unassigned := w.rent(t, "Budi Santoso", uuid.Nil, 650_000)

	tests := []struct {
		name    string
		p       authz.Principal
		invoice uuid.UUID
		wantErr error
	}{
		{"admin", w.admin, unassigned.invoiceID, nil},
		{"karyawan, a tenant they collect from", w.karyawan, assigned.invoiceID, nil},
		{"karyawan, a tenant someone else collects from", w.karyawan, unassigned.invoiceID, authz.ErrForbidden},
		{"supervisor", w.supervisor, assigned.invoiceID, authz.ErrForbidden},
		{"an invoice that does not exist", w.admin, uuid.New(), paylink.ErrInvoiceNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := w.svc.Mint(ctx, tc.p, tc.invoice)
			_, revokeErr := w.svc.RevokeAll(ctx, tc.p, tc.invoice)
			if tc.wantErr == nil {
				require.NoError(t, err)
				require.NoError(t, revokeErr)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
			require.ErrorIs(t, revokeErr, tc.wantErr)
		})
	}

	t.Run("denials are audited", func(t *testing.T) {
		var denied int
		for _, row := range env.AuditLog(t, w.org.ID) {
			if row.Action == authz.ActionInvoiceShare && row.Decision == "deny" {
				denied++
			}
		}
		require.Equal(t, 4, denied, "mint and revoke, for the karyawan and the supervisor")
	})

	t.Run("another organization's invoice is not found", func(t *testing.T) {
		b := newWorld(t, env, "Kos Bulanan Sejahtera", "0812615")
		_, err := b.svc.Mint(ctx, b.admin, assigned.invoiceID)
		require.ErrorIs(t, err, paylink.ErrInvoiceNotFound)
		_, err = b.svc.RevokeAll(ctx, b.admin, assigned.invoiceID)
		require.ErrorIs(t, err, paylink.ErrInvoiceNotFound)
	})
}

func TestResolve(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	a := newWorld(t, env, "Griya Sejahtera", "0812616")
	b := newWorld(t, env, "Kos Bulanan Sejahtera", "0812617")
	ra := a.rent(t, "Rina Wulandari", uuid.Nil, 700_000)
	rb := b.rent(t, "Budi Santoso", uuid.Nil, 650_000)
	tokenA, tokenB := a.mint(t, a.admin, ra.invoiceID), b.mint(t, b.admin, rb.invoiceID)

	org, link, err := a.svc.Resolve(ctx, tokenA)
	require.NoError(t, err)
	require.Equal(t, a.org.ID, org)
	require.Equal(t, a.links(t)[0].ID, link)
	org, _, err = a.svc.Resolve(ctx, tokenB)
	require.NoError(t, err)
	require.Equal(t, b.org.ID, org, "the token decides the organization, not the service")

	page, err := a.svc.PublicPage(ctx, tokenB)
	require.NoError(t, err)
	require.Equal(t, "Budi Santoso", page.TenantName)
	require.Equal(t, "Kos Bulanan Sejahtera", page.OrganizationName)

	_, err = b.svc.RevokeAll(ctx, b.admin, rb.invoiceID)
	require.NoError(t, err)
	unknown := strings.Repeat("A", 43)
	for name, token := range map[string]string{
		"garbage": "not-a-token", "well-formed but unknown": unknown, "another organization's revoked link": tokenB,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := a.svc.PublicPage(ctx, token)
			require.ErrorIs(t, err, paylink.ErrLinkNotFound)
		})
	}
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
}

// TestPaymentLinkSchema checks the database side as the app role: links and bank accounts are
// isolated per organization and cannot be deleted, a link changes only by being revoked, and the
// lookup function returns two ids only, to the app role only.
func TestPaymentLinkSchema(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	a := newWorld(t, env, "Griya Sejahtera", "0812618")
	b := newWorld(t, env, "Kos Bulanan Sejahtera", "0812619")
	ra := a.rent(t, "Rina Wulandari", uuid.Nil, 700_000)
	rb := b.rent(t, "Budi Santoso", uuid.Nil, 650_000)
	a.mint(t, a.admin, ra.invoiceID)
	b.mint(t, b.admin, rb.invoiceID)
	for _, w := range []*world{a, b} {
		_, err := w.svc.SetBankAccount(ctx, w.org.ID, paylink.BankAccount{BankName: "BCA", AccountNumber: "1234567890", AccountHolder: "Pemilik " + w.prefix})
		require.NoError(t, err)
	}
	linkA := a.links(t)[0]
	tx := func(w *world, fn func(tx pgx.Tx) error) error { return env.Store.WithOrgTx(ctx, w.org.ID, fn) }

	t.Run("each organization sees only its own links and bank account", func(t *testing.T) {
		for _, w := range []*world{a, b} {
			for _, table := range []string{"payment_link", "organization_bank_account"} {
				var all, foreign int
				require.NoError(t, tx(w, func(tx pgx.Tx) error {
					return tx.QueryRow(ctx, fmt.Sprintf(
						"SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM %s", table)).Scan(&all, &foreign)
				}))
				require.Equal(t, 1, all, table)
				require.Zero(t, foreign, table)
			}
		}
		var n int
		require.NoError(t, tx(b, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM payment_link WHERE id = $1", linkA.ID).Scan(&n)
		}))
		require.Zero(t, n, "organization B cannot read A's link by id")

		app, err := pgx.Connect(ctx, env.URLs.App)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, app.Close(ctx)) })
		require.NoError(t, app.QueryRow(ctx, "SELECT count(*) FROM payment_link").Scan(&n))
		require.Zero(t, n, "no scope, no rows")
		_, err = app.Exec(ctx, "SELECT set_config('app.payment_link_lookup', 'on', false)")
		require.NoError(t, err)
		require.NoError(t, app.QueryRow(ctx, "SELECT count(*) FROM payment_link").Scan(&n))
		require.Zero(t, n, "the lookup policy is for the function's owner, not the app role")
	})

	t.Run("the app role cannot delete or rewrite a link, only revoke it", func(t *testing.T) {
		err := tx(a, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "DELETE FROM payment_link WHERE id = $1", linkA.ID)
			return err
		})
		requireCode(t, err, "42501")
		err = tx(a, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE payment_link SET token_hash = $2 WHERE id = $1", linkA.ID, make([]byte, 32))
			return err
		})
		requireCode(t, err, "42501")
		err = tx(a, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE payment_link SET invoice_id = $2 WHERE id = $1", linkA.ID, ra.invoiceID)
			return err
		})
		requireCode(t, err, "42501")
		err = tx(a, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "DELETE FROM organization_bank_account")
			return err
		})
		requireCode(t, err, "42501")
	})

	t.Run("a link's tenant is its invoice's tenant, and its hash is 32 bytes", func(t *testing.T) {
		other := a.rent(t, "Dewi Lestari", uuid.Nil, 600_000)
		insert := func(tenantID uuid.UUID, hash []byte) error {
			return tx(a, func(tx pgx.Tx) error {
				return dbgen.New(tx).InsertPaymentLink(ctx, dbgen.InsertPaymentLinkParams{
					ID: uuid.New(), OrganizationID: a.org.ID, InvoiceID: ra.invoiceID, TenantID: tenantID, TokenHash: hash, CreatedAt: time.Now(),
				})
			})
		}
		requireCode(t, insert(other.tenantID, make([]byte, 32)), "23503")
		requireCode(t, insert(ra.tenantID, make([]byte, 31)), "23514")
	})

	t.Run("payment_link_lookup returns two ids, to the app role only", func(t *testing.T) {
		owner, err := pgx.Connect(ctx, env.URLs.Owner)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, owner.Close(ctx)) })
		var result string
		var definer bool
		var config []string
		require.NoError(t, owner.QueryRow(ctx, `
			SELECT pg_get_function_result(p.oid), p.prosecdef, coalesce(p.proconfig, '{}')
			FROM pg_proc p WHERE p.proname = 'payment_link_lookup'`).Scan(&result, &definer, &config))
		require.Equal(t, "TABLE(organization_id uuid, link_id uuid)", result)
		require.True(t, definer)
		require.Contains(t, config, "search_path=pg_catalog, public")

		_, err = owner.Exec(ctx, "CREATE ROLE link_stranger NOLOGIN")
		require.NoError(t, err)
		var app, stranger bool
		require.NoError(t, owner.QueryRow(ctx, `
			SELECT has_function_privilege('cashflow_app', 'payment_link_lookup(bytea)', 'EXECUTE'),
			       has_function_privilege('link_stranger', 'payment_link_lookup(bytea)', 'EXECUTE')`).Scan(&app, &stranger))
		require.True(t, app)
		require.False(t, stranger, "PUBLIC must not have EXECUTE")

		org, link, err := env.Store.LookupPaymentLink(ctx, linkA.TokenHash)
		require.NoError(t, err, "the app role runs it with no scope")
		require.Equal(t, a.org.ID, org)
		require.Equal(t, linkA.ID, link)
	})

	t.Run("it works for an owner held to row-level security, as in production", func(t *testing.T) {
		owner, err := pgx.Connect(ctx, env.URLs.Owner)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, owner.Close(ctx)) })
		// The test database's owner is a superuser, which skips row-level security, so hand the
		// function to an ordinary role first. Without the policy it would see nothing.
		for _, stmt := range []string{
			"CREATE ROLE link_fn_owner NOLOGIN NOSUPERUSER NOBYPASSRLS",
			"GRANT SELECT ON payment_link TO link_fn_owner",
			"ALTER FUNCTION payment_link_lookup(bytea) OWNER TO link_fn_owner",
		} {
			_, err := owner.Exec(ctx, stmt)
			require.NoError(t, err, stmt)
		}
		_, _, err = env.Store.LookupPaymentLink(ctx, linkA.TokenHash)
		require.Error(t, err, "an owner without the policy is held to the organization scope")

		_, err = owner.Exec(ctx, "ALTER POLICY payment_link_lookup ON payment_link TO link_fn_owner")
		require.NoError(t, err)
		org, link, err := env.Store.LookupPaymentLink(ctx, linkA.TokenHash)
		require.NoError(t, err)
		require.Equal(t, a.org.ID, org)
		require.Equal(t, linkA.ID, link)
	})
}
