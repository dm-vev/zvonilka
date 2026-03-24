package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

type sqlConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Store persists messenger state in PostgreSQL.
type Store struct {
	db     *sql.DB
	tx     *sql.Tx
	schema string
}

// New constructs a PostgreSQL-backed conversation store.
func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, conversation.ErrInvalidInput
	}

	return &Store{
		db:     db,
		schema: normalizeSchema(schema),
	}, nil
}

// WithinTx executes the callback within a single database transaction.
func (s *Store) WithinTx(ctx context.Context, fn func(conversation.Store) error) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return conversation.ErrInvalidInput
	}

	return s.withTransaction(ctx, fn)
}

func (s *Store) withTransaction(ctx context.Context, fn func(conversation.Store) error) error {
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
		return fmt.Errorf("begin postgres transaction: %w", err)
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
				if mappedErr := mapConstraintError(commitErr, conversation.ErrConflict); mappedErr != nil {
					return errors.Join(
						domainstorage.UnwrapCommit(err),
						fmt.Errorf("commit postgres transaction: %w", mappedErr),
					)
				}

				return errors.Join(
					domainstorage.UnwrapCommit(err),
					fmt.Errorf("commit postgres transaction: %w", commitErr),
				)
			}

			return domainstorage.UnwrapCommit(err)
		}

		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback postgres transaction: %w", rollbackErr))
		}

		return err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		if mappedErr := mapConstraintError(commitErr, conversation.ErrConflict); mappedErr != nil {
			return fmt.Errorf("commit postgres transaction: %w", mappedErr)
		}
		return fmt.Errorf("commit postgres transaction: %w", commitErr)
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
		return conversation.ErrInvalidInput
	}

	return nil
}

func (s *Store) requireContext(ctx context.Context) error {
	if ctx == nil {
		return conversation.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}

func (s *Store) table(name string) string {
	return qualifiedName(s.schema, name)
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

var _ conversation.Store = (*Store)(nil)
