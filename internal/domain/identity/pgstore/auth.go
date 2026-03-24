package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveLoginChallenge stores or replaces a login challenge by primary key.
func (s *Store) SaveLoginChallenge(ctx context.Context, challenge identity.LoginChallenge) (identity.LoginChallenge, error) {
	if err := s.requireStore(); err != nil {
		return identity.LoginChallenge{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return identity.LoginChallenge{}, err
	}
	if challenge.ID == "" {
		return identity.LoginChallenge{}, identity.ErrInvalidInput
	}

	targetsRaw, err := encodeTargets(challenge.Targets)
	if err != nil {
		return identity.LoginChallenge{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, account_id, account_kind, code_hash, delivery_channel, targets, expires_at, created_at, used_at, used
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (id) DO UPDATE SET
	account_id = EXCLUDED.account_id,
	account_kind = EXCLUDED.account_kind,
	code_hash = EXCLUDED.code_hash,
	delivery_channel = EXCLUDED.delivery_channel,
	targets = EXCLUDED.targets,
	expires_at = EXCLUDED.expires_at,
	used_at = EXCLUDED.used_at,
	used = EXCLUDED.used
RETURNING %s
`, s.table("identity_login_challenges"), loginChallengeColumnList)

	saved, err := scanLoginChallenge(s.conn().QueryRowContext(ctx, query,
		challenge.ID,
		challenge.AccountID,
		challenge.AccountKind,
		challenge.CodeHash,
		challenge.DeliveryChannel,
		targetsRaw,
		challenge.ExpiresAt.UTC(),
		challenge.CreatedAt.UTC(),
		nullTime(challenge.UsedAt),
		challenge.Used,
	))
	if err != nil {
		if mappedErr := mapConstraintError(err, identity.ErrNotFound); mappedErr != nil {
			return identity.LoginChallenge{}, mappedErr
		}
		return identity.LoginChallenge{}, fmt.Errorf("save login challenge %s: %w", challenge.ID, err)
	}

	return saved, nil
}

// LoginChallengeByID resolves a login challenge by primary key.
func (s *Store) LoginChallengeByID(ctx context.Context, challengeID string) (identity.LoginChallenge, error) {
	if err := s.requireStore(); err != nil {
		return identity.LoginChallenge{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return identity.LoginChallenge{}, err
	}
	if strings.TrimSpace(challengeID) == "" {
		return identity.LoginChallenge{}, identity.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, loginChallengeColumnList, s.table("identity_login_challenges"))
	challenge, err := scanLoginChallenge(s.conn().QueryRowContext(ctx, query, challengeID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.LoginChallenge{}, identity.ErrNotFound
		}
		return identity.LoginChallenge{}, fmt.Errorf("load login challenge %s: %w", challengeID, err)
	}

	return challenge, nil
}

// DeleteLoginChallenge removes a login challenge from the store.
func (s *Store) DeleteLoginChallenge(ctx context.Context, challengeID string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if challengeID == "" {
		return identity.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.table("identity_login_challenges"))
	result, err := s.conn().ExecContext(ctx, query, challengeID)
	if err != nil {
		return fmt.Errorf("delete login challenge %s: %w", challengeID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete login challenge %s: %w", challengeID, err)
	}
	if rowsAffected == 0 {
		return identity.ErrNotFound
	}

	return nil
}
