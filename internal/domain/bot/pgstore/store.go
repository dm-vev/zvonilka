package pgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

type sqlConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Store persists bot state in PostgreSQL.
type Store struct {
	db     *sql.DB
	tx     *sql.Tx
	schema string
}

// New constructs a PostgreSQL-backed bot store.
func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, bot.ErrInvalidInput
	}

	return &Store{
		db:     db,
		schema: normalizeSchema(schema),
	}, nil
}

// WithinTx executes the callback within a single transaction.
func (s *Store) WithinTx(ctx context.Context, fn func(bot.Store) error) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return bot.ErrInvalidInput
	}
	if s.tx != nil {
		return fn(s)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin bot transaction: %w", err)
	}

	txStore := &Store{
		db:     s.db,
		tx:     tx,
		schema: s.schema,
	}
	if err := fn(txStore); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback bot transaction: %w", rollbackErr))
		}

		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit bot transaction: %w", err)
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
		return bot.ErrInvalidInput
	}
	if strings.TrimSpace(s.schema) == "" {
		return bot.ErrInvalidInput
	}

	return nil
}

func (s *Store) requireContext(ctx context.Context) error {
	if ctx == nil {
		return bot.ErrInvalidInput
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
		return bot.ErrConflict
	case isForeignKeyViolation(err):
		return bot.ErrNotFound
	case isCheckViolation(err):
		return bot.ErrInvalidInput
	default:
		return nil
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func isCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}

func encodeUpdate(update bot.Update) ([]byte, error) {
	return json.Marshal(update)
}

func decodeUpdate(raw []byte) (bot.Update, error) {
	if len(raw) == 0 {
		return bot.Update{}, bot.ErrInvalidInput
	}

	var update bot.Update
	if err := json.Unmarshal(raw, &update); err != nil {
		return bot.Update{}, err
	}

	return update, nil
}

func encodeAllowedUpdates(values []bot.UpdateType) ([]byte, error) {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, string(value))
	}

	return json.Marshal(items)
}

func decodeAllowedUpdates(raw []byte) ([]bot.UpdateType, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}

	result := make([]bot.UpdateType, 0, len(items))
	for _, item := range items {
		result = append(result, bot.UpdateType(item))
	}

	return result, nil
}

var _ bot.Store = (*Store)(nil)
