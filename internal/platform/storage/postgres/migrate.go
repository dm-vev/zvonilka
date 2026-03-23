package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ApplyMigrations executes ordered SQL migration files against the database.
func ApplyMigrations(ctx context.Context, db *sql.DB, migrationsPath string, schema string) error {
	return applyMigrations(ctx, db, migrationsPath, schema)
}

// applyMigrations executes ordered SQL migration files against the database.
func applyMigrations(ctx context.Context, db *sql.DB, migrationsPath string, schema string) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if db == nil {
		return errors.New("database handle is required")
	}
	if migrationsPath == "" {
		return errors.New("migrations path is required")
	}

	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("read postgres migrations from %s: %w", migrationsPath, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		files = append(files, entry.Name())
	}
	slices.Sort(files)
	if len(files) == 0 {
		return fmt.Errorf("read postgres migrations from %s: %w", migrationsPath, fs.ErrNotExist)
	}

	for _, name := range files {
		path := filepath.Join(migrationsPath, name)
		if err := applyMigration(ctx, db, path, schema); err != nil {
			return err
		}
	}

	return nil
}

// applyMigration runs a single migration file in its own transaction.
func applyMigration(ctx context.Context, db *sql.DB, path string, schema string) error {
	script, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read postgres migration %s: %w", path, err)
	}

	sqlText := strings.ReplaceAll(string(script), "{{schema}}", quoteIdentifier(normalizeSchema(schema)))

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin postgres migration %s: %w", path, err)
	}

	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(
				fmt.Errorf("execute postgres migration %s: %w", path, err),
				fmt.Errorf("rollback postgres migration %s: %w", path, rollbackErr),
			)
		}

		return fmt.Errorf("execute postgres migration %s: %w", path, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit postgres migration %s: %w", path, err)
	}

	return nil
}
