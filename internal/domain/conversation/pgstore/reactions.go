package pgstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SaveMessageReaction inserts or updates a message reaction row.
func (s *Store) SaveMessageReaction(ctx context.Context, reaction conversation.MessageReaction) (conversation.MessageReaction, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.MessageReaction{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.MessageReaction{}, err
	}
	if reaction.MessageID == "" {
		return conversation.MessageReaction{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveMessageReaction(ctx, reaction)
	}

	var saved conversation.MessageReaction
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveMessageReaction(ctx, reaction)
		return saveErr
	})
	if err != nil {
		return conversation.MessageReaction{}, err
	}

	return saved, nil
}

func (s *Store) saveMessageReaction(ctx context.Context, reaction conversation.MessageReaction) (conversation.MessageReaction, error) {
	reaction.MessageID = strings.TrimSpace(reaction.MessageID)
	reaction.AccountID = strings.TrimSpace(reaction.AccountID)
	reaction.Reaction = strings.TrimSpace(reaction.Reaction)
	if reaction.MessageID == "" || reaction.AccountID == "" || reaction.Reaction == "" {
		return conversation.MessageReaction{}, conversation.ErrInvalidInput
	}
	if reaction.CreatedAt.IsZero() || reaction.UpdatedAt.IsZero() || reaction.UpdatedAt.Before(reaction.CreatedAt) {
		return conversation.MessageReaction{}, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	message_id, account_id, reaction, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5
)
ON CONFLICT (message_id, account_id) DO UPDATE SET
	reaction = EXCLUDED.reaction,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("conversation_message_reactions"), reactionColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		reaction.MessageID,
		reaction.AccountID,
		reaction.Reaction,
		reaction.CreatedAt.UTC(),
		reaction.UpdatedAt.UTC(),
	)

	saved, err := scanReaction(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.MessageReaction{}, mappedErr
		}
		return conversation.MessageReaction{}, fmt.Errorf("save message reaction %s/%s: %w", reaction.MessageID, reaction.AccountID, err)
	}

	return saved, nil
}

// DeleteMessageReaction removes a message reaction row.
func (s *Store) DeleteMessageReaction(ctx context.Context, messageID string, accountID string) error {
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if err := s.requireStore(); err != nil {
		return err
	}
	if strings.TrimSpace(messageID) == "" || strings.TrimSpace(accountID) == "" {
		return conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.deleteMessageReaction(ctx, messageID, accountID)
	}

	return s.withTransaction(ctx, func(tx conversation.Store) error {
		return tx.(*Store).deleteMessageReaction(ctx, messageID, accountID)
	})
}

func (s *Store) deleteMessageReaction(ctx context.Context, messageID string, accountID string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE message_id = $1 AND account_id = $2`, s.table("conversation_message_reactions"))
	if _, err := s.conn().ExecContext(ctx, query, strings.TrimSpace(messageID), strings.TrimSpace(accountID)); err != nil {
		return fmt.Errorf("delete message reaction %s/%s: %w", messageID, accountID, err)
	}

	return nil
}

func (s *Store) reactionsByMessageIDs(ctx context.Context, messageIDs []string) (map[string][]conversation.MessageReaction, error) {
	if len(messageIDs) == 0 {
		return make(map[string][]conversation.MessageReaction), nil
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE message_id = ANY($1)
ORDER BY message_id ASC, updated_at DESC, account_id ASC
`, reactionColumnList, s.table("conversation_message_reactions"))
	rows, err := s.conn().QueryContext(ctx, query, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("list message reactions: %w", err)
	}
	defer rows.Close()

	reactions := make(map[string][]conversation.MessageReaction, len(messageIDs))
	for rows.Next() {
		reaction, scanErr := scanReaction(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan message reaction: %w", scanErr)
		}
		reactions[reaction.MessageID] = append(reactions[reaction.MessageID], reaction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate message reactions: %w", err)
	}

	return reactions, nil
}
