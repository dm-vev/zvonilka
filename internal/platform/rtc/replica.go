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
	Transport domaincall.TransportStats
	Relay     []snapshotRelayTrack
}

type snapshotRelayTrack struct {
	SourceAccountID string
	SourceDeviceID  string
	TrackID         string
	StreamID        string
	Kind            string
	ScreenShare     bool
	CodecMimeType   string
	CodecClockRate  uint32
	CodecChannels   uint32
}

func cloneSessionSnapshot(value sessionSnapshot) sessionSnapshot {
	value.CallID = strings.TrimSpace(value.CallID)
	value.Conversation = strings.TrimSpace(value.Conversation)
	if len(value.Participants) == 0 {
		value.Participants = nil
		return value
	}

	cloned := make([]snapshotParticipant, len(value.Participants))
	for i := range value.Participants {
		cloned[i] = cloneSnapshotParticipant(value.Participants[i])
	}
	value.Participants = cloned
	return value
}

func cloneSnapshotParticipant(value snapshotParticipant) snapshotParticipant {
	value.Transport = cloneTransportStats(value.Transport)
	if len(value.Relay) > 0 {
		value.Relay = append([]snapshotRelayTrack(nil), value.Relay...)
	}
	return value
}

func cloneTransportStats(value domaincall.TransportStats) domaincall.TransportStats {
	value.RecentSamples = cloneTransportQualitySamples(value.RecentSamples)
	value.RecentQoSSamples = cloneTransportQoSSamples(value.RecentQoSSamples)
	return value
}

func cloneTransportQualitySamples(values []domaincall.TransportQualitySample) []domaincall.TransportQualitySample {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]domaincall.TransportQualitySample, len(values))
	copy(cloned, values)
	return cloned
}

func cloneTransportQoSSamples(values []domaincall.TransportQoSSample) []domaincall.TransportQoSSample {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]domaincall.TransportQoSSample, len(values))
	copy(cloned, values)
	return cloned
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
	for key, participant := range active.participants {
		transport := cloneTransportStats(active.standbyStats[key])
		relay := append([]snapshotRelayTrack(nil), active.standbyRelay[key]...)
		if value := active.peers[key]; value != nil {
			transport = cloneTransportStats(value.snapshotStats())
			relay = value.snapshotRelayBlueprints()
		}
		snapshot.Participants = append(snapshot.Participants, snapshotParticipant{
			AccountID: participant.accountID,
			DeviceID:  participant.deviceID,
			WithVideo: participant.withVideo,
			Media:     participant.media,
			Transport: transport,
			Relay:     relay,
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
	if active.standbyStats == nil {
		active.standbyStats = make(map[string]domaincall.TransportStats)
	}
	if active.standbyRelay == nil {
		active.standbyRelay = make(map[string][]snapshotRelayTrack)
	}
	for _, item := range snapshot.Participants {
		key := participantKey(item.AccountID, item.DeviceID)
		active.participants[key] = participant{
			accountID: item.AccountID,
			deviceID:  item.DeviceID,
			withVideo: item.WithVideo,
			media:     item.Media,
		}
		if hasTransportStats(item.Transport) {
			active.standbyStats[key] = cloneTransportStats(item.Transport)
		} else {
			delete(active.standbyStats, key)
		}
		if len(item.Relay) > 0 {
			active.standbyRelay[key] = append([]snapshotRelayTrack(nil), item.Relay...)
		} else {
			delete(active.standbyRelay, key)
		}
	}

	return nil
}

func hasTransportStats(value domaincall.TransportStats) bool {
	return !value.LastUpdatedAt.IsZero() ||
		value.PeerConnectionState != "" ||
		value.IceConnectionState != "" ||
		value.SignalingState != "" ||
		value.Quality != "" ||
		value.AdaptationRevision != 0 ||
		value.ReconnectAttempt != 0 ||
		value.RelayPackets != 0 ||
		value.RelayBytes != 0 ||
		value.RelayWriteErrors != 0 ||
		len(value.RecentSamples) > 0 ||
		len(value.RecentQoSSamples) > 0
}
