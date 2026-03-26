package teststore

import (
	"sync"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// NewMemoryStore builds a concurrency-safe in-memory conversation store for tests.
func NewMemoryStore() conversation.Store {
	return &memoryStore{
		conversationsByID:       make(map[string]conversation.Conversation),
		topicsByKey:             make(map[string]conversation.ConversationTopic),
		membersByKey:            make(map[string]conversation.ConversationMember),
		invitesByKey:            make(map[string]conversation.ConversationInvite),
		messagesByID:            make(map[string]conversation.Message),
		reactionsByKey:          make(map[string]conversation.MessageReaction),
		readStatesByKey:         make(map[string]conversation.ReadState),
		syncStatesByDevice:      make(map[string]conversation.SyncState),
		eventsByID:              make(map[string]conversation.EventEnvelope),
		moderationPoliciesByKey: make(map[string]conversation.ModerationPolicy),
		moderationReportsByID:   make(map[string]conversation.ModerationReport),
		moderationActionsByID:   make(map[string]conversation.ModerationAction),
		moderationRestrictions:  make(map[string]conversation.ModerationRestriction),
		moderationRateStates:    make(map[string]conversation.ModerationRateState),
	}
}

type memoryStore struct {
	mu sync.RWMutex

	conversationsByID       map[string]conversation.Conversation
	topicsByKey             map[string]conversation.ConversationTopic
	membersByKey            map[string]conversation.ConversationMember
	invitesByKey            map[string]conversation.ConversationInvite
	messagesByID            map[string]conversation.Message
	reactionsByKey          map[string]conversation.MessageReaction
	readStatesByKey         map[string]conversation.ReadState
	syncStatesByDevice      map[string]conversation.SyncState
	eventsByID              map[string]conversation.EventEnvelope
	moderationPoliciesByKey map[string]conversation.ModerationPolicy
	moderationReportsByID   map[string]conversation.ModerationReport
	moderationActionsByID   map[string]conversation.ModerationAction
	moderationRestrictions  map[string]conversation.ModerationRestriction
	moderationRateStates    map[string]conversation.ModerationRateState
	eventOrder              []string
	nextSequence            uint64
}

func conversationMemberKey(conversationID, accountID string) string {
	return conversationID + "|" + accountID
}

func topicKey(conversationID, topicID string) string {
	return conversationID + "|" + topicID
}

func inviteKey(conversationID, inviteID string) string {
	return conversationID + "|" + inviteID
}

func readStateKey(conversationID, accountID, deviceID string) string {
	return conversationID + "|" + accountID + "|" + deviceID
}

var _ conversation.Store = (*memoryStore)(nil)
