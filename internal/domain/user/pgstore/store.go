package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

type sqlConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Store persists user state in PostgreSQL.
type Store struct {
	db     *sql.DB
	tx     *sql.Tx
	schema string
}

// New constructs a PostgreSQL-backed user store.
func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, domainuser.ErrInvalidInput
	}

	return &Store{db: db, schema: normalizeSchema(schema)}, nil
}

func (s *Store) WithinTx(ctx context.Context, fn func(domainuser.Store) error) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return domainuser.ErrInvalidInput
	}
	if s.tx != nil {
		return fn(s)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin user postgres transaction: %w", err)
	}

	txStore := &Store{db: s.db, tx: tx, schema: s.schema}
	if err := fn(txStore); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback user postgres transaction: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return mapped
		}
		return fmt.Errorf("commit user postgres transaction: %w", err)
	}
	return nil
}

func (s *Store) conn() sqlConn {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

func (s *Store) requireStore() error {
	if s == nil || s.db == nil {
		return domainuser.ErrInvalidInput
	}
	return nil
}

func (s *Store) requireContext(ctx context.Context) error {
	if ctx == nil {
		return domainuser.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Store) table(name string) string {
	return qualifiedName(s.schema, name)
}

func normalizeSchema(schema string) string {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return "public"
	}
	return schema
}

func qualifiedName(schema string, name string) string {
	if schema == "" {
		return name
	}
	return schema + "." + name
}

func mapConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}

	switch pgErr.Code {
	case "23505":
		return domainuser.ErrConflict
	case "23503":
		return domainuser.ErrNotFound
	case "23514":
		return domainuser.ErrInvalidInput
	default:
		return nil
	}
}

var _ domainuser.Store = (*Store)(nil)
