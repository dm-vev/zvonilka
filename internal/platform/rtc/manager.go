package rtc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
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
	now      func() time.Time
	sessions map[string]*session
}

type session struct {
	id           string
	callID       string
	conversation string
	participants map[string]participant
}

type participant struct {
	accountID string
	deviceID  string
	withVideo bool
}

// NewManager constructs an in-process RTC session manager.
func NewManager(endpoint string, tokenTTL time.Duration) *Manager {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = "webrtc://gateway/calls"
	}
	if tokenTTL <= 0 {
		tokenTTL = 15 * time.Minute
	}

	return &Manager{
		endpoint: endpoint,
		tokenTTL: tokenTTL,
		now:      func() time.Time { return time.Now().UTC() },
		sessions: make(map[string]*session),
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
		}, nil
	}

	m.sessions[sessionID] = &session{
		id:           sessionID,
		callID:       callRow.ID,
		conversation: callRow.ConversationID,
		participants: make(map[string]participant),
	}

	return domaincall.RuntimeSession{
		SessionID:       sessionID,
		RuntimeEndpoint: m.endpoint,
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

var _ domaincall.Runtime = (*Manager)(nil)
