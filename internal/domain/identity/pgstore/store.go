package pgstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

type sqlConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Store persists identity state in PostgreSQL.
type Store struct {
	db     *sql.DB
	tx     *sql.Tx
	schema string
}

// New constructs a PostgreSQL-backed identity store.
func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, identity.ErrInvalidInput
	}

	return &Store{
		db:     db,
		schema: normalizeSchema(schema),
	}, nil
}

// WithinTx executes the callback within a single database transaction.
func (s *Store) WithinTx(ctx context.Context, fn func(identity.Store) error) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return identity.ErrInvalidInput
	}

	return s.withTransaction(ctx, fn)
}

func (s *Store) withTransaction(ctx context.Context, fn func(identity.Store) error) error {
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
		return identity.ErrInvalidInput
	}

	return nil
}

func (s *Store) requireContext(ctx context.Context) error {
	if ctx == nil {
		return identity.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}

func (s *Store) requireContext(ctx context.Context) error {
	if ctx == nil {
		return identity.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}

func (s *Store) table(name string) string {
	return qualifiedName(s.schema, name)
}

func (s *Store) advisoryLockKey(resource string, value string) int64 {
	sum := sha256.Sum256([]byte(resource + ":" + value))
	key := binary.BigEndian.Uint64(sum[:8]) &^ (1 << 63)
	return int64(key)
}

func (s *Store) lockIdentifiers(ctx context.Context, identifiers ...string) error {
	seen := make(map[string]struct{}, len(identifiers))
	keys := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		identifier = strings.TrimSpace(identifier)
		if identifier == "" {
			continue
		}
		if _, ok := seen[identifier]; ok {
			continue
		}
		seen[identifier] = struct{}{}
		keys = append(keys, identifier)
	}

	sort.Strings(keys)
	for _, key := range keys {
		if _, err := s.conn().ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", s.advisoryLockKey("identity", key)); err != nil {
			return fmt.Errorf("lock identity resource %q: %w", key, err)
		}
	}

	return nil
}

func (s *Store) accountLockKeys(account identity.Account) []string {
	keys := make([]string, 0, 5)
	if strings.TrimSpace(account.ID) != "" {
		keys = append(keys, lockKey("identity:account:id", account.ID))
	}
	if strings.TrimSpace(account.Username) != "" {
		keys = append(keys, lockKey("identity:username", account.Username))
	}
	if strings.TrimSpace(account.Email) != "" {
		keys = append(keys, lockKey("identity:email", account.Email))
	}
	if strings.TrimSpace(account.Phone) != "" {
		keys = append(keys, lockKey("identity:phone", account.Phone))
	}
	if strings.TrimSpace(account.BotTokenHash) != "" {
		keys = append(keys, lockKey("identity:bot", account.BotTokenHash))
	}

	return keys
}

func (s *Store) joinLockKeys(joinRequest identity.JoinRequest) []string {
	keys := make([]string, 0, 4)
	if strings.TrimSpace(joinRequest.ID) != "" {
		keys = append(keys, lockKey("identity:join:id", joinRequest.ID))
	}
	if strings.TrimSpace(joinRequest.Username) != "" {
		keys = append(keys, lockKey("identity:username", joinRequest.Username))
	}
	if strings.TrimSpace(joinRequest.Email) != "" {
		keys = append(keys, lockKey("identity:email", joinRequest.Email))
	}
	if strings.TrimSpace(joinRequest.Phone) != "" {
		keys = append(keys, lockKey("identity:phone", joinRequest.Phone))
	}

	return keys
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

func notFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

var _ identity.Store = (*Store)(nil)
