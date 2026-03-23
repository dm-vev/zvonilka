package teststore

import (
	"sync"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// NewMemoryStore builds a concurrency-safe in-memory identity store for tests.
//
// The implementation keeps explicit secondary indexes so the service can exercise
// the same uniqueness and lookup paths it will use against a real database later.
func NewMemoryStore() identity.Store {
	return &memoryStore{
		joinRequestsByID:         make(map[string]identity.JoinRequest),
		joinRequestIDsByUsername: make(map[string]string),
		joinRequestIDsByEmail:    make(map[string]string),
		joinRequestIDsByPhone:    make(map[string]string),
		accountsByID:             make(map[string]identity.Account),
		accountIDsByUsername:     make(map[string]string),
		accountIDsByEmail:        make(map[string]string),
		accountIDsByPhone:        make(map[string]string),
		accountIDsByBotHash:      make(map[string]string),
		challengesByID:           make(map[string]identity.LoginChallenge),
		devicesByID:              make(map[string]identity.Device),
		deviceIDsByAccount:       make(map[string]map[string]struct{}),
		sessionsByID:             make(map[string]identity.Session),
		sessionIDsByAccount:      make(map[string]map[string]struct{}),
	}
}

// memoryStore is a test-only identity store with explicit indexes for lookups and conflicts.
//
// All state lives behind a single RWMutex so the tests can probe concurrent write paths
// without relying on a real database.
type memoryStore struct {
	mu sync.RWMutex

	joinRequestsByID         map[string]identity.JoinRequest
	joinRequestIDsByUsername map[string]string
	joinRequestIDsByEmail    map[string]string
	joinRequestIDsByPhone    map[string]string
	accountsByID             map[string]identity.Account
	accountIDsByUsername     map[string]string
	accountIDsByEmail        map[string]string
	accountIDsByPhone        map[string]string
	accountIDsByBotHash      map[string]string
	challengesByID           map[string]identity.LoginChallenge
	devicesByID              map[string]identity.Device
	deviceIDsByAccount       map[string]map[string]struct{}
	sessionsByID             map[string]identity.Session
	sessionIDsByAccount      map[string]map[string]struct{}
}

// accountByIndex resolves an account through one of the secondary indexes.
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

	return cloneAccount(account), nil
}

// indexAccountLocked updates all secondary account indexes for a freshly saved account.
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

// deleteAccountIndexes removes the secondary indexes that point at an account.
func (s *memoryStore) deleteAccountIndexes(account identity.Account) {
	delete(s.accountIDsByUsername, account.Username)
	delete(s.accountIDsByEmail, account.Email)
	delete(s.accountIDsByPhone, account.Phone)
	delete(s.accountIDsByBotHash, account.BotTokenHash)
}

// indexJoinRequestLocked updates all secondary join-request indexes for a pending request.
func (s *memoryStore) indexJoinRequestLocked(joinRequest identity.JoinRequest) {
	if joinRequest.Username != "" {
		s.joinRequestIDsByUsername[joinRequest.Username] = joinRequest.ID
	}
	if joinRequest.Email != "" {
		s.joinRequestIDsByEmail[joinRequest.Email] = joinRequest.ID
	}
	if joinRequest.Phone != "" {
		s.joinRequestIDsByPhone[joinRequest.Phone] = joinRequest.ID
	}
}

// deleteJoinRequestIndexes removes the secondary indexes that point at a join request.
func (s *memoryStore) deleteJoinRequestIndexes(joinRequest identity.JoinRequest) {
	delete(s.joinRequestIDsByUsername, joinRequest.Username)
	delete(s.joinRequestIDsByEmail, joinRequest.Email)
	delete(s.joinRequestIDsByPhone, joinRequest.Phone)
}

// hasJoinRequestConflictLocked reports whether any other pending join request already uses the same identifiers.
//
// Stale pending requests are converted to expired rows before the conflict is reported,
// which lets a fresh submission reuse the same identifiers once the TTL has elapsed.
func (s *memoryStore) hasJoinRequestConflictLocked(
	requestedAt time.Time,
	joinRequestID string,
	username string,
	email string,
	phone string,
) bool {
	if username != "" {
		if s.joinRequestConflictLocked(requestedAt, joinRequestID, username, s.joinRequestIDsByUsername) {
			return true
		}
	}

	if email != "" {
		if s.joinRequestConflictLocked(requestedAt, joinRequestID, email, s.joinRequestIDsByEmail) {
			return true
		}
	}

	if phone != "" {
		if s.joinRequestConflictLocked(requestedAt, joinRequestID, phone, s.joinRequestIDsByPhone) {
			return true
		}
	}

	return false
}

// joinRequestConflictLocked checks one secondary index for a conflicting pending request.
func (s *memoryStore) joinRequestConflictLocked(
	requestedAt time.Time,
	joinRequestID string,
	identifier string,
	index map[string]string,
) bool {
	otherID, ok := index[identifier]
	if !ok || otherID == joinRequestID {
		return false
	}

	existing, ok := s.joinRequestsByID[otherID]
	if !ok {
		delete(index, identifier)
		return false
	}

	if existing.Status != identity.JoinRequestStatusPending || !existing.ReviewedAt.IsZero() {
		delete(index, identifier)
		return false
	}

	if requestedAt.Before(existing.ExpiresAt) {
		return true
	}

	s.expireJoinRequestLocked(existing, requestedAt)
	return false
}

// expireJoinRequestLocked marks a stale pending request as expired and removes its indexes.
func (s *memoryStore) expireJoinRequestLocked(joinRequest identity.JoinRequest, expiredAt time.Time) {
	joinRequest.Status = identity.JoinRequestStatusExpired
	joinRequest.ReviewedAt = expiredAt
	joinRequest.ReviewedBy = ""
	joinRequest.DecisionReason = "join request expired"

	s.joinRequestsByID[joinRequest.ID] = joinRequest
	s.deleteJoinRequestIndexes(joinRequest)
}

// deviceIDsForAccountLocked returns the device-ID set for an account, creating it on demand.
func (s *memoryStore) deviceIDsForAccountLocked(accountID string) map[string]struct{} {
	ids, ok := s.deviceIDsByAccount[accountID]
	if !ok || ids == nil {
		ids = make(map[string]struct{})
		s.deviceIDsByAccount[accountID] = ids
	}
	return ids
}

// sessionIDsForAccountLocked returns the session-ID set for an account, creating it on demand.
func (s *memoryStore) sessionIDsForAccountLocked(accountID string) map[string]struct{} {
	ids, ok := s.sessionIDsByAccount[accountID]
	if !ok || ids == nil {
		ids = make(map[string]struct{})
		s.sessionIDsByAccount[accountID] = ids
	}
	return ids
}

var _ identity.Store = (*memoryStore)(nil)
