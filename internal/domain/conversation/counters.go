package conversation

import (
	"context"
	"fmt"
	"strings"
)

func (s *Service) decorateConversationCounters(
	ctx context.Context,
	accountID string,
	conversations []Conversation,
) ([]Conversation, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" || len(conversations) == 0 {
		return conversations, nil
	}

	conversationIDs := make([]string, 0, len(conversations))
	for _, conversation := range conversations {
		conversationIDs = append(conversationIDs, conversation.ID)
	}

	counters, err := s.store.ConversationCountersByAccount(ctx, accountID, conversationIDs)
	if err != nil {
		return nil, fmt.Errorf("load conversation counters for account %s: %w", accountID, err)
	}

	for idx := range conversations {
		counter, ok := counters[conversations[idx].ID]
		if !ok {
			continue
		}
		conversations[idx].UnreadCount = counter.UnreadCount
		conversations[idx].UnreadMentionCount = counter.UnreadMentionCount
	}

	return conversations, nil
}
