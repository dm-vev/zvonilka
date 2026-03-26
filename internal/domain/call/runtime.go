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
	STUNURLs       []string
	TURNURLs       []string
	TURNSecret     string
}

// RuntimeSession describes one active media room.
type RuntimeSession struct {
	SessionID       string
	RuntimeEndpoint string
}

// RuntimeJoin describes one participant admission to the media room.
type RuntimeJoin struct {
	SessionID       string
	SessionToken    string
	RuntimeEndpoint string
	ExpiresAt       time.Time
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
	LeaveSession(ctx context.Context, sessionID string, accountID string, deviceID string) error
	CloseSession(ctx context.Context, sessionID string) error
}

func (c RTCConfig) normalize() RTCConfig {
	if c.CredentialTTL <= 0 {
		c.CredentialTTL = 15 * time.Minute
	}
	c.PublicEndpoint = strings.TrimSpace(c.PublicEndpoint)
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
