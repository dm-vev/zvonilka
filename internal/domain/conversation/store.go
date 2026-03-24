package conversation

import "context"

// Store persists conversation state.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	SaveConversation(ctx context.Context, conversation Conversation) (Conversation, error)
	ConversationByID(ctx context.Context, conversationID string) (Conversation, error)
	ConversationsByAccountID(ctx context.Context, accountID string) ([]Conversation, error)

	SaveTopic(ctx context.Context, topic ConversationTopic) (ConversationTopic, error)
	TopicByConversationAndID(ctx context.Context, conversationID string, topicID string) (ConversationTopic, error)
	TopicsByConversationID(ctx context.Context, conversationID string) ([]ConversationTopic, error)

	SaveConversationMember(ctx context.Context, member ConversationMember) (ConversationMember, error)
	ConversationMemberByConversationAndAccount(ctx context.Context, conversationID string, accountID string) (ConversationMember, error)
	ConversationMembersByConversationID(ctx context.Context, conversationID string) ([]ConversationMember, error)

	SaveMessage(ctx context.Context, message Message) (Message, error)
	MessageByID(ctx context.Context, conversationID string, messageID string) (Message, error)
	MessagesByConversationID(ctx context.Context, conversationID string, threadID string, fromSequence uint64, limit int) ([]Message, error)

	SaveReadState(ctx context.Context, state ReadState) (ReadState, error)
	ReadStateByConversationAndDevice(ctx context.Context, conversationID string, deviceID string) (ReadState, error)
	ReadStatesByDevice(ctx context.Context, deviceID string) ([]ReadState, error)

	SaveSyncState(ctx context.Context, state SyncState) (SyncState, error)
	SyncStateByDevice(ctx context.Context, deviceID string) (SyncState, error)

	SaveEvent(ctx context.Context, event EventEnvelope) (EventEnvelope, error)
	EventsAfterSequence(ctx context.Context, fromSequence uint64, limit int, conversationIDs []string) ([]EventEnvelope, error)
}
