package pgstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

// DeleteOverride removes one account's per-conversation override.
func (s *Store) DeleteOverride(ctx context.Context, conversationID string, accountID string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	conversationID = strings.TrimSpace(conversationID)
	accountID = strings.TrimSpace(accountID)
	if conversationID == "" || accountID == "" {
		return notification.ErrInvalidInput
	}
	query := fmt.Sprintf(`DELETE FROM %s WHERE conversation_id = $1 AND account_id = $2`,
		s.table("notification_conversation_overrides"))
	result, err := s.conn().ExecContext(ctx, query, conversationID, accountID)
	if err != nil {
		return fmt.Errorf("delete notification override: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted notification overrides: %w", err)
	}
	if count == 0 {
		return notification.ErrNotFound
	}
	return nil
}
