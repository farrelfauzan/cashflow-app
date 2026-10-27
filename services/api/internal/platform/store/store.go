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
)

// ErrNoOrganization is returned when WithOrgTx is called without an organization. It fails
// before any query runs so a missing organization can never mean "all organizations".
var ErrNoOrganization = errors.New("store: organization id is required")

// Store wraps the connection pool.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// Open connects to PostgreSQL and checks the connection. Use the app role's URL, never the
// owner role's.
func Open(ctx context.Context, url string, log *slog.Logger) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
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

// Close releases every connection.
func (s *Store) Close() { s.pool.Close() }

// Ping reports whether the database answers.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}
	return nil
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
	return s.inTx(ctx, map[string]string{"app.org_id": org.String()}, fn)
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
	return s.inTx(ctx, settings, fn)
}

func (s *Store) inTx(ctx context.Context, settings map[string]string, fn func(tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
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
