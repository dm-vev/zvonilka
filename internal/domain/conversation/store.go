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

	SaveConversationInvite(ctx context.Context, invite ConversationInvite) (ConversationInvite, error)
	ConversationInviteByConversationAndID(ctx context.Context, conversationID string, inviteID string) (ConversationInvite, error)
	ConversationInvitesByConversationID(ctx context.Context, conversationID string) ([]ConversationInvite, error)

	SaveMessage(ctx context.Context, message Message) (Message, error)
	MessageByID(ctx context.Context, conversationID string, messageID string) (Message, error)
	MessagesByConversationID(ctx context.Context, conversationID string, threadID string, fromSequence uint64, limit int) ([]Message, error)
	SaveMessageReaction(ctx context.Context, reaction MessageReaction) (MessageReaction, error)
	DeleteMessageReaction(ctx context.Context, messageID string, accountID string) error

	SaveReadState(ctx context.Context, state ReadState) (ReadState, error)
	ReadStateByConversationAndDevice(ctx context.Context, conversationID string, deviceID string) (ReadState, error)
	ReadStatesByDevice(ctx context.Context, deviceID string) ([]ReadState, error)
	ConversationCountersByAccount(ctx context.Context, accountID string, conversationIDs []string) (map[string]ConversationCounters, error)

	SaveSyncState(ctx context.Context, state SyncState) (SyncState, error)
	SyncStateByDevice(ctx context.Context, deviceID string) (SyncState, error)

	SaveEvent(ctx context.Context, event EventEnvelope) (EventEnvelope, error)
	EventsAfterSequence(ctx context.Context, fromSequence uint64, limit int, conversationIDs []string) ([]EventEnvelope, error)

	SaveModerationPolicy(ctx context.Context, policy ModerationPolicy) (ModerationPolicy, error)
	ModerationPolicyByTarget(ctx context.Context, targetKind ModerationTargetKind, targetID string) (ModerationPolicy, error)
	SaveModerationReport(ctx context.Context, report ModerationReport) (ModerationReport, error)
	ModerationReportByID(ctx context.Context, reportID string) (ModerationReport, error)
	ModerationReportsByTarget(ctx context.Context, targetKind ModerationTargetKind, targetID string) ([]ModerationReport, error)
	SaveModerationAction(ctx context.Context, action ModerationAction) (ModerationAction, error)
	ModerationActionsByTarget(ctx context.Context, targetKind ModerationTargetKind, targetID string) ([]ModerationAction, error)
	SaveModerationRestriction(ctx context.Context, restriction ModerationRestriction) (ModerationRestriction, error)
	ModerationRestrictionByTargetAndAccount(ctx context.Context, targetKind ModerationTargetKind, targetID string, accountID string) (ModerationRestriction, error)
	ModerationRestrictionsByTarget(ctx context.Context, targetKind ModerationTargetKind, targetID string) ([]ModerationRestriction, error)
	DeleteModerationRestriction(ctx context.Context, targetKind ModerationTargetKind, targetID string, accountID string) error
	SaveModerationRateState(ctx context.Context, state ModerationRateState) (ModerationRateState, error)
	ModerationRateStateByTargetAndAccount(ctx context.Context, targetKind ModerationTargetKind, targetID string, accountID string) (ModerationRateState, error)
}
