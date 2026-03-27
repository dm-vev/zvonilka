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
	accountID string
	deviceID  string
	withVideo bool
	pc        *webrtc.PeerConnection
	signals   chan domaincall.RuntimeSignal
	tracks    map[string]*webrtc.TrackLocalStaticRTP
}

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
		return m.drainSignals(peerConn), nil
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

// LeaveSession removes one device from a running session.
func (m *Manager) LeaveSession(_ context.Context, sessionID string, accountID string, deviceID string) error {
	sessionID = strings.TrimSpace(sessionID)
	accountID = strings.TrimSpace(accountID)
	deviceID = strings.TrimSpace(deviceID)
	if sessionID == "" || accountID == "" || deviceID == "" {
		return domaincall.ErrInvalidInput
	}

	var peerConn *peer

	m.mu.Lock()
	active, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return domaincall.ErrNotFound
	}
	delete(active.participants, participantKey(accountID, deviceID))
	peerConn = active.peers[participantKey(accountID, deviceID)]
	delete(active.peers, participantKey(accountID, deviceID))
	m.mu.Unlock()

	if peerConn != nil && peerConn.pc != nil {
		if err := peerConn.pc.Close(); err != nil {
			return fmt.Errorf("close participant peer: %w", err)
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
		signals:   make(chan domaincall.RuntimeSignal, 32),
		tracks:    make(map[string]*webrtc.TrackLocalStaticRTP),
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
		select {
		case value.signals <- signal:
		default:
		}
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

	signals := make([]domaincall.RuntimeSignal, 0)
	for {
		select {
		case signal := <-peerConn.signals:
			signals = append(signals, signal)
		default:
			return signals
		}
	}
}

func (m *Manager) handleRemoteTrack(sessionID string, sourceKey string, track *webrtc.TrackRemote) {
	if track == nil {
		return
	}

	targets, relays := m.prepareRelayTargets(sessionID, sourceKey, track)
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
		for _, relay := range relays {
			if relay == nil {
				continue
			}
			_ = relay.WriteRTP(packet.Clone())
		}
	}
}

func (m *Manager) prepareRelayTargets(
	sessionID string,
	sourceKey string,
	track *webrtc.TrackRemote,
) ([]*peer, []*webrtc.TrackLocalStaticRTP) {
	m.mu.Lock()
	defer m.mu.Unlock()

	active, ok := m.sessions[sessionID]
	if !ok {
		return nil, nil
	}

	targets := make([]*peer, 0)
	relays := make([]*webrtc.TrackLocalStaticRTP, 0)
	trackKey := relayTrackKey(sourceKey, track.ID(), track.StreamID(), track.Kind().String())
	for key, target := range active.peers {
		if key == sourceKey || target == nil {
			continue
		}
		if existing := target.tracks[trackKey]; existing != nil {
			relays = append(relays, existing)
			continue
		}

		localTrack, err := webrtc.NewTrackLocalStaticRTP(track.Codec().RTPCodecCapability, track.ID(), track.StreamID())
		if err != nil {
			continue
		}
		if _, err := target.pc.AddTrack(localTrack); err != nil {
			continue
		}
		target.tracks[trackKey] = localTrack
		relays = append(relays, localTrack)
		targets = append(targets, target)
	}

	return targets, relays
}

func (m *Manager) renegotiatePeer(sessionID string, target *peer) error {
	if target == nil || target.pc == nil {
		return domaincall.ErrInvalidInput
	}

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
	select {
	case target.signals <- domaincall.RuntimeSignal{
		TargetAccountID: target.accountID,
		TargetDeviceID:  target.deviceID,
		SessionID:       sessionID,
		Description: &domaincall.SessionDescription{
			Type: "offer",
			SDP:  local.SDP,
		},
	}:
	default:
	}

	return nil
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
