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

	return sessions, nil
}

// RevokeSession marks a single session as revoked.
func (s *Service) RevokeSession(ctx context.Context, params RevokeSessionParams) (Session, error) {
	if err := s.validateContext(ctx, "revoke session"); err != nil {
		return Session{}, err
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

	return updatedSession, nil
}

// RevokeAllSessions marks every active session for the account as revoked.
func (s *Service) RevokeAllSessions(ctx context.Context, accountID string, params RevokeAllSessionsParams) (uint32, error) {
	if err := s.validateContext(ctx, "revoke all sessions"); err != nil {
		return 0, err
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

	return revoked, nil
}
