package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
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

	if err := ensureMigrationTable(ctx, db); err != nil {
		return err
	}

	for _, name := range files {
		path := filepath.Join(migrationsPath, name)
		if err := applyMigration(ctx, db, path, schema, name); err != nil {
			return err
		}
	}

	return nil
}

// applyMigration runs a single migration file in its own transaction.
func applyMigration(ctx context.Context, db *sql.DB, path string, schema string, name string) error {
	script, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read postgres migration %s: %w", path, err)
	}

	sqlText := strings.ReplaceAll(string(script), "{{schema}}", quoteIdentifier(normalizeSchema(schema)))
	schemaName := normalizeSchema(schema)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin postgres migration %s: %w", path, err)
	}

	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLockKey(schemaName, name)); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(
				fmt.Errorf("lock postgres migration %s: %w", path, err),
				fmt.Errorf("rollback postgres migration %s: %w", path, rollbackErr),
			)
		}

		return fmt.Errorf("lock postgres migration %s: %w", path, err)
	}

	var applied int
	checkQuery := fmt.Sprintf(
		`SELECT 1 FROM %s WHERE schema_name = $1 AND migration_name = $2 LIMIT 1`,
		migrationTableName(),
	)
	if err := tx.QueryRowContext(ctx, checkQuery, schemaName, name).Scan(&applied); err == nil {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit postgres migration %s: %w", path, err)
		}
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(
				fmt.Errorf("check postgres migration %s: %w", path, err),
				fmt.Errorf("rollback postgres migration %s: %w", path, rollbackErr),
			)
		}

		return fmt.Errorf("check postgres migration %s: %w", path, err)
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

	insertQuery := fmt.Sprintf(
		`INSERT INTO %s (schema_name, migration_name, applied_at) VALUES ($1, $2, CURRENT_TIMESTAMP)`,
		migrationTableName(),
	)
	if _, err := tx.ExecContext(ctx, insertQuery, schemaName, name); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(
				fmt.Errorf("record postgres migration %s: %w", path, err),
				fmt.Errorf("rollback postgres migration %s: %w", path, rollbackErr),
			)
		}

		return fmt.Errorf("record postgres migration %s: %w", path, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit postgres migration %s: %w", path, err)
	}

	return nil
}

func ensureMigrationTable(ctx context.Context, db *sql.DB) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if db == nil {
		return errors.New("database handle is required")
	}

	query := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (\n"+
			"\tschema_name TEXT NOT NULL,\n"+
			"\tmigration_name TEXT NOT NULL,\n"+
			"\tapplied_at TIMESTAMPTZ NOT NULL,\n"+
			"\tPRIMARY KEY (schema_name, migration_name)\n"+
			")",
		migrationTableName(),
	)
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("ensure postgres migration table: %w", err)
	}

	return nil
}

func migrationTableName() string {
	return qualifiedName(defaultSchema, "schema_migrations")
}

func migrationLockKey(schemaName string, migrationName string) int64 {
	sum := sha256.Sum256([]byte("postgres:migration:" + schemaName + ":" + migrationName))
	key := binary.BigEndian.Uint64(sum[:8]) &^ (1 << 63)
	return int64(key)
}
