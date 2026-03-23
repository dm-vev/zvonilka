package teststore

import (
	"context"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveLoginChallenge stores or replaces a login challenge by primary key.
func (s *memoryStore) SaveLoginChallenge(_ context.Context, challenge identity.LoginChallenge) (identity.LoginChallenge, error) {
	if challenge.ID == "" {
		return identity.LoginChallenge{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.challengesByID[challenge.ID] = challenge
	return challenge, nil
}

// LoginChallengeByID resolves a login challenge by primary key.
func (s *memoryStore) LoginChallengeByID(_ context.Context, challengeID string) (identity.LoginChallenge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	challenge, ok := s.challengesByID[challengeID]
	if !ok {
		return identity.LoginChallenge{}, identity.ErrNotFound
	}

	return challenge, nil
}

// DeleteLoginChallenge removes a login challenge from the store.
func (s *memoryStore) DeleteLoginChallenge(_ context.Context, challengeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.challengesByID, challengeID)
	return nil
}
