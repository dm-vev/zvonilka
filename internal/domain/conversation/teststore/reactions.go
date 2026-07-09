package teststore

import (
	"context"
	"sort"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func (s *memoryStore) SaveMessageReaction(ctx context.Context, reaction conversation.MessageReaction) (conversation.MessageReaction, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.MessageReaction{}, err
	}
	reaction.MessageID = strings.TrimSpace(reaction.MessageID)
	reaction.AccountID = strings.TrimSpace(reaction.AccountID)
	reaction.Reaction = strings.TrimSpace(reaction.Reaction)
	if reaction.MessageID == "" || reaction.AccountID == "" || reaction.Reaction == "" {
		return conversation.MessageReaction{}, conversation.ErrInvalidInput
	}
	if reaction.CreatedAt.IsZero() || reaction.UpdatedAt.IsZero() || reaction.UpdatedAt.Before(reaction.CreatedAt) {
		return conversation.MessageReaction{}, conversation.ErrInvalidInput
	}
	if _, ok := s.messagesByID[reaction.MessageID]; !ok {
		return conversation.MessageReaction{}, conversation.ErrNotFound
	}

	key := reactionKey(reaction.MessageID, reaction.AccountID)
	s.reactionsByKey[key] = cloneReaction(reaction)

	return cloneReaction(reaction), nil
}

func (s *memoryStore) DeleteMessageReaction(ctx context.Context, messageID string, accountID string) error {
	if err := s.validateWrite(ctx); err != nil {
		return err
	}
	messageID = strings.TrimSpace(messageID)
	accountID = strings.TrimSpace(accountID)
	if messageID == "" || accountID == "" {
		return conversation.ErrInvalidInput
	}

	delete(s.reactionsByKey, reactionKey(messageID, accountID))
	return nil
}

func (s *memoryStore) reactionsByMessageIDs(messageIDs []string) map[string][]conversation.MessageReaction {
	if len(messageIDs) == 0 {
		return make(map[string][]conversation.MessageReaction)
	}

	allowed := make(map[string]struct{}, len(messageIDs))
	for _, messageID := range messageIDs {
		messageID = strings.TrimSpace(messageID)
		if messageID == "" {
			continue
		}
		allowed[messageID] = struct{}{}
	}

	reactions := make(map[string][]conversation.MessageReaction, len(allowed))
	for _, reaction := range s.reactionsByKey {
		if _, ok := allowed[reaction.MessageID]; !ok {
			continue
		}
		reactions[reaction.MessageID] = append(reactions[reaction.MessageID], cloneReaction(reaction))
	}

	for messageID := range reactions {
		sort.Slice(reactions[messageID], func(i, j int) bool {
			if reactions[messageID][i].UpdatedAt.Equal(reactions[messageID][j].UpdatedAt) {
				return reactions[messageID][i].AccountID < reactions[messageID][j].AccountID
			}
			return reactions[messageID][i].UpdatedAt.After(reactions[messageID][j].UpdatedAt)
		})
	}

	return reactions
}

func reactionKey(messageID, accountID string) string {
	return messageID + "|" + accountID
}
