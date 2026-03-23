package identity

import (
	"context"
	"fmt"
)

// ListDevices returns all known devices for an account.
//
// The store remains the source of truth; the service does not rewrite device state on
// this read path.
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
//
// The returned slice is normalized into a read-model view where only one active session
// is flagged as current. That keeps the persisted data simple while still giving callers
// a deterministic primary session for UI rendering.
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
//
// The method clears the current-session flag in the persisted row so later reads no
// longer treat it as the primary session.
func (s *Service) RevokeSession(ctx context.Context, params RevokeSessionParams) (Session, error) {
	if err := s.validateContext(ctx, "revoke session"); err != nil {
		return Session{}, err
	}
	fingerprint := revokeSessionFingerprint(params)
	if params.IdempotencyKey != "" {
		if session, ok, err := s.idempotency.revokeSessionResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return Session{}, err
		} else if ok {
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
		s.idempotency.storeRevokeSessionResult(params.IdempotencyKey, fingerprint, updatedSession, s.currentTime())
	}

	return updatedSession, nil
}

// RevokeAllSessions marks every active session for the account as revoked.
//
// The bulk update runs inside a store transaction so either all active sessions are
// revoked or none of them are.
func (s *Service) RevokeAllSessions(ctx context.Context, accountID string, params RevokeAllSessionsParams) (uint32, error) {
	if err := s.validateContext(ctx, "revoke all sessions"); err != nil {
		return 0, err
	}
	fingerprint := revokeAllSessionsFingerprint(accountID, params)
	if params.IdempotencyKey != "" {
		if revoked, ok, err := s.idempotency.revokeAllSessionsResult(params.IdempotencyKey, fingerprint, s.currentTime()); err != nil {
			return 0, err
		} else if ok {
			return revoked, nil
		}
	}
	if accountID == "" {
		return 0, ErrInvalidInput
	}

	var revoked uint32
	err := s.store.WithinTx(ctx, func(tx Store) error {
		sessions, loadErr := tx.SessionsByAccountID(ctx, accountID)
		if loadErr != nil {
			return fmt.Errorf("list sessions for account %s: %w", accountID, loadErr)
		}

		now := params.RequestedAt
		if now.IsZero() {
			now = s.currentTime()
		}

		for _, session := range sessions {
			if session.Status == SessionStatusRevoked {
				continue
			}

			session.Status = SessionStatusRevoked
			session.RevokedAt = now
			session.Current = false
			if _, loadErr := tx.UpdateSession(ctx, session); loadErr != nil {
				return fmt.Errorf("revoke session %s for account %s: %w", session.ID, accountID, loadErr)
			}
			revoked++
		}

		return nil
	})
	if err != nil {
		return revoked, err
	}

	if params.IdempotencyKey != "" {
		s.idempotency.storeRevokeAllSessionsResult(params.IdempotencyKey, fingerprint, revoked, s.currentTime())
	}

	return revoked, nil
}

// normalizeCurrentSessions turns the persisted session list into a stable read model.
//
// Only one active session is marked current. All others are cleared so the caller can
// render a single primary row without relying on store-specific ordering.
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

// isPreferredCurrentSession picks a deterministic winner when multiple active sessions
// are competing for the read-model current flag.
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
