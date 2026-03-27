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
	"strings"
	"sync"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

// Manager keeps RTC sessions inside the server process.
type Manager struct {
	mu       sync.Mutex
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
}

type participant struct {
	accountID string
	deviceID  string
	withVideo bool
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

// LeaveSession removes one device from a running session.
func (m *Manager) LeaveSession(_ context.Context, sessionID string, accountID string, deviceID string) error {
	sessionID = strings.TrimSpace(sessionID)
	accountID = strings.TrimSpace(accountID)
	deviceID = strings.TrimSpace(deviceID)
	if sessionID == "" || accountID == "" || deviceID == "" {
		return domaincall.ErrInvalidInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	active, ok := m.sessions[sessionID]
	if !ok {
		return domaincall.ErrNotFound
	}
	delete(active.participants, participantKey(accountID, deviceID))
	return nil
}

// CloseSession tears down a running session.
func (m *Manager) CloseSession(_ context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return domaincall.ErrInvalidInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[sessionID]; !ok {
		return domaincall.ErrNotFound
	}
	delete(m.sessions, sessionID)
	return nil
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

var _ domaincall.Runtime = (*Manager)(nil)
