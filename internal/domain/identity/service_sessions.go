package identity

import (
	"context"
	"fmt"
)

// ListDevices returns all known devices for an account.
func (s *Service) ListDevices(ctx context.Context, accountID string) ([]Device, error) {
	if err := s.validateContext(ctx, "list devices"); err != nil {
		return nil, err
	}
	if accountID == "" {
		return nil, ErrInvalidInput
	}

	devices, err := s.store.DevicesByAccountID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list devices for account %s: %w", accountID, err)
	}

	return devices, nil
}

// ListSessions returns all known sessions for an account.
func (s *Service) ListSessions(ctx context.Context, accountID string) ([]Session, error) {
	if err := s.validateContext(ctx, "list sessions"); err != nil {
		return nil, err
	}
	if accountID == "" {
		return nil, ErrInvalidInput
	}

	sessions, err := s.store.SessionsByAccountID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list sessions for account %s: %w", accountID, err)
	}

	return normalizeCurrentSessions(sessions), nil
}

// RevokeSession marks a single session as revoked.
func (s *Service) RevokeSession(ctx context.Context, params RevokeSessionParams) (Session, error) {
	if err := s.validateContext(ctx, "revoke session"); err != nil {
		return Session{}, err
	}
	if params.IdempotencyKey != "" {
		if session, ok := s.idempotency.revokeSessionResult(params.IdempotencyKey); ok {
			return session, nil
		}
	}
	if params.SessionID == "" {
		return Session{}, ErrInvalidInput
	}

	session, err := s.store.SessionByID(ctx, params.SessionID)
	if err != nil {
		return Session{}, fmt.Errorf("load session %s: %w", params.SessionID, err)
	}
	if session.Status == SessionStatusRevoked {
		return session, nil
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	session.Status = SessionStatusRevoked
	session.RevokedAt = now
	session.Current = false

	updatedSession, err := s.store.UpdateSession(ctx, session)
	if err != nil {
		return Session{}, fmt.Errorf("revoke session %s: %w", session.ID, err)
	}

	if params.IdempotencyKey != "" {
		s.idempotency.storeRevokeSessionResult(params.IdempotencyKey, updatedSession)
	}

	return updatedSession, nil
}

// RevokeAllSessions marks every active session for the account as revoked.
func (s *Service) RevokeAllSessions(ctx context.Context, accountID string, params RevokeAllSessionsParams) (uint32, error) {
	if err := s.validateContext(ctx, "revoke all sessions"); err != nil {
		return 0, err
	}
	if params.IdempotencyKey != "" {
		if revoked, ok := s.idempotency.revokeAllSessionsResult(params.IdempotencyKey); ok {
			return revoked, nil
		}
	}
	if accountID == "" {
		return 0, ErrInvalidInput
	}

	sessions, err := s.store.SessionsByAccountID(ctx, accountID)
	if err != nil {
		return 0, fmt.Errorf("list sessions for account %s: %w", accountID, err)
	}

	now := params.RequestedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var revoked uint32
	for _, session := range sessions {
		if session.Status == SessionStatusRevoked {
			continue
		}

		session.Status = SessionStatusRevoked
		session.RevokedAt = now
		session.Current = false
		if _, err := s.store.UpdateSession(ctx, session); err != nil {
			return revoked, fmt.Errorf("revoke session %s for account %s: %w", session.ID, accountID, err)
		}
		revoked++
	}

	if params.IdempotencyKey != "" {
		s.idempotency.storeRevokeAllSessionsResult(params.IdempotencyKey, revoked)
	}

	return revoked, nil
}

func normalizeCurrentSessions(sessions []Session) []Session {
	if len(sessions) == 0 {
		return sessions
	}

	currentIndex := -1
	for i := range sessions {
		sessions[i].Current = false
		if sessions[i].Status != SessionStatusActive {
			continue
		}
		if currentIndex == -1 || isPreferredCurrentSession(sessions[i], sessions[currentIndex]) {
			currentIndex = i
		}
	}

	if currentIndex >= 0 {
		sessions[currentIndex].Current = true
	}

	return sessions
}

func isPreferredCurrentSession(candidate, current Session) bool {
	if candidate.CreatedAt.After(current.CreatedAt) {
		return true
	}
	if candidate.CreatedAt.Before(current.CreatedAt) {
		return false
	}
	if candidate.LastSeenAt.After(current.LastSeenAt) {
		return true
	}
	if candidate.LastSeenAt.Before(current.LastSeenAt) {
		return false
	}

	return candidate.ID > current.ID
}
