package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

// EnsurePublicID returns one stable public ID for the internal bot entity ID.
func (s *Store) EnsurePublicID(
	ctx context.Context,
	kind bot.PublicIDKind,
	internalID string,
) (int64, error) {
	if err := s.requireStore(); err != nil {
		return 0, err
	}
	if err := s.requireContext(ctx); err != nil {
		return 0, err
	}

	kind = bot.PublicIDKind(strings.TrimSpace(string(kind)))
	internalID = strings.TrimSpace(internalID)
	if kind == "" || internalID == "" {
		return 0, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (entity_kind, internal_id)
VALUES ($1, $2)
ON CONFLICT (entity_kind, internal_id) DO UPDATE SET internal_id = EXCLUDED.internal_id
RETURNING public_id
`, s.table("bot_public_ids"))

	var publicID int64
	if err := s.conn().QueryRowContext(ctx, query, string(kind), internalID).Scan(&publicID); err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return 0, mapped
		}

		return 0, fmt.Errorf("ensure bot public id for %s/%s: %w", kind, internalID, err)
	}

	return publicID, nil
}

// InternalIDByPublic resolves one internal entity ID by stable public ID.
func (s *Store) InternalIDByPublic(
	ctx context.Context,
	kind bot.PublicIDKind,
	publicID int64,
) (string, error) {
	if err := s.requireStore(); err != nil {
		return "", err
	}
	if err := s.requireContext(ctx); err != nil {
		return "", err
	}

	kind = bot.PublicIDKind(strings.TrimSpace(string(kind)))
	if kind == "" || publicID <= 0 {
		return "", bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT internal_id
FROM %s
WHERE entity_kind = $1 AND public_id = $2
`, s.table("bot_public_ids"))

	var internalID string
	if err := s.conn().QueryRowContext(ctx, query, string(kind), publicID).Scan(&internalID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return "", mapped
		}

		return "", fmt.Errorf("resolve bot public id %s/%d: %w", kind, publicID, err)
	}

	return internalID, nil
}
