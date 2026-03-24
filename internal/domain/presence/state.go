package presence

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// PresenceState describes a user's explicit presence preference.
type PresenceState string

// Presence states used by the presence domain.
const (
	PresenceStateUnspecified PresenceState = ""
	PresenceStateOffline     PresenceState = "offline"
	PresenceStateOnline      PresenceState = "online"
	PresenceStateAway        PresenceState = "away"
	PresenceStateBusy        PresenceState = "busy"
	PresenceStateInvisible   PresenceState = "invisible"
)

var (
	// ErrInvalidInput indicates that the caller supplied malformed presence input.
	ErrInvalidInput = errors.New("invalid input")
	// ErrNotFound indicates that no presence record exists for the requested account.
	ErrNotFound = errors.New("not found")
)

// Presence stores the explicit presence settings for an account.
type Presence struct {
	AccountID    string
	State        PresenceState
	CustomStatus string
	HiddenUntil  time.Time
	UpdatedAt    time.Time
}

// Snapshot resolves the presence state together with the derived last-seen time.
type Snapshot struct {
	AccountID      string
	State          PresenceState
	CustomStatus   string
	LastSeenAt     time.Time
	LastSeenHidden bool
	UpdatedAt      time.Time
}

// SetParams captures the state update request for an account.
type SetParams struct {
	AccountID       string
	State           PresenceState
	CustomStatus    string
	HideLastSeenFor time.Duration
	RecordedAt      time.Time
}

// GetParams captures the read request for an account presence snapshot.
type GetParams struct {
	AccountID       string
	ViewerAccountID string
}

func normalizePresenceState(value PresenceState) (PresenceState, error) {
	value = PresenceState(strings.ToLower(strings.TrimSpace(string(value))))
	switch value {
	case PresenceStateUnspecified, PresenceStateOffline, PresenceStateOnline, PresenceStateAway, PresenceStateBusy, PresenceStateInvisible:
		return value, nil
	default:
		return PresenceStateUnspecified, fmt.Errorf("presence state %q: %w", value, ErrInvalidInput)
	}
}

func normalizePresence(p Presence) (Presence, error) {
	var err error
	p.State, err = normalizePresenceState(p.State)
	if err != nil {
		return Presence{}, err
	}

	p.AccountID = strings.TrimSpace(p.AccountID)
	p.CustomStatus = strings.TrimSpace(p.CustomStatus)
	if p.AccountID == "" {
		return Presence{}, ErrInvalidInput
	}
	if p.HiddenUntil.Before(time.Time{}) {
		p.HiddenUntil = time.Time{}
	}

	if p.State == PresenceStateUnspecified {
		p.State = PresenceStateOffline
	}

	return p, nil
}
