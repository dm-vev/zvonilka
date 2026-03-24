package conversation

import "time"

// CreateConversationParams describes a new conversation to persist.
type CreateConversationParams struct {
	OwnerAccountID   string
	Kind             ConversationKind
	Title            string
	Description      string
	AvatarMediaID    string
	MemberAccountIDs []string
	Settings         ConversationSettings
	CreatedAt        time.Time
}

// GetConversationParams identifies a conversation lookup.
type GetConversationParams struct {
	ConversationID string
	AccountID      string
}

// ListConversationsParams filters a member's conversation list.
type ListConversationsParams struct {
	AccountID       string
	IncludeArchived bool
	IncludeMuted    bool
	IncludeHidden   bool
}

// ListMessagesParams filters a message list request.
type ListMessagesParams struct {
	AccountID      string
	ConversationID string
	ThreadID       string
	FromSequence   uint64
	Limit          int
	IncludeDeleted bool
}

// SendMessageParams describes a message send request.
type SendMessageParams struct {
	ConversationID  string
	SenderAccountID string
	SenderDeviceID  string
	Draft           MessageDraft
	CausationID     string
	CorrelationID   string
	CreatedAt       time.Time
}

// ReplyMessageParams describes a reply send request.
type ReplyMessageParams struct {
	ConversationID   string
	SenderAccountID  string
	SenderDeviceID   string
	ReplyToMessageID string
	Draft            MessageDraft
	CausationID      string
	CorrelationID    string
	CreatedAt        time.Time
}

// EditMessageParams describes a message edit request.
type EditMessageParams struct {
	ConversationID string
	MessageID      string
	ActorAccountID string
	ActorDeviceID  string
	Draft          MessageDraft
	EditedAt       time.Time
}

// DeleteMessageParams describes a message deletion request.
type DeleteMessageParams struct {
	ConversationID string
	MessageID      string
	ActorAccountID string
	ActorDeviceID  string
	DeletedAt      time.Time
}

// PinMessageParams pins or unpins a message.
type PinMessageParams struct {
	ConversationID string
	MessageID      string
	ActorAccountID string
	ActorDeviceID  string
	Pinned         bool
	UpdatedAt      time.Time
}

// AddMessageReactionParams stores a reaction for a message.
type AddMessageReactionParams struct {
	ConversationID string
	MessageID      string
	ActorAccountID string
	ActorDeviceID  string
	Reaction       string
	CreatedAt      time.Time
}

// RemoveMessageReactionParams removes a reaction from a message.
type RemoveMessageReactionParams struct {
	ConversationID string
	MessageID      string
	ActorAccountID string
	ActorDeviceID  string
	Reaction       string
	RemovedAt      time.Time
}

// RecordDeliveryParams captures a delivery watermark update.
type RecordDeliveryParams struct {
	ConversationID           string
	AccountID                string
	DeviceID                 string
	MessageID                string
	DeliveredThroughSequence uint64
	CausationID              string
	CorrelationID            string
	CreatedAt                time.Time
}

// MarkReadParams captures a read watermark update.
type MarkReadParams struct {
	ConversationID      string
	AccountID           string
	DeviceID            string
	ReadThroughSequence uint64
	CausationID         string
	CorrelationID       string
	CreatedAt           time.Time
}

// PullEventsParams filters the sync event stream.
type PullEventsParams struct {
	DeviceID        string
	FromSequence    uint64
	Limit           int
	ConversationIDs []string
}

// AcknowledgeEventsParams updates the device ack cursor.
type AcknowledgeEventsParams struct {
	DeviceID      string
	AckedSequence uint64
	EventIDs      []string
}

// GetSyncStateParams identifies the device sync state to resolve.
type GetSyncStateParams struct {
	DeviceID string
}

// CreateTopicParams describes a new topic to persist.
type CreateTopicParams struct {
	ConversationID   string
	CreatorAccountID string
	Title            string
	CreatedAt        time.Time
}

// GetTopicParams identifies a topic lookup.
type GetTopicParams struct {
	ConversationID string
	TopicID        string
	AccountID      string
}

// ListTopicsParams filters the topic list for a conversation.
type ListTopicsParams struct {
	ConversationID  string
	AccountID       string
	IncludeArchived bool
	IncludeClosed   bool
}

// RenameTopicParams renames an existing topic.
type RenameTopicParams struct {
	ConversationID string
	TopicID        string
	ActorAccountID string
	Title          string
	UpdatedAt      time.Time
}

// ArchiveTopicParams archives or unarchives a topic.
type ArchiveTopicParams struct {
	ConversationID string
	TopicID        string
	ActorAccountID string
	Archived       bool
	UpdatedAt      time.Time
}

// PinTopicParams pins or unpins a topic.
type PinTopicParams struct {
	ConversationID string
	TopicID        string
	ActorAccountID string
	Pinned         bool
	UpdatedAt      time.Time
}

// CloseTopicParams closes or reopens a topic.
type CloseTopicParams struct {
	ConversationID string
	TopicID        string
	ActorAccountID string
	Closed         bool
	UpdatedAt      time.Time
}
