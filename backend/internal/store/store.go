// Package store owns the Postgres schema and data access.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrUnsupportedDSN = errors.New("unsupported database DSN")

// MigrateUp applies all pending migrations; an up-to-date schema is not an error.
func MigrateUp(dsn string) error {
	return withMigrate(dsn, (*migrate.Migrate).Up)
}

// MigrateDown reverts every migration.
func MigrateDown(dsn string) error {
	return withMigrate(dsn, (*migrate.Migrate).Down)
}

// Open returns a pool that has answered a ping.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

func withMigrate(dsn string, step func(*migrate.Migrate) error) (err error) {
	url, err := migrateURL(dsn)
	if err != nil {
		return err
	}
	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, url)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		err = errors.Join(err, srcErr, dbErr)
	}()

	if err := step(m); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// migrateURL switches a postgres:// DSN to golang-migrate's pgx v5 driver and pins its
// bookkeeping table to public: with user "buddy", search_path "$user" would move it into our schema.
func migrateURL(dsn string) (string, error) {
	const table = `x-migrations-table="public"."schema_migrations"&x-migrations-table-quoted=1`
	for _, scheme := range []string{"postgres://", "postgresql://"} {
		if rest, ok := strings.CutPrefix(dsn, scheme); ok {
			sep := "?"
			if strings.Contains(rest, "?") {
				sep = "&"
			}
			return "pgx5://" + rest + sep + table, nil
		}
	}
	return "", fmt.Errorf("%w: want postgres:// or postgresql://", ErrUnsupportedDSN)
}
