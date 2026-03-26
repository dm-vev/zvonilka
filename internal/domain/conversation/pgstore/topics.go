package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SaveTopic inserts or updates a topic row.
func (s *Store) SaveTopic(ctx context.Context, topic conversation.ConversationTopic) (conversation.ConversationTopic, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.ConversationTopic{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.ConversationTopic{}, err
	}
	if topic.ConversationID == "" {
		return conversation.ConversationTopic{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveTopic(ctx, topic)
	}

	var saved conversation.ConversationTopic
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveTopic(ctx, topic)
		return saveErr
	})
	if err != nil {
		return conversation.ConversationTopic{}, err
	}

	return saved, nil
}

func (s *Store) saveTopic(ctx context.Context, topic conversation.ConversationTopic) (conversation.ConversationTopic, error) {
	topic.ConversationID = strings.TrimSpace(topic.ConversationID)
	topic.ID = strings.TrimSpace(topic.ID)
	topic.RootMessageID = strings.TrimSpace(topic.RootMessageID)
	topic.Title = strings.TrimSpace(topic.Title)
	topic.CreatedByAccountID = strings.TrimSpace(topic.CreatedByAccountID)
	if topic.ConversationID == "" || topic.CreatedByAccountID == "" || topic.Title == "" {
		return conversation.ConversationTopic{}, conversation.ErrInvalidInput
	}
	if topic.IsGeneral && topic.ID != "" {
		return conversation.ConversationTopic{}, conversation.ErrInvalidInput
	}
	if topic.IsGeneral && topic.RootMessageID != "" {
		return conversation.ConversationTopic{}, conversation.ErrInvalidInput
	}
	if !topic.IsGeneral && topic.ID == "" {
		return conversation.ConversationTopic{}, conversation.ErrInvalidInput
	}
	if topic.CreatedAt.IsZero() || topic.UpdatedAt.IsZero() || topic.UpdatedAt.Before(topic.CreatedAt) {
		return conversation.ConversationTopic{}, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	conversation_id, id, root_message_id, title, created_by_account_id, is_general, archived, pinned, closed,
	last_sequence, message_count, created_at, updated_at, last_message_at, archived_at, closed_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9,
	$10, $11, $12, $13, $14, $15, $16
)
ON CONFLICT (conversation_id, id) DO UPDATE SET
	root_message_id = EXCLUDED.root_message_id,
	title = EXCLUDED.title,
	created_by_account_id = EXCLUDED.created_by_account_id,
	is_general = EXCLUDED.is_general,
	archived = EXCLUDED.archived,
	pinned = EXCLUDED.pinned,
	closed = EXCLUDED.closed,
	last_sequence = EXCLUDED.last_sequence,
	message_count = EXCLUDED.message_count,
	updated_at = EXCLUDED.updated_at,
	last_message_at = EXCLUDED.last_message_at,
	archived_at = EXCLUDED.archived_at,
	closed_at = EXCLUDED.closed_at
RETURNING %s
`, s.table("conversation_topics"), topicColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		topic.ConversationID,
		topic.ID,
		nullString(topic.RootMessageID),
		topic.Title,
		topic.CreatedByAccountID,
		topic.IsGeneral,
		topic.Archived,
		topic.Pinned,
		topic.Closed,
		topic.LastSequence,
		topic.MessageCount,
		topic.CreatedAt.UTC(),
		topic.UpdatedAt.UTC(),
		encodeTime(topic.LastMessageAt),
		encodeTime(topic.ArchivedAt),
		encodeTime(topic.ClosedAt),
	)

	saved, err := scanTopic(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.ConversationTopic{}, mappedErr
		}
		return conversation.ConversationTopic{}, fmt.Errorf("save topic %s/%s: %w", topic.ConversationID, topic.ID, err)
	}

	return saved, nil
}

// TopicByConversationAndID resolves a topic by its composite key.
func (s *Store) TopicByConversationAndID(ctx context.Context, conversationID string, topicID string) (conversation.ConversationTopic, error) {
	if err := s.requireStore(); err != nil {
		return conversation.ConversationTopic{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.ConversationTopic{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	topicID = strings.TrimSpace(topicID)
	if conversationID == "" {
		return conversation.ConversationTopic{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE conversation_id = $1 AND id = $2`, topicColumnList, s.table("conversation_topics"))
	row := s.conn().QueryRowContext(ctx, query, conversationID, topicID)
	topic, err := scanTopic(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.ConversationTopic{}, conversation.ErrNotFound
		}
		return conversation.ConversationTopic{}, fmt.Errorf("load topic %s/%s: %w", conversationID, topicID, err)
	}

	return topic, nil
}

// TopicsByConversationID lists all topics for a conversation.
func (s *Store) TopicsByConversationID(ctx context.Context, conversationID string) ([]conversation.ConversationTopic, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE conversation_id = $1 ORDER BY is_general DESC, pinned DESC, last_sequence DESC, updated_at DESC, id ASC`, topicColumnList, s.table("conversation_topics"))
	rows, err := s.conn().QueryContext(ctx, query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list topics for conversation %s: %w", conversationID, err)
	}
	defer rows.Close()

	topics := make([]conversation.ConversationTopic, 0)
	for rows.Next() {
		topic, scanErr := scanTopic(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan topic for conversation %s: %w", conversationID, scanErr)
		}
		topics = append(topics, topic)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topics for conversation %s: %w", conversationID, err)
	}

	return topics, nil
}
