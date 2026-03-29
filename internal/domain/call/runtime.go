package call

import (
	"context"
	"strings"
	"time"
)

// RTCConfig controls ICE and runtime connectivity.
type RTCConfig struct {
	PublicEndpoint string
	CredentialTTL  time.Duration
	NodeID         string
	CandidateHost  string
	UDPPortMin     int
	UDPPortMax     int
	HealthTTL      time.Duration
	HealthTimeout  time.Duration
	STUNURLs       []string
	TURNURLs       []string
	TURNSecret     string
	Nodes          []RTCNode
}

// RTCNode describes one logical media-plane node.
type RTCNode struct {
	ID              string
	Endpoint        string
	ControlEndpoint string
}

// RuntimeSession describes one active media room.
type RuntimeSession struct {
	SessionID       string
	RuntimeEndpoint string
	IceUfrag        string
	IcePwd          string
	DTLSFingerprint string
	CandidateHost   string
	CandidatePort   int
}

// RuntimeJoin describes one participant admission to the media room.
type RuntimeJoin struct {
	SessionID       string
	SessionToken    string
	RuntimeEndpoint string
	ExpiresAt       time.Time
	IceUfrag        string
	IcePwd          string
	DTLSFingerprint string
	CandidateHost   string
	CandidatePort   int
}

// RuntimeSignal describes one server-generated signaling payload.
type RuntimeSignal struct {
	TargetAccountID string
	TargetDeviceID  string
	SessionID       string
	Description     *SessionDescription
	IceCandidate    *Candidate
	Metadata        map[string]string
}

// RuntimeStats describes live transport stats for one joined participant device.
type RuntimeStats struct {
	AccountID string
	DeviceID  string
	Transport TransportStats
}

// RuntimeParticipant describes one participant join request for the media plane.
type RuntimeParticipant struct {
	CallID    string
	AccountID string
	DeviceID  string
	WithVideo bool
	Media     MediaState
}

// Runtime manages media-room lifecycle in the external RTC plane.
type Runtime interface {
	EnsureSession(ctx context.Context, call Call) (RuntimeSession, error)
	JoinSession(ctx context.Context, sessionID string, participant RuntimeParticipant) (RuntimeJoin, error)
	PublishDescription(
		ctx context.Context,
		sessionID string,
		participant RuntimeParticipant,
		description SessionDescription,
	) ([]RuntimeSignal, error)
	PublishCandidate(
		ctx context.Context,
		sessionID string,
		participant RuntimeParticipant,
		candidate Candidate,
	) ([]RuntimeSignal, error)
	UpdateParticipant(
		ctx context.Context,
		sessionID string,
		participant RuntimeParticipant,
	) error
	AcknowledgeAdaptation(
		ctx context.Context,
		sessionID string,
		participant RuntimeParticipant,
		adaptationRevision uint64,
		appliedProfile string,
	) error
	SessionStats(ctx context.Context, sessionID string) ([]RuntimeStats, error)
	LeaveSession(ctx context.Context, sessionID string, accountID string, deviceID string) error
	CloseSession(ctx context.Context, sessionID string) error
}

func (c RTCConfig) normalize() RTCConfig {
	if c.CredentialTTL <= 0 {
		c.CredentialTTL = 15 * time.Minute
	}
	if c.UDPPortMin <= 0 {
		c.UDPPortMin = 40000
	}
	if c.UDPPortMax <= 0 {
		c.UDPPortMax = 40100
	}
	if c.UDPPortMax < c.UDPPortMin {
		c.UDPPortMax = c.UDPPortMin
	}
	if c.HealthTTL <= 0 {
		c.HealthTTL = 2 * time.Second
	}
	if c.HealthTimeout <= 0 {
		c.HealthTimeout = 1 * time.Second
	}
	c.PublicEndpoint = strings.TrimSpace(c.PublicEndpoint)
	c.NodeID = strings.TrimSpace(c.NodeID)
	c.CandidateHost = strings.TrimSpace(c.CandidateHost)
	c.TURNSecret = strings.TrimSpace(c.TURNSecret)
	c.STUNURLs = trimList(c.STUNURLs)
	c.TURNURLs = trimList(c.TURNURLs)
	c.Nodes = normalizeRTCNodes(c.Nodes)

	return c
}

// NormalizeForPlatform returns a normalized RTC config for runtime integration layers.
func (c RTCConfig) NormalizeForPlatform() RTCConfig {
	return c.normalize()
}

func trimList(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
	}

	return result
}

func normalizeRTCNodes(values []RTCNode) []RTCNode {
	if len(values) == 0 {
		return nil
	}

	result := make([]RTCNode, 0, len(values))
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Endpoint = strings.TrimSpace(value.Endpoint)
		value.ControlEndpoint = strings.TrimSpace(value.ControlEndpoint)
		if value.ID == "" || value.Endpoint == "" {
			continue
		}
		result = append(result, value)
	}

	return result
}

func (c RTCConfig) endpointForSession(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		nodeID := NodeIDFromSessionID(sessionID)
		for _, node := range c.Nodes {
			if node.ID == nodeID && node.Endpoint != "" {
				return node.Endpoint
			}
		}
	}

	return c.PublicEndpoint
}

// NodeIDFromSessionID extracts the owning media-node identifier from a session id.
func NodeIDFromSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}

	nodeID, _, found := strings.Cut(sessionID, ":")
	if !found {
		return ""
	}

	return strings.TrimSpace(nodeID)
}
