package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/presence"
)

// Store persists presence state in PostgreSQL.
type Store struct {
	db     *sql.DB
	tx     *sql.Tx
	schema string
}

// New constructs a PostgreSQL-backed presence store.
func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, presence.ErrInvalidInput
	}
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return nil, presence.ErrInvalidInput
	}

	return &Store{db: db, schema: schema}, nil
}

func (s *Store) conn() queryer {
	if s != nil && s.tx != nil {
		return s.tx
	}
	return s.db
}

func (s *Store) requireStore() error {
	if s == nil || s.db == nil {
		return presence.ErrInvalidInput
	}
	if strings.TrimSpace(s.schema) == "" {
		return presence.ErrInvalidInput
	}
	return nil
}

func (s *Store) requireContext(ctx context.Context) error {
	if ctx == nil {
		return presence.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// WithinTx runs the callback inside a database transaction.
func (s *Store) WithinTx(ctx context.Context, fn func(presence.Store) error) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return presence.ErrInvalidInput
	}
	if s.tx != nil {
		return fn(s)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin presence transaction: %w", err)
	}

	txStore := &Store{db: s.db, tx: tx, schema: s.schema}
	if err := fn(txStore); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback presence transaction: %w", rollbackErr))
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		if mappedErr := mapConstraintError(err, presence.ErrInvalidInput); mappedErr != nil {
			return mappedErr
		}
		return fmt.Errorf("commit presence transaction: %w", err)
	}

	return nil
}

// SavePresence inserts or updates a presence row.
func (s *Store) SavePresence(ctx context.Context, next presence.Presence) (presence.Presence, error) {
	if err := s.requireStore(); err != nil {
		return presence.Presence{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return presence.Presence{}, err
	}
	if s.tx != nil {
		return s.savePresence(ctx, next)
	}

	var saved presence.Presence
	err := s.WithinTx(ctx, func(tx presence.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).savePresence(ctx, next)
		return saveErr
	})
	if err != nil {
		return presence.Presence{}, err
	}

	return saved, nil
}

func (s *Store) savePresence(ctx context.Context, next presence.Presence) (presence.Presence, error) {
	next, err := presence.Normalize(next)
	if err != nil {
		return presence.Presence{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	account_id, state, custom_status, hidden_until, updated_at
) VALUES (
	$1, $2, $3, $4, $5
)
ON CONFLICT (account_id) DO UPDATE SET
	state = EXCLUDED.state,
	custom_status = EXCLUDED.custom_status,
	hidden_until = EXCLUDED.hidden_until,
	updated_at = EXCLUDED.updated_at
RETURNING account_id, state, custom_status, hidden_until, updated_at
`, s.table("user_presence"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		next.AccountID,
		next.State,
		next.CustomStatus,
		nullTime(next.HiddenUntil),
		next.UpdatedAt.UTC(),
	)

	saved, err := scanPresence(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, presence.ErrNotFound); mappedErr != nil {
			return presence.Presence{}, mappedErr
		}
		return presence.Presence{}, fmt.Errorf("save presence for account %s: %w", next.AccountID, err)
	}

	return saved, nil
}

// PresenceByAccountID resolves one presence row by account id.
func (s *Store) PresenceByAccountID(ctx context.Context, accountID string) (presence.Presence, error) {
	if err := s.requireStore(); err != nil {
		return presence.Presence{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return presence.Presence{}, err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return presence.Presence{}, presence.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT account_id, state, custom_status, hidden_until, updated_at
FROM %s
WHERE account_id = $1
`, s.table("user_presence"))
	row := s.conn().QueryRowContext(ctx, query, accountID)
	presenceRow, err := scanPresence(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return presence.Presence{}, presence.ErrNotFound
		}
		return presence.Presence{}, fmt.Errorf("load presence for account %s: %w", accountID, err)
	}

	return presenceRow, nil
}

// PresencesByAccountIDs resolves several presence rows in one query.
func (s *Store) PresencesByAccountIDs(ctx context.Context, accountIDs []string) (map[string]presence.Presence, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	accountIDs = presence.NormalizeIDs(accountIDs)
	if len(accountIDs) == 0 {
		return make(map[string]presence.Presence), nil
	}

	query := fmt.Sprintf(`
SELECT account_id, state, custom_status, hidden_until, updated_at
FROM %s
WHERE account_id = ANY($1)
ORDER BY account_id ASC
`, s.table("user_presence"))
	rows, err := s.conn().QueryContext(ctx, query, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("load presences: %w", err)
	}
	defer rows.Close()

	result := make(map[string]presence.Presence, len(accountIDs))
	for rows.Next() {
		rowPresence, scanErr := scanPresence(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan presence: %w", scanErr)
		}
		result[rowPresence.AccountID] = rowPresence
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate presences: %w", err)
	}

	return result, nil
}

func (s *Store) table(name string) string {
	return fmt.Sprintf("%s.%s", s.schema, name)
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func nullTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func scanPresence(scanner interface {
	Scan(dest ...any) error
}) (presence.Presence, error) {
	var (
		accountID    string
		state        string
		customStatus string
		hiddenUntil  sql.NullTime
		updatedAt    time.Time
	)
	if err := scanner.Scan(
		&accountID,
		&state,
		&customStatus,
		&hiddenUntil,
		&updatedAt,
	); err != nil {
		return presence.Presence{}, err
	}

	row := presence.Presence{
		AccountID:    strings.TrimSpace(accountID),
		State:        presence.PresenceState(strings.TrimSpace(state)),
		CustomStatus: strings.TrimSpace(customStatus),
		UpdatedAt:    updatedAt.UTC(),
	}
	if hiddenUntil.Valid {
		row.HiddenUntil = hiddenUntil.Time.UTC()
	}

	return presence.Normalize(row)
}
