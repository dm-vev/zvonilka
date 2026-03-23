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
	cfg config.Configuration

	once      sync.Once
	db        *sql.DB
	openErr   error
	closeOnce sync.Once
	closeErr  error
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

	b.once.Do(func() {
		b.openErr = b.open(ctx)
	})

	return b.db, b.openErr
}

// Close closes the shared database pool once.
func (b *Bootstrap) Close(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("context is required")
	}

	b.closeOnce.Do(func() {
		if b.db == nil {
			return
		}
		b.closeErr = b.db.Close()
	})

	return b.closeErr
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
		_ = db.Close()
		return fmt.Errorf("ping postgres database: %w", err)
	}

	migrationsPath := b.cfg.Infrastructure.Postgres.MigrationsPath
	if migrationsPath == "" {
		migrationsPath = "deploy/migrations/postgres"
	}
	if err := applyMigrations(ctx, db, migrationsPath, b.cfg.Infrastructure.Postgres.Schema); err != nil {
		_ = db.Close()
		return err
	}

	b.db = db
	return nil
}
