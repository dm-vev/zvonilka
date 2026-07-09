package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dm-vev/zvonilka/internal/domain/callhook"
)

type queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Store persists call hook jobs in PostgreSQL.
type Store struct {
	db     *sql.DB
	tx     *sql.Tx
	schema string
}

// New constructs a PostgreSQL-backed callhook store.
func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, callhook.ErrInvalidInput
	}
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return nil, callhook.ErrInvalidInput
	}

	return &Store{db: db, schema: schema}, nil
}

func (s *Store) conn() queryer {
	if s != nil && s.tx != nil {
		return s.tx
	}
	return s.db
}

func (s *Store) table(name string) string {
	return fmt.Sprintf(`"%s"."%s"`, s.schema, name)
}

func (s *Store) requireStore() error {
	if s == nil || s.db == nil || strings.TrimSpace(s.schema) == "" {
		return callhook.ErrInvalidInput
	}
	return nil
}

func (s *Store) requireContext(ctx context.Context) error {
	if ctx == nil {
		return callhook.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// WithinTx runs one transaction.
func (s *Store) WithinTx(ctx context.Context, fn func(callhook.Store) error) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return callhook.ErrInvalidInput
	}
	if s.tx != nil {
		return fn(s)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin callhook transaction: %w", err)
	}

	txStore := &Store{db: s.db, tx: tx, schema: s.schema}
	if err := fn(txStore); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback callhook transaction: %w", rollbackErr))
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return mappedErr
		}

		return fmt.Errorf("commit callhook transaction: %w", err)
	}

	return nil
}

func mapConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}

	switch pgErr.Code {
	case "23505":
		return callhook.ErrConflict
	case "23503":
		return callhook.ErrNotFound
	case "23514":
		return callhook.ErrInvalidInput
	default:
		return nil
	}
}

var _ callhook.Store = (*Store)(nil)
