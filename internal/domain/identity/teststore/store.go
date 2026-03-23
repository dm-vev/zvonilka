package teststore

import (
	"sync"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// NewMemoryStore builds a concurrency-safe in-memory identity store for tests.
func NewMemoryStore() identity.Store {
	return &memoryStore{
		joinRequestsByID:     make(map[string]identity.JoinRequest),
		accountsByID:         make(map[string]identity.Account),
		accountIDsByUsername: make(map[string]string),
		accountIDsByEmail:    make(map[string]string),
		accountIDsByPhone:    make(map[string]string),
		accountIDsByBotHash:  make(map[string]string),
		challengesByID:       make(map[string]identity.LoginChallenge),
		devicesByID:          make(map[string]identity.Device),
		deviceIDsByAccount:   make(map[string]map[string]struct{}),
		sessionsByID:         make(map[string]identity.Session),
		sessionIDsByAccount:  make(map[string]map[string]struct{}),
	}
}

type memoryStore struct {
	mu sync.RWMutex

	joinRequestsByID     map[string]identity.JoinRequest
	accountsByID         map[string]identity.Account
	accountIDsByUsername map[string]string
	accountIDsByEmail    map[string]string
	accountIDsByPhone    map[string]string
	accountIDsByBotHash  map[string]string
	challengesByID       map[string]identity.LoginChallenge
	devicesByID          map[string]identity.Device
	deviceIDsByAccount   map[string]map[string]struct{}
	sessionsByID         map[string]identity.Session
	sessionIDsByAccount  map[string]map[string]struct{}
}

func (s *memoryStore) accountByIndex(index map[string]string, key string) (identity.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	accountID, ok := index[key]
	if !ok {
		return identity.Account{}, identity.ErrNotFound
	}

	account, ok := s.accountsByID[accountID]
	if !ok {
		return identity.Account{}, identity.ErrNotFound
	}

	return account, nil
}

func (s *memoryStore) indexAccountLocked(account identity.Account) {
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

func (s *memoryStore) deleteAccountIndexes(account identity.Account) {
	delete(s.accountIDsByUsername, account.Username)
	delete(s.accountIDsByEmail, account.Email)
	delete(s.accountIDsByPhone, account.Phone)
	delete(s.accountIDsByBotHash, account.BotTokenHash)
}

func (s *memoryStore) deviceIDsForAccountLocked(accountID string) map[string]struct{} {
	ids, ok := s.deviceIDsByAccount[accountID]
	if !ok {
		ids = make(map[string]struct{})
		s.deviceIDsByAccount[accountID] = ids
	}
	return ids
}

func (s *memoryStore) sessionIDsForAccountLocked(accountID string) map[string]struct{} {
	ids, ok := s.sessionIDsByAccount[accountID]
	if !ok {
		ids = make(map[string]struct{})
		s.sessionIDsByAccount[accountID] = ids
	}
	return ids
}

var _ identity.Store = (*memoryStore)(nil)
