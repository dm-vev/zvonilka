package pgstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// ConversationCountersByAccount resolves unread counters for the requested conversations.
func (s *Store) ConversationCountersByAccount(ctx context.Context, accountID string, conversationIDs []string) (map[string]conversation.ConversationCounters, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	accountID = strings.TrimSpace(accountID)
	conversationIDs = normalizeIDs(conversationIDs)
	if accountID == "" {
		return nil, conversation.ErrInvalidInput
	}
	if len(conversationIDs) == 0 {
		return make(map[string]conversation.ConversationCounters), nil
	}

	query := fmt.Sprintf(`
WITH requested AS (
	SELECT unnest($2::text[]) AS conversation_id
),
members AS (
	SELECT conversation_id, role, joined_at
	FROM %s
	WHERE account_id = $1 AND conversation_id = ANY($2) AND left_at IS NULL AND banned = FALSE
),
watermarks AS (
	SELECT conversation_id, COALESCE(MAX(last_read_sequence), 0) AS last_read_sequence
	FROM %s
	WHERE account_id = $1 AND conversation_id = ANY($2)
	GROUP BY conversation_id
),
counts AS (
	SELECT
		m.conversation_id,
		COUNT(*) FILTER (
			WHERE m.sequence > COALESCE(w.last_read_sequence, 0)
				AND m.deleted_at IS NULL
				AND m.status <> 'deleted'
				AND m.sender_account_id <> $1
				AND m.created_at >= members.joined_at
				AND (
					members.role IN ('owner', 'admin')
					OR NOT EXISTS (
						SELECT 1
						FROM %s AS rs
						WHERE rs.state = 'shadowed'
							AND rs.account_id = m.sender_account_id
							AND (
								(rs.target_kind = 'conversation' AND rs.target_id = m.conversation_id)
								OR (
									rs.target_kind = 'topic'
									AND rs.target_id = CASE
										WHEN m.thread_id = '' THEN ''
										ELSE m.conversation_id || %s || m.thread_id
									END
								)
							)
							AND (rs.expires_at IS NULL OR rs.expires_at > CURRENT_TIMESTAMP)
					)
				)
		) AS unread_count,
		COUNT(*) FILTER (
			WHERE m.sequence > COALESCE(w.last_read_sequence, 0)
				AND m.deleted_at IS NULL
				AND m.status <> 'deleted'
				AND m.sender_account_id <> $1
				AND m.created_at >= members.joined_at
				AND mm.account_id IS NOT NULL
				AND (
					members.role IN ('owner', 'admin')
					OR NOT EXISTS (
						SELECT 1
						FROM %s AS rs
						WHERE rs.state = 'shadowed'
							AND rs.account_id = m.sender_account_id
							AND (
								(rs.target_kind = 'conversation' AND rs.target_id = m.conversation_id)
								OR (
									rs.target_kind = 'topic'
									AND rs.target_id = CASE
										WHEN m.thread_id = '' THEN ''
										ELSE m.conversation_id || %s || m.thread_id
									END
								)
							)
							AND (rs.expires_at IS NULL OR rs.expires_at > CURRENT_TIMESTAMP)
					)
				)
		) AS unread_mention_count
	FROM %s AS m
	JOIN members ON members.conversation_id = m.conversation_id
	LEFT JOIN watermarks w ON w.conversation_id = m.conversation_id
	LEFT JOIN %s AS mm ON mm.message_id = m.id AND mm.account_id = $1
	WHERE m.conversation_id = ANY($2)
	GROUP BY m.conversation_id
)
SELECT requested.conversation_id, COALESCE(counts.unread_count, 0), COALESCE(counts.unread_mention_count, 0)
FROM requested
LEFT JOIN counts ON counts.conversation_id = requested.conversation_id
ORDER BY requested.conversation_id ASC
`, s.table("conversation_members"), s.table("conversation_read_states"), s.table("conversation_moderation_restrictions"), "E'\\x1f'", s.table("conversation_moderation_restrictions"), "E'\\x1f'", s.table("conversation_messages"), s.table("conversation_message_mentions"))

	rows, err := s.conn().QueryContext(ctx, query, accountID, conversationIDs)
	if err != nil {
		return nil, fmt.Errorf("list conversation counters for account %s: %w", accountID, err)
	}
	defer rows.Close()

	counters := make(map[string]conversation.ConversationCounters, len(conversationIDs))
	for rows.Next() {
		var counter conversation.ConversationCounters
		if err := rows.Scan(
			&counter.ConversationID,
			&counter.UnreadCount,
			&counter.UnreadMentionCount,
		); err != nil {
			return nil, fmt.Errorf("scan conversation counters for account %s: %w", accountID, err)
		}
		counters[counter.ConversationID] = counter
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation counters for account %s: %w", accountID, err)
	}

	for _, conversationID := range conversationIDs {
		if _, ok := counters[conversationID]; ok {
			continue
		}
		counters[conversationID] = conversation.ConversationCounters{ConversationID: conversationID}
	}

	return counters, nil
}
