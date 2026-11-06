//go:build integration

package qris_test

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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

const webOrigin = "https://app.test"

// world is one organization with a kos, its admin, and the payment link service.
type world struct {
	env    *testenv.Env
	org    testenv.Org
	admin  authz.Principal
	kos    property.Property
	links  *paylink.Service
	prefix string
	n      int
}

// renter is a tenant with one monthly lease, its first invoice and a link to it.
type renter struct {
	tenantID, invoiceID, linkID uuid.UUID
	token                       string
}

func newWorld(t *testing.T, env *testenv.Env, name, prefix string) *world {
	t.Helper()
	w := &world{env: env, org: env.CreateOrg(t, name, prefix), prefix: prefix,
		links: paylink.NewService(env.Store, env.Authz, env.Clock, webOrigin)}
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	var err error
	w.kos, err = env.Properties.Create(context.Background(), w.admin, property.PropertyInput{Name: "Kos Melati " + prefix, Kind: property.KindKos})
	require.NoError(t, err)
	return w
}

// rent lets a new room to a new tenant from today, generates the first invoice and mints a link.
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

func (w *world) tx(fn func(ctx context.Context, tx pgx.Tx) error) error {
	ctx := context.Background()
	return w.env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error { return fn(ctx, tx) })
}

// attempt is an InsertQrisAttempt for r: pending, Rp700.000, expiring 15 minutes from now.
func (w *world) attempt(r renter) dbgen.InsertQrisAttemptParams {
	now := w.env.Clock.Now()
	return dbgen.InsertQrisAttemptParams{
		ID: uuid.New(), OrganizationID: w.org.ID, TenantID: r.tenantID, PaymentLinkID: r.linkID,
		Amount: 700_000, TransactionID: uuid.NewString(), OrderID: "AUTOGOPAY-1793000000-1",
		QrString: "00020101021226610016ID.CO.QRIS.WWW", Status: qris.AttemptPending,
		ExpiresAt: now.Add(15 * time.Minute), CreatedAt: now,
	}
}

func (w *world) insert(p dbgen.InsertQrisAttemptParams) error {
	return w.tx(func(ctx context.Context, tx pgx.Tx) error { return dbgen.New(tx).InsertQrisAttempt(ctx, p) })
}

func (w *world) exec(sql string, args ...any) error {
	return w.tx(func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
}

// TestQrisAttemptSchema checks the table's rules as the app role.
func TestQrisAttemptSchema(t *testing.T) {
	env := testenv.New(t)
	a := newWorld(t, env, "Griya Sejahtera", "0812631")
	b := newWorld(t, env, "Kos Bulanan Sejahtera", "0812632")
	rina := a.rent(t, "Rina Wulandari")
	dewi := b.rent(t, "Dewi Lestari")
	first := a.attempt(rina)
	require.NoError(t, a.insert(first))
	require.NoError(t, b.insert(b.attempt(dewi)))

	t.Run("each organization sees only its own attempts", func(t *testing.T) {
		for _, w := range []*world{a, b} {
			var all, foreign int
			require.NoError(t, w.tx(func(ctx context.Context, tx pgx.Tx) error {
				return tx.QueryRow(ctx,
					"SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM qris_attempt").Scan(&all, &foreign)
			}))
			require.Equal(t, 1, all)
			require.Zero(t, foreign)
		}
		var n int
		require.NoError(t, b.tx(func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM qris_attempt WHERE id = $1", first.ID).Scan(&n)
		}))
		require.Zero(t, n, "organization B cannot read A's attempt by id")
		tag := 0
		require.NoError(t, b.tx(func(ctx context.Context, tx pgx.Tx) error {
			ct, err := tx.Exec(ctx, "UPDATE qris_attempt SET status = 'cancelled', closed_at = now() WHERE id = $1", first.ID)
			tag = int(ct.RowsAffected())
			return err
		}))
		require.Zero(t, tag, "organization B cannot change A's attempt")
		requireCode(t, b.insert(func() dbgen.InsertQrisAttemptParams {
			p := b.attempt(dewi)
			p.OrganizationID = a.org.ID
			return p
		}()), "42501")
	})

	t.Run("the app role cannot rewrite or delete an attempt", func(t *testing.T) {
		tests := []struct {
			name string
			sql  string
			args []any
		}{
			{"amount", "UPDATE qris_attempt SET amount = 1 WHERE id = $1", []any{first.ID}},
			{"qr_string", "UPDATE qris_attempt SET qr_string = 'x' WHERE id = $1", []any{first.ID}},
			{"transaction_id", "UPDATE qris_attempt SET transaction_id = 'x' WHERE id = $1", []any{first.ID}},
			{"expires_at", "UPDATE qris_attempt SET expires_at = now() WHERE id = $1", []any{first.ID}},
			{"delete", "DELETE FROM qris_attempt WHERE id = $1", []any{first.ID}},
			{"truncate", "TRUNCATE qris_attempt", nil},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				requireCode(t, a.exec(tc.sql, tc.args...), "42501")
			})
		}
	})

	t.Run("the app role may ask for a check on a pending attempt", func(t *testing.T) {
		require.NoError(t, a.exec("UPDATE qris_attempt SET check_requested_at = now() WHERE id = $1", first.ID))
	})

	t.Run("a tenant has one pending attempt at a time", func(t *testing.T) {
		requireCode(t, a.insert(a.attempt(rina)), "23505")
		other := a.rent(t, "Budi Santoso")
		require.NoError(t, a.insert(a.attempt(other)), "another tenant has their own")
	})

	t.Run("a transaction id is unique across organizations", func(t *testing.T) {
		p := b.attempt(b.rent(t, "Sari Indah"))
		p.TransactionID = first.TransactionID
		requireCode(t, b.insert(p), "23505")
	})

	t.Run("the attempt's tenant is its link's tenant", func(t *testing.T) {
		other := a.rent(t, "Agus Pratama")
		p := a.attempt(other)
		p.PaymentLinkID = rina.linkID
		requireCode(t, a.insert(p), "23503")
	})

	t.Run("column checks", func(t *testing.T) {
		closed := env.Clock.Now()
		tests := []struct {
			name   string
			change func(p *dbgen.InsertQrisAttemptParams)
		}{
			{"zero amount", func(p *dbgen.InsertQrisAttemptParams) { p.Amount = 0 }},
			{"amount above Rp10 juta", func(p *dbgen.InsertQrisAttemptParams) { p.Amount = 10_000_001 }},
			{"empty transaction id", func(p *dbgen.InsertQrisAttemptParams) { p.TransactionID = "" }},
			{"empty QR string", func(p *dbgen.InsertQrisAttemptParams) { p.QrString = "" }},
			{"QR string too long", func(p *dbgen.InsertQrisAttemptParams) { p.QrString = strings.Repeat("0", 1025) }},
			{"unknown status", func(p *dbgen.InsertQrisAttemptParams) { p.Status = "settlement" }},
			{"pending with a closed time", func(p *dbgen.InsertQrisAttemptParams) { p.ClosedAt = &closed }},
			{"closed with no closed time", func(p *dbgen.InsertQrisAttemptParams) { p.Status = qris.AttemptExpired }},
		}
		r := a.rent(t, "Wati Kusuma")
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				p := a.attempt(r)
				tc.change(&p)
				requireCode(t, a.insert(p), "23514")
			})
		}
	})

	t.Run("expired and cancelled attempts can still turn paid, and paid is final", func(t *testing.T) {
		r := a.rent(t, "Joko Susilo")
		for _, from := range []string{qris.AttemptExpired, qris.AttemptCancelled} {
			t.Run(from, func(t *testing.T) {
				p := a.attempt(r)
				require.NoError(t, a.insert(p))
				require.NoError(t, a.exec("UPDATE qris_attempt SET status = $2, closed_at = now() WHERE id = $1", p.ID, from))
				require.NoError(t, a.exec("UPDATE qris_attempt SET status = 'paid' WHERE id = $1", p.ID))
				for _, stmt := range []string{
					"UPDATE qris_attempt SET status = 'expired' WHERE id = $1",
					"UPDATE qris_attempt SET check_requested_at = now() WHERE id = $1",
					"UPDATE qris_attempt SET closed_at = now() WHERE id = $1",
				} {
					requireCode(t, a.exec(stmt, p.ID), "42501")
				}
			})
		}
	})

	// A reconcile that copies AutoGoPay's lagging "pending" back onto a closed attempt would
	// revive a dead QR and take the tenant's one pending slot.
	t.Run("expired and cancelled attempts never go back to pending", func(t *testing.T) {
		r := a.rent(t, "Rina Marlina")
		for _, from := range []string{qris.AttemptExpired, qris.AttemptCancelled} {
			t.Run(from, func(t *testing.T) {
				p := a.attempt(r)
				require.NoError(t, a.insert(p))
				require.NoError(t, a.exec("UPDATE qris_attempt SET status = $2, closed_at = now() WHERE id = $1", p.ID, from))
				requireCode(t, a.exec("UPDATE qris_attempt SET status = 'pending', closed_at = NULL WHERE id = $1", p.ID), "42501")
			})
		}
	})
}
