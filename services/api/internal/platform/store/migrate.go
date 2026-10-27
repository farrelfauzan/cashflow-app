package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver for goose
	"github.com/pressly/goose/v3"

	"github.com/farrelfauzan/cashflow-app/services/api/db"
)

// Migrate applies every pending migration. url must use the owner role: the app role cannot
// create tables and must not be able to.
func Migrate(ctx context.Context, url string, log *slog.Logger) error {
	conn, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("opening database for migration: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.ErrorContext(ctx, "closing migration connection", "error", err)
		}
	}()

	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("locating embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, conn, migrations)
	if err != nil {
		return fmt.Errorf("creating migration provider: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	for _, r := range results {
		log.InfoContext(ctx, "migration applied", "version", r.Source.Version, "duration", r.Duration)
	}
	if len(results) == 0 {
		log.InfoContext(ctx, "database is up to date")
	}
	return nil
}
