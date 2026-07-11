package notification

import (
	"errors"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

var (
	// ErrInvalidInput indicates that the caller supplied malformed notification input.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotFound indicates that no notification row exists for the requested key.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates that the requested change conflicts with existing state.
	ErrConflict = errors.New("conflict")
)

// NotificationKind identifies the source of a notification fanout decision.
type NotificationKind string

// Notification kinds used by the notification domain.
const (
	NotificationKindUnspecified NotificationKind = ""
	NotificationKindDirect      NotificationKind = "direct"
	NotificationKindGroup       NotificationKind = "group"
	NotificationKindChannel     NotificationKind = "channel"
	NotificationKindMention     NotificationKind = "mention"
	NotificationKindReply       NotificationKind = "reply"
)

// DeliveryMode identifies how a notification should be routed to a device.
type DeliveryMode string

// Delivery modes used by the notification domain.
const (
	DeliveryModeUnspecified DeliveryMode = ""
	DeliveryModeInApp       DeliveryMode = "in_app"
	DeliveryModePush        DeliveryMode = "push"
)

// DeliveryState identifies the lifecycle of a notification delivery hint.
type DeliveryState string

// SettingsScope identifies a TDLib notification settings scope.
type SettingsScope string

const (
	SettingsScopeUnspecified  SettingsScope = ""
	SettingsScopePrivateChats SettingsScope = "private_chats"
	SettingsScopeGroupChats   SettingsScope = "group_chats"
	SettingsScopeChannelChats SettingsScope = "channel_chats"
)

// ReactionSource selects whose reactions produce notifications.
type ReactionSource string

const (
	ReactionSourceUnspecified ReactionSource = ""
	ReactionSourceNone        ReactionSource = "none"
	ReactionSourceContacts    ReactionSource = "contacts"
	ReactionSourceAll         ReactionSource = "all"
)

// Delivery states used by the notification domain.
const (
	DeliveryStateUnspecified DeliveryState = ""
	DeliveryStateQueued      DeliveryState = "queued"
	DeliveryStateSuppressed  DeliveryState = "suppressed"
	DeliveryStateDelivered   DeliveryState = "delivered"
	DeliveryStateFailed      DeliveryState = "failed"
)

// QuietHours stores a recurring quiet-hours window.
type QuietHours struct {
	Enabled     bool
	StartMinute int
	EndMinute   int
	Timezone    string
}

// Preference stores per-account notification preferences.
type Preference struct {
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

// ConversationOverride stores per-chat notification overrides.
type ConversationOverride struct {
	ConversationID                              string
	AccountID                                   string
	Muted                                       bool
	MentionsOnly                                bool
	MutedUntil                                  time.Time
	UpdatedAt                                   time.Time
	ShowPreview                                 bool
	SoundID                                     int64
	MuteStories                                 bool
	StorySoundID                                int64
	ShowStorySender                             bool
	DisablePinnedMessageNotifications           bool
	DisableMentionNotifications                 bool
	UseDefaultMuteFor                           bool
	UseDefaultSound                             bool
	UseDefaultShowPreview                       bool
	UseDefaultMuteStories                       bool
	UseDefaultStorySound                        bool
	UseDefaultShowStorySender                   bool
	UseDefaultDisablePinnedMessageNotifications bool
	UseDefaultDisableMentionNotifications       bool
}

// ScopeSettings stores notification behavior for a TDLib chat scope.
type ScopeSettings struct {
	AccountID                         string
	Scope                             SettingsScope
	MutedUntil                        time.Time
	ShowPreview                       bool
	SoundID                           int64
	MuteStories                       bool
	StorySoundID                      int64
	ShowStorySender                   bool
	DisablePinnedMessageNotifications bool
	DisableMentionNotifications       bool
	UseDefaultMuteStories             bool
	UpdatedAt                         time.Time
}

// ReactionSettings stores reaction notification behavior for an account.
type ReactionSettings struct {
	AccountID             string
	MessageReactionSource ReactionSource
	StoryReactionSource   ReactionSource
	PollVoteSource        ReactionSource
	SoundID               int64
	ShowPreview           bool
	UpdatedAt             time.Time
}

// SavedSound links an account notification sound to an existing media asset.
type SavedSound struct {
	SoundID   int64
	AccountID string
	MediaID   string
	Title     string
	CreatedAt time.Time
}

// PushToken stores a device push token registration.
type PushToken struct {
	ID        string
	AccountID string
	DeviceID  string
	Provider  string
	Token     string
	Platform  identity.DevicePlatform
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
	RevokedAt time.Time
}

// Delivery stores one notification fanout hint for a device or in-app target.
type Delivery struct {
	ID             string
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
	State          DeliveryState
	Priority       int
	Attempts       int
	NextAttemptAt  time.Time
	LeaseToken     string
	LeaseExpiresAt time.Time
	LastAttemptAt  time.Time
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// WorkerCursor stores a fanout worker sequence watermark.
type WorkerCursor struct {
	Name         string
	LastSequence uint64
	UpdatedAt    time.Time
}

func defaultPreference(accountID string, now time.Time) Preference {
	return Preference{
		AccountID:      strings.TrimSpace(accountID),
		Enabled:        true,
		DirectEnabled:  true,
		GroupEnabled:   true,
		ChannelEnabled: true,
		MentionEnabled: true,
		ReplyEnabled:   true,
		UpdatedAt:      now.UTC(),
	}
}

func defaultScopeSettings(accountID string, scope SettingsScope, now time.Time) ScopeSettings {
	return ScopeSettings{AccountID: strings.TrimSpace(accountID), Scope: scope, ShowPreview: true, SoundID: -1, StorySoundID: -1, ShowStorySender: true, UseDefaultMuteStories: true, UpdatedAt: now.UTC()}
}

func defaultReactionSettings(accountID string, now time.Time) ReactionSettings {
	return ReactionSettings{AccountID: strings.TrimSpace(accountID), MessageReactionSource: ReactionSourceContacts, StoryReactionSource: ReactionSourceContacts, PollVoteSource: ReactionSourceContacts, SoundID: -1, ShowPreview: true, UpdatedAt: now.UTC()}
}

func (p Preference) normalize(now time.Time) (Preference, error) {
	p.AccountID = strings.TrimSpace(p.AccountID)
	if p.AccountID == "" {
		return Preference{}, ErrInvalidInput
	}
	if p.QuietHours.Enabled {
		p.QuietHours.Timezone = strings.TrimSpace(p.QuietHours.Timezone)
		if p.QuietHours.Timezone == "" {
			p.QuietHours.Timezone = "UTC"
		}
		if p.QuietHours.StartMinute < 0 || p.QuietHours.StartMinute > 24*60-1 {
			return Preference{}, ErrInvalidInput
		}
		if p.QuietHours.EndMinute < 0 || p.QuietHours.EndMinute > 24*60-1 {
			return Preference{}, ErrInvalidInput
		}
	} else {
		p.QuietHours = QuietHours{}
	}
	if p.MutedUntil.Before(time.Time{}) {
		p.MutedUntil = time.Time{}
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now.UTC()
	}

	return p, nil
}

func (o ConversationOverride) normalize(now time.Time) (ConversationOverride, error) {
	o.ConversationID = strings.TrimSpace(o.ConversationID)
	o.AccountID = strings.TrimSpace(o.AccountID)
	if o.ConversationID == "" || o.AccountID == "" {
		return ConversationOverride{}, ErrInvalidInput
	}
	if o.SoundID < -1 || o.StorySoundID < -1 {
		return ConversationOverride{}, ErrInvalidInput
	}
	if o.MutedUntil.Before(time.Time{}) {
		o.MutedUntil = time.Time{}
	}
	if o.UpdatedAt.IsZero() {
		o.UpdatedAt = now.UTC()
	}

	return o, nil
}

func (s ScopeSettings) normalize(now time.Time) (ScopeSettings, error) {
	s.AccountID = strings.TrimSpace(s.AccountID)
	if s.AccountID == "" || !validSettingsScope(s.Scope) || s.SoundID < -1 || s.StorySoundID < -1 {
		return ScopeSettings{}, ErrInvalidInput
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now.UTC()
	}
	return s, nil
}

func (r ReactionSettings) normalize(now time.Time) (ReactionSettings, error) {
	r.AccountID = strings.TrimSpace(r.AccountID)
	if r.AccountID == "" || !validReactionSource(r.MessageReactionSource) || !validReactionSource(r.StoryReactionSource) || !validReactionSource(r.PollVoteSource) || r.SoundID < -1 {
		return ReactionSettings{}, ErrInvalidInput
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = now.UTC()
	}
	return r, nil
}

func (s SavedSound) normalize(now time.Time) (SavedSound, error) {
	s.AccountID = strings.TrimSpace(s.AccountID)
	s.MediaID = strings.TrimSpace(s.MediaID)
	s.Title = strings.TrimSpace(s.Title)
	if s.SoundID < 0 || s.AccountID == "" || s.MediaID == "" {
		return SavedSound{}, ErrInvalidInput
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now.UTC()
	}
	return s, nil
}

func validSettingsScope(scope SettingsScope) bool {
	return scope == SettingsScopePrivateChats || scope == SettingsScopeGroupChats || scope == SettingsScopeChannelChats
}

func validReactionSource(source ReactionSource) bool {
	return source == ReactionSourceNone || source == ReactionSourceContacts || source == ReactionSourceAll
}

func (t PushToken) normalize(now time.Time) (PushToken, error) {
	t.ID = strings.TrimSpace(t.ID)
	t.AccountID = strings.TrimSpace(t.AccountID)
	t.DeviceID = strings.TrimSpace(t.DeviceID)
	t.Provider = strings.TrimSpace(strings.ToLower(t.Provider))
	t.Token = strings.TrimSpace(t.Token)
	if t.ID == "" || t.AccountID == "" || t.DeviceID == "" || t.Provider == "" || t.Token == "" {
		return PushToken{}, ErrInvalidInput
	}
	if t.Platform == identity.DevicePlatformUnspecified {
		return PushToken{}, ErrInvalidInput
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now.UTC()
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = t.CreatedAt
	}

	return t, nil
}

func (d Delivery) normalize(now time.Time) (Delivery, error) {
	d.ID = strings.TrimSpace(d.ID)
	d.DedupKey = strings.TrimSpace(d.DedupKey)
	d.EventID = strings.TrimSpace(d.EventID)
	d.ConversationID = strings.TrimSpace(d.ConversationID)
	d.AccountID = strings.TrimSpace(d.AccountID)
	if d.ID == "" || d.DedupKey == "" || d.EventID == "" || d.ConversationID == "" || d.AccountID == "" {
		return Delivery{}, ErrInvalidInput
	}
	d.DeviceID = strings.TrimSpace(d.DeviceID)
	d.PushTokenID = strings.TrimSpace(d.PushTokenID)
	d.MessageID = strings.TrimSpace(d.MessageID)
	d.Reason = strings.TrimSpace(d.Reason)
	if d.Kind == NotificationKindUnspecified || d.Mode == DeliveryModeUnspecified {
		return Delivery{}, ErrInvalidInput
	}
	if d.Mode == DeliveryModePush && (d.DeviceID == "" || d.PushTokenID == "") {
		return Delivery{}, ErrInvalidInput
	}
	if d.State == DeliveryStateUnspecified {
		d.State = DeliveryStateQueued
	}
	if d.Attempts < 0 {
		d.Attempts = 0
	}
	if d.NextAttemptAt.IsZero() {
		d.NextAttemptAt = now.UTC()
	}
	d.LeaseToken = strings.TrimSpace(d.LeaseToken)
	if d.LeaseToken == "" {
		d.LeaseExpiresAt = time.Time{}
	} else if d.LeaseExpiresAt.IsZero() {
		return Delivery{}, ErrInvalidInput
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now.UTC()
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = d.CreatedAt
	}

	return d, nil
}

func (c WorkerCursor) normalize(now time.Time) (WorkerCursor, error) {
	c.Name = strings.TrimSpace(strings.ToLower(c.Name))
	if c.Name == "" {
		return WorkerCursor{}, ErrInvalidInput
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now.UTC()
	}

	return c, nil
}
