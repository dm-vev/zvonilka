package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

type sqlConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Store persists notification state in PostgreSQL.
type Store struct {
	db     *sql.DB
	tx     *sql.Tx
	schema string
}

// New constructs a PostgreSQL-backed notification store.
func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, notification.ErrInvalidInput
	}

	return &Store{
		db:     db,
		schema: normalizeSchema(schema),
	}, nil
}

// WithinTx executes the callback within a single database transaction.
func (s *Store) WithinTx(ctx context.Context, fn func(notification.Store) error) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return notification.ErrInvalidInput
	}

	if s.tx != nil {
		return fn(s)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin notification transaction: %w", err)
	}

	txStore := &Store{
		db:     s.db,
		tx:     tx,
		schema: s.schema,
	}

	if err := fn(txStore); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback notification transaction: %w", rollbackErr))
		}

		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit notification transaction: %w", err)
	}

	return nil
}

func (s *Store) conn() sqlConn {
	if s == nil {
		return nil
	}
	if s.tx != nil {
		return s.tx
	}

	return s.db
}

func (s *Store) requireStore() error {
	if s == nil || s.db == nil {
		return notification.ErrInvalidInput
	}
	if strings.TrimSpace(s.schema) == "" {
		return notification.ErrInvalidInput
	}

	return nil
}

func (s *Store) requireContext(ctx context.Context) error {
	if ctx == nil {
		return notification.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}

func (s *Store) table(name string) string {
	return qualifiedName(s.schema, name)
}

func mapConstraintError(err error) error {
	switch {
	case isUniqueViolation(err):
		return notification.ErrConflict
	case isForeignKeyViolation(err):
		return notification.ErrNotFound
	case isCheckViolation(err):
		return notification.ErrInvalidInput
	default:
		return nil
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}

	return false
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return true
	}

	return false
}

func isCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return true
	}

	return false
}

var _ notification.Store = (*Store)(nil)
