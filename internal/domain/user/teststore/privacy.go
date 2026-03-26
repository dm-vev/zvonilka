package teststore

import (
	"context"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

func (s *memoryStore) SavePrivacy(_ context.Context, privacy domainuser.Privacy) (domainuser.Privacy, error) {
	if privacy.AccountID == "" {
		return domainuser.Privacy{}, domainuser.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.privacies[privacy.AccountID] = privacy
	return privacy, nil
}

func (s *memoryStore) PrivacyByAccountID(_ context.Context, accountID string) (domainuser.Privacy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	privacy, ok := s.privacies[accountID]
	if !ok {
		return domainuser.Privacy{}, domainuser.ErrNotFound
	}

	return privacy, nil
}
