package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dm-vev/zvonilka/internal/domain/search"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

type sqlConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Store persists search documents in PostgreSQL.
type Store struct {
	db     *sql.DB
	tx     *sql.Tx
	schema string
}

// New constructs a PostgreSQL-backed search store.
func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, search.ErrInvalidInput
	}

	return &Store{
		db:     db,
		schema: normalizeSchema(schema),
	}, nil
}

// WithinTx executes the callback within a single database transaction.
func (s *Store) WithinTx(ctx context.Context, fn func(search.Store) error) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return search.ErrInvalidInput
	}

	return s.withTransaction(ctx, fn)
}

func (s *Store) withTransaction(ctx context.Context, fn func(search.Store) error) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if s.tx != nil {
		return fn(s)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin search transaction: %w", err)
	}

	txStore := &Store{
		db:     s.db,
		tx:     tx,
		schema: s.schema,
	}

	err = fn(txStore)
	if err != nil {
		if domainstorage.IsCommit(err) {
			if commitErr := tx.Commit(); commitErr != nil {
				return errors.Join(domainstorage.UnwrapCommit(err), fmt.Errorf("commit search transaction: %w", commitErr))
			}

			return domainstorage.UnwrapCommit(err)
		}

		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback search transaction: %w", rollbackErr))
		}

		return err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		if mappedErr := mapConstraintError(commitErr); mappedErr != nil {
			return fmt.Errorf("commit search transaction: %w", mappedErr)
		}
		return fmt.Errorf("commit search transaction: %w", commitErr)
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
		return search.ErrInvalidInput
	}
	if strings.TrimSpace(s.schema) == "" {
		return search.ErrInvalidInput
	}

	return nil
}

func (s *Store) requireContext(ctx context.Context) error {
	if ctx == nil {
		return search.ErrInvalidInput
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
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}

	switch pgErr.Code {
	case "23505":
		return search.ErrConflict
	case "23503":
		return search.ErrNotFound
	case "23514":
		return search.ErrInvalidInput
	default:
		return nil
	}
}

var _ search.Store = (*Store)(nil)
