package identity

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// NewMemoryStore builds a concurrency-safe in-memory store.
func NewMemoryStore() Store {
	return &memoryStore{
		joinRequestsByID:     make(map[string]JoinRequest),
		accountsByID:         make(map[string]Account),
		accountIDsByUsername: make(map[string]string),
		accountIDsByEmail:    make(map[string]string),
		accountIDsByPhone:    make(map[string]string),
		accountIDsByBotHash:  make(map[string]string),
		challengesByID:       make(map[string]LoginChallenge),
		devicesByID:          make(map[string]Device),
		deviceIDsByAccount:   make(map[string]map[string]struct{}),
		sessionsByID:         make(map[string]Session),
		sessionIDsByAccount:  make(map[string]map[string]struct{}),
	}
}

type memoryStore struct {
	mu sync.RWMutex

	joinRequestsByID     map[string]JoinRequest
	accountsByID         map[string]Account
	accountIDsByUsername map[string]string
	accountIDsByEmail    map[string]string
	accountIDsByPhone    map[string]string
	accountIDsByBotHash  map[string]string
	challengesByID       map[string]LoginChallenge
	devicesByID          map[string]Device
	deviceIDsByAccount   map[string]map[string]struct{}
	sessionsByID         map[string]Session
	sessionIDsByAccount  map[string]map[string]struct{}
}

func (s *memoryStore) SaveJoinRequest(_ context.Context, joinRequest JoinRequest) (JoinRequest, error) {
	if joinRequest.ID == "" {
		return JoinRequest{}, ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.joinRequestsByID[joinRequest.ID] = joinRequest
	return joinRequest, nil
}

func (s *memoryStore) JoinRequestByID(_ context.Context, joinRequestID string) (JoinRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	joinRequest, ok := s.joinRequestsByID[joinRequestID]
	if !ok {
		return JoinRequest{}, ErrNotFound
	}

	return joinRequest, nil
}

func (s *memoryStore) JoinRequestsByStatus(_ context.Context, status JoinRequestStatus) ([]JoinRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	joinRequests := make([]JoinRequest, 0)
	for _, joinRequest := range s.joinRequestsByID {
		if joinRequest.Status == status {
			joinRequests = append(joinRequests, joinRequest)
		}
	}

	sort.Slice(joinRequests, func(i, j int) bool {
		return joinRequests[i].RequestedAt.Before(joinRequests[j].RequestedAt)
	})

	return joinRequests, nil
}

func (s *memoryStore) SaveAccount(_ context.Context, account Account) (Account, error) {
	if account.ID == "" {
		return Account{}, ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if previous, ok := s.accountsByID[account.ID]; ok {
		s.deleteAccountIndexes(previous)
	}

	s.accountsByID[account.ID] = account
	s.indexAccountLocked(account)

	return account, nil
}

func (s *memoryStore) AccountByID(_ context.Context, accountID string) (Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	account, ok := s.accountsByID[accountID]
	if !ok {
		return Account{}, ErrNotFound
	}

	return account, nil
}

func (s *memoryStore) AccountByUsername(_ context.Context, username string) (Account, error) {
	return s.accountByIndex(s.accountIDsByUsername, username)
}

func (s *memoryStore) AccountByEmail(_ context.Context, email string) (Account, error) {
	return s.accountByIndex(s.accountIDsByEmail, email)
}

func (s *memoryStore) AccountByPhone(_ context.Context, phone string) (Account, error) {
	return s.accountByIndex(s.accountIDsByPhone, phone)
}

func (s *memoryStore) AccountByBotTokenHash(_ context.Context, tokenHash string) (Account, error) {
	return s.accountByIndex(s.accountIDsByBotHash, tokenHash)
}

func (s *memoryStore) accountByIndex(index map[string]string, key string) (Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	accountID, ok := index[key]
	if !ok {
		return Account{}, ErrNotFound
	}

	account, ok := s.accountsByID[accountID]
	if !ok {
		return Account{}, ErrNotFound
	}

	return account, nil
}

func (s *memoryStore) indexAccountLocked(account Account) {
	if account.Username != "" {
		s.accountIDsByUsername[account.Username] = account.ID
	}
	if account.Email != "" {
		s.accountIDsByEmail[account.Email] = account.ID
	}
	if account.Phone != "" {
		s.accountIDsByPhone[account.Phone] = account.ID
	}
	if account.BotTokenHash != "" {
		s.accountIDsByBotHash[account.BotTokenHash] = account.ID
	}
}

func (s *memoryStore) deleteAccountIndexes(account Account) {
	delete(s.accountIDsByUsername, account.Username)
	delete(s.accountIDsByEmail, account.Email)
	delete(s.accountIDsByPhone, account.Phone)
	delete(s.accountIDsByBotHash, account.BotTokenHash)
}

func (s *memoryStore) SaveLoginChallenge(_ context.Context, challenge LoginChallenge) (LoginChallenge, error) {
	if challenge.ID == "" {
		return LoginChallenge{}, ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.challengesByID[challenge.ID] = challenge
	return challenge, nil
}

func (s *memoryStore) LoginChallengeByID(_ context.Context, challengeID string) (LoginChallenge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	challenge, ok := s.challengesByID[challengeID]
	if !ok {
		return LoginChallenge{}, ErrNotFound
	}

	return challenge, nil
}

func (s *memoryStore) DeleteLoginChallenge(_ context.Context, challengeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.challengesByID, challengeID)
	return nil
}

func (s *memoryStore) SaveDevice(_ context.Context, device Device) (Device, error) {
	if device.ID == "" {
		return Device{}, ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.devicesByID[device.ID] = device
	if _, ok := s.deviceIDsByAccount[device.AccountID]; !ok {
		s.deviceIDsByAccount[device.AccountID] = make(map[string]struct{})
	}
	s.deviceIDsByAccount[device.AccountID][device.ID] = struct{}{}

	return device, nil
}

func (s *memoryStore) DeviceByID(_ context.Context, deviceID string) (Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	device, ok := s.devicesByID[deviceID]
	if !ok {
		return Device{}, ErrNotFound
	}

	return device, nil
}

func (s *memoryStore) DevicesByAccountID(_ context.Context, accountID string) ([]Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	deviceIDs, ok := s.deviceIDsByAccount[accountID]
	if !ok {
		return []Device{}, nil
	}

	devices := make([]Device, 0, len(deviceIDs))
	for deviceID := range deviceIDs {
		device, ok := s.devicesByID[deviceID]
		if !ok {
			continue
		}
		devices = append(devices, device)
	}

	return devices, nil
}

func (s *memoryStore) SaveSession(_ context.Context, session Session) (Session, error) {
	if session.ID == "" {
		return Session{}, ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessionsByID[session.ID] = session
	if _, ok := s.sessionIDsByAccount[session.AccountID]; !ok {
		s.sessionIDsByAccount[session.AccountID] = make(map[string]struct{})
	}
	s.sessionIDsByAccount[session.AccountID][session.ID] = struct{}{}

	return session, nil
}

func (s *memoryStore) SessionByID(_ context.Context, sessionID string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessionsByID[sessionID]
	if !ok {
		return Session{}, ErrNotFound
	}

	return session, nil
}

func (s *memoryStore) SessionsByAccountID(_ context.Context, accountID string) ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionIDs, ok := s.sessionIDsByAccount[accountID]
	if !ok {
		return []Session{}, nil
	}

	sessions := make([]Session, 0, len(sessionIDs))
	for sessionID := range sessionIDs {
		session, ok := s.sessionsByID[sessionID]
		if !ok {
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (s *memoryStore) UpdateSession(_ context.Context, session Session) (Session, error) {
	if session.ID == "" {
		return Session{}, ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessionsByID[session.ID]; !ok {
		return Session{}, ErrNotFound
	}

	s.sessionsByID[session.ID] = session
	if _, ok := s.sessionIDsByAccount[session.AccountID]; !ok {
		return Session{}, fmt.Errorf("session account index missing: %w", ErrNotFound)
	}
	s.sessionIDsByAccount[session.AccountID][session.ID] = struct{}{}

	return session, nil
}
