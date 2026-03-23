package teststore

import (
	"context"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveAccount inserts or replaces an account while keeping the uniqueness indexes consistent.
//
// The method rejects conflicting usernames, emails, phones, and bot token hashes atomically
// under the store lock so the service can rely on a single write-side conflict check.
func (s *memoryStore) SaveAccount(_ context.Context, account identity.Account) (identity.Account, error) {
	if account.ID == "" {
		return identity.Account{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.hasAccountConflictLocked(account.ID, account.Username, account.Email, account.Phone, account.BotTokenHash) {
		return identity.Account{}, identity.ErrConflict
	}

	if previous, ok := s.accountsByID[account.ID]; ok {
		s.deleteAccountIndexes(previous)
	}

	storedAccount := cloneAccount(account)
	s.accountsByID[storedAccount.ID] = storedAccount
	s.indexAccountLocked(storedAccount)

	return cloneAccount(storedAccount), nil
}

// DeleteAccount removes an account and all of its secondary indexes.
func (s *memoryStore) DeleteAccount(_ context.Context, accountID string) error {
	if accountID == "" {
		return identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	account, ok := s.accountsByID[accountID]
	if !ok {
		return identity.ErrNotFound
	}

	s.deleteAccountLocked(account)
	return nil
}

// AccountByID resolves an account by its primary key.
func (s *memoryStore) AccountByID(_ context.Context, accountID string) (identity.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	account, ok := s.accountsByID[accountID]
	if !ok {
		return identity.Account{}, identity.ErrNotFound
	}

	return cloneAccount(account), nil
}

// AccountByUsername resolves an account by its normalized username.
func (s *memoryStore) AccountByUsername(_ context.Context, username string) (identity.Account, error) {
	return s.accountByIndex(s.accountIDsByUsername, username)
}

// AccountByEmail resolves an account by its normalized email.
func (s *memoryStore) AccountByEmail(_ context.Context, email string) (identity.Account, error) {
	return s.accountByIndex(s.accountIDsByEmail, email)
}

// AccountByPhone resolves an account by its normalized phone number.
func (s *memoryStore) AccountByPhone(_ context.Context, phone string) (identity.Account, error) {
	return s.accountByIndex(s.accountIDsByPhone, phone)
}

// AccountByBotTokenHash resolves a bot account by its token hash.
func (s *memoryStore) AccountByBotTokenHash(_ context.Context, tokenHash string) (identity.Account, error) {
	return s.accountByIndex(s.accountIDsByBotHash, tokenHash)
}

// deleteAccountLocked removes an account and all records that point to it.
func (s *memoryStore) deleteAccountLocked(account identity.Account) {
	delete(s.accountsByID, account.ID)
	s.deleteAccountIndexes(account)

	for deviceID, device := range s.devicesByID {
		if device.AccountID != account.ID {
			continue
		}
		delete(s.devicesByID, deviceID)
		s.removeDeviceIndexLocked(account.ID, deviceID)
	}

	for sessionID, session := range s.sessionsByID {
		if session.AccountID != account.ID {
			continue
		}
		delete(s.sessionsByID, sessionID)
		s.removeSessionIndexLocked(account.ID, sessionID)
	}

	for challengeID, challenge := range s.challengesByID {
		if challenge.AccountID != account.ID {
			continue
		}
		delete(s.challengesByID, challengeID)
	}
}

// hasAccountConflictLocked reports whether any uniqueness index already points at another account.
func (s *memoryStore) hasAccountConflictLocked(
	accountID string,
	username string,
	email string,
	phone string,
	botTokenHash string,
) bool {
	if username != "" {
		if otherID, ok := s.accountIDsByUsername[username]; ok && otherID != accountID {
			return true
		}
	}

	if email != "" {
		if otherID, ok := s.accountIDsByEmail[email]; ok && otherID != accountID {
			return true
		}
	}

	if phone != "" {
		if otherID, ok := s.accountIDsByPhone[phone]; ok && otherID != accountID {
			return true
		}
	}

	if botTokenHash != "" {
		if otherID, ok := s.accountIDsByBotHash[botTokenHash]; ok && otherID != accountID {
			return true
		}
	}

	return false
}
