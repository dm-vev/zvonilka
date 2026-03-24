package teststore

import (
	"context"
	"sort"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func (s *memoryStore) SaveMessage(ctx context.Context, message conversation.Message) (conversation.Message, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.Message{}, err
	}
	message.ID = strings.TrimSpace(message.ID)
	message.ConversationID = strings.TrimSpace(message.ConversationID)
	message.SenderAccountID = strings.TrimSpace(message.SenderAccountID)
	message.SenderDeviceID = strings.TrimSpace(message.SenderDeviceID)
	message.MentionAccountIDs = normalizeIDs(message.MentionAccountIDs)
	if message.ID == "" || message.ConversationID == "" || message.SenderAccountID == "" || message.SenderDeviceID == "" {
		return conversation.Message{}, conversation.ErrInvalidInput
	}
	if message.Kind == conversation.MessageKindUnspecified {
		return conversation.Message{}, conversation.ErrInvalidInput
	}
	if message.Status == conversation.MessageStatusUnspecified {
		return conversation.Message{}, conversation.ErrInvalidInput
	}
	if err := conversation.ValidateMessagePayload(message.Payload, false); err != nil {
		return conversation.Message{}, err
	}

	conversation.StripMessageHints(&message)

	s.messagesByID[message.ID] = cloneMessage(message)

	saved := cloneMessage(message)
	saved.MentionAccountIDs = append([]string(nil), message.MentionAccountIDs...)
	saved.Reactions = s.reactionsByMessageIDs([]string{saved.ID})[saved.ID]
	return saved, nil
}

func (s *memoryStore) MessageByID(ctx context.Context, conversationID string, messageID string) (conversation.Message, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.Message{}, err
	}
	messageID = strings.TrimSpace(messageID)
	conversationID = strings.TrimSpace(conversationID)
	message, ok := s.messagesByID[messageID]
	if !ok || message.ConversationID != conversationID {
		return conversation.Message{}, conversation.ErrNotFound
	}

	saved := cloneMessage(message)
	saved.MentionAccountIDs = append([]string(nil), message.MentionAccountIDs...)
	saved.Reactions = s.reactionsByMessageIDs([]string{saved.ID})[saved.ID]
	return saved, nil
}

func (s *memoryStore) MessagesByConversationID(ctx context.Context, conversationID string, threadID string, fromSequence uint64, limit int) ([]conversation.Message, error) {
	if err := s.validateRead(ctx); err != nil {
		return nil, err
	}
	conversationID = strings.TrimSpace(conversationID)
	threadID = strings.TrimSpace(threadID)
	if conversationID == "" {
		return nil, conversation.ErrInvalidInput
	}

	rows := make([]conversation.Message, 0)
	for _, message := range s.messagesByID {
		if message.ConversationID != conversationID || strings.TrimSpace(message.ThreadID) != threadID {
			continue
		}
		rows = append(rows, cloneMessage(message))
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Sequence == rows[j].Sequence {
			if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
				return rows[i].ID < rows[j].ID
			}
			return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		}
		return rows[i].Sequence < rows[j].Sequence
	})

	filtered := rows[:0]
	for _, message := range rows {
		if message.Sequence <= fromSequence {
			continue
		}
		filtered = append(filtered, message)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}

	reactions := s.reactionsByMessageIDs(func() []string {
		ids := make([]string, 0, len(filtered))
		for _, message := range filtered {
			ids = append(ids, message.ID)
		}
		return ids
	}())
	for idx := range filtered {
		filtered[idx].Reactions = reactions[filtered[idx].ID]
	}

	return filtered, nil
}
