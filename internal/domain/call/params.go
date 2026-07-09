package call

// StartParams describes one outbound call attempt.
type StartParams struct {
	ConversationID string
	AccountID      string
	DeviceID       string
	WithVideo      bool
}

// AcceptParams describes one accepted incoming call.
type AcceptParams struct {
	CallID    string
	AccountID string
	DeviceID  string
}

// DeclineParams describes one declined incoming call.
type DeclineParams struct {
	CallID    string
	AccountID string
	DeviceID  string
}

// CancelParams describes one caller-side cancellation.
type CancelParams struct {
	CallID    string
	AccountID string
	DeviceID  string
}

// EndParams describes one active call termination.
type EndParams struct {
	CallID    string
	AccountID string
	DeviceID  string
	Reason    EndReason
}

// JoinParams describes one participant room join.
type JoinParams struct {
	CallID    string
	AccountID string
	DeviceID  string
	WithVideo bool
}

// ReconnectParams describes one already-joined participant reconnect to the active media session.
type ReconnectParams struct {
	CallID    string
	AccountID string
	DeviceID  string
}

// HandoffParams describes one device handoff for the same account.
type HandoffParams struct {
	CallID       string
	AccountID    string
	FromDeviceID string
	ToDeviceID   string
}

// MigrateParams describes one explicit active-session migration request.
type MigrateParams struct {
	CallID    string
	AccountID string
	DeviceID  string
}

// PublishDescriptionParams describes one SDP publication for a joined participant.
type PublishDescriptionParams struct {
	CallID      string
	SessionID   string
	AccountID   string
	DeviceID    string
	Description SessionDescription
}

// PublishCandidateParams describes one ICE-candidate publication for a joined participant.
type PublishCandidateParams struct {
	CallID       string
	SessionID    string
	AccountID    string
	DeviceID     string
	IceCandidate Candidate
}

// LeaveParams describes one participant leave.
type LeaveParams struct {
	CallID    string
	AccountID string
	DeviceID  string
}

// UpdateParams describes one media-state update.
type UpdateParams struct {
	CallID    string
	AccountID string
	DeviceID  string
	Media     MediaState
}

// UpdateRecordingParams describes one call-recording state update.
type UpdateRecordingParams struct {
	CallID    string
	AccountID string
	DeviceID  string
	Enabled   bool
}

// UpdateTranscriptionParams describes one call-transcription state update.
type UpdateTranscriptionParams struct {
	CallID    string
	AccountID string
	DeviceID  string
	Enabled   bool
}

// RaiseHandParams describes one raise-hand update for a joined participant.
type RaiseHandParams struct {
	CallID    string
	AccountID string
	DeviceID  string
	Raised    bool
}

// ModerateParticipantParams describes one moderator-issued participant control update.
type ModerateParticipantParams struct {
	CallID         string
	AccountID      string
	DeviceID       string
	TargetDeviceID string
	HostMutedAudio bool
	HostMutedVideo bool
	LowerHand      bool
}

// MuteAllParticipantsParams describes one moderator-issued mute-all update.
type MuteAllParticipantsParams struct {
	CallID     string
	AccountID  string
	DeviceID   string
	MuteAudio  bool
	MuteVideo  bool
	LowerHands bool
}

// RemoveParticipantParams describes one moderator-issued participant removal.
type RemoveParticipantParams struct {
	CallID         string
	AccountID      string
	DeviceID       string
	TargetDeviceID string
}

// TransferHostParams describes one group-call host transfer.
type TransferHostParams struct {
	CallID          string
	AccountID       string
	DeviceID        string
	TargetAccountID string
}

// UpdateStageModeParams describes one stage-mode toggle for a group call.
type UpdateStageModeParams struct {
	CallID    string
	AccountID string
	DeviceID  string
	Enabled   bool
}

// PinSpeakerParams describes one pinned-speaker update for a group call.
type PinSpeakerParams struct {
	CallID         string
	AccountID      string
	DeviceID       string
	TargetDeviceID string
	Pinned         bool
}

// AcknowledgeAdaptationParams describes one acknowledged server-issued adaptation revision.
type AcknowledgeAdaptationParams struct {
	CallID             string
	SessionID          string
	AccountID          string
	DeviceID           string
	AdaptationRevision uint64
	AppliedProfile     string
}

// GetParams identifies one visible call.
type GetParams struct {
	CallID    string
	AccountID string
}

// ListParams filters calls in one conversation.
type ListParams struct {
	ConversationID string
	AccountID      string
	IncludeEnded   bool
}

// IceParams requests the RTC server list for a visible call.
type IceParams struct {
	CallID    string
	AccountID string
}

// EventParams filters the call event stream.
type EventParams struct {
	FromSequence   uint64
	CallID         string
	ConversationID string
	Limit          int
	AccountID      string
	DeviceID       string
}
