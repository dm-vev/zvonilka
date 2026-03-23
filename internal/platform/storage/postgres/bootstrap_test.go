package postgres

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/dm-vev/zvonilka/internal/platform/config"
)

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

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
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
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS \"tenant\".alpha (id INT PRIMARY KEY);")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS \"tenant\".beta (id INT PRIMARY KEY);")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
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
