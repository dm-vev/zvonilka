package pgstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func normalizeIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	ids := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ids = append(ids, value)
	}

	return ids
}

func (s *Store) replaceMentions(ctx context.Context, messageID string, mentionAccountIDs []string) error {
	if _, err := s.conn().ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE message_id = $1`, s.table("conversation_message_mentions")), messageID); err != nil {
		return fmt.Errorf("replace mentions for message %s: %w", messageID, err)
	}

	for idx, accountID := range mentionAccountIDs {
		query := fmt.Sprintf(`
INSERT INTO %s (
	message_id, mention_index, account_id
) VALUES (
	$1, $2, $3
)`, s.table("conversation_message_mentions"))

		if _, err := s.conn().ExecContext(ctx, query, messageID, idx, accountID); err != nil {
			if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
				return mappedErr
			}
			return fmt.Errorf("insert mention %s:%d: %w", messageID, idx, err)
		}
	}

	return nil
}

func (s *Store) mentionsByMessageIDs(ctx context.Context, messageIDs []string) (map[string][]string, error) {
	if len(messageIDs) == 0 {
		return make(map[string][]string), nil
	}

	query := fmt.Sprintf(`
SELECT message_id, account_id
FROM %s
WHERE message_id = ANY($1)
ORDER BY message_id ASC, mention_index ASC
`, s.table("conversation_message_mentions"))
	rows, err := s.conn().QueryContext(ctx, query, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("list mentions: %w", err)
	}
	defer rows.Close()

	mentions := make(map[string][]string, len(messageIDs))
	for rows.Next() {
		var messageID string
		var accountID string
		if err := rows.Scan(&messageID, &accountID); err != nil {
			return nil, fmt.Errorf("scan mention message id: %w", err)
		}
		mentions[messageID] = append(mentions[messageID], accountID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mentions: %w", err)
	}

	return mentions, nil
}
