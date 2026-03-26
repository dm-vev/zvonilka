package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveSessionCredential stores or replaces one hashed bearer token for a session/kind pair.
func (s *Store) SaveSessionCredential(
	ctx context.Context,
	credential identity.SessionCredential,
) (identity.SessionCredential, error) {
	if err := s.requireStore(); err != nil {
		return identity.SessionCredential{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return identity.SessionCredential{}, err
	}

	credential.SessionID = strings.TrimSpace(credential.SessionID)
	credential.AccountID = strings.TrimSpace(credential.AccountID)
	credential.DeviceID = strings.TrimSpace(credential.DeviceID)
	credential.TokenHash = strings.TrimSpace(credential.TokenHash)
	if credential.SessionID == "" || credential.AccountID == "" || credential.DeviceID == "" || credential.TokenHash == "" {
		return identity.SessionCredential{}, identity.ErrInvalidInput
	}
	if credential.Kind != identity.SessionCredentialKindAccess && credential.Kind != identity.SessionCredentialKindRefresh {
		return identity.SessionCredential{}, identity.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	session_id, account_id, device_id, kind, token_hash, expires_at, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (session_id, kind) DO UPDATE SET
	account_id = EXCLUDED.account_id,
	device_id = EXCLUDED.device_id,
	token_hash = EXCLUDED.token_hash,
	expires_at = EXCLUDED.expires_at,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("identity_session_credentials"), credentialColumnList)

	saved, err := scanCredential(s.conn().QueryRowContext(
		ctx,
		query,
		credential.SessionID,
		credential.AccountID,
		credential.DeviceID,
		credential.Kind,
		credential.TokenHash,
		credential.ExpiresAt.UTC(),
		credential.CreatedAt.UTC(),
		credential.UpdatedAt.UTC(),
	))
	if err != nil {
		if mappedErr := mapConstraintError(err, identity.ErrNotFound); mappedErr != nil {
			return identity.SessionCredential{}, mappedErr
		}
		return identity.SessionCredential{}, fmt.Errorf(
			"save session credential %s:%s: %w",
			credential.SessionID,
			credential.Kind,
			err,
		)
	}

	return saved, nil
}

// SessionCredentialByTokenHash resolves one stored hashed bearer token.
func (s *Store) SessionCredentialByTokenHash(
	ctx context.Context,
	tokenHash string,
	kind identity.SessionCredentialKind,
) (identity.SessionCredential, error) {
	if err := s.requireStore(); err != nil {
		return identity.SessionCredential{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return identity.SessionCredential{}, err
	}

	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return identity.SessionCredential{}, identity.ErrNotFound
	}

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE token_hash = $1 AND kind = $2`,
		credentialColumnList,
		s.table("identity_session_credentials"),
	)
	credential, err := scanCredential(s.conn().QueryRowContext(ctx, query, tokenHash, kind))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.SessionCredential{}, identity.ErrNotFound
		}
		return identity.SessionCredential{}, fmt.Errorf("load session credential: %w", err)
	}

	return credential, nil
}

// DeleteSessionCredentialsBySessionID removes all persisted bearer tokens for one session.
func (s *Store) DeleteSessionCredentialsBySessionID(ctx context.Context, sessionID string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return identity.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE session_id = $1`, s.table("identity_session_credentials"))
	result, err := s.conn().ExecContext(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("delete session credentials for %s: %w", sessionID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete session credentials for %s: %w", sessionID, err)
	}
	if rowsAffected == 0 {
		return identity.ErrNotFound
	}

	return nil
}
