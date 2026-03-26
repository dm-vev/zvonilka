package teststore

import (
	"context"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveSessionCredential stores one hashed bearer token and replaces the previous token for the same session/kind.
func (s *memoryStore) SaveSessionCredential(
	_ context.Context,
	credential identity.SessionCredential,
) (identity.SessionCredential, error) {
	if credential.SessionID == "" || credential.AccountID == "" || credential.DeviceID == "" || credential.TokenHash == "" {
		return identity.SessionCredential{}, identity.ErrInvalidInput
	}
	if credential.Kind != identity.SessionCredentialKindAccess && credential.Kind != identity.SessionCredentialKindRefresh {
		return identity.SessionCredential{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessionsByID[credential.SessionID]
	if !ok {
		return identity.SessionCredential{}, identity.ErrNotFound
	}
	if session.AccountID != credential.AccountID || session.DeviceID != credential.DeviceID {
		return identity.SessionCredential{}, identity.ErrConflict
	}

	device, ok := s.devicesByID[credential.DeviceID]
	if !ok {
		return identity.SessionCredential{}, identity.ErrNotFound
	}
	if device.AccountID != credential.AccountID || device.SessionID != credential.SessionID {
		return identity.SessionCredential{}, identity.ErrConflict
	}

	key := credentialKey(credential.SessionID, credential.Kind)
	if previousHash, ok := s.credentialHashesByKey[key]; ok && previousHash != credential.TokenHash {
		delete(s.credentialsByHash, previousHash)
	}

	saved := credential
	s.credentialsByHash[saved.TokenHash] = saved
	s.credentialHashesByKey[key] = saved.TokenHash

	return saved, nil
}

// SessionCredentialByTokenHash resolves a stored hashed bearer token.
func (s *memoryStore) SessionCredentialByTokenHash(
	_ context.Context,
	tokenHash string,
	kind identity.SessionCredentialKind,
) (identity.SessionCredential, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return identity.SessionCredential{}, identity.ErrNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	credential, ok := s.credentialsByHash[tokenHash]
	if !ok {
		return identity.SessionCredential{}, identity.ErrNotFound
	}
	if kind != identity.SessionCredentialKindUnspecified && credential.Kind != kind {
		return identity.SessionCredential{}, identity.ErrNotFound
	}

	return credential, nil
}

// DeleteSessionCredentialsBySessionID removes all persisted bearer tokens for one session.
func (s *memoryStore) DeleteSessionCredentialsBySessionID(_ context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	deleted := false
	for _, kind := range []identity.SessionCredentialKind{
		identity.SessionCredentialKindAccess,
		identity.SessionCredentialKindRefresh,
	} {
		key := credentialKey(sessionID, kind)
		hash, ok := s.credentialHashesByKey[key]
		if !ok {
			continue
		}
		delete(s.credentialHashesByKey, key)
		delete(s.credentialsByHash, hash)
		deleted = true
	}

	if !deleted {
		return identity.ErrNotFound
	}

	return nil
}

func credentialKey(sessionID string, kind identity.SessionCredentialKind) string {
	return sessionID + ":" + string(kind)
}

// SaveAccountCredential stores one hashed account secret.
func (s *memoryStore) SaveAccountCredential(
	_ context.Context,
	credential identity.AccountCredential,
) (identity.AccountCredential, error) {
	if credential.AccountID == "" || credential.SecretHash == "" {
		return identity.AccountCredential{}, identity.ErrInvalidInput
	}
	if credential.Kind != identity.AccountCredentialKindPassword &&
		credential.Kind != identity.AccountCredentialKindRecovery {
		return identity.AccountCredential{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.accountCredentialsByKey[accountCredentialKey(credential.AccountID, credential.Kind)] = credential
	return credential, nil
}

// AccountCredentialByAccountID resolves one stored account secret.
func (s *memoryStore) AccountCredentialByAccountID(
	_ context.Context,
	accountID string,
	kind identity.AccountCredentialKind,
) (identity.AccountCredential, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return identity.AccountCredential{}, identity.ErrNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	credential, ok := s.accountCredentialsByKey[accountCredentialKey(accountID, kind)]
	if !ok {
		return identity.AccountCredential{}, identity.ErrNotFound
	}

	return credential, nil
}

func accountCredentialKey(accountID string, kind identity.AccountCredentialKind) string {
	return accountID + ":" + string(kind)
}
