package conversation

import "time"

// CreateConversationParams describes a new conversation to persist.
type CreateConversationParams struct {
	OwnerAccountID  string
	Kind            ConversationKind
	Title           string
	Description     string
	AvatarMediaID   string
	MemberAccountIDs []string
	Settings        ConversationSettings
	CreatedAt       time.Time
}

// GetConversationParams identifies a conversation lookup.
type GetConversationParams struct {
	ConversationID string
	AccountID      string
}

// ListConversationsParams filters a member's conversation list.
type ListConversationsParams struct {
	AccountID      string
	IncludeArchived bool
	IncludeMuted    bool
	IncludeHidden   bool
}

// ListMessagesParams filters a message list request.
type ListMessagesParams struct {
	AccountID      string
	ConversationID string
	FromSequence   uint64
	Limit          int
	IncludeDeleted bool
}

// SendMessageParams describes a message send request.
type SendMessageParams struct {
	ConversationID    string
	SenderAccountID   string
	SenderDeviceID    string
	Draft             MessageDraft
	CausationID       string
	CorrelationID     string
	CreatedAt         time.Time
}

// RecordDeliveryParams captures a delivery watermark update.
type RecordDeliveryParams struct {
	ConversationID        string
	AccountID             string
	DeviceID              string
	MessageID             string
	DeliveredThroughSequence uint64
	CausationID           string
	CorrelationID         string
	CreatedAt             time.Time
}

// MarkReadParams captures a read watermark update.
type MarkReadParams struct {
	ConversationID     string
	AccountID          string
	DeviceID           string
	ReadThroughSequence uint64
	CausationID        string
	CorrelationID      string
	CreatedAt          time.Time
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
