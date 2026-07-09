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

// SaveOverride inserts or updates a per-chat notification override.
func (s *Store) SaveOverride(ctx context.Context, override notification.ConversationOverride) (notification.ConversationOverride, error) {
	if err := s.requireStore(); err != nil {
		return notification.ConversationOverride{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.ConversationOverride{}, err
	}
	if s.tx != nil {
		return s.saveOverride(ctx, override)
	}

	var saved notification.ConversationOverride
	err := s.WithinTx(ctx, func(tx notification.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveOverride(ctx, override)
		return saveErr
	})
	if err != nil {
		return notification.ConversationOverride{}, err
	}

	return saved, nil
}

func (s *Store) saveOverride(ctx context.Context, override notification.ConversationOverride) (notification.ConversationOverride, error) {
	now := time.Now().UTC()
	override, err := notification.NormalizeConversationOverride(override, now)
	if err != nil {
		return notification.ConversationOverride{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	conversation_id,
	account_id,
	muted,
	mentions_only,
	muted_until,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6
)
ON CONFLICT (conversation_id, account_id) DO UPDATE SET
	muted = EXCLUDED.muted,
	mentions_only = EXCLUDED.mentions_only,
	muted_until = EXCLUDED.muted_until,
	updated_at = EXCLUDED.updated_at
RETURNING conversation_id, account_id, muted, mentions_only, muted_until, updated_at
`, s.table("notification_conversation_overrides"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		override.ConversationID,
		override.AccountID,
		override.Muted,
		override.MentionsOnly,
		encodeTime(override.MutedUntil),
		override.UpdatedAt.UTC(),
	)

	saved, err := scanOverride(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.ConversationOverride{}, notification.ErrNotFound
		}
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return notification.ConversationOverride{}, mappedErr
		}

		return notification.ConversationOverride{}, fmt.Errorf(
			"save notification override for conversation %s and account %s: %w",
			override.ConversationID,
			override.AccountID,
			err,
		)
	}

	return saved, nil
}

// OverrideByConversationAndAccount resolves one per-chat override row.
func (s *Store) OverrideByConversationAndAccount(ctx context.Context, conversationID string, accountID string) (notification.ConversationOverride, error) {
	if err := s.requireStore(); err != nil {
		return notification.ConversationOverride{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.ConversationOverride{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	accountID = strings.TrimSpace(accountID)
	if conversationID == "" || accountID == "" {
		return notification.ConversationOverride{}, notification.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT conversation_id, account_id, muted, mentions_only, muted_until, updated_at
FROM %s
WHERE conversation_id = $1 AND account_id = $2
`, s.table("notification_conversation_overrides"))

	row := s.conn().QueryRowContext(ctx, query, conversationID, accountID)
	override, err := scanOverride(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.ConversationOverride{}, notification.ErrNotFound
		}
		return notification.ConversationOverride{}, fmt.Errorf(
			"load notification override for conversation %s and account %s: %w",
			conversationID,
			accountID,
			err,
		)
	}

	return override, nil
}
