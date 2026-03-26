package conversation

import "time"

// ModerationTargetKind identifies the scope of a moderation policy or action.
type ModerationTargetKind string

// Moderation target kinds supported by the conversation domain.
const (
	ModerationTargetKindUnspecified  ModerationTargetKind = ""
	ModerationTargetKindConversation ModerationTargetKind = "conversation"
	ModerationTargetKindTopic        ModerationTargetKind = "topic"
	ModerationTargetKindChannel      ModerationTargetKind = "channel"
)

// ModerationPolicy describes the effective moderation policy for a target.
type ModerationPolicy struct {
	TargetKind               ModerationTargetKind
	TargetID                 string
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
	UpdatedAt                time.Time
}

// ModerationReportStatus describes the lifecycle of a moderation report.
type ModerationReportStatus string

// Moderation report statuses supported by the conversation domain.
const (
	ModerationReportStatusUnspecified ModerationReportStatus = ""
	ModerationReportStatusPending     ModerationReportStatus = "pending"
	ModerationReportStatusResolved    ModerationReportStatus = "resolved"
	ModerationReportStatusRejected    ModerationReportStatus = "rejected"
)

// ModerationReport describes a user-submitted complaint or complaint review.
type ModerationReport struct {
	ID                  string
	TargetKind          ModerationTargetKind
	TargetID            string
	ReporterAccountID   string
	TargetAccountID     string
	Reason              string
	Details             string
	Status              ModerationReportStatus
	ReviewedByAccountID string
	ReviewedAt          time.Time
	Resolution          string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ModerationActionType describes a moderation audit event.
type ModerationActionType string

// Moderation action types supported by the conversation domain.
const (
	ModerationActionTypeUnspecified   ModerationActionType = ""
	ModerationActionTypeBan           ModerationActionType = "ban"
	ModerationActionTypeKick          ModerationActionType = "kick"
	ModerationActionTypeMute          ModerationActionType = "mute"
	ModerationActionTypeUnmute        ModerationActionType = "unmute"
	ModerationActionTypeShadowBan     ModerationActionType = "shadow_ban"
	ModerationActionTypeShadowUnban   ModerationActionType = "shadow_unban"
	ModerationActionTypePolicySet     ModerationActionType = "policy_set"
	ModerationActionTypeReportSet     ModerationActionType = "report_set"
	ModerationActionTypeReportResolve ModerationActionType = "report_resolve"
	ModerationActionTypeReportReject  ModerationActionType = "report_reject"
	ModerationActionTypeSlowModeSet   ModerationActionType = "slow_mode_set"
	ModerationActionTypeAntiSpamSet   ModerationActionType = "anti_spam_set"
)

// ModerationAction records an immutable audit log entry.
type ModerationAction struct {
	ID              string
	TargetKind      ModerationTargetKind
	TargetID        string
	ActorAccountID  string
	TargetAccountID string
	Type            ModerationActionType
	Duration        time.Duration
	Reason          string
	Metadata        map[string]string
	CreatedAt       time.Time
}

// ModerationRestrictionState describes the moderation status of an account within a target.
type ModerationRestrictionState string

// Moderation restriction states supported by the conversation domain.
const (
	ModerationRestrictionStateUnspecified ModerationRestrictionState = ""
	ModerationRestrictionStateMuted       ModerationRestrictionState = "muted"
	ModerationRestrictionStateBanned      ModerationRestrictionState = "banned"
	ModerationRestrictionStateShadowed    ModerationRestrictionState = "shadowed"
)

// ModerationRestriction describes an active moderation constraint.
type ModerationRestriction struct {
	TargetKind         ModerationTargetKind
	TargetID           string
	AccountID          string
	State              ModerationRestrictionState
	AppliedByAccountID string
	Reason             string
	ExpiresAt          time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ModerationRateState tracks recent write activity for slow-mode enforcement.
type ModerationRateState struct {
	TargetKind      ModerationTargetKind
	TargetID        string
	AccountID       string
	LastWriteAt     time.Time
	WindowStartedAt time.Time
	WindowCount     uint64
	UpdatedAt       time.Time
}

// ModerationDecision describes the effect of a moderation check.
type ModerationDecision struct {
	Allowed      bool
	ShadowHidden bool
	RetryAfter   time.Duration
}
