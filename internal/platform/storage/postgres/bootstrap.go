package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/dm-vev/zvonilka/internal/platform/config"
)

var openDatabase = sql.Open

// Bootstrap owns the shared PostgreSQL connection pool and migrations.
type Bootstrap struct {
	mu  sync.Mutex
	cfg config.Configuration
	db  *sql.DB
}

// NewBootstrap constructs a PostgreSQL bootstrap from the service configuration.
func NewBootstrap(cfg config.Configuration) *Bootstrap {
	return &Bootstrap{cfg: cfg}
}

// Open returns the shared PostgreSQL pool, applying migrations on first use.
func (b *Bootstrap) Open(ctx context.Context) (*sql.DB, error) {
	if b == nil {
		return nil, errors.New("postgres bootstrap is required")
	}
	if ctx == nil {
		return nil, errors.New("context is required")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.db != nil {
		return b.db, nil
	}

	if err := b.open(ctx); err != nil {
		return nil, err
	}

	return b.db, nil
}

// Close closes the current database pool if it is open.
func (b *Bootstrap) Close(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("context is required")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	db := b.db
	b.db = nil

	if db == nil {
		return nil
	}

	if err := db.Close(); err != nil {
		return fmt.Errorf("close postgres database: %w", err)
	}

	return nil
}

func (b *Bootstrap) open(ctx context.Context) error {
	if !b.cfg.Infrastructure.Postgres.Enabled {
		return fmt.Errorf("postgres is not enabled")
	}
	if b.cfg.Infrastructure.Postgres.DSN == "" {
		return errors.New("postgres DSN is required")
	}

	db, err := openDatabase("pgx", b.cfg.Infrastructure.Postgres.DSN)
	if err != nil {
		return fmt.Errorf("open postgres database: %w", err)
	}

	db.SetMaxOpenConns(b.cfg.Infrastructure.Postgres.MaxOpenConns)
	db.SetMaxIdleConns(b.cfg.Infrastructure.Postgres.MaxIdleConns)
	db.SetConnMaxLifetime(b.cfg.Infrastructure.Postgres.ConnMaxLifetime)
	db.SetConnMaxIdleTime(b.cfg.Infrastructure.Postgres.ConnMaxIdleTime)

	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()

	if err := db.PingContext(pingCtx); err != nil {
		return errors.Join(
			fmt.Errorf("ping postgres database: %w", err),
			closeDatabase(db),
		)
	}

	migrationsPath := b.cfg.Infrastructure.Postgres.MigrationsPath
	if migrationsPath == "" {
		migrationsPath = "deploy/migrations/postgres"
	}
	if err := applyMigrations(ctx, db, migrationsPath, b.cfg.Infrastructure.Postgres.Schema); err != nil {
		return errors.Join(err, closeDatabase(db))
	}

	b.db = db
	return nil
}

func closeDatabase(db *sql.DB) error {
	if db == nil {
		return nil
	}

	if err := db.Close(); err != nil {
		return fmt.Errorf("close postgres database: %w", err)
	}

	return nil
}
