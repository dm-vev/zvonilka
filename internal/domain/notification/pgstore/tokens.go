package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

// SavePushToken inserts or refreshes a push token.
func (s *Store) SavePushToken(ctx context.Context, token notification.PushToken) (notification.PushToken, error) {
	if err := s.requireStore(); err != nil {
		return notification.PushToken{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.PushToken{}, err
	}
	if s.tx != nil {
		return s.savePushToken(ctx, token)
	}

	var saved notification.PushToken
	err := s.WithinTx(ctx, func(tx notification.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).savePushToken(ctx, token)
		return saveErr
	})
	if err != nil {
		return notification.PushToken{}, err
	}

	return saved, nil
}

func (s *Store) savePushToken(ctx context.Context, token notification.PushToken) (notification.PushToken, error) {
	now := time.Now().UTC()
	token, err := notification.NormalizePushToken(token, now)
	if err != nil {
		return notification.PushToken{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id,
	account_id,
	device_id,
	provider,
	token,
	platform,
	enabled,
	created_at,
	updated_at,
	revoked_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (device_id) DO UPDATE SET
	id = EXCLUDED.id,
	account_id = EXCLUDED.account_id,
	provider = EXCLUDED.provider,
	token = EXCLUDED.token,
	platform = EXCLUDED.platform,
	enabled = EXCLUDED.enabled,
	created_at = EXCLUDED.created_at,
	updated_at = EXCLUDED.updated_at,
	revoked_at = EXCLUDED.revoked_at
WHERE %s.account_id = EXCLUDED.account_id
RETURNING id, account_id, device_id, provider, token, platform, enabled, created_at, updated_at, revoked_at
`, s.table("notification_push_tokens"), s.table("notification_push_tokens"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		token.ID,
		token.AccountID,
		token.DeviceID,
		token.Provider,
		token.Token,
		token.Platform,
		token.Enabled,
		token.CreatedAt.UTC(),
		token.UpdatedAt.UTC(),
		encodeTime(token.RevokedAt),
	)

	saved, err := scanPushToken(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.PushToken{}, notification.ErrConflict
		}
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return notification.PushToken{}, mappedErr
		}

		return notification.PushToken{}, fmt.Errorf("save push token for account %s and device %s: %w", token.AccountID, token.DeviceID, err)
	}

	return saved, nil
}

// PushTokenByID resolves a push token by its ID.
func (s *Store) PushTokenByID(ctx context.Context, tokenID string) (notification.PushToken, error) {
	if err := s.requireStore(); err != nil {
		return notification.PushToken{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.PushToken{}, err
	}
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return notification.PushToken{}, notification.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT id, account_id, device_id, provider, token, platform, enabled, created_at, updated_at, revoked_at
FROM %s
WHERE id = $1
`, s.table("notification_push_tokens"))

	row := s.conn().QueryRowContext(ctx, query, tokenID)
	token, err := scanPushToken(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.PushToken{}, notification.ErrNotFound
		}
		return notification.PushToken{}, fmt.Errorf("load push token %s: %w", tokenID, err)
	}

	return token, nil
}

// PushTokensByAccountID lists active push tokens for an account.
func (s *Store) PushTokensByAccountID(ctx context.Context, accountID string) ([]notification.PushToken, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, notification.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT id, account_id, device_id, provider, token, platform, enabled, created_at, updated_at, revoked_at
FROM %s
WHERE account_id = $1 AND enabled = TRUE AND revoked_at IS NULL
ORDER BY created_at ASC, id ASC
`, s.table("notification_push_tokens"))

	rows, err := s.conn().QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("list push tokens for account %s: %w", accountID, err)
	}
	defer rows.Close()

	tokens := make([]notification.PushToken, 0)
	for rows.Next() {
		token, scanErr := scanPushToken(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan push token for account %s: %w", accountID, scanErr)
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate push tokens for account %s: %w", accountID, err)
	}

	return tokens, nil
}

// DeletePushToken removes a push token by ID.
func (s *Store) DeletePushToken(ctx context.Context, tokenID string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return notification.ErrInvalidInput
	}

	query := fmt.Sprintf(`
DELETE FROM %s
WHERE id = $1
RETURNING id
`, s.table("notification_push_tokens"))

	row := s.conn().QueryRowContext(ctx, query, tokenID)
	var deleted string
	if err := row.Scan(&deleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.ErrNotFound
		}
		return fmt.Errorf("delete push token %s: %w", tokenID, err)
	}

	return nil
}
