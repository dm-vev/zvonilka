package teststore

import (
	"sync"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// NewMemoryStore builds a concurrency-safe in-memory conversation store for tests.
func NewMemoryStore() conversation.Store {
	return &memoryStore{
		conversationsByID:  make(map[string]conversation.Conversation),
		topicsByKey:        make(map[string]conversation.ConversationTopic),
		membersByKey:       make(map[string]conversation.ConversationMember),
		messagesByID:       make(map[string]conversation.Message),
		reactionsByKey:     make(map[string]conversation.MessageReaction),
		readStatesByKey:    make(map[string]conversation.ReadState),
		syncStatesByDevice: make(map[string]conversation.SyncState),
		eventsByID:         make(map[string]conversation.EventEnvelope),
	}
}

type memoryStore struct {
	mu sync.RWMutex

	conversationsByID  map[string]conversation.Conversation
	topicsByKey        map[string]conversation.ConversationTopic
	membersByKey       map[string]conversation.ConversationMember
	messagesByID       map[string]conversation.Message
	reactionsByKey     map[string]conversation.MessageReaction
	readStatesByKey    map[string]conversation.ReadState
	syncStatesByDevice map[string]conversation.SyncState
	eventsByID         map[string]conversation.EventEnvelope
	eventOrder         []string
	nextSequence       uint64
}

func conversationMemberKey(conversationID, accountID string) string {
	return conversationID + "|" + accountID
}

func topicKey(conversationID, topicID string) string {
	return conversationID + "|" + topicID
}

func readStateKey(conversationID, accountID, deviceID string) string {
	return conversationID + "|" + accountID + "|" + deviceID
}

var _ conversation.Store = (*memoryStore)(nil)
