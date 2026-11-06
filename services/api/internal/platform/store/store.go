// Package store owns the PostgreSQL connection pool.
//
// The pool is private on purpose. Business code reaches the database only through WithOrgTx,
// which sets app.org_id for the transaction so the row-level security policies know which
// organization the request belongs to. A linter rule forbids importing pgxpool anywhere else.
package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// ErrNoOrganization is returned when WithOrgTx is called without an organization. It fails
// before any query runs so a missing organization can never mean "all organizations".
var ErrNoOrganization = errors.New("store: organization id is required")

// Store wraps the connection pool.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// Option changes how Open builds the pool.
type Option func(*pgxpool.Config)

// WithMaxConns sets the pool's size. Without it pgx picks the larger of 4 and the number of CPUs,
// which on a small server is 4.
func WithMaxConns(n int32) Option {
	return func(c *pgxpool.Config) { c.MaxConns = n }
}

// Open connects to PostgreSQL and checks the connection. Use the app role's URL, never the
// owner role's.
func Open(ctx context.Context, url string, log *slog.Logger, opts ...Option) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
	for _, o := range opts {
		o(cfg)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("opening connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return &Store{pool: pool, log: log}, nil
}

// MaxConns is the pool's size.
func (s *Store) MaxConns() int32 { return s.pool.Config().MaxConns }

// Close releases every connection.
func (s *Store) Close() { s.pool.Close() }

// Ping reports whether the database answers.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}
	return nil
}

// ActiveOrganizationIDs returns every active organization's id, for a scheduled job that then
// runs one task per organization inside WithOrgTx. It is the one read outside an organization or
// auth scope, and it goes through active_organization_ids(), a database function that returns
// ids only (phase B, slice 2). LookupPaymentLink is the other, narrower one: it maps one token
// hash to one link's ids.
func (s *Store) ActiveOrganizationIDs(ctx context.Context) ([]uuid.UUID, error) {
	ids, err := dbgen.New(s.pool).ActiveOrganizationIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing organization ids: %w", err)
	}
	return ids, nil
}

// ErrPaymentLinkNotFound means no payment link has the token hash given to LookupPaymentLink.
var ErrPaymentLinkNotFound = errors.New("store: payment link not found")

// LookupPaymentLink maps the SHA-256 of a payment link's token to the link's organization and id,
// for a public request that arrives with no session (the tenant payment page, decision 39). It
// runs as the app role with no scope set and goes through payment_link_lookup(), a database
// function that returns those two ids only; the caller then reads the link inside
// WithOrgTx or WithOrgSnapshot for that organization, which decides whether it still works. A
// revoked link is found like any other. An unknown hash returns ErrPaymentLinkNotFound.
func (s *Store) LookupPaymentLink(ctx context.Context, tokenHash []byte) (org, link uuid.UUID, err error) {
	row, err := dbgen.New(s.pool).LookupPaymentLink(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrPaymentLinkNotFound
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("looking up payment link: %w", err)
	}
	return row.OrganizationID, row.LinkID, nil
}

// ErrWebhookNotFound means no AutoGoPay account has the webhook id given to LookupAutogopayWebhook.
var ErrWebhookNotFound = errors.New("store: webhook not found")

// LookupAutogopayWebhook maps the id in an AutoGoPay webhook URL to its account's organization,
// for a callback that arrives with no session (decision 39). Like LookupPaymentLink it runs as the
// app role with no scope set, through autogopay_webhook_lookup(), which returns the organization id
// only; the caller reads the key inside WithOrgTx for it. An unknown id returns ErrWebhookNotFound.
func (s *Store) LookupAutogopayWebhook(ctx context.Context, webhookID string) (uuid.UUID, error) {
	org, err := dbgen.New(s.pool).LookupAutogopayWebhook(ctx, webhookID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrWebhookNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("looking up webhook: %w", err)
	}
	return org, nil
}

// WithOrgTx runs fn in a transaction that is scoped to one organization. The transaction
// commits when fn returns nil and rolls back otherwise.
//
// set_config(..., true) limits the setting to this transaction, so a pooled connection never
// carries one organization's id into the next request.
func (s *Store) WithOrgTx(ctx context.Context, org uuid.UUID, fn func(tx pgx.Tx) error) error {
	if org == uuid.Nil {
		return ErrNoOrganization
	}
	return s.inTx(ctx, pgx.TxOptions{}, map[string]string{"app.org_id": org.String()}, fn)
}

// WithOrgSnapshot is WithOrgTx for reads that must agree with each other: a read-only,
// REPEATABLE READ transaction, so every statement in fn sees the same snapshot (the one taken at
// its first query). A screen that shows a tenant's invoices next to their totals uses it, so an
// invoice committed by the worker in between cannot show up in one and not the other. fn must
// not write: PostgreSQL refuses writes in a read-only transaction. A denial is still audited,
// since authz writes denials in a transaction of its own.
func (s *Store) WithOrgSnapshot(ctx context.Context, org uuid.UUID, fn func(tx pgx.Tx) error) error {
	if org == uuid.Nil {
		return ErrNoOrganization
	}
	opts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	return s.inTx(ctx, opts, map[string]string{"app.org_id": org.String()}, fn)
}

// WithAuthTx runs fn in the auth scope, which login and session handling need: it can read
// every staff user, OTP challenge and session, and no organization data. Pass the staff user's
// id once it is known, so the user can also list their own memberships; pass uuid.Nil before
// login. Only the auth and member packages should call this.
func (s *Store) WithAuthTx(ctx context.Context, user uuid.UUID, fn func(tx pgx.Tx) error) error {
	settings := map[string]string{"app.scope": "auth"}
	if user != uuid.Nil {
		settings["app.user_id"] = user.String()
	}
	return s.inTx(ctx, pgx.TxOptions{}, settings, fn)
}

func (s *Store) inTx(ctx context.Context, opts pgx.TxOptions, settings map[string]string, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		// Rollback after a successful commit reports ErrTxClosed, which is expected.
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			s.log.ErrorContext(ctx, "rolling back transaction", "error", err)
		}
	}()

	for name, value := range settings {
		if _, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", name, value); err != nil {
			return fmt.Errorf("setting %s: %w", name, err)
		}
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}
