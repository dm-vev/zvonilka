package call

import "time"

// State identifies the lifecycle stage of a call.
type State string

const (
	// StateUnspecified is the zero value.
	StateUnspecified State = ""
	// StateRinging indicates an unanswered call.
	StateRinging State = "ringing"
	// StateActive indicates an answered call.
	StateActive State = "active"
	// StateEnded indicates a completed call.
	StateEnded State = "ended"
)

// EndReason identifies why a call ended.
type EndReason string

const (
	// EndReasonUnspecified is the zero value.
	EndReasonUnspecified EndReason = ""
	// EndReasonCancelled means the initiator cancelled the call.
	EndReasonCancelled EndReason = "cancelled"
	// EndReasonDeclined means the target explicitly declined the call.
	EndReasonDeclined EndReason = "declined"
	// EndReasonMissed means the call timed out unanswered.
	EndReasonMissed EndReason = "missed"
	// EndReasonEnded means a participant ended an active call.
	EndReasonEnded EndReason = "ended"
	// EndReasonFailed means the call failed before completion.
	EndReasonFailed EndReason = "failed"
)

// InviteState identifies the state of one call invite.
type InviteState string

const (
	// InviteStateUnspecified is the zero value.
	InviteStateUnspecified InviteState = ""
	// InviteStatePending indicates a live incoming invite.
	InviteStatePending InviteState = "pending"
	// InviteStateAccepted indicates an accepted invite.
	InviteStateAccepted InviteState = "accepted"
	// InviteStateDeclined indicates a declined invite.
	InviteStateDeclined InviteState = "declined"
	// InviteStateCancelled indicates a cancelled invite.
	InviteStateCancelled InviteState = "cancelled"
	// InviteStateExpired indicates an expired invite.
	InviteStateExpired InviteState = "expired"
)

// ParticipantState identifies the state of one joined participant device.
type ParticipantState string

const (
	// ParticipantStateUnspecified is the zero value.
	ParticipantStateUnspecified ParticipantState = ""
	// ParticipantStateJoined indicates an active participant.
	ParticipantStateJoined ParticipantState = "joined"
	// ParticipantStateLeft indicates a departed participant.
	ParticipantStateLeft ParticipantState = "left"
)

// EventType identifies a call event.
type EventType string

const (
	// EventTypeUnspecified is the zero value.
	EventTypeUnspecified EventType = ""
	// EventTypeStarted indicates a new call.
	EventTypeStarted EventType = "call.started"
	// EventTypeInvited indicates a ringing invite.
	EventTypeInvited EventType = "call.invited"
	// EventTypeAccepted indicates an accepted invite.
	EventTypeAccepted EventType = "call.accepted"
	// EventTypeDeclined indicates a declined invite.
	EventTypeDeclined EventType = "call.declined"
	// EventTypeJoined indicates a joined participant.
	EventTypeJoined EventType = "call.joined"
	// EventTypeLeft indicates a departed participant.
	EventTypeLeft EventType = "call.left"
	// EventTypeMediaUpdated indicates changed participant media state.
	EventTypeMediaUpdated EventType = "call.media_updated"
	// EventTypeEnded indicates a finished call.
	EventTypeEnded EventType = "call.ended"
	// EventTypeSignalDescription indicates a published SDP description.
	EventTypeSignalDescription EventType = "call.signal_description"
	// EventTypeSignalCandidate indicates a published ICE candidate.
	EventTypeSignalCandidate EventType = "call.signal_candidate"
)

// MediaState describes the participant media toggles visible to other clients.
type MediaState struct {
	AudioMuted    bool
	VideoMuted    bool
	CameraEnabled bool
}

// TransportStats describes live transport quality counters for one participant device.
type TransportQualitySample struct {
	Quality            string
	RecommendedProfile string
	RecordedAt         time.Time
}

// TransportStats describes live transport quality counters for one participant device.
type TransportStats struct {
	PeerConnectionState      string
	IceConnectionState       string
	SignalingState           string
	Quality                  string
	RecommendedProfile       string
	RecommendationReason     string
	VideoFallbackRecommended bool
	ReconnectRecommended     bool
	SuppressOutgoingVideo    bool
	SuppressIncomingVideo    bool
	SuppressOutgoingAudio    bool
	SuppressIncomingAudio    bool
	ReconnectAttempt         uint32
	ReconnectBackoffUntil    time.Time
	QualityTrend             string
	DegradedTransitions      uint32
	RecoveredTransitions     uint32
	LastQualityChangeAt      time.Time
	RecentSamples            []TransportQualitySample
	RelayTracks              uint32
	RelayPackets             uint64
	RelayBytes               uint64
	RelayWriteErrors         uint64
	LastUpdatedAt            time.Time
}

// QualitySummary describes aggregated quality state for one call.
type QualitySummary struct {
	WorstQuality              string
	DominantProfile           string
	ParticipantCount          uint32
	VideoFallbackParticipants uint32
	ReconnectParticipants     uint32
	OutgoingVideoSuppressed   uint32
	IncomingVideoSuppressed   uint32
	OutgoingAudioSuppressed   uint32
	IncomingAudioSuppressed   uint32
	DegradedTransitions       uint32
	RecoveredTransitions      uint32
	LastChangedAt             time.Time
}

// IceServer describes one STUN or TURN server returned to a client.
type IceServer struct {
	URLs       []string
	Username   string
	Credential string
	ExpiresAt  time.Time
}

// Invite describes one pending or completed call invite.
type Invite struct {
	CallID     string
	AccountID  string
	State      InviteState
	ExpiresAt  time.Time
	AnsweredAt time.Time
	UpdatedAt  time.Time
}

// Participant describes one joined device in a call.
type Participant struct {
	CallID     string
	AccountID  string
	DeviceID   string
	State      ParticipantState
	MediaState MediaState
	Transport  TransportStats
	JoinedAt   time.Time
	LeftAt     time.Time
	UpdatedAt  time.Time
}

// Call describes one direct call and its current state.
type Call struct {
	ID                 string
	ConversationID     string
	InitiatorAccountID string
	ActiveSessionID    string
	RequestedVideo     bool
	State              State
	EndReason          EndReason
	StartedAt          time.Time
	AnsweredAt         time.Time
	EndedAt            time.Time
	UpdatedAt          time.Time
	Invites            []Invite
	Participants       []Participant
	QualitySummary     QualitySummary
}

// Event describes one persisted call event.
type Event struct {
	EventID        string
	CallID         string
	ConversationID string
	EventType      EventType
	ActorAccountID string
	ActorDeviceID  string
	Sequence       uint64
	Metadata       map[string]string
	CreatedAt      time.Time
	Call           Call
}

// JoinDetails returns the client transport payload for a joined participant.
type JoinDetails struct {
	SessionID       string
	SessionToken    string
	RuntimeEndpoint string
	ExpiresAt       time.Time
	IceUfrag        string
	IcePwd          string
	DTLSFingerprint string
	CandidateHost   string
	CandidatePort   int
	IceServers      []IceServer
}

// SessionDescription describes one SDP payload exchanged through call signaling.
type SessionDescription struct {
	Type string
	SDP  string
}

// Candidate describes one ICE candidate payload exchanged through call signaling.
type Candidate struct {
	Candidate        string
	SDPMid           string
	SDPMLineIndex    uint32
	UsernameFragment string
}
