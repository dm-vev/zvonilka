package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/dm-vev/zvonilka/internal/platform/config"
)

type migrationExpectation struct {
	name   string
	script string
	err    error
}

func expectMigrationBootstrap(mock sqlmock.Sqlmock, schema string, migrations ...migrationExpectation) {
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS " + qualifiedName("public", "schema_migrations"))).
		WillReturnResult(sqlmock.NewResult(0, 0))

	for _, migration := range migrations {
		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).
			WithArgs(sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
			`SELECT 1 FROM %s WHERE schema_name = $1 AND migration_name = $2 LIMIT 1`,
			qualifiedName("public", "schema_migrations"),
		))).
			WithArgs(schema, migration.name).
			WillReturnError(sql.ErrNoRows)
		execExpectation := mock.ExpectExec(regexp.QuoteMeta(strings.ReplaceAll(migration.script, "{{schema}}", quoteIdentifier(schema))))
		if migration.err != nil {
			execExpectation.WillReturnError(migration.err)
			mock.ExpectRollback()
			return
		}
		execExpectation.WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
			`INSERT INTO %s (schema_name, migration_name, applied_at) VALUES ($1, $2, CURRENT_TIMESTAMP)`,
			qualifiedName("public", "schema_migrations"),
		))).
			WithArgs(schema, migration.name).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()
	}
}

func TestBootstrapOpenAppliesMigrationsAndCloses(t *testing.T) {
	dir := t.TempDir()
	first := "0002.sql"
	second := "0001.sql"

	firstScript := "CREATE TABLE IF NOT EXISTS {{schema}}.beta (id INT PRIMARY KEY);"
	secondScript := "CREATE TABLE IF NOT EXISTS {{schema}}.alpha (id INT PRIMARY KEY);"

	if err := os.WriteFile(filepath.Join(dir, first), []byte(firstScript), 0o600); err != nil {
		t.Fatalf("write first migration: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, second), []byte(secondScript), 0o600); err != nil {
		t.Fatalf("write second migration: %v", err)
	}

	db, mock, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	originalOpen := openDatabase
	openDatabase = func(string, string) (*sql.DB, error) {
		return db, nil
	}
	t.Cleanup(func() {
		openDatabase = originalOpen
	})

	mock.ExpectPing()
	expectMigrationBootstrap(mock, "tenant",
		migrationExpectation{name: "0001.sql", script: secondScript},
		migrationExpectation{name: "0002.sql", script: firstScript},
	)
	mock.ExpectClose()

	bootstrap := NewBootstrap(config.Configuration{
		Service: config.ServiceConfig{Name: "controlplane"},
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled:         true,
				DSN:             "postgres://zvonilka",
				MigrationsPath:  dir,
				Schema:          "tenant",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
		},
	})

	opened, err := bootstrap.Open(context.Background())
	if err != nil {
		t.Fatalf("open bootstrap: %v", err)
	}
	if opened != db {
		t.Fatal("expected shared database handle")
	}

	if err := bootstrap.Close(context.Background()); err != nil {
		t.Fatalf("close bootstrap: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestBootstrapOpenRetriesAfterTransientFailure(t *testing.T) {
	dir := t.TempDir()
	migration := "0001.sql"
	script := "CREATE TABLE IF NOT EXISTS {{schema}}.alpha (id INT PRIMARY KEY);"

	if err := os.WriteFile(filepath.Join(dir, migration), []byte(script), 0o600); err != nil {
		t.Fatalf("write migration: %v", err)
	}

	db1, mock1, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock first db: %v", err)
	}
	db2, mock2, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock second db: %v", err)
	}
	t.Cleanup(func() {
		_ = db1.Close()
		_ = db2.Close()
	})

	originalOpen := openDatabase
	openCalls := 0
	openDatabase = func(string, string) (*sql.DB, error) {
		openCalls++
		if openCalls == 1 {
			return db1, nil
		}
		return db2, nil
	}
	t.Cleanup(func() {
		openDatabase = originalOpen
	})

	mock1.ExpectPing().WillReturnError(errors.New("transient ping failure"))
	mock1.ExpectClose()

	mock2.ExpectPing()
	expectMigrationBootstrap(mock2, "tenant",
		migrationExpectation{name: "0001.sql", script: script},
	)
	mock2.ExpectClose()

	bootstrap := NewBootstrap(config.Configuration{
		Service: config.ServiceConfig{Name: "controlplane"},
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled:         true,
				DSN:             "postgres://zvonilka",
				MigrationsPath:  dir,
				Schema:          "tenant",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
		},
	})

	if _, err := bootstrap.Open(context.Background()); err == nil {
		t.Fatal("expected transient open failure")
	}

	opened, err := bootstrap.Open(context.Background())
	if err != nil {
		t.Fatalf("retry open bootstrap: %v", err)
	}
	if opened != db2 {
		t.Fatal("expected second database handle after retry")
	}

	if err := bootstrap.Close(context.Background()); err != nil {
		t.Fatalf("close bootstrap after retry: %v", err)
	}

	if err := mock1.ExpectationsWereMet(); err != nil {
		t.Fatalf("first db expectations: %v", err)
	}
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Fatalf("second db expectations: %v", err)
	}
}

func TestBootstrapOpenRetriesAfterMigrationFailure(t *testing.T) {
	dir := t.TempDir()
	script := "CREATE TABLE IF NOT EXISTS {{schema}}.alpha (id INT PRIMARY KEY);"
	if err := os.WriteFile(filepath.Join(dir, "0001.sql"), []byte(script), 0o600); err != nil {
		t.Fatalf("write migration: %v", err)
	}

	firstDB, firstMock, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock first db: %v", err)
	}
	secondDB, secondMock, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock second db: %v", err)
	}
	t.Cleanup(func() {
		_ = firstDB.Close()
		_ = secondDB.Close()
	})

	originalOpen := openDatabase
	openCalls := 0
	openDatabase = func(string, string) (*sql.DB, error) {
		openCalls++
		if openCalls == 1 {
			return firstDB, nil
		}
		return secondDB, nil
	}
	t.Cleanup(func() {
		openDatabase = originalOpen
	})

	firstMock.ExpectPing()
	expectMigrationBootstrap(firstMock, "tenant",
		migrationExpectation{name: "0001.sql", script: script, err: errors.New("migration failed")},
	)
	firstMock.ExpectClose()

	secondMock.ExpectPing()
	expectMigrationBootstrap(secondMock, "tenant",
		migrationExpectation{name: "0001.sql", script: script},
	)
	secondMock.ExpectClose()

	bootstrap := NewBootstrap(config.Configuration{
		Service: config.ServiceConfig{Name: "controlplane"},
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled:         true,
				DSN:             "postgres://zvonilka",
				MigrationsPath:  dir,
				Schema:          "tenant",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
		},
	})

	if _, err := bootstrap.Open(context.Background()); err == nil {
		t.Fatal("expected migration failure")
	}

	opened, err := bootstrap.Open(context.Background())
	if err != nil {
		t.Fatalf("retry open bootstrap: %v", err)
	}
	if opened != secondDB {
		t.Fatal("expected second database handle after retry")
	}

	if err := bootstrap.Close(context.Background()); err != nil {
		t.Fatalf("close bootstrap after retry: %v", err)
	}

	if err := firstMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("first db expectations: %v", err)
	}
	if err := secondMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("second db expectations: %v", err)
	}
}

func TestBootstrapOpenReportsCleanupErrorOnMigrationFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "0001.sql"),
		[]byte("CREATE TABLE IF NOT EXISTS {{schema}}.alpha (id INT PRIMARY KEY);"),
		0o600,
	); err != nil {
		t.Fatalf("write migration: %v", err)
	}

	db, mock, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	originalOpen := openDatabase
	openDatabase = func(string, string) (*sql.DB, error) {
		return db, nil
	}
	t.Cleanup(func() {
		openDatabase = originalOpen
	})

	mock.ExpectPing()
	expectMigrationBootstrap(mock, "tenant",
		migrationExpectation{name: "0001.sql", script: "CREATE TABLE IF NOT EXISTS {{schema}}.alpha (id INT PRIMARY KEY);", err: errors.New("migration failed")},
	)
	mock.ExpectClose().WillReturnError(errors.New("close failed"))

	bootstrap := NewBootstrap(config.Configuration{
		Service: config.ServiceConfig{Name: "controlplane"},
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled:         true,
				DSN:             "postgres://zvonilka",
				MigrationsPath:  dir,
				Schema:          "tenant",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
		},
	})

	_, err = bootstrap.Open(context.Background())
	if err == nil {
		t.Fatal("expected migration failure")
	}
	if !strings.Contains(err.Error(), "migration failed") {
		t.Fatalf("expected migration error in %v", err)
	}
	if !strings.Contains(err.Error(), "close failed") {
		t.Fatalf("expected close error in %v", err)
	}

	if err := bootstrap.Close(context.Background()); err != nil {
		t.Fatalf("close bootstrap after failed open: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestBootstrapCloseAllowsReopen(t *testing.T) {
	dir := t.TempDir()
	migration := "0001.sql"
	script := "CREATE TABLE IF NOT EXISTS {{schema}}.alpha (id INT PRIMARY KEY);"

	if err := os.WriteFile(filepath.Join(dir, migration), []byte(script), 0o600); err != nil {
		t.Fatalf("write migration: %v", err)
	}

	db1, mock1, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock first db: %v", err)
	}
	db2, mock2, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock second db: %v", err)
	}
	t.Cleanup(func() {
		_ = db1.Close()
		_ = db2.Close()
	})

	originalOpen := openDatabase
	openCalls := 0
	openDatabase = func(string, string) (*sql.DB, error) {
		openCalls++
		if openCalls == 1 {
			return db1, nil
		}
		return db2, nil
	}
	t.Cleanup(func() {
		openDatabase = originalOpen
	})

	mock1.ExpectPing()
	expectMigrationBootstrap(mock1, "tenant",
		migrationExpectation{name: "0001.sql", script: script},
	)
	mock1.ExpectClose()

	mock2.ExpectPing()
	expectMigrationBootstrap(mock2, "tenant",
		migrationExpectation{name: "0001.sql", script: script},
	)
	mock2.ExpectClose()

	bootstrap := NewBootstrap(config.Configuration{
		Service: config.ServiceConfig{Name: "controlplane"},
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled:         true,
				DSN:             "postgres://zvonilka",
				MigrationsPath:  dir,
				Schema:          "tenant",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
		},
	})

	first, err := bootstrap.Open(context.Background())
	if err != nil {
		t.Fatalf("open bootstrap first time: %v", err)
	}
	if first != db1 {
		t.Fatal("expected first database handle")
	}

	if err := bootstrap.Close(context.Background()); err != nil {
		t.Fatalf("close bootstrap first time: %v", err)
	}

	second, err := bootstrap.Open(context.Background())
	if err != nil {
		t.Fatalf("open bootstrap second time: %v", err)
	}
	if second != db2 {
		t.Fatal("expected second database handle after close")
	}

	if err := bootstrap.Close(context.Background()); err != nil {
		t.Fatalf("close bootstrap second time: %v", err)
	}

	if err := mock1.ExpectationsWereMet(); err != nil {
		t.Fatalf("first db expectations: %v", err)
	}
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Fatalf("second db expectations: %v", err)
	}
}

func TestBootstrapOpenClosesAfterMigrationFailure(t *testing.T) {
	dir := t.TempDir()
	firstMigration := "0001.sql"
	secondMigration := "0002.sql"

	if err := os.WriteFile(
		filepath.Join(dir, firstMigration),
		[]byte("CREATE TABLE IF NOT EXISTS {{schema}}.alpha (id INT PRIMARY KEY);"),
		0o600,
	); err != nil {
		t.Fatalf("write first migration: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, secondMigration),
		[]byte("CREATE TABLE IF NOT EXISTS {{schema}}.beta (id INT PRIMARY KEY);"),
		0o600,
	); err != nil {
		t.Fatalf("write second migration: %v", err)
	}

	db1, mock1, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock first db: %v", err)
	}
	db2, mock2, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock second db: %v", err)
	}
	t.Cleanup(func() {
		_ = db1.Close()
		_ = db2.Close()
	})

	originalOpen := openDatabase
	openCalls := 0
	openDatabase = func(string, string) (*sql.DB, error) {
		openCalls++
		if openCalls == 1 {
			return db1, nil
		}
		return db2, nil
	}
	t.Cleanup(func() {
		openDatabase = originalOpen
	})

	mock1.ExpectPing()
	expectMigrationBootstrap(mock1, "tenant",
		migrationExpectation{name: "0001.sql", script: "CREATE TABLE IF NOT EXISTS {{schema}}.alpha (id INT PRIMARY KEY);"},
		migrationExpectation{name: "0002.sql", script: "CREATE TABLE IF NOT EXISTS {{schema}}.beta (id INT PRIMARY KEY);", err: errors.New("migration failed")},
	)
	mock1.ExpectClose()

	mock2.ExpectPing()
	expectMigrationBootstrap(mock2, "tenant",
		migrationExpectation{name: "0001.sql", script: "CREATE TABLE IF NOT EXISTS {{schema}}.alpha (id INT PRIMARY KEY);"},
		migrationExpectation{name: "0002.sql", script: "CREATE TABLE IF NOT EXISTS {{schema}}.beta (id INT PRIMARY KEY);"},
	)
	mock2.ExpectClose()

	bootstrap := NewBootstrap(config.Configuration{
		Service: config.ServiceConfig{Name: "controlplane"},
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled:         true,
				DSN:             "postgres://zvonilka",
				MigrationsPath:  dir,
				Schema:          "tenant",
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
		},
	})

	if _, err := bootstrap.Open(context.Background()); err == nil {
		t.Fatal("expected migration failure")
	}

	opened, err := bootstrap.Open(context.Background())
	if err != nil {
		t.Fatalf("retry open bootstrap after migration failure: %v", err)
	}
	if opened != db2 {
		t.Fatal("expected second database handle after migration retry")
	}

	if err := bootstrap.Close(context.Background()); err != nil {
		t.Fatalf("close bootstrap after migration retry: %v", err)
	}

	if err := mock1.ExpectationsWereMet(); err != nil {
		t.Fatalf("first db expectations: %v", err)
	}
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Fatalf("second db expectations: %v", err)
	}
}

func TestBootstrapQuotesSchemaIdentifiers(t *testing.T) {
	dir := t.TempDir()
	migration := "0001.sql"
	script := "CREATE TABLE IF NOT EXISTS {{schema}}.alpha (id INT PRIMARY KEY);"

	if err := os.WriteFile(filepath.Join(dir, migration), []byte(script), 0o600); err != nil {
		t.Fatalf("write migration: %v", err)
	}

	db, mock, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.MonitorPingsOption(true),
	)
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	originalOpen := openDatabase
	openDatabase = func(string, string) (*sql.DB, error) {
		return db, nil
	}
	t.Cleanup(func() {
		openDatabase = originalOpen
	})

	mock.ExpectPing()
	expectMigrationBootstrap(mock, `ten"ant`,
		migrationExpectation{name: "0001.sql", script: script},
	)
	mock.ExpectClose()

	bootstrap := NewBootstrap(config.Configuration{
		Service: config.ServiceConfig{Name: "controlplane"},
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled:         true,
				DSN:             "postgres://zvonilka",
				MigrationsPath:  dir,
				Schema:          `ten"ant`,
				MaxOpenConns:    1,
				MaxIdleConns:    1,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Minute,
			},
		},
	})

	if _, err := bootstrap.Open(context.Background()); err != nil {
		t.Fatalf("open bootstrap with quoted schema: %v", err)
	}

	if err := bootstrap.Close(context.Background()); err != nil {
		t.Fatalf("close bootstrap with quoted schema: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
