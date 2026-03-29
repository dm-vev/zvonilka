package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dm-vev/zvonilka/internal/domain/e2ee"
)

type sqlConn interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Store struct {
	db     *sql.DB
	tx     *sql.Tx
	schema string
}

func New(db *sql.DB, schema string) (*Store, error) {
	if db == nil {
		return nil, e2ee.ErrInvalidInput
	}
	return &Store{db: db, schema: normalizeSchema(schema)}, nil
}

func (s *Store) WithinTx(ctx context.Context, fn func(e2ee.Store) error) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return e2ee.ErrInvalidInput
	}
	if s.tx != nil {
		return fn(s)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin postgres transaction: %w", err)
	}
	txStore := &Store{db: s.db, tx: tx, schema: s.schema}
	if err := fn(txStore); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("rollback postgres transaction: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit postgres transaction: %w", mapConstraintError(err))
	}
	return nil
}

func (s *Store) SaveSignedPreKey(ctx context.Context, accountID string, deviceID string, value e2ee.SignedPreKey) (e2ee.SignedPreKey, error) {
	if err := s.requireContext(ctx); err != nil {
		return e2ee.SignedPreKey{}, err
	}
	query := fmt.Sprintf(`
INSERT INTO %s (
	account_id, device_id, key_id, algorithm, public_key, signature, created_at, rotated_at, expires_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())
ON CONFLICT (account_id, device_id)
DO UPDATE SET
	key_id = EXCLUDED.key_id,
	algorithm = EXCLUDED.algorithm,
	public_key = EXCLUDED.public_key,
	signature = EXCLUDED.signature,
	created_at = EXCLUDED.created_at,
	rotated_at = EXCLUDED.rotated_at,
	expires_at = EXCLUDED.expires_at,
	updated_at = NOW()
RETURNING key_id, algorithm, public_key, signature, created_at, rotated_at, expires_at
`, s.table("e2ee_signed_prekeys"))
	var saved e2ee.SignedPreKey
	err := s.conn().QueryRowContext(
		ctx,
		query,
		accountID,
		deviceID,
		value.Key.KeyID,
		value.Key.Algorithm,
		value.Key.PublicKey,
		value.Signature,
		nullTime(value.Key.CreatedAt),
		nullTime(value.Key.RotatedAt),
		nullTime(value.Key.ExpiresAt),
	).Scan(
		&saved.Key.KeyID,
		&saved.Key.Algorithm,
		&saved.Key.PublicKey,
		&saved.Signature,
		&saved.Key.CreatedAt,
		&saved.Key.RotatedAt,
		&saved.Key.ExpiresAt,
	)
	if err != nil {
		return e2ee.SignedPreKey{}, mapConstraintError(err)
	}
	return saved, nil
}

func (s *Store) SignedPreKeyByDevice(ctx context.Context, accountID string, deviceID string) (e2ee.SignedPreKey, error) {
	query := fmt.Sprintf(`
SELECT key_id, algorithm, public_key, signature, created_at, rotated_at, expires_at
FROM %s
WHERE account_id = $1 AND device_id = $2
`, s.table("e2ee_signed_prekeys"))
	var value e2ee.SignedPreKey
	err := s.conn().QueryRowContext(ctx, query, accountID, deviceID).Scan(
		&value.Key.KeyID,
		&value.Key.Algorithm,
		&value.Key.PublicKey,
		&value.Signature,
		&value.Key.CreatedAt,
		&value.Key.RotatedAt,
		&value.Key.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return e2ee.SignedPreKey{}, e2ee.ErrNotFound
	}
	if err != nil {
		return e2ee.SignedPreKey{}, err
	}
	return value, nil
}

func (s *Store) DeleteOneTimePreKeysByDevice(ctx context.Context, accountID string, deviceID string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE account_id = $1 AND device_id = $2 AND claimed_at IS NULL`, s.table("e2ee_one_time_prekeys"))
	if _, err := s.conn().ExecContext(ctx, query, accountID, deviceID); err != nil {
		return err
	}
	return nil
}

func (s *Store) SaveOneTimePreKeys(ctx context.Context, accountID string, deviceID string, values []e2ee.OneTimePreKey) error {
	for _, value := range values {
		query := fmt.Sprintf(`
INSERT INTO %s (
	account_id, device_id, key_id, algorithm, public_key, created_at, rotated_at, expires_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (account_id, device_id, key_id)
DO UPDATE SET
	algorithm = EXCLUDED.algorithm,
	public_key = EXCLUDED.public_key,
	created_at = EXCLUDED.created_at,
	rotated_at = EXCLUDED.rotated_at,
	expires_at = EXCLUDED.expires_at
`, s.table("e2ee_one_time_prekeys"))
		if _, err := s.conn().ExecContext(
			ctx,
			query,
			accountID,
			deviceID,
			value.Key.KeyID,
			value.Key.Algorithm,
			value.Key.PublicKey,
			nullTime(value.Key.CreatedAt),
			nullTime(value.Key.RotatedAt),
			nullTime(value.Key.ExpiresAt),
		); err != nil {
			return mapConstraintError(err)
		}
	}
	return nil
}

func (s *Store) ClaimOneTimePreKey(
	ctx context.Context,
	accountID string,
	deviceID string,
	claimedByAccountID string,
	claimedByDeviceID string,
) (e2ee.OneTimePreKey, error) {
	query := fmt.Sprintf(`
WITH selected AS (
	SELECT account_id, device_id, key_id
	FROM %s
	WHERE account_id = $1 AND device_id = $2 AND claimed_at IS NULL
	ORDER BY created_at, key_id
	FOR UPDATE SKIP LOCKED
	LIMIT 1
)
UPDATE %s AS keys
SET claimed_at = NOW(), claimed_by_account_id = $3, claimed_by_device_id = $4
FROM selected
WHERE keys.account_id = selected.account_id
  AND keys.device_id = selected.device_id
  AND keys.key_id = selected.key_id
RETURNING keys.key_id, keys.algorithm, keys.public_key, keys.created_at, keys.rotated_at, keys.expires_at, keys.claimed_at, keys.claimed_by_account_id, keys.claimed_by_device_id
`, s.table("e2ee_one_time_prekeys"), s.table("e2ee_one_time_prekeys"))
	var value e2ee.OneTimePreKey
	err := s.conn().QueryRowContext(ctx, query, accountID, deviceID, claimedByAccountID, claimedByDeviceID).Scan(
		&value.Key.KeyID,
		&value.Key.Algorithm,
		&value.Key.PublicKey,
		&value.Key.CreatedAt,
		&value.Key.RotatedAt,
		&value.Key.ExpiresAt,
		&value.ClaimedAt,
		&value.ClaimedByAccountID,
		&value.ClaimedByDeviceID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return e2ee.OneTimePreKey{}, e2ee.ErrNotFound
	}
	if err != nil {
		return e2ee.OneTimePreKey{}, err
	}
	return value, nil
}

func (s *Store) CountAvailableOneTimePreKeys(ctx context.Context, accountID string, deviceID string) (uint32, error) {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE account_id = $1 AND device_id = $2 AND claimed_at IS NULL`, s.table("e2ee_one_time_prekeys"))
	var count uint32
	if err := s.conn().QueryRowContext(ctx, query, accountID, deviceID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) conn() sqlConn {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

func (s *Store) requireStore() error {
	if s == nil || s.db == nil {
		return e2ee.ErrInvalidInput
	}
	return nil
}

func (s *Store) requireContext(ctx context.Context) error {
	if ctx == nil {
		return e2ee.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Store) table(name string) string {
	return qualifiedName(s.schema, name)
}

func normalizeSchema(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "public"
	}
	return value
}

func qualifiedName(schema string, name string) string {
	return `"` + normalizeSchema(schema) + `"."` + strings.TrimSpace(name) + `"`
}

func mapConstraintError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23503", "23514":
			return fmt.Errorf("%w: %v", e2ee.ErrConflict, err)
		}
	}
	return err
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

var _ e2ee.Store = (*Store)(nil)
