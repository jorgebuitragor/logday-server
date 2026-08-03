package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var rawMigrationsFS embed.FS

// Migrate applies any pending migrations embedded in the binary.
func Migrate(ctx context.Context, database *sql.DB) error {
	migrationsFS, err := fs.Sub(rawMigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("preparing migrations filesystem: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, database, migrationsFS)
	if err != nil {
		return fmt.Errorf("creating migration provider: %w", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
