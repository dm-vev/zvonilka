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
	CandidateHost  string
	UDPPortMin     int
	UDPPortMax     int
	STUNURLs       []string
	TURNURLs       []string
	TURNSecret     string
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

// RuntimeParticipant describes one participant join request for the media plane.
type RuntimeParticipant struct {
	CallID    string
	AccountID string
	DeviceID  string
	WithVideo bool
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
	c.PublicEndpoint = strings.TrimSpace(c.PublicEndpoint)
	c.CandidateHost = strings.TrimSpace(c.CandidateHost)
	c.TURNSecret = strings.TrimSpace(c.TURNSecret)
	c.STUNURLs = trimList(c.STUNURLs)
	c.TURNURLs = trimList(c.TURNURLs)

	return c
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
