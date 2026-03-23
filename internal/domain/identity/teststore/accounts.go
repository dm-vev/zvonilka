package teststore

import (
	"context"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

func (s *memoryStore) SaveAccount(_ context.Context, account identity.Account) (identity.Account, error) {
	if account.ID == "" {
		return identity.Account{}, identity.ErrInvalidInput
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

func (s *memoryStore) AccountByID(_ context.Context, accountID string) (identity.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	account, ok := s.accountsByID[accountID]
	if !ok {
		return identity.Account{}, identity.ErrNotFound
	}

	return account, nil
}

func (s *memoryStore) AccountByUsername(_ context.Context, username string) (identity.Account, error) {
	return s.accountByIndex(s.accountIDsByUsername, username)
}

func (s *memoryStore) AccountByEmail(_ context.Context, email string) (identity.Account, error) {
	return s.accountByIndex(s.accountIDsByEmail, email)
}

func (s *memoryStore) AccountByPhone(_ context.Context, phone string) (identity.Account, error) {
	return s.accountByIndex(s.accountIDsByPhone, phone)
}

func (s *memoryStore) AccountByBotTokenHash(_ context.Context, tokenHash string) (identity.Account, error) {
	return s.accountByIndex(s.accountIDsByBotHash, tokenHash)
}
