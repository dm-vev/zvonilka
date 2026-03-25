package teststore

import (
	"context"
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

func (s *memoryStore) ConversationCountersByAccount(ctx context.Context, accountID string, conversationIDs []string) (map[string]conversation.ConversationCounters, error) {
	if err := s.validateRead(ctx); err != nil {
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

	requested := make(map[string]struct{}, len(conversationIDs))
	for _, conversationID := range conversationIDs {
		requested[conversationID] = struct{}{}
	}

	watermarks := make(map[string]uint64, len(conversationIDs))
	for _, state := range s.readStatesByKey {
		if state.AccountID != accountID {
			continue
		}
		if _, ok := requested[state.ConversationID]; !ok {
			continue
		}
		if state.LastReadSequence > watermarks[state.ConversationID] {
			watermarks[state.ConversationID] = state.LastReadSequence
		}
	}

	counters := make(map[string]conversation.ConversationCounters, len(conversationIDs))
	for _, conversationID := range conversationIDs {
		counters[conversationID] = conversation.ConversationCounters{ConversationID: conversationID}
	}

	for _, message := range s.messagesByID {
		if _, ok := requested[message.ConversationID]; !ok {
			continue
		}
		member, ok := s.membersByKey[conversationMemberKey(message.ConversationID, accountID)]
		if !ok || !member.LeftAt.IsZero() || member.Banned {
			continue
		}
		if !message.DeletedAt.IsZero() || message.Status == conversation.MessageStatusDeleted {
			continue
		}
		if message.SenderAccountID == accountID {
			continue
		}
		if message.CreatedAt.Before(member.JoinedAt) {
			continue
		}
		if message.Sequence <= watermarks[message.ConversationID] {
			continue
		}

		counter := counters[message.ConversationID]
		counter.UnreadCount++
		for _, mentionAccountID := range message.MentionAccountIDs {
			if mentionAccountID == accountID {
				counter.UnreadMentionCount++
				break
			}
		}
		counters[message.ConversationID] = counter
	}

	return counters, nil
}
