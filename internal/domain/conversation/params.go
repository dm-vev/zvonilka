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

// GetMessageParams identifies a single-message lookup.
type GetMessageParams struct {
	ConversationID string
	MessageID      string
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

// PublishUserUpdateParams describes a fan-out user update event.
type PublishUserUpdateParams struct {
	AccountID   string
	DeviceID    string
	PayloadType string
	Metadata    map[string]string
	CreatedAt   time.Time
}

// CreateTopicParams describes a new topic to persist.
type CreateTopicParams struct {
	ConversationID   string
	RootMessageID    string
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

// SetModerationPolicyParams describes a moderation policy override write.
type SetModerationPolicyParams struct {
	TargetKind               ModerationTargetKind
	TargetID                 string
	ActorAccountID           string
	OnlyAdminsCanWrite       bool
	OnlyAdminsCanAddMembers  bool
	AllowReactions           bool
	AllowForwards            bool
	AllowThreads             bool
	RequireEncryptedMessages bool
	RequireJoinApproval      bool
	PinnedMessagesOnlyAdmins bool
	SlowModeInterval         time.Duration
	AntiSpamWindow           time.Duration
	AntiSpamBurstLimit       int
	ShadowMode               bool
	CreatedAt                time.Time
}

// SubmitModerationReportParams describes a complaint submission.
type SubmitModerationReportParams struct {
	TargetKind        ModerationTargetKind
	TargetID          string
	ReporterAccountID string
	TargetAccountID   string
	Reason            string
	Details           string
	CreatedAt         time.Time
}

// ResolveModerationReportParams describes a moderation report resolution.
type ResolveModerationReportParams struct {
	ReportID          string
	ResolverAccountID string
	Resolved          bool
	Resolution        string
	ReviewedAt        time.Time
}

// ApplyModerationRestrictionParams describes a moderation restriction write.
type ApplyModerationRestrictionParams struct {
	TargetKind      ModerationTargetKind
	TargetID        string
	ActorAccountID  string
	TargetAccountID string
	State           ModerationRestrictionState
	Reason          string
	Duration        time.Duration
	CreatedAt       time.Time
}

// LiftModerationRestrictionParams removes an active moderation restriction.
type LiftModerationRestrictionParams struct {
	TargetKind      ModerationTargetKind
	TargetID        string
	ActorAccountID  string
	TargetAccountID string
	Reason          string
	CreatedAt       time.Time
}

// CheckModerationWriteParams evaluates whether a message may be accepted.
type CheckModerationWriteParams struct {
	TargetKind     ModerationTargetKind
	TargetID       string
	ActorAccountID string
	ActorRole      MemberRole
	BasePolicy     ModerationPolicy
	CreatedAt      time.Time
}

// RecordModerationWriteParams records a successful write for slow-mode tracking.
type RecordModerationWriteParams struct {
	TargetKind     ModerationTargetKind
	TargetID       string
	ActorAccountID string
	AntiSpamWindow time.Duration
	CreatedAt      time.Time
}
