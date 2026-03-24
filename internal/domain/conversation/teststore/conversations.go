package teststore

import (
	"context"
	"sort"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func (s *memoryStore) SaveConversation(ctx context.Context, conversationRow conversation.Conversation) (conversation.Conversation, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.Conversation{}, err
	}
	if strings.TrimSpace(conversationRow.ID) == "" {
		return conversation.Conversation{}, conversation.ErrInvalidInput
	}

	conversationRow = cloneConversation(conversationRow)
	s.conversationsByID[conversationRow.ID] = conversationRow
	return cloneConversation(conversationRow), nil
}

func (s *memoryStore) ConversationByID(ctx context.Context, conversationID string) (conversation.Conversation, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.Conversation{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return conversation.Conversation{}, conversation.ErrNotFound
	}

	row, ok := s.conversationsByID[conversationID]
	if !ok {
		return conversation.Conversation{}, conversation.ErrNotFound
	}

	return cloneConversation(row), nil
}

func (s *memoryStore) ConversationsByAccountID(ctx context.Context, accountID string) ([]conversation.Conversation, error) {
	if err := s.validateRead(ctx); err != nil {
		return nil, err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, conversation.ErrInvalidInput
	}

	seen := make(map[string]struct{})
	rows := make([]conversation.Conversation, 0)
	for _, member := range s.membersByKey {
		if member.AccountID != accountID || !member.LeftAt.IsZero() || member.Banned {
			continue
		}
		if _, ok := seen[member.ConversationID]; ok {
			continue
		}
		row, ok := s.conversationsByID[member.ConversationID]
		if !ok {
			continue
		}
		seen[member.ConversationID] = struct{}{}
		rows = append(rows, cloneConversation(row))
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LastSequence == rows[j].LastSequence {
			if rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
				return rows[i].ID < rows[j].ID
			}
			return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
		}
		return rows[i].LastSequence > rows[j].LastSequence
	})

	return rows, nil
}

func (s *memoryStore) validateWrite(ctx context.Context) error {
	if ctx == nil {
		return conversation.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *memoryStore) validateRead(ctx context.Context) error {
	return s.validateWrite(ctx)
}
