package teststore

import (
	"context"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/storage"
)

// WithinTx executes the callback against an isolated snapshot and commits it on success.
//
// The parent store stays locked for the whole transaction so the in-memory fake behaves
// like a serializable transactional backend in tests.
func (s *memoryStore) WithinTx(ctx context.Context, fn func(identity.Store) error) error {
	if ctx == nil {
		return identity.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if fn == nil {
		return identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx := s.cloneLocked()
	err := fn(&tx)
	if err == nil {
		s.replaceLocked(&tx)
		return nil
	}
	if storage.IsCommit(err) {
		s.replaceLocked(&tx)
		return storage.UnwrapCommit(err)
	}

	return err
}

// cloneLocked creates a deep copy of the store state for transactional work.
func (s *memoryStore) cloneLocked() memoryStore {
	return memoryStore{
		joinRequestsByID:         cloneJoinRequests(s.joinRequestsByID),
		joinRequestIDsByUsername: cloneStringMap(s.joinRequestIDsByUsername),
		joinRequestIDsByEmail:    cloneStringMap(s.joinRequestIDsByEmail),
		joinRequestIDsByPhone:    cloneStringMap(s.joinRequestIDsByPhone),
		accountsByID:             cloneAccounts(s.accountsByID),
		accountIDsByUsername:     cloneStringMap(s.accountIDsByUsername),
		accountIDsByEmail:        cloneStringMap(s.accountIDsByEmail),
		accountIDsByPhone:        cloneStringMap(s.accountIDsByPhone),
		accountIDsByBotHash:      cloneStringMap(s.accountIDsByBotHash),
		challengesByID:           cloneChallenges(s.challengesByID),
		devicesByID:              cloneDevices(s.devicesByID),
		deviceIDsByAccount:       cloneStringSetMap(s.deviceIDsByAccount),
		sessionsByID:             cloneSessions(s.sessionsByID),
		sessionIDsByAccount:      cloneStringSetMap(s.sessionIDsByAccount),
		credentialsByHash:        cloneCredentials(s.credentialsByHash),
		credentialHashesByKey:    cloneStringMap(s.credentialHashesByKey),
	}
}

// replaceLocked replaces the committed store state with a transactional snapshot.
func (s *memoryStore) replaceLocked(tx *memoryStore) {
	s.joinRequestsByID = cloneJoinRequests(tx.joinRequestsByID)
	s.joinRequestIDsByUsername = cloneStringMap(tx.joinRequestIDsByUsername)
	s.joinRequestIDsByEmail = cloneStringMap(tx.joinRequestIDsByEmail)
	s.joinRequestIDsByPhone = cloneStringMap(tx.joinRequestIDsByPhone)
	s.accountsByID = cloneAccounts(tx.accountsByID)
	s.accountIDsByUsername = cloneStringMap(tx.accountIDsByUsername)
	s.accountIDsByEmail = cloneStringMap(tx.accountIDsByEmail)
	s.accountIDsByPhone = cloneStringMap(tx.accountIDsByPhone)
	s.accountIDsByBotHash = cloneStringMap(tx.accountIDsByBotHash)
	s.challengesByID = cloneChallenges(tx.challengesByID)
	s.devicesByID = cloneDevices(tx.devicesByID)
	s.deviceIDsByAccount = cloneStringSetMap(tx.deviceIDsByAccount)
	s.sessionsByID = cloneSessions(tx.sessionsByID)
	s.sessionIDsByAccount = cloneStringSetMap(tx.sessionIDsByAccount)
	s.credentialsByHash = cloneCredentials(tx.credentialsByHash)
	s.credentialHashesByKey = cloneStringMap(tx.credentialHashesByKey)
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return make(map[string]string)
	}

	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}

	return dst
}

func cloneStringSetMap(src map[string]map[string]struct{}) map[string]map[string]struct{} {
	if len(src) == 0 {
		return make(map[string]map[string]struct{})
	}

	dst := make(map[string]map[string]struct{}, len(src))
	for key, set := range src {
		clonedSet := make(map[string]struct{}, len(set))
		for value := range set {
			clonedSet[value] = struct{}{}
		}
		dst[key] = clonedSet
	}

	return dst
}

func cloneAccounts(src map[string]identity.Account) map[string]identity.Account {
	if len(src) == 0 {
		return make(map[string]identity.Account)
	}

	dst := make(map[string]identity.Account, len(src))
	for key, account := range src {
		dst[key] = cloneAccount(account)
	}

	return dst
}

func cloneJoinRequests(src map[string]identity.JoinRequest) map[string]identity.JoinRequest {
	if len(src) == 0 {
		return make(map[string]identity.JoinRequest)
	}

	dst := make(map[string]identity.JoinRequest, len(src))
	for key, joinRequest := range src {
		dst[key] = joinRequest
	}

	return dst
}

func cloneChallenges(src map[string]identity.LoginChallenge) map[string]identity.LoginChallenge {
	if len(src) == 0 {
		return make(map[string]identity.LoginChallenge)
	}

	dst := make(map[string]identity.LoginChallenge, len(src))
	for key, challenge := range src {
		dst[key] = cloneChallenge(challenge)
	}

	return dst
}

func cloneDevices(src map[string]identity.Device) map[string]identity.Device {
	if len(src) == 0 {
		return make(map[string]identity.Device)
	}

	dst := make(map[string]identity.Device, len(src))
	for key, device := range src {
		dst[key] = device
	}

	return dst
}

func cloneSessions(src map[string]identity.Session) map[string]identity.Session {
	if len(src) == 0 {
		return make(map[string]identity.Session)
	}

	dst := make(map[string]identity.Session, len(src))
	for key, session := range src {
		dst[key] = session
	}

	return dst
}

func cloneCredentials(src map[string]identity.SessionCredential) map[string]identity.SessionCredential {
	if len(src) == 0 {
		return make(map[string]identity.SessionCredential)
	}

	dst := make(map[string]identity.SessionCredential, len(src))
	for key, credential := range src {
		dst[key] = credential
	}

	return dst
}

func cloneAccount(account identity.Account) identity.Account {
	account.Roles = append([]identity.Role(nil), account.Roles...)
	return account
}

func cloneChallenge(challenge identity.LoginChallenge) identity.LoginChallenge {
	challenge.Targets = append([]identity.LoginTarget(nil), challenge.Targets...)
	return challenge
}

var _ identity.Store = (*memoryStore)(nil)
