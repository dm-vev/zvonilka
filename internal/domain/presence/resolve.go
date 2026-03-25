package presence

import (
	"context"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

func (s *Service) lastSeenAt(ctx context.Context, account identity.Account) (time.Time, error) {
	lastSeen := account.LastAuthAt.UTC()

	sessions, err := s.identity.SessionsByAccountID(ctx, account.ID)
	if err != nil {
		return time.Time{}, err
	}
	for _, session := range sessions {
		if session.Status != identity.SessionStatusActive {
			continue
		}
		if session.LastSeenAt.After(lastSeen) {
			lastSeen = session.LastSeenAt.UTC()
		}
	}

	devices, err := s.identity.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		return time.Time{}, err
	}
	for _, device := range devices {
		if device.Status != identity.DeviceStatusActive {
			continue
		}
		if device.LastSeenAt.After(lastSeen) {
			lastSeen = device.LastSeenAt.UTC()
		}
	}

	return lastSeen, nil
}

func (s *Service) snapshot(viewerAccountID string, record Presence, lastSeenAt time.Time) Snapshot {
	state := record.State
	hasExplicitRecord := strings.TrimSpace(record.AccountID) != ""

	now := s.currentTime()
	online := !lastSeenAt.IsZero() && now.Sub(lastSeenAt) <= s.settings.OnlineWindow

	switch state {
	case PresenceStateInvisible:
		if viewerAccountID != record.AccountID {
			state = PresenceStateOffline
		}
	case PresenceStateOffline, PresenceStateOnline, PresenceStateAway, PresenceStateBusy:
		// Keep the explicit state as-is.
	default:
		if hasExplicitRecord {
			state = PresenceStateOffline
			break
		}
		if online {
			state = PresenceStateOnline
		} else {
			state = PresenceStateOffline
		}
	}

	hidden := false
	if viewerAccountID != record.AccountID && !record.HiddenUntil.IsZero() && now.Before(record.HiddenUntil) {
		hidden = true
	}
	if viewerAccountID != record.AccountID && state == PresenceStateInvisible {
		hidden = true
	}

	snapshot := Snapshot{
		AccountID:    record.AccountID,
		State:        state,
		CustomStatus: record.CustomStatus,
		UpdatedAt:    record.UpdatedAt,
	}
	if hidden {
		snapshot.LastSeenHidden = true
		return snapshot
	}

	snapshot.LastSeenAt = lastSeenAt
	return snapshot
}
