//go:build integration

// Package pgtest starts a throwaway PostgreSQL for integration tests, with the same roles as
// production: an owner that runs migrations and an app role that cannot bypass row-level
// security.
package pgtest

import (
	"context"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/farrelfauzan/cashflow-app/services/api/db"
)

// URLs to the test database.
type URLs struct {
	Owner string // runs migrations; never used by application code
	App   string // what the API and worker use
}

// Start runs PostgreSQL in a container for the length of the test and creates the app role.
// It does not apply migrations.
func Start(t *testing.T) URLs {
	t.Helper()
	ctx := context.Background()

	ctr, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("cashflow"),
		postgres.WithUsername("cashflow_owner"),
		postgres.WithPassword("owner-test-password"),
		postgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	owner, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	conn, err := pgx.Connect(ctx, owner)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, db.InitSQL)
	require.NoError(t, err)
	require.NoError(t, conn.Close(ctx))

	app, err := url.Parse(owner)
	require.NoError(t, err)
	app.User = url.UserPassword("cashflow_app", "cashflow_app_dev")
	return URLs{Owner: owner, App: app.String()}
}
