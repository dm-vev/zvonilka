package notification

import (
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SetPreferenceParams updates account-level notification preferences.
type SetPreferenceParams struct {
	AccountID      string
	Enabled        bool
	DirectEnabled  bool
	GroupEnabled   bool
	ChannelEnabled bool
	MentionEnabled bool
	ReplyEnabled   bool
	QuietHours     QuietHours
	MutedUntil     time.Time
	UpdatedAt      time.Time
}

// SetOverrideParams updates conversation-level notification overrides.
type SetOverrideParams struct {
	ConversationID string
	AccountID      string
	Muted          bool
	MentionsOnly   bool
	MutedUntil     time.Time
	UpdatedAt      time.Time
}

// RegisterPushTokenParams registers or refreshes a push token.
type RegisterPushTokenParams struct {
	AccountID string
	DeviceID  string
	Provider  string
	Token     string
	Platform  identity.DevicePlatform
	CreatedAt time.Time
}

// RevokePushTokenParams revokes a push token by ID.
type RevokePushTokenParams struct {
	TokenID   string
	RevokedAt time.Time
}

// QueueDeliveryParams creates a notification delivery hint.
type QueueDeliveryParams struct {
	DedupKey       string
	EventID        string
	ConversationID string
	MessageID      string
	AccountID      string
	DeviceID       string
	PushTokenID    string
	Kind           NotificationKind
	Reason         string
	Mode           DeliveryMode
	Priority       int
	Attempts       int
	NextAttemptAt  time.Time
	State          DeliveryState
	CreatedAt      time.Time
}

// RetryDeliveryParams schedules a retry for a delivery hint.
type RetryDeliveryParams struct {
	DeliveryID string
	LastError  string
	RetryAt    time.Time
}

// SaveWorkerCursorParams advances the worker cursor.
type SaveWorkerCursorParams struct {
	Name         string
	LastSequence uint64
	UpdatedAt    time.Time
}
