package rtc

import (
	"context"
	"strings"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

type sessionSnapshot struct {
	CallID       string
	Conversation string
	Participants []snapshotParticipant
}

type snapshotParticipant struct {
	AccountID string
	DeviceID  string
	WithVideo bool
	Media     domaincall.MediaState
}

func cloneSessionSnapshot(value sessionSnapshot) sessionSnapshot {
	value.CallID = strings.TrimSpace(value.CallID)
	value.Conversation = strings.TrimSpace(value.Conversation)
	if len(value.Participants) == 0 {
		value.Participants = nil
		return value
	}

	cloned := make([]snapshotParticipant, len(value.Participants))
	copy(cloned, value.Participants)
	value.Participants = cloned
	return value
}

func (m *Manager) ExportSessionSnapshot(_ context.Context, sessionID string) (sessionSnapshot, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return sessionSnapshot{}, domaincall.ErrInvalidInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	active := m.sessions[sessionID]
	if active == nil {
		return sessionSnapshot{}, domaincall.ErrNotFound
	}

	snapshot := sessionSnapshot{
		CallID:       active.callID,
		Conversation: active.conversation,
		Participants: make([]snapshotParticipant, 0, len(active.participants)),
	}
	for _, participant := range active.participants {
		snapshot.Participants = append(snapshot.Participants, snapshotParticipant{
			AccountID: participant.accountID,
			DeviceID:  participant.deviceID,
			WithVideo: participant.withVideo,
			Media:     participant.media,
		})
	}

	return cloneSessionSnapshot(snapshot), nil
}

func (m *Manager) SaveReplica(_ context.Context, snapshot sessionSnapshot) error {
	snapshot = cloneSessionSnapshot(snapshot)
	if strings.TrimSpace(snapshot.CallID) == "" {
		return domaincall.ErrInvalidInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.replicas == nil {
		m.replicas = make(map[string]sessionSnapshot)
	}
	m.replicas[snapshot.CallID] = snapshot
	return nil
}

func (m *Manager) RestoreReplica(_ context.Context, callID string, sessionID string) error {
	callID = strings.TrimSpace(callID)
	sessionID = strings.TrimSpace(sessionID)
	if callID == "" || sessionID == "" {
		return domaincall.ErrInvalidInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	snapshot, ok := m.replicas[callID]
	if !ok {
		return domaincall.ErrNotFound
	}
	active := m.sessions[sessionID]
	if active == nil {
		return domaincall.ErrNotFound
	}

	active.callID = snapshot.CallID
	active.conversation = snapshot.Conversation
	if active.participants == nil {
		active.participants = make(map[string]participant)
	}
	for _, item := range snapshot.Participants {
		active.participants[participantKey(item.AccountID, item.DeviceID)] = participant{
			accountID: item.AccountID,
			deviceID:  item.DeviceID,
			withVideo: item.WithVideo,
			media:     item.Media,
		}
	}

	return nil
}
