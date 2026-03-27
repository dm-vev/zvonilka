package rtc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

// Manager keeps RTC sessions inside the server process.
type Manager struct {
	mu       sync.Mutex
	api      *webrtc.API
	endpoint string
	tokenTTL time.Duration
	host     string
	portMin  int
	portMax  int
	nextPort int
	dtlsFP   string
	now      func() time.Time
	sessions map[string]*session
}

type session struct {
	id           string
	callID       string
	conversation string
	iceUfrag     string
	icePwd       string
	host         string
	port         int
	participants map[string]participant
	peers        map[string]*peer
}

type participant struct {
	accountID string
	deviceID  string
	withVideo bool
}

type peer struct {
	accountID              string
	deviceID               string
	withVideo              bool
	pc                     *webrtc.PeerConnection
	signalMu               sync.Mutex
	signals                []domaincall.RuntimeSignal
	transportSignal        domaincall.RuntimeSignal
	hasTransportSignal     bool
	tracks                 map[string]*relayTrack
	mu                     sync.Mutex
	offering               bool
	queued                 bool
	lastPC                 string
	lastICE                string
	lastSIG                string
	stats                  domaincall.TransportStats
	consecutiveRelayErrors uint32
	reconnectAttempts      uint32
}

type relayTrack struct {
	sourceKey string
	track     *webrtc.TrackLocalStaticRTP
	sender    *webrtc.RTPSender
}

type relayTarget struct {
	peer  *peer
	track *webrtc.TrackLocalStaticRTP
}

const (
	telemetryKindKey                  = "telemetry_kind"
	telemetryKindTransport            = "transport_state"
	telemetryPCStateKey               = "peer_connection_state"
	telemetryICEStateKey              = "ice_connection_state"
	telemetrySignalingKey             = "signaling_state"
	telemetryQualityKey               = "transport_quality"
	telemetryProfileKey               = "recommended_profile"
	telemetryReasonKey                = "recommendation_reason"
	telemetryVideoKey                 = "video_fallback_recommended"
	telemetryReconnectKey             = "reconnect_recommended"
	telemetrySuppressOutgoingVideoKey = "suppress_outgoing_video"
	telemetrySuppressIncomingVideoKey = "suppress_incoming_video"
	telemetrySuppressOutgoingAudioKey = "suppress_outgoing_audio"
	telemetrySuppressIncomingAudioKey = "suppress_incoming_audio"
	telemetryReconnectAttemptKey      = "reconnect_attempt"
	telemetryReconnectBackoffKey      = "reconnect_backoff_until"
	telemetryAdaptationRevisionKey    = "adaptation_revision"
	telemetryPendingAdaptationKey     = "pending_adaptation"
	telemetryAckedRevisionKey         = "acked_adaptation_revision"
	telemetryAppliedProfileKey        = "applied_profile"
	maxQualitySamples                 = 8
	maxPendingSignals                 = 256
)

// NewManager constructs an in-process RTC session manager.
func NewManager(endpoint string, tokenTTL time.Duration, opts ...Option) *Manager {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = "webrtc://gateway/calls"
	}
	if tokenTTL <= 0 {
		tokenTTL = 15 * time.Minute
	}

	manager := &Manager{
		endpoint: endpoint,
		tokenTTL: tokenTTL,
		host:     "127.0.0.1",
		portMin:  40000,
		portMax:  40100,
		nextPort: 40000,
		dtlsFP:   mustDTLSFingerprint(),
		now:      func() time.Time { return time.Now().UTC() },
		sessions: make(map[string]*session),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(manager)
		}
	}
	if manager.portMin <= 0 {
		manager.portMin = 40000
	}
	if manager.portMax < manager.portMin {
		manager.portMax = manager.portMin
	}
	if manager.nextPort < manager.portMin || manager.nextPort > manager.portMax {
		manager.nextPort = manager.portMin
	}
	manager.api = manager.newAPI()

	return manager
}

// Option configures the in-server RTC manager.
type Option func(*Manager)

// WithCandidateHost overrides the candidate host returned to clients.
func WithCandidateHost(host string) Option {
	return func(manager *Manager) {
		if manager != nil && strings.TrimSpace(host) != "" {
			manager.host = strings.TrimSpace(host)
		}
	}
}

// WithUDPPortRange overrides the UDP media-plane port range.
func WithUDPPortRange(min int, max int) Option {
	return func(manager *Manager) {
		if manager == nil {
			return
		}
		if min > 0 {
			manager.portMin = min
		}
		if max > 0 {
			manager.portMax = max
		}
	}
}

// EnsureSession creates or resolves the active media session for a call.
func (m *Manager) EnsureSession(_ context.Context, callRow domaincall.Call) (domaincall.RuntimeSession, error) {
	if strings.TrimSpace(callRow.ID) == "" {
		return domaincall.RuntimeSession{}, domaincall.ErrInvalidInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sessionID := strings.TrimSpace(callRow.ActiveSessionID)
	if sessionID == "" {
		sessionID = "rtc_" + callRow.ID
	}
	if active, ok := m.sessions[sessionID]; ok {
		return domaincall.RuntimeSession{
			SessionID:       active.id,
			RuntimeEndpoint: m.endpoint,
			IceUfrag:        active.iceUfrag,
			IcePwd:          active.icePwd,
			DTLSFingerprint: m.dtlsFP,
			CandidateHost:   active.host,
			CandidatePort:   active.port,
		}, nil
	}

	iceUfrag, err := randomToken(8)
	if err != nil {
		return domaincall.RuntimeSession{}, fmt.Errorf("generate ice ufrag: %w", err)
	}
	icePwd, err := randomToken(24)
	if err != nil {
		return domaincall.RuntimeSession{}, fmt.Errorf("generate ice pwd: %w", err)
	}
	port, err := m.allocatePortLocked()
	if err != nil {
		return domaincall.RuntimeSession{}, err
	}

	m.sessions[sessionID] = &session{
		id:           sessionID,
		callID:       callRow.ID,
		conversation: callRow.ConversationID,
		iceUfrag:     iceUfrag,
		icePwd:       icePwd,
		host:         m.host,
		port:         port,
		participants: make(map[string]participant),
		peers:        make(map[string]*peer),
	}

	return domaincall.RuntimeSession{
		SessionID:       sessionID,
		RuntimeEndpoint: m.endpoint,
		IceUfrag:        iceUfrag,
		IcePwd:          icePwd,
		DTLSFingerprint: m.dtlsFP,
		CandidateHost:   m.host,
		CandidatePort:   port,
	}, nil
}

// JoinSession admits one participant device to a session.
func (m *Manager) JoinSession(
	_ context.Context,
	sessionID string,
	participantRow domaincall.RuntimeParticipant,
) (domaincall.RuntimeJoin, error) {
	sessionID = strings.TrimSpace(sessionID)
	participantRow.AccountID = strings.TrimSpace(participantRow.AccountID)
	participantRow.DeviceID = strings.TrimSpace(participantRow.DeviceID)
	if sessionID == "" || participantRow.AccountID == "" || participantRow.DeviceID == "" {
		return domaincall.RuntimeJoin{}, domaincall.ErrInvalidInput
	}

	token, err := randomToken(24)
	if err != nil {
		return domaincall.RuntimeJoin{}, fmt.Errorf("generate rtc session token: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	active, ok := m.sessions[sessionID]
	if !ok {
		return domaincall.RuntimeJoin{}, domaincall.ErrNotFound
	}
	active.participants[participantKey(participantRow.AccountID, participantRow.DeviceID)] = participant{
		accountID: participantRow.AccountID,
		deviceID:  participantRow.DeviceID,
		withVideo: participantRow.WithVideo,
	}

	return domaincall.RuntimeJoin{
		SessionID:       active.id,
		SessionToken:    token,
		RuntimeEndpoint: m.endpoint,
		ExpiresAt:       m.now().Add(m.tokenTTL),
		IceUfrag:        active.iceUfrag,
		IcePwd:          active.icePwd,
		DTLSFingerprint: m.dtlsFP,
		CandidateHost:   active.host,
		CandidatePort:   active.port,
	}, nil
}

// PublishDescription applies one remote SDP description and returns server-generated signals.
func (m *Manager) PublishDescription(
	_ context.Context,
	sessionID string,
	participantRow domaincall.RuntimeParticipant,
	description domaincall.SessionDescription,
) ([]domaincall.RuntimeSignal, error) {
	sessionID = strings.TrimSpace(sessionID)
	participantRow.AccountID = strings.TrimSpace(participantRow.AccountID)
	participantRow.DeviceID = strings.TrimSpace(participantRow.DeviceID)
	description.Type = strings.ToLower(strings.TrimSpace(description.Type))

	if sessionID == "" || participantRow.AccountID == "" || participantRow.DeviceID == "" {
		return nil, domaincall.ErrInvalidInput
	}
	if strings.TrimSpace(description.SDP) == "" {
		return nil, domaincall.ErrInvalidInput
	}

	active, peerConn, err := m.ensurePeer(sessionID, participantRow)
	if err != nil {
		return nil, err
	}

	switch description.Type {
	case "offer":
		if err := peerConn.pc.SetRemoteDescription(webrtc.SessionDescription{
			Type: webrtc.SDPTypeOffer,
			SDP:  description.SDP,
		}); err != nil {
			return nil, fmt.Errorf("set remote description: %w", err)
		}

		answer, err := peerConn.pc.CreateAnswer(nil)
		if err != nil {
			return nil, fmt.Errorf("create answer: %w", err)
		}
		gatherComplete := webrtc.GatheringCompletePromise(peerConn.pc)
		if err := peerConn.pc.SetLocalDescription(answer); err != nil {
			return nil, fmt.Errorf("set local description: %w", err)
		}
		<-gatherComplete

		signals := m.drainSignals(peerConn)
		local := peerConn.pc.LocalDescription()
		if local == nil {
			return signals, nil
		}

		signals = append([]domaincall.RuntimeSignal{{
			TargetAccountID: participantRow.AccountID,
			TargetDeviceID:  participantRow.DeviceID,
			SessionID:       active.id,
			Description: &domaincall.SessionDescription{
				Type: "answer",
				SDP:  local.SDP,
			},
		}}, signals...)

		return signals, nil
	case "answer":
		if err := peerConn.pc.SetRemoteDescription(webrtc.SessionDescription{
			Type: webrtc.SDPTypeAnswer,
			SDP:  description.SDP,
		}); err != nil {
			return nil, fmt.Errorf("set remote answer: %w", err)
		}
		signals := m.drainSignals(peerConn)
		if shouldOfferAgain := peerConn.finishOffer(); shouldOfferAgain {
			if err := m.emitRenegotiationOffer(active.id, peerConn); err != nil {
				return nil, fmt.Errorf("emit queued renegotiation offer: %w", err)
			}
			signals = append(signals, m.drainSignals(peerConn)...)
		}
		return signals, nil
	default:
		return nil, domaincall.ErrInvalidInput
	}
}

// PublishCandidate applies one remote ICE candidate.
func (m *Manager) PublishCandidate(
	_ context.Context,
	sessionID string,
	participantRow domaincall.RuntimeParticipant,
	candidate domaincall.Candidate,
) ([]domaincall.RuntimeSignal, error) {
	sessionID = strings.TrimSpace(sessionID)
	participantRow.AccountID = strings.TrimSpace(participantRow.AccountID)
	participantRow.DeviceID = strings.TrimSpace(participantRow.DeviceID)
	candidate.Candidate = strings.TrimSpace(candidate.Candidate)
	candidate.SDPMid = strings.TrimSpace(candidate.SDPMid)
	candidate.UsernameFragment = strings.TrimSpace(candidate.UsernameFragment)

	if sessionID == "" || participantRow.AccountID == "" || participantRow.DeviceID == "" {
		return nil, domaincall.ErrInvalidInput
	}
	if candidate.Candidate == "" {
		return nil, domaincall.ErrInvalidInput
	}

	_, peerConn, err := m.ensurePeer(sessionID, participantRow)
	if err != nil {
		return nil, err
	}

	if err := peerConn.pc.AddICECandidate(webrtc.ICECandidateInit{
		Candidate:        candidate.Candidate,
		SDPMid:           stringPtr(candidate.SDPMid),
		SDPMLineIndex:    uint16Ptr(uint16(candidate.SDPMLineIndex)),
		UsernameFragment: stringPtr(candidate.UsernameFragment),
	}); err != nil {
		return nil, fmt.Errorf("add ice candidate: %w", err)
	}

	return m.drainSignals(peerConn), nil
}

// SessionStats returns live transport stats for joined peers in one runtime session.
func (m *Manager) SessionStats(_ context.Context, sessionID string) ([]domaincall.RuntimeStats, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, domaincall.ErrInvalidInput
	}

	m.mu.Lock()
	active := m.sessions[sessionID]
	if active == nil {
		m.mu.Unlock()
		return nil, domaincall.ErrNotFound
	}
	peers := make([]*peer, 0, len(active.peers))
	for _, value := range active.peers {
		if value != nil {
			peers = append(peers, value)
		}
	}
	m.mu.Unlock()

	stats := make([]domaincall.RuntimeStats, 0, len(peers))
	for _, value := range peers {
		stats = append(stats, domaincall.RuntimeStats{
			AccountID: value.accountID,
			DeviceID:  value.deviceID,
			Transport: value.snapshotStats(),
		})
	}

	return stats, nil
}

// AcknowledgeAdaptation records that one participant device applied the current server-issued adaptation.
func (m *Manager) AcknowledgeAdaptation(
	_ context.Context,
	sessionID string,
	participantRow domaincall.RuntimeParticipant,
	adaptationRevision uint64,
	appliedProfile string,
) error {
	sessionID = strings.TrimSpace(sessionID)
	participantRow.AccountID = strings.TrimSpace(participantRow.AccountID)
	participantRow.DeviceID = strings.TrimSpace(participantRow.DeviceID)
	appliedProfile = strings.TrimSpace(appliedProfile)
	if sessionID == "" || participantRow.AccountID == "" || participantRow.DeviceID == "" {
		return domaincall.ErrInvalidInput
	}
	if adaptationRevision == 0 || appliedProfile == "" {
		return domaincall.ErrInvalidInput
	}

	_, peerConn, err := m.ensurePeer(sessionID, participantRow)
	if err != nil {
		return err
	}

	peerConn.mu.Lock()
	defer peerConn.mu.Unlock()

	if adaptationRevision != peerConn.stats.AdaptationRevision {
		return domaincall.ErrConflict
	}

	peerConn.stats.AckedAdaptationRevision = adaptationRevision
	peerConn.stats.AppliedProfile = appliedProfile
	peerConn.stats.AppliedAt = time.Now().UTC()
	peerConn.stats.PendingAdaptation = false
	peerConn.stats.LastUpdatedAt = peerConn.stats.AppliedAt

	return nil
}

// LeaveSession removes one device from a running session.
func (m *Manager) LeaveSession(_ context.Context, sessionID string, accountID string, deviceID string) error {
	sessionID = strings.TrimSpace(sessionID)
	accountID = strings.TrimSpace(accountID)
	deviceID = strings.TrimSpace(deviceID)
	if sessionID == "" || accountID == "" || deviceID == "" {
		return domaincall.ErrInvalidInput
	}

	var peerConn *peer
	var affected []*peer

	m.mu.Lock()
	active, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return domaincall.ErrNotFound
	}
	delete(active.participants, participantKey(accountID, deviceID))
	peerConn = active.peers[participantKey(accountID, deviceID)]
	delete(active.peers, participantKey(accountID, deviceID))
	affected = m.removeSourceRelayTracksLocked(active, participantKey(accountID, deviceID))
	m.mu.Unlock()

	if peerConn != nil && peerConn.pc != nil {
		if err := peerConn.pc.Close(); err != nil {
			return fmt.Errorf("close participant peer: %w", err)
		}
	}
	for _, target := range affected {
		if err := m.renegotiatePeer(sessionID, target); err != nil {
			return fmt.Errorf("renegotiate relay cleanup: %w", err)
		}
	}

	return nil
}

// CloseSession tears down a running session.
func (m *Manager) CloseSession(_ context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return domaincall.ErrInvalidInput
	}

	m.mu.Lock()
	active, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return domaincall.ErrNotFound
	}
	delete(m.sessions, sessionID)
	peers := make([]*peer, 0, len(active.peers))
	for _, peerConn := range active.peers {
		peers = append(peers, peerConn)
	}
	m.mu.Unlock()

	var firstErr error
	for _, peerConn := range peers {
		if peerConn == nil || peerConn.pc == nil {
			continue
		}
		if err := peerConn.pc.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close peer connection: %w", err)
		}
	}

	return firstErr
}

func (m *Manager) ensurePeer(sessionID string, participantRow domaincall.RuntimeParticipant) (*session, *peer, error) {
	key := participantKey(participantRow.AccountID, participantRow.DeviceID)

	m.mu.Lock()
	active, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return nil, nil, domaincall.ErrNotFound
	}
	if joined, ok := active.participants[key]; !ok || joined.accountID != participantRow.AccountID {
		m.mu.Unlock()
		return nil, nil, domaincall.ErrForbidden
	}
	if existing := active.peers[key]; existing != nil {
		m.mu.Unlock()
		return active, existing, nil
	}
	m.mu.Unlock()

	created, err := m.newPeer(active, participantRow)
	if err != nil {
		return nil, nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	active, ok = m.sessions[sessionID]
	if !ok {
		_ = created.pc.Close()
		return nil, nil, domaincall.ErrNotFound
	}
	if existing := active.peers[key]; existing != nil {
		_ = created.pc.Close()
		return active, existing, nil
	}
	active.peers[key] = created

	return active, created, nil
}

func (m *Manager) newPeer(active *session, participantRow domaincall.RuntimeParticipant) (*peer, error) {
	configuration := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{},
	}
	pc, err := m.api.NewPeerConnection(configuration)
	if err != nil {
		return nil, fmt.Errorf("new peer connection: %w", err)
	}

	value := &peer{
		accountID: participantRow.AccountID,
		deviceID:  participantRow.DeviceID,
		withVideo: participantRow.WithVideo,
		pc:        pc,
		tracks:    make(map[string]*relayTrack),
	}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		json := candidate.ToJSON()
		signal := domaincall.RuntimeSignal{
			TargetAccountID: participantRow.AccountID,
			TargetDeviceID:  participantRow.DeviceID,
			SessionID:       active.id,
			IceCandidate: &domaincall.Candidate{
				Candidate:        json.Candidate,
				SDPMid:           derefString(json.SDPMid),
				UsernameFragment: derefString(json.UsernameFragment),
			},
		}
		if json.SDPMLineIndex != nil {
			signal.IceCandidate.SDPMLineIndex = uint32(*json.SDPMLineIndex)
		}
		value.enqueueSignal(signal)
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		m.emitPeerTelemetry(active.id, value, telemetryPCStateKey, state.String())
	})
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		m.emitPeerTelemetry(active.id, value, telemetryICEStateKey, state.String())
	})
	pc.OnSignalingStateChange(func(state webrtc.SignalingState) {
		m.emitPeerTelemetry(active.id, value, telemetrySignalingKey, state.String())
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		go m.handleRemoteTrack(active.id, participantKey(participantRow.AccountID, participantRow.DeviceID), track)
	})

	return value, nil
}

func (m *Manager) drainSignals(peerConn *peer) []domaincall.RuntimeSignal {
	if peerConn == nil {
		return nil
	}

	peerConn.signalMu.Lock()
	signals := peerConn.signals
	peerConn.signals = nil
	var transportSignal domaincall.RuntimeSignal
	hasTransportSignal := peerConn.hasTransportSignal
	if hasTransportSignal {
		transportSignal = peerConn.transportSignal
		peerConn.transportSignal = domaincall.RuntimeSignal{}
		peerConn.hasTransportSignal = false
	}
	peerConn.signalMu.Unlock()
	if !hasTransportSignal {
		return signals
	}
	signals = append(signals, transportSignal)
	return signals
}

func (m *Manager) emitPeerTelemetry(sessionID string, target *peer, key string, value string) {
	if target == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(key) == "" {
		return
	}

	metadata := target.updateTelemetry(key, value)
	if len(metadata) == 0 {
		return
	}

	target.enqueueSignal(domaincall.RuntimeSignal{
		TargetAccountID: target.accountID,
		TargetDeviceID:  target.deviceID,
		SessionID:       sessionID,
		Metadata:        metadata,
	})
}

func (m *Manager) handleRemoteTrack(sessionID string, sourceKey string, track *webrtc.TrackRemote) {
	if track == nil {
		return
	}

	trackKey := relayTrackKey(sourceKey, track.ID(), track.StreamID(), track.Kind().String())
	defer m.cleanupRelayTrack(sessionID, trackKey)

	targets, relays := m.prepareRelayTargets(sessionID, sourceKey, track)
	if len(targets) > 0 && track.Kind() == webrtc.RTPCodecTypeVideo {
		m.requestRelayKeyframe(sessionID, sourceKey, track)
	}
	for _, target := range targets {
		if err := m.renegotiatePeer(sessionID, target); err != nil {
			continue
		}
	}
	if len(relays) == 0 {
		consumeRemoteTrack(track)
		return
	}

	for {
		packet, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		rawPacket, err := packet.Marshal()
		if err != nil {
			continue
		}
		for _, relay := range relays {
			if relay.peer == nil || relay.track == nil {
				continue
			}
			if _, err := relay.track.Write(rawPacket); err != nil {
				if metadata := relay.peer.recordRelayWrite(0, true); len(metadata) > 0 {
					relay.peer.enqueueSignal(domaincall.RuntimeSignal{
						TargetAccountID: relay.peer.accountID,
						TargetDeviceID:  relay.peer.deviceID,
						SessionID:       sessionID,
						Metadata:        metadata,
					})
				}
				continue
			}
			if metadata := relay.peer.recordRelayWrite(uint64(len(rawPacket)), false); len(metadata) > 0 {
				relay.peer.enqueueSignal(domaincall.RuntimeSignal{
					TargetAccountID: relay.peer.accountID,
					TargetDeviceID:  relay.peer.deviceID,
					SessionID:       sessionID,
					Metadata:        metadata,
				})
			}
		}
	}
}

func (m *Manager) prepareRelayTargets(
	sessionID string,
	sourceKey string,
	track *webrtc.TrackRemote,
) ([]*peer, []relayTarget) {
	m.mu.Lock()
	defer m.mu.Unlock()

	active, ok := m.sessions[sessionID]
	if !ok {
		return nil, nil
	}

	targets := make([]*peer, 0)
	relays := make([]relayTarget, 0)
	trackKey := relayTrackKey(sourceKey, track.ID(), track.StreamID(), track.Kind().String())
	for key, target := range active.peers {
		if key == sourceKey || target == nil {
			continue
		}
		if existing := target.tracks[trackKey]; existing != nil {
			relays = append(relays, relayTarget{peer: target, track: existing.track})
			continue
		}

		localTrack, err := webrtc.NewTrackLocalStaticRTP(track.Codec().RTPCodecCapability, track.ID(), track.StreamID())
		if err != nil {
			continue
		}
		sender, err := target.pc.AddTrack(localTrack)
		if err != nil {
			continue
		}
		target.tracks[trackKey] = &relayTrack{
			sourceKey: sourceKey,
			track:     localTrack,
			sender:    sender,
		}
		relays = append(relays, relayTarget{peer: target, track: localTrack})
		targets = append(targets, target)
		go m.forwardSenderRTCP(sessionID, target.tracks[trackKey])
	}

	return targets, relays
}

func (m *Manager) cleanupRelayTrack(sessionID string, trackKey string) {
	sessionID = strings.TrimSpace(sessionID)
	trackKey = strings.TrimSpace(trackKey)
	if sessionID == "" || trackKey == "" {
		return
	}

	m.mu.Lock()
	active, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return
	}
	affected := m.removeRelayTrackLocked(active, trackKey)
	m.mu.Unlock()

	for _, target := range affected {
		if err := m.renegotiatePeer(sessionID, target); err != nil {
			continue
		}
	}
}

func (m *Manager) requestRelayKeyframe(sessionID string, sourceKey string, track *webrtc.TrackRemote) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(sourceKey) == "" || track == nil {
		return
	}

	m.mu.Lock()
	active := m.sessions[sessionID]
	if active == nil {
		m.mu.Unlock()
		return
	}
	source := active.peers[sourceKey]
	m.mu.Unlock()
	if source == nil || source.pc == nil {
		return
	}

	_ = source.pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{
		SenderSSRC: 0,
		MediaSSRC:  uint32(track.SSRC()),
	}})
}

func (m *Manager) removeSourceRelayTracksLocked(active *session, sourceKey string) []*peer {
	if active == nil || sourceKey == "" {
		return nil
	}

	prefix := sourceKey + "|"
	affected := make([]*peer, 0)
	for _, target := range active.peers {
		if target == nil || len(target.tracks) == 0 {
			continue
		}

		removed := false
		for trackKey, relay := range target.tracks {
			if !strings.HasPrefix(trackKey, prefix) {
				continue
			}
			if relay != nil && relay.sender != nil {
				_ = target.pc.RemoveTrack(relay.sender)
			}
			delete(target.tracks, trackKey)
			removed = true
		}
		if removed {
			affected = append(affected, target)
		}
	}

	return affected
}

func (m *Manager) removeRelayTrackLocked(active *session, trackKey string) []*peer {
	if active == nil || trackKey == "" {
		return nil
	}

	affected := make([]*peer, 0)
	for _, target := range active.peers {
		if target == nil {
			continue
		}
		relay := target.tracks[trackKey]
		if relay == nil {
			continue
		}
		if relay.sender != nil {
			_ = target.pc.RemoveTrack(relay.sender)
		}
		delete(target.tracks, trackKey)
		affected = append(affected, target)
	}

	return affected
}

func (m *Manager) renegotiatePeer(sessionID string, target *peer) error {
	if target == nil || target.pc == nil {
		return domaincall.ErrInvalidInput
	}
	if !target.beginOffer() {
		return nil
	}

	if err := m.emitRenegotiationOffer(sessionID, target); err != nil {
		target.abortOffer()
		return err
	}

	return nil
}

func (m *Manager) emitRenegotiationOffer(sessionID string, target *peer) error {
	offer, err := target.pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create renegotiation offer: %w", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(target.pc)
	if err := target.pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set renegotiation local description: %w", err)
	}
	<-gatherComplete

	local := target.pc.LocalDescription()
	if local == nil {
		return nil
	}
	target.enqueueSignal(domaincall.RuntimeSignal{
		TargetAccountID: target.accountID,
		TargetDeviceID:  target.deviceID,
		SessionID:       sessionID,
		Description: &domaincall.SessionDescription{
			Type: "offer",
			SDP:  local.SDP,
		},
	})

	return nil
}

func (p *peer) beginOffer() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.offering {
		p.queued = true
		return false
	}
	p.offering = true
	p.queued = false

	return true
}

func (p *peer) finishOffer() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.offering = false
	queued := p.queued
	p.queued = false
	if queued {
		p.offering = true
	}

	return queued
}

func (p *peer) abortOffer() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.offering = false
	p.queued = false
}

func (p *peer) updateTelemetry(key string, value string) map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()

	value = strings.TrimSpace(value)
	prevQuality := p.stats.Quality
	prevProfile := p.stats.RecommendedProfile
	prevReason := p.stats.RecommendationReason
	prevVideo := p.stats.VideoFallbackRecommended
	prevReconnect := p.stats.ReconnectRecommended
	changed := false
	switch key {
	case telemetryPCStateKey:
		if p.lastPC == value {
			return nil
		}
		p.lastPC = value
		changed = true
	case telemetryICEStateKey:
		if p.lastICE == value {
			return nil
		}
		p.lastICE = value
		changed = true
	case telemetrySignalingKey:
		if p.lastSIG == value {
			return nil
		}
		p.lastSIG = value
		changed = true
	default:
		return nil
	}
	if !changed {
		return nil
	}

	metadata := map[string]string{
		telemetryKindKey:      telemetryKindTransport,
		telemetryPCStateKey:   p.lastPC,
		telemetryICEStateKey:  p.lastICE,
		telemetrySignalingKey: p.lastSIG,
		telemetryQualityKey:   deriveTransportQuality(p.lastPC, p.lastICE),
	}
	p.stats.PeerConnectionState = p.lastPC
	p.stats.IceConnectionState = p.lastICE
	p.stats.SignalingState = p.lastSIG
	p.stats.Quality = metadata[telemetryQualityKey]
	p.applyRecommendationsLocked()
	p.recordQualityTransitionLocked(prevQuality)
	p.stats.LastUpdatedAt = time.Now().UTC()
	metadata[telemetryProfileKey] = p.stats.RecommendedProfile
	metadata[telemetryReasonKey] = p.stats.RecommendationReason
	metadata[telemetryVideoKey] = boolString(p.stats.VideoFallbackRecommended)
	metadata[telemetryReconnectKey] = boolString(p.stats.ReconnectRecommended)
	metadata[telemetrySuppressOutgoingVideoKey] = boolString(p.stats.SuppressOutgoingVideo)
	metadata[telemetrySuppressIncomingVideoKey] = boolString(p.stats.SuppressIncomingVideo)
	metadata[telemetrySuppressOutgoingAudioKey] = boolString(p.stats.SuppressOutgoingAudio)
	metadata[telemetrySuppressIncomingAudioKey] = boolString(p.stats.SuppressIncomingAudio)
	metadata[telemetryReconnectAttemptKey] = fmt.Sprintf("%d", p.stats.ReconnectAttempt)
	metadata[telemetryAdaptationRevisionKey] = fmt.Sprintf("%d", p.stats.AdaptationRevision)
	metadata[telemetryPendingAdaptationKey] = boolString(p.stats.PendingAdaptation)
	metadata[telemetryAckedRevisionKey] = fmt.Sprintf("%d", p.stats.AckedAdaptationRevision)
	metadata[telemetryAppliedProfileKey] = p.stats.AppliedProfile
	if !p.stats.ReconnectBackoffUntil.IsZero() {
		metadata[telemetryReconnectBackoffKey] = p.stats.ReconnectBackoffUntil.Format(time.RFC3339Nano)
	}
	if p.stats.RecommendedProfile == prevProfile &&
		p.stats.RecommendationReason == prevReason &&
		p.stats.VideoFallbackRecommended == prevVideo &&
		p.stats.ReconnectRecommended == prevReconnect {
		return metadata
	}

	return metadata
}

func (p *peer) enqueueSignal(signal domaincall.RuntimeSignal) {
	if p == nil {
		return
	}

	p.signalMu.Lock()
	defer p.signalMu.Unlock()

	if isTransportSignal(signal) {
		p.transportSignal = signal
		p.hasTransportSignal = true
		return
	}

	if len(p.signals) >= maxPendingSignals {
		p.signals = append(p.signals[1:], signal)
		return
	}

	p.signals = append(p.signals, signal)
}

func isTransportSignal(signal domaincall.RuntimeSignal) bool {
	return signal.Description == nil &&
		signal.IceCandidate == nil &&
		len(signal.Metadata) > 0 &&
		signal.Metadata[telemetryKindKey] == telemetryKindTransport
}

func (p *peer) recordRelayWrite(size uint64, failed bool) map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()

	prevQuality := p.stats.Quality
	prevProfile := p.stats.RecommendedProfile
	prevReason := p.stats.RecommendationReason
	prevVideo := p.stats.VideoFallbackRecommended
	prevReconnect := p.stats.ReconnectRecommended
	if failed {
		p.consecutiveRelayErrors++
		p.stats.RelayWriteErrors++
	} else {
		p.consecutiveRelayErrors = 0
		p.stats.RelayPackets++
		p.stats.RelayBytes += size
	}
	p.stats.RelayTracks = uint32(len(p.tracks))
	p.applyRecommendationsLocked()
	p.recordQualityTransitionLocked(prevQuality)
	p.stats.LastUpdatedAt = time.Now().UTC()
	if p.stats.RecommendedProfile == prevProfile &&
		p.stats.RecommendationReason == prevReason &&
		p.stats.VideoFallbackRecommended == prevVideo &&
		p.stats.ReconnectRecommended == prevReconnect &&
		p.stats.Quality == prevQuality {
		return nil
	}

	return map[string]string{
		telemetryKindKey:                  telemetryKindTransport,
		telemetryQualityKey:               p.stats.Quality,
		telemetryProfileKey:               p.stats.RecommendedProfile,
		telemetryReasonKey:                p.stats.RecommendationReason,
		telemetryVideoKey:                 boolString(p.stats.VideoFallbackRecommended),
		telemetryReconnectKey:             boolString(p.stats.ReconnectRecommended),
		telemetrySuppressOutgoingVideoKey: boolString(p.stats.SuppressOutgoingVideo),
		telemetrySuppressIncomingVideoKey: boolString(p.stats.SuppressIncomingVideo),
		telemetrySuppressOutgoingAudioKey: boolString(p.stats.SuppressOutgoingAudio),
		telemetrySuppressIncomingAudioKey: boolString(p.stats.SuppressIncomingAudio),
		telemetryReconnectAttemptKey:      fmt.Sprintf("%d", p.stats.ReconnectAttempt),
		telemetryAdaptationRevisionKey:    fmt.Sprintf("%d", p.stats.AdaptationRevision),
		telemetryPendingAdaptationKey:     boolString(p.stats.PendingAdaptation),
		telemetryAckedRevisionKey:         fmt.Sprintf("%d", p.stats.AckedAdaptationRevision),
		telemetryAppliedProfileKey:        p.stats.AppliedProfile,
		telemetryReconnectBackoffKey:      p.stats.ReconnectBackoffUntil.Format(time.RFC3339Nano),
	}
}

func (p *peer) recordQoSFeedback(packets []rtcp.Packet) map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(packets) == 0 {
		return nil
	}

	prevEscalation := p.stats.QoSEscalation
	prevTrend := p.stats.QoSTrend
	prevLoss := p.stats.PacketLossPct
	prevJitter := p.stats.JitterScore

	lossPct := p.stats.PacketLossPct
	jitterScore := p.stats.JitterScore
	updated := false
	for _, packet := range packets {
		switch value := packet.(type) {
		case *rtcp.ReceiverReport:
			for _, report := range value.Reports {
				reportLoss := float64(report.FractionLost) * 100 / 256
				if reportLoss > lossPct {
					lossPct = reportLoss
				}
				if report.Jitter > jitterScore {
					jitterScore = report.Jitter
				}
				updated = true
			}
		case *rtcp.TransportLayerNack:
			p.stats.QoSBadStreak++
			updated = true
		case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
			if p.stats.QoSBadStreak < 1 {
				p.stats.QoSBadStreak = 1
			}
			updated = true
		}
	}
	if !updated {
		return nil
	}

	p.stats.PacketLossPct = lossPct
	p.stats.JitterScore = jitterScore
	p.applyQoSEscalationLocked()
	p.applyRecommendationsLocked()
	p.appendQoSSampleLocked()
	p.stats.LastQoSUpdatedAt = time.Now().UTC()

	if p.stats.QoSEscalation == prevEscalation &&
		p.stats.QoSTrend == prevTrend &&
		p.stats.PacketLossPct == prevLoss &&
		p.stats.JitterScore == prevJitter {
		return nil
	}

	return map[string]string{
		telemetryKindKey:  telemetryKindTransport,
		"packet_loss_pct": fmt.Sprintf("%.2f", p.stats.PacketLossPct),
		"jitter_score":    fmt.Sprintf("%d", p.stats.JitterScore),
		"qos_escalation":  p.stats.QoSEscalation,
		"qos_trend":       p.stats.QoSTrend,
		"qos_bad_streak":  fmt.Sprintf("%d", p.stats.QoSBadStreak),
	}
}

func (p *peer) applyRecommendationsLocked() {
	prevProfile := p.stats.RecommendedProfile
	prevReason := p.stats.RecommendationReason
	prevVideo := p.stats.VideoFallbackRecommended
	prevReconnect := p.stats.ReconnectRecommended
	prevSuppressOutgoingVideo := p.stats.SuppressOutgoingVideo
	prevSuppressIncomingVideo := p.stats.SuppressIncomingVideo
	prevSuppressOutgoingAudio := p.stats.SuppressOutgoingAudio
	prevSuppressIncomingAudio := p.stats.SuppressIncomingAudio

	profile := "full"
	reason := ""
	videoFallback := false
	reconnect := false

	switch p.stats.Quality {
	case "failed":
		profile = "reconnect"
		reason = "transport_failed"
		reconnect = true
	case "degraded":
		if p.withVideo {
			profile = "audio_only"
			reason = "transport_degraded"
			videoFallback = true
		}
	}
	if !reconnect && p.withVideo && p.consecutiveRelayErrors >= 3 {
		profile = "audio_only"
		reason = "relay_write_errors"
		videoFallback = true
	}
	switch p.stats.QoSEscalation {
	case "elevated":
		if profile == "full" && p.withVideo {
			profile = "audio_only"
			reason = "qos_elevated"
			videoFallback = true
		}
	case "critical":
		profile = "reconnect"
		reason = "qos_critical"
		videoFallback = false
		reconnect = true
	}

	p.stats.RecommendedProfile = profile
	p.stats.RecommendationReason = reason
	p.stats.VideoFallbackRecommended = videoFallback
	p.stats.ReconnectRecommended = reconnect
	p.stats.SuppressOutgoingVideo = videoFallback
	p.stats.SuppressIncomingVideo = videoFallback
	p.stats.SuppressOutgoingAudio = reconnect
	p.stats.SuppressIncomingAudio = reconnect
	if p.stats.RecommendedProfile != prevProfile ||
		p.stats.RecommendationReason != prevReason ||
		p.stats.VideoFallbackRecommended != prevVideo ||
		p.stats.ReconnectRecommended != prevReconnect ||
		p.stats.SuppressOutgoingVideo != prevSuppressOutgoingVideo ||
		p.stats.SuppressIncomingVideo != prevSuppressIncomingVideo ||
		p.stats.SuppressOutgoingAudio != prevSuppressOutgoingAudio ||
		p.stats.SuppressIncomingAudio != prevSuppressIncomingAudio {
		p.stats.AdaptationRevision++
		p.stats.PendingAdaptation = true
	}
	if !reconnect {
		p.reconnectAttempts = 0
		p.stats.ReconnectAttempt = 0
		p.stats.ReconnectBackoffUntil = time.Time{}
	}
}

func (p *peer) applyQoSEscalationLocked() {
	prevEscalation := p.stats.QoSEscalation
	prevLoss := p.stats.PacketLossPct
	prevJitter := p.stats.JitterScore

	switch {
	case p.stats.PacketLossPct >= 15 || p.stats.JitterScore >= 3000 || p.stats.QoSBadStreak >= 3:
		p.stats.QoSEscalation = "critical"
	case p.stats.PacketLossPct >= 5 || p.stats.JitterScore >= 1000 || p.stats.QoSBadStreak >= 1:
		p.stats.QoSEscalation = "elevated"
	default:
		p.stats.QoSEscalation = "normal"
		p.stats.QoSBadStreak = 0
	}

	switch {
	case p.stats.QoSEscalation == prevEscalation:
		if p.stats.QoSTrend == "" {
			p.stats.QoSTrend = "stable"
		}
	case qosEscalationRank(p.stats.QoSEscalation) > qosEscalationRank(prevEscalation):
		p.stats.QoSTrend = "worsening"
	case qosEscalationRank(p.stats.QoSEscalation) < qosEscalationRank(prevEscalation):
		p.stats.QoSTrend = "recovering"
	default:
		if p.stats.PacketLossPct > prevLoss || p.stats.JitterScore > prevJitter {
			p.stats.QoSTrend = "worsening"
		} else if p.stats.PacketLossPct < prevLoss || p.stats.JitterScore < prevJitter {
			p.stats.QoSTrend = "recovering"
		} else if p.stats.QoSTrend == "" {
			p.stats.QoSTrend = "stable"
		}
	}
}

func (p *peer) appendQoSSampleLocked() {
	sample := domaincall.TransportQoSSample{
		PacketLossPct: p.stats.PacketLossPct,
		JitterScore:   p.stats.JitterScore,
		Escalation:    p.stats.QoSEscalation,
		RecordedAt:    time.Now().UTC(),
	}
	if len(p.stats.RecentQoSSamples) > 0 {
		last := p.stats.RecentQoSSamples[len(p.stats.RecentQoSSamples)-1]
		if last.PacketLossPct == sample.PacketLossPct &&
			last.JitterScore == sample.JitterScore &&
			last.Escalation == sample.Escalation {
			p.stats.RecentQoSSamples[len(p.stats.RecentQoSSamples)-1] = sample
			return
		}
	}
	p.stats.RecentQoSSamples = append(p.stats.RecentQoSSamples, sample)
	if len(p.stats.RecentQoSSamples) > maxQualitySamples {
		p.stats.RecentQoSSamples = append(
			[]domaincall.TransportQoSSample(nil),
			p.stats.RecentQoSSamples[len(p.stats.RecentQoSSamples)-maxQualitySamples:]...,
		)
	}
}

func (p *peer) recordQualityTransitionLocked(prevQuality string) {
	currentQuality := strings.TrimSpace(p.stats.Quality)
	if currentQuality == "" {
		return
	}
	if prevQuality != "" && prevQuality != currentQuality {
		now := time.Now().UTC()
		if qualityRank(currentQuality) > qualityRank(prevQuality) {
			p.stats.QualityTrend = "improving"
			p.stats.RecoveredTransitions++
		} else if qualityRank(currentQuality) < qualityRank(prevQuality) {
			p.stats.QualityTrend = "degrading"
			p.stats.DegradedTransitions++
		}
		p.stats.LastQualityChangeAt = now
		if currentQuality == "failed" {
			p.reconnectAttempts++
			p.stats.ReconnectAttempt = p.reconnectAttempts
			p.stats.ReconnectBackoffUntil = now.Add(reconnectBackoffDuration(p.reconnectAttempts))
		}
	} else if p.stats.QualityTrend == "" {
		p.stats.QualityTrend = "stable"
	}

	if prevQuality == currentQuality && p.stats.RecommendedProfile == "full" {
		p.stats.QualityTrend = "stable"
	}
	p.appendQualitySampleLocked(currentQuality)
}

func (p *peer) appendQualitySampleLocked(quality string) {
	if quality == "" {
		return
	}

	sample := domaincall.TransportQualitySample{
		Quality:            quality,
		RecommendedProfile: p.stats.RecommendedProfile,
		RecordedAt:         time.Now().UTC(),
	}
	if len(p.stats.RecentSamples) > 0 {
		last := p.stats.RecentSamples[len(p.stats.RecentSamples)-1]
		if last.Quality == sample.Quality && last.RecommendedProfile == sample.RecommendedProfile {
			p.stats.RecentSamples[len(p.stats.RecentSamples)-1] = sample
			return
		}
	}

	p.stats.RecentSamples = append(p.stats.RecentSamples, sample)
	if len(p.stats.RecentSamples) > maxQualitySamples {
		p.stats.RecentSamples = append([]domaincall.TransportQualitySample(nil), p.stats.RecentSamples[len(p.stats.RecentSamples)-maxQualitySamples:]...)
	}
}

func (p *peer) snapshotStats() domaincall.TransportStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := p.stats
	stats.RelayTracks = uint32(len(p.tracks))

	return stats
}

func boolString(value bool) string {
	if value {
		return "true"
	}

	return "false"
}

func qualityRank(value string) int {
	switch strings.TrimSpace(value) {
	case "failed":
		return 0
	case "degraded":
		return 1
	case "connecting":
		return 2
	case "connected":
		return 3
	default:
		return -1
	}
}

func qosEscalationRank(value string) int {
	switch strings.TrimSpace(value) {
	case "critical":
		return 2
	case "elevated":
		return 1
	case "normal":
		return 0
	default:
		return -1
	}
}

func reconnectBackoffDuration(attempt uint32) time.Duration {
	if attempt == 0 {
		return 0
	}

	backoff := 5 * time.Second
	for i := uint32(1); i < attempt; i++ {
		backoff *= 2
		if backoff >= 30*time.Second {
			return 30 * time.Second
		}
	}
	if backoff > 30*time.Second {
		return 30 * time.Second
	}

	return backoff
}

func deriveTransportQuality(peerState string, iceState string) string {
	switch {
	case peerState == webrtc.PeerConnectionStateFailed.String() || iceState == webrtc.ICEConnectionStateFailed.String():
		return "failed"
	case peerState == webrtc.PeerConnectionStateDisconnected.String() || iceState == webrtc.ICEConnectionStateDisconnected.String():
		return "degraded"
	case peerState == webrtc.PeerConnectionStateConnected.String() || iceState == webrtc.ICEConnectionStateConnected.String():
		return "connected"
	case peerState == webrtc.PeerConnectionStateConnecting.String() || iceState == webrtc.ICEConnectionStateChecking.String():
		return "connecting"
	default:
		return "unknown"
	}
}

func (m *Manager) newAPI() *webrtc.API {
	settingEngine := webrtc.SettingEngine{}
	if err := settingEngine.SetEphemeralUDPPortRange(uint16(m.portMin), uint16(m.portMax)); err != nil {
		panic(err)
	}
	if strings.TrimSpace(m.host) != "" {
		settingEngine.SetNAT1To1IPs([]string{m.host}, webrtc.ICECandidateTypeHost)
	}

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}
	interceptorRegistry := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		panic(err)
	}

	return webrtc.NewAPI(
		webrtc.WithSettingEngine(settingEngine),
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
	)
}

func (m *Manager) allocatePortLocked() (int, error) {
	size := m.portMax - m.portMin + 1
	if size <= 0 {
		return 0, domaincall.ErrInvalidInput
	}

	for i := 0; i < size; i++ {
		port := m.nextPort
		if port < m.portMin || port > m.portMax {
			port = m.portMin
		}
		m.nextPort = port + 1
		if m.nextPort > m.portMax {
			m.nextPort = m.portMin
		}
		if !m.portInUseLocked(port) {
			return port, nil
		}
	}

	return 0, fmt.Errorf("allocate media port: %w", domaincall.ErrConflict)
}

func (m *Manager) portInUseLocked(port int) bool {
	for _, active := range m.sessions {
		if active.port == port {
			return true
		}
	}
	return false
}

func participantKey(accountID string, deviceID string) string {
	return accountID + "|" + deviceID
}

func randomToken(size int) (string, error) {
	if size <= 0 {
		return "", domaincall.ErrInvalidInput
	}

	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(data), nil
}

func mustDTLSFingerprint() string {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	publicKey, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(publicKey)
	encoded := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(encoded)/2)
	for i := 0; i < len(encoded); i += 2 {
		parts = append(parts, encoded[i:i+2])
	}
	return "sha-256 " + strings.Join(parts, ":")
}

func consumeRemoteTrack(track *webrtc.TrackRemote) {
	if track == nil {
		return
	}

	buffer := make([]byte, 1600)
	for {
		if _, _, err := track.Read(buffer); err != nil {
			if err != io.EOF {
				return
			}
			return
		}
	}
}

func (m *Manager) forwardSenderRTCP(sessionID string, relay *relayTrack) {
	if relay == nil || relay.sender == nil {
		return
	}

	for {
		packets, _, err := relay.sender.ReadRTCP()
		if err != nil {
			return
		}
		m.writeSourceRTCP(sessionID, relay.sourceKey, packets)
	}
}

func (m *Manager) writeSourceRTCP(sessionID string, sourceKey string, packets []rtcp.Packet) {
	if sessionID == "" || sourceKey == "" || len(packets) == 0 {
		return
	}

	m.mu.Lock()
	active := m.sessions[sessionID]
	if active == nil {
		m.mu.Unlock()
		return
	}
	source := active.peers[sourceKey]
	m.mu.Unlock()
	if source == nil || source.pc == nil {
		return
	}
	if metadata := source.recordQoSFeedback(packets); len(metadata) > 0 {
		source.enqueueSignal(domaincall.RuntimeSignal{
			TargetAccountID: source.accountID,
			TargetDeviceID:  source.deviceID,
			SessionID:       sessionID,
			Metadata:        metadata,
		})
	}
	_ = source.pc.WriteRTCP(packets)
}

func relayTrackKey(sourceKey string, trackID string, streamID string, kind string) string {
	return sourceKey + "|" + trackID + "|" + streamID + "|" + kind
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func uint16Ptr(value uint16) *uint16 {
	return &value
}

var _ domaincall.Runtime = (*Manager)(nil)
