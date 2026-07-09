package teststore

import (
	"context"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveSession stores a session and indexes it by account for later lookup.
func (s *memoryStore) SaveSession(_ context.Context, session identity.Session) (identity.Session, error) {
	if session.ID == "" {
		return identity.Session{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if previous, ok := s.sessionsByID[session.ID]; ok {
		s.removeSessionIndexLocked(previous.AccountID, previous.ID)
	}

	s.sessionsByID[session.ID] = session
	s.sessionIDsForAccountLocked(session.AccountID)[session.ID] = struct{}{}

	return session, nil
}

// DeleteSession removes a session and detaches it from the owning account index.
func (s *memoryStore) DeleteSession(_ context.Context, sessionID string) error {
	if sessionID == "" {
		return identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessionsByID[sessionID]
	if !ok {
		return identity.ErrNotFound
	}

	delete(s.sessionsByID, sessionID)
	s.removeSessionIndexLocked(session.AccountID, sessionID)

	return nil
}

// SessionByID resolves a session by its primary key.
func (s *memoryStore) SessionByID(_ context.Context, sessionID string) (identity.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessionsByID[sessionID]
	if !ok {
		return identity.Session{}, identity.ErrNotFound
	}

	return session, nil
}

// SessionsByAccountID lists all known sessions for an account.
func (s *memoryStore) SessionsByAccountID(_ context.Context, accountID string) ([]identity.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionIDs, ok := s.sessionIDsByAccount[accountID]
	if !ok {
		return []identity.Session{}, nil
	}

	sessions := make([]identity.Session, 0, len(sessionIDs))
	for sessionID := range sessionIDs {
		session, ok := s.sessionsByID[sessionID]
		if !ok {
			continue
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// UpdateSession replaces an existing session row while preserving the account index.
func (s *memoryStore) UpdateSession(_ context.Context, session identity.Session) (identity.Session, error) {
	if session.ID == "" {
		return identity.Session{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessionsByID[session.ID]; !ok {
		return identity.Session{}, identity.ErrNotFound
	}

	if previous, ok := s.sessionsByID[session.ID]; ok {
		s.removeSessionIndexLocked(previous.AccountID, previous.ID)
	}

	s.sessionsByID[session.ID] = session
	s.sessionIDsForAccountLocked(session.AccountID)[session.ID] = struct{}{}

	return session, nil
}

// removeSessionIndexLocked detaches a session from the per-account index.
func (s *memoryStore) removeSessionIndexLocked(accountID string, sessionID string) {
	if sessionIDs, ok := s.sessionIDsByAccount[accountID]; ok {
		delete(sessionIDs, sessionID)
		if len(sessionIDs) == 0 {
			delete(s.sessionIDsByAccount, accountID)
		}
	}
}
