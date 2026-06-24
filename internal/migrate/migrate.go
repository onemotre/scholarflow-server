// Package migrate applies database schema migrations at service startup.
//
// Migrations are embedded into the binary (see package migrations) and applied
// via goose's library API, so deploys no longer require a manual
// `goose ... up` step or the goose CLI. A Postgres advisory session lock makes
// it safe for the api and worker to run this concurrently: one acquires the
// lock and migrates, the others wait and then find nothing pending.
package migrate

import (
	"context"
	"database/sql"
	"fmt"

	// Registers the "pgx" database/sql driver used to open the migration
	// connection. goose works over database/sql, not pgxpool.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"scholarflow_server/migrations"
)

// Run applies all pending migrations against databaseURL. It is idempotent:
// calling it when the schema is already current is a no-op.
func Run(ctx context.Context, databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open migration db: %w", err)
	}
	defer db.Close()

	// The session locker holds a Postgres advisory lock on a single
	// connection for the duration of the migration, so cap the pool at one
	// connection to keep the lock and the migrations on the same session.
	db.SetMaxOpenConns(1)

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create migration locker: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
