package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveSession stores a session and indexes it by account.
func (s *Store) SaveSession(ctx context.Context, session identity.Session) (identity.Session, error) {
	if session.ID == "" {
		return identity.Session{}, identity.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, account_id, device_id, device_name, device_platform, ip_address, user_agent, status, current, created_at, last_seen_at, revoked_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (id) DO UPDATE SET
	account_id = EXCLUDED.account_id,
	device_id = EXCLUDED.device_id,
	device_name = EXCLUDED.device_name,
	device_platform = EXCLUDED.device_platform,
	ip_address = EXCLUDED.ip_address,
	user_agent = EXCLUDED.user_agent,
	status = EXCLUDED.status,
	current = EXCLUDED.current,
	last_seen_at = EXCLUDED.last_seen_at,
	revoked_at = EXCLUDED.revoked_at
RETURNING %s
`, s.table("identity_sessions"), sessionColumnList)

	saved, err := scanSession(s.conn().QueryRowContext(ctx, query,
		session.ID,
		session.AccountID,
		session.DeviceID,
		session.DeviceName,
		session.DevicePlatform,
		session.IPAddress,
		session.UserAgent,
		session.Status,
		session.Current,
		session.CreatedAt.UTC(),
		session.LastSeenAt.UTC(),
		nullTime(session.RevokedAt),
	))
	if err != nil {
		if isUniqueViolation(err) {
			return identity.Session{}, identity.ErrConflict
		}
		if isForeignKeyViolation(err) {
			return identity.Session{}, identity.ErrNotFound
		}
		return identity.Session{}, fmt.Errorf("save session %s: %w", session.ID, err)
	}

	return saved, nil
}

// DeleteSession removes a session by primary key.
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return identity.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.table("identity_sessions"))
	result, err := s.conn().ExecContext(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("delete session %s: %w", sessionID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete session %s: %w", sessionID, err)
	}
	if rowsAffected == 0 {
		return identity.ErrNotFound
	}

	return nil
}

// SessionByID resolves a session by primary key.
func (s *Store) SessionByID(ctx context.Context, sessionID string) (identity.Session, error) {
	if strings.TrimSpace(sessionID) == "" {
		return identity.Session{}, identity.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, sessionColumnList, s.table("identity_sessions"))
	session, err := scanSession(s.conn().QueryRowContext(ctx, query, sessionID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.Session{}, identity.ErrNotFound
		}
		return identity.Session{}, fmt.Errorf("load session %s: %w", sessionID, err)
	}

	return session, nil
}

// SessionsByAccountID lists sessions for an account.
func (s *Store) SessionsByAccountID(ctx context.Context, accountID string) ([]identity.Session, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE account_id = $1 ORDER BY created_at ASC, id ASC`,
		sessionColumnList,
		s.table("identity_sessions"),
	)
	rows, err := s.conn().QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("list sessions for account %s: %w", accountID, err)
	}
	defer rows.Close()

	sessions := make([]identity.Session, 0)
	for rows.Next() {
		session, scanErr := scanSession(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan session for account %s: %w", accountID, scanErr)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions for account %s: %w", accountID, err)
	}

	return sessions, nil
}

// UpdateSession replaces an existing session row while preserving the account index.
func (s *Store) UpdateSession(ctx context.Context, session identity.Session) (identity.Session, error) {
	if session.ID == "" {
		return identity.Session{}, identity.ErrInvalidInput
	}

	query := fmt.Sprintf(`
UPDATE %s
SET account_id = $2,
    device_id = $3,
    device_name = $4,
    device_platform = $5,
    ip_address = $6,
    user_agent = $7,
    status = $8,
    current = $9,
    last_seen_at = $10,
    revoked_at = $11
WHERE id = $1
RETURNING %s
`, s.table("identity_sessions"), sessionColumnList)

	updated, err := scanSession(s.conn().QueryRowContext(ctx, query,
		session.ID,
		session.AccountID,
		session.DeviceID,
		session.DeviceName,
		session.DevicePlatform,
		session.IPAddress,
		session.UserAgent,
		session.Status,
		session.Current,
		session.LastSeenAt.UTC(),
		nullTime(session.RevokedAt),
	))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.Session{}, identity.ErrNotFound
		}
		if isUniqueViolation(err) {
			return identity.Session{}, identity.ErrConflict
		}
		if isForeignKeyViolation(err) {
			return identity.Session{}, identity.ErrNotFound
		}
		return identity.Session{}, fmt.Errorf("update session %s: %w", session.ID, err)
	}

	return updated, nil
}
