package conversation

import "time"

// ConversationKind distinguishes direct, group, channel, and saved-message spaces.
type ConversationKind string

// Conversation kinds supported by the messenger domain.
const (
	ConversationKindUnspecified   ConversationKind = ""
	ConversationKindDirect        ConversationKind = "direct"
	ConversationKindGroup         ConversationKind = "group"
	ConversationKindChannel       ConversationKind = "channel"
	ConversationKindSavedMessages ConversationKind = "saved_messages"
)

// MemberRole identifies the privilege level of a conversation member.
type MemberRole string

// Member roles supported by the messenger domain.
const (
	MemberRoleUnspecified MemberRole = ""
	MemberRoleOwner       MemberRole = "owner"
	MemberRoleAdmin       MemberRole = "admin"
	MemberRoleMember      MemberRole = "member"
	MemberRoleGuest       MemberRole = "guest"
)

// MessageKind identifies the payload family for a conversation message.
type MessageKind string

// Message kinds supported by the messenger domain.
const (
	MessageKindUnspecified MessageKind = ""
	MessageKindText        MessageKind = "text"
	MessageKindImage       MessageKind = "image"
	MessageKindVideo       MessageKind = "video"
	MessageKindDocument    MessageKind = "document"
	MessageKindVoice       MessageKind = "voice"
	MessageKindSticker     MessageKind = "sticker"
	MessageKindGIF         MessageKind = "gif"
	MessageKindSystem      MessageKind = "system"
)

// MessageStatus describes the lifecycle of a message.
type MessageStatus string

// Message statuses supported by the messenger domain.
const (
	MessageStatusUnspecified MessageStatus = ""
	MessageStatusPending     MessageStatus = "pending"
	MessageStatusSent        MessageStatus = "sent"
	MessageStatusDelivered   MessageStatus = "delivered"
	MessageStatusRead        MessageStatus = "read"
	MessageStatusFailed      MessageStatus = "failed"
	MessageStatusDeleted     MessageStatus = "deleted"
)

// AttachmentKind identifies the concrete asset family.
type AttachmentKind string

// Attachment kinds supported by the messenger domain.
const (
	AttachmentKindUnspecified AttachmentKind = ""
	AttachmentKindImage       AttachmentKind = "image"
	AttachmentKindVideo       AttachmentKind = "video"
	AttachmentKindDocument    AttachmentKind = "document"
	AttachmentKindVoice       AttachmentKind = "voice"
	AttachmentKindSticker     AttachmentKind = "sticker"
	AttachmentKindGIF         AttachmentKind = "gif"
	AttachmentKindAvatar      AttachmentKind = "avatar"
	AttachmentKindFile        AttachmentKind = "file"
)

// EventType identifies a syncable domain event.
type EventType string

// Event types emitted by the messenger domain.
const (
	EventTypeUnspecified            EventType = ""
	EventTypeConversationCreated    EventType = "conversation.created"
	EventTypeConversationUpdated    EventType = "conversation.updated"
	EventTypeConversationMembers    EventType = "conversation.members_changed"
	EventTypeUserUpdated            EventType = "user.updated"
	EventTypeAdminActionRecorded    EventType = "admin_action.recorded"
	EventTypeMessageCreated         EventType = "message.created"
	EventTypeMessageDelivered       EventType = "message.delivered"
	EventTypeMessageRead            EventType = "message.read"
	EventTypeMessageEdited          EventType = "message.edited"
	EventTypeMessageDeleted         EventType = "message.deleted"
	EventTypeMessagePinned          EventType = "message.pinned"
	EventTypeMessageReactionAdded   EventType = "message.reaction_added"
	EventTypeMessageReactionUpdated EventType = "message.reaction_updated"
	EventTypeMessageReactionRemoved EventType = "message.reaction_removed"
	EventTypeSyncAcknowledged       EventType = "sync.acknowledged"
	EventTypeTopicCreated           EventType = "topic.created"
	EventTypeTopicUpdated           EventType = "topic.updated"
	EventTypeTopicArchived          EventType = "topic.archived"
	EventTypeTopicPinned            EventType = "topic.pinned"
	EventTypeTopicClosed            EventType = "topic.closed"
)

// ConversationSettings controls encryption, write, and moderation behavior for a conversation.
type ConversationSettings struct {
	OnlyAdminsCanWrite       bool
	OnlyAdminsCanAddMembers  bool
	AllowReactions           bool
	AllowForwards            bool
	AllowThreads             bool
	RequireEncryptedMessages bool
	RequireTrustedDevices    bool
	RequireJoinApproval      bool
	PinnedMessagesOnlyAdmins bool
	SlowModeInterval         time.Duration
}

// Conversation describes a chat, channel, or saved-message space.
type Conversation struct {
	ID                 string
	Kind               ConversationKind
	Title              string
	Description        string
	AvatarMediaID      string
	OwnerAccountID     string
	Settings           ConversationSettings
	Archived           bool
	Muted              bool
	Pinned             bool
	Hidden             bool
	LastSequence       uint64
	UnreadCount        uint64
	UnreadMentionCount uint64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastMessageAt      time.Time
}

// ConversationTopic describes a conversation topic or the general thread root.
type ConversationTopic struct {
	ConversationID     string
	ID                 string
	RootMessageID      string
	Title              string
	CreatedByAccountID string
	IsGeneral          bool
	Archived           bool
	Pinned             bool
	Closed             bool
	LastSequence       uint64
	MessageCount       uint64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastMessageAt      time.Time
	ArchivedAt         time.Time
	ClosedAt           time.Time
}

// ConversationMember describes a participant in a conversation.
type ConversationMember struct {
	ConversationID     string
	AccountID          string
	Role               MemberRole
	InvitedByAccountID string
	Muted              bool
	Banned             bool
	JoinedAt           time.Time
	LeftAt             time.Time
}

// ConversationInvite describes one reusable invite link for a conversation.
type ConversationInvite struct {
	ID                 string
	ConversationID     string
	Code               string
	CreatedByAccountID string
	AllowedRoles       []MemberRole
	ExpiresAt          time.Time
	MaxUses            uint32
	UseCount           uint32
	Revoked            bool
	RevokedAt          time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// AttachmentRef describes a media attachment in a message draft or stored message.
type AttachmentRef struct {
	MediaID   string
	Kind      AttachmentKind
	FileName  string
	MimeType  string
	SizeBytes uint64
	SHA256Hex string
	Width     uint32
	Height    uint32
	Duration  time.Duration
	Caption   string
}

// EncryptedPayload describes the opaque payload carried by a conversation message.
type EncryptedPayload struct {
	KeyID      string
	Algorithm  string
	Nonce      []byte
	Ciphertext []byte
	AAD        []byte
	Metadata   map[string]string
}

// MessageReference describes a quoted or replied-to message.
type MessageReference struct {
	ConversationID  string
	MessageID       string
	SenderAccountID string
	MessageKind     MessageKind
	Snippet         string
}

// MessageReaction describes a reaction applied by one account to a message.
type MessageReaction struct {
	MessageID string
	AccountID string
	Reaction  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MessageDraft captures client-supplied message content before persistence.
type MessageDraft struct {
	ClientMessageID     string
	Kind                MessageKind
	Payload             EncryptedPayload
	Attachments         []AttachmentRef
	MentionAccountIDs   []string
	ReplyTo             MessageReference
	ThreadID            string
	DeliverAt           time.Time
	Silent              bool
	DisableLinkPreviews bool
	Metadata            map[string]string
}

// Message describes a persisted conversation message.
type Message struct {
	ID                  string
	ConversationID      string
	SenderAccountID     string
	SenderDeviceID      string
	ClientMessageID     string
	Sequence            uint64
	Kind                MessageKind
	Status              MessageStatus
	Payload             EncryptedPayload
	Attachments         []AttachmentRef
	MentionAccountIDs   []string
	ReplyTo             MessageReference
	ThreadID            string
	Silent              bool
	Pinned              bool
	DisableLinkPreviews bool
	ViewCount           uint64
	Metadata            map[string]string
	Reactions           []MessageReaction
	CreatedAt           time.Time
	UpdatedAt           time.Time
	EditedAt            time.Time
	DeletedAt           time.Time
}

// ReadState tracks the read and delivery watermark per device and conversation.
type ReadState struct {
	ConversationID        string
	AccountID             string
	DeviceID              string
	LastReadSequence      uint64
	LastDeliveredSequence uint64
	LastAckedSequence     uint64
	UpdatedAt             time.Time
}

// SyncState tracks device-level sync cursors.
type SyncState struct {
	DeviceID               string
	AccountID              string
	LastAppliedSequence    uint64
	LastAckedSequence      uint64
	ServerTime             time.Time
	ConversationWatermarks map[string]uint64
}

// ConversationCounters describes derived unread badge counts for a conversation.
type ConversationCounters struct {
	ConversationID     string
	UnreadCount        uint64
	UnreadMentionCount uint64
}

// EventEnvelope describes an emitted sync event.
type EventEnvelope struct {
	EventID             string
	EventType           EventType
	ConversationID      string
	ActorAccountID      string
	ActorDeviceID       string
	CausationID         string
	CorrelationID       string
	MessageID           string
	Sequence            uint64
	PayloadType         string
	Payload             EncryptedPayload
	ReadThroughSequence uint64
	Metadata            map[string]string
	CreatedAt           time.Time
}
