// Package db embeds the SQL files that ship inside the binaries.
package db

import "embed"

// Migrations holds the goose migrations. cmd/migrate applies them as the owner role.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// InitSQL creates the runtime database role and its default privileges. The dev compose file
// mounts it into the Postgres init directory, and integration tests run it against a fresh
// container. Production roles are created by whoever provisions the database.
//
//go:embed init/01-roles.sql
var InitSQL string
