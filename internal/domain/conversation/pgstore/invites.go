package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SaveConversationInvite inserts or updates a conversation invite row.
func (s *Store) SaveConversationInvite(ctx context.Context, invite conversation.ConversationInvite) (conversation.ConversationInvite, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.ConversationInvite{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.ConversationInvite{}, err
	}

	if s.tx != nil {
		return s.saveConversationInvite(ctx, invite)
	}

	var saved conversation.ConversationInvite
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveConversationInvite(ctx, invite)
		return saveErr
	})
	if err != nil {
		return conversation.ConversationInvite{}, err
	}

	return saved, nil
}

func (s *Store) saveConversationInvite(ctx context.Context, invite conversation.ConversationInvite) (conversation.ConversationInvite, error) {
	invite.ID = strings.TrimSpace(invite.ID)
	invite.ConversationID = strings.TrimSpace(invite.ConversationID)
	invite.Code = strings.TrimSpace(invite.Code)
	invite.CreatedByAccountID = strings.TrimSpace(invite.CreatedByAccountID)
	if invite.ID == "" || invite.ConversationID == "" || invite.Code == "" || invite.CreatedByAccountID == "" || len(invite.AllowedRoles) == 0 {
		return conversation.ConversationInvite{}, conversation.ErrInvalidInput
	}

	allowedRoles, err := encodeInviteRoles(invite.AllowedRoles)
	if err != nil {
		return conversation.ConversationInvite{}, fmt.Errorf("encode invite roles for %s: %w", invite.ID, err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, conversation_id, code, created_by_account_id, allowed_roles, expires_at, max_uses, use_count, revoked, revoked_at, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (id) DO UPDATE SET
	code = EXCLUDED.code,
	created_by_account_id = EXCLUDED.created_by_account_id,
	allowed_roles = EXCLUDED.allowed_roles,
	expires_at = EXCLUDED.expires_at,
	max_uses = EXCLUDED.max_uses,
	use_count = EXCLUDED.use_count,
	revoked = EXCLUDED.revoked,
	revoked_at = EXCLUDED.revoked_at,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("conversation_invites"), conversationInviteColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		invite.ID,
		invite.ConversationID,
		invite.Code,
		invite.CreatedByAccountID,
		allowedRoles,
		encodeTime(invite.ExpiresAt),
		invite.MaxUses,
		invite.UseCount,
		invite.Revoked,
		encodeTime(invite.RevokedAt),
		invite.CreatedAt.UTC(),
		invite.UpdatedAt.UTC(),
	)

	saved, err := scanConversationInvite(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrConflict); mappedErr != nil {
			return conversation.ConversationInvite{}, mappedErr
		}
		return conversation.ConversationInvite{}, fmt.Errorf("save conversation invite %s: %w", invite.ID, err)
	}

	return saved, nil
}

// ConversationInviteByConversationAndID resolves one invite row.
func (s *Store) ConversationInviteByConversationAndID(
	ctx context.Context,
	conversationID string,
	inviteID string,
) (conversation.ConversationInvite, error) {
	if err := s.requireStore(); err != nil {
		return conversation.ConversationInvite{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.ConversationInvite{}, err
	}

	conversationID = strings.TrimSpace(conversationID)
	inviteID = strings.TrimSpace(inviteID)
	if conversationID == "" || inviteID == "" {
		return conversation.ConversationInvite{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE conversation_id = $1 AND id = $2`,
		conversationInviteColumnList,
		s.table("conversation_invites"),
	)
	row := s.conn().QueryRowContext(ctx, query, conversationID, inviteID)
	invite, err := scanConversationInvite(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.ConversationInvite{}, conversation.ErrNotFound
		}
		return conversation.ConversationInvite{}, fmt.Errorf("load invite %s in %s: %w", inviteID, conversationID, err)
	}

	return invite, nil
}

// ConversationInvitesByConversationID lists invites for one conversation.
func (s *Store) ConversationInvitesByConversationID(
	ctx context.Context,
	conversationID string,
) ([]conversation.ConversationInvite, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE conversation_id = $1 ORDER BY created_at DESC, id ASC`,
		conversationInviteColumnList,
		s.table("conversation_invites"),
	)
	rows, err := s.conn().QueryContext(ctx, query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list invites for conversation %s: %w", conversationID, err)
	}
	defer rows.Close()

	invites := make([]conversation.ConversationInvite, 0)
	for rows.Next() {
		invite, scanErr := scanConversationInvite(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan invite for conversation %s: %w", conversationID, scanErr)
		}
		invites = append(invites, invite)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invites for conversation %s: %w", conversationID, err)
	}

	return invites, nil
}
