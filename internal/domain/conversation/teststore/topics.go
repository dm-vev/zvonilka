package teststore

import (
	"context"
	"sort"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func (s *memoryStore) SaveTopic(ctx context.Context, topic conversation.ConversationTopic) (conversation.ConversationTopic, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.ConversationTopic{}, err
	}
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

	key := topicKey(topic.ConversationID, topic.ID)
	s.topicsByKey[key] = cloneTopic(topic)
	return cloneTopic(topic), nil
}

func (s *memoryStore) TopicByConversationAndID(ctx context.Context, conversationID string, topicID string) (conversation.ConversationTopic, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.ConversationTopic{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := topicKey(strings.TrimSpace(conversationID), strings.TrimSpace(topicID))
	topic, ok := s.topicsByKey[key]
	if !ok {
		return conversation.ConversationTopic{}, conversation.ErrNotFound
	}

	return cloneTopic(topic), nil
}

func (s *memoryStore) TopicsByConversationID(ctx context.Context, conversationID string) ([]conversation.ConversationTopic, error) {
	if err := s.validateRead(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, conversation.ErrInvalidInput
	}

	topics := make([]conversation.ConversationTopic, 0)
	for _, topic := range s.topicsByKey {
		if topic.ConversationID != conversationID {
			continue
		}
		topics = append(topics, cloneTopic(topic))
	}

	sort.Slice(topics, func(i, j int) bool {
		if topics[i].IsGeneral != topics[j].IsGeneral {
			return topics[i].IsGeneral
		}
		if topics[i].Pinned != topics[j].Pinned {
			return topics[i].Pinned
		}
		if topics[i].LastSequence == topics[j].LastSequence {
			if topics[i].UpdatedAt.Equal(topics[j].UpdatedAt) {
				return topics[i].ID < topics[j].ID
			}
			return topics[i].UpdatedAt.After(topics[j].UpdatedAt)
		}
		return topics[i].LastSequence > topics[j].LastSequence
	})

	return topics, nil
}
