package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SaveConversationMember inserts or updates a conversation member row.
func (s *Store) SaveConversationMember(ctx context.Context, member conversation.ConversationMember) (conversation.ConversationMember, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.ConversationMember{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.ConversationMember{}, err
	}
	if member.ConversationID == "" || member.AccountID == "" {
		return conversation.ConversationMember{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveConversationMember(ctx, member)
	}

	var saved conversation.ConversationMember
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveConversationMember(ctx, member)
		return saveErr
	})
	if err != nil {
		return conversation.ConversationMember{}, err
	}

	return saved, nil
}

func (s *Store) saveConversationMember(ctx context.Context, member conversation.ConversationMember) (conversation.ConversationMember, error) {
	member.ConversationID = strings.TrimSpace(member.ConversationID)
	member.AccountID = strings.TrimSpace(member.AccountID)
	member.InvitedByAccountID = strings.TrimSpace(member.InvitedByAccountID)
	if member.ConversationID == "" || member.AccountID == "" || member.Role == conversation.MemberRoleUnspecified {
		return conversation.ConversationMember{}, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	conversation_id, account_id, role, invited_by_account_id, muted, banned, joined_at, left_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (conversation_id, account_id) DO UPDATE SET
	role = EXCLUDED.role,
	invited_by_account_id = EXCLUDED.invited_by_account_id,
	muted = EXCLUDED.muted,
	banned = EXCLUDED.banned,
	joined_at = EXCLUDED.joined_at,
	left_at = EXCLUDED.left_at
RETURNING %s
`, s.table("conversation_members"), conversationMemberColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		member.ConversationID,
		member.AccountID,
		member.Role,
		nullString(member.InvitedByAccountID),
		member.Muted,
		member.Banned,
		member.JoinedAt.UTC(),
		encodeTime(member.LeftAt),
	)

	saved, err := scanConversationMember(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.ConversationMember{}, mappedErr
		}
		return conversation.ConversationMember{}, fmt.Errorf("save conversation member %s/%s: %w", member.ConversationID, member.AccountID, err)
	}

	return saved, nil
}

// ConversationMemberByConversationAndAccount resolves a single member row.
func (s *Store) ConversationMemberByConversationAndAccount(ctx context.Context, conversationID string, accountID string) (conversation.ConversationMember, error) {
	if err := s.requireStore(); err != nil {
		return conversation.ConversationMember{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.ConversationMember{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	accountID = strings.TrimSpace(accountID)
	if conversationID == "" || accountID == "" {
		return conversation.ConversationMember{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE conversation_id = $1 AND account_id = $2`, conversationMemberColumnList, s.table("conversation_members"))
	row := s.conn().QueryRowContext(ctx, query, conversationID, accountID)
	member, err := scanConversationMember(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.ConversationMember{}, conversation.ErrNotFound
		}
		return conversation.ConversationMember{}, fmt.Errorf("load conversation member %s/%s: %w", conversationID, accountID, err)
	}

	return member, nil
}

// ConversationMembersByConversationID lists the members of a conversation.
func (s *Store) ConversationMembersByConversationID(ctx context.Context, conversationID string) ([]conversation.ConversationMember, error) {
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

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE conversation_id = $1 ORDER BY joined_at ASC, account_id ASC`, conversationMemberColumnList, s.table("conversation_members"))
	rows, err := s.conn().QueryContext(ctx, query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list conversation members %s: %w", conversationID, err)
	}
	defer rows.Close()

	members := make([]conversation.ConversationMember, 0)
	for rows.Next() {
		member, scanErr := scanConversationMember(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan conversation member %s: %w", conversationID, scanErr)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation members %s: %w", conversationID, err)
	}

	return members, nil
}

func nullString(value string) sql.NullString {
	if strings.TrimSpace(value) == "" {
		return sql.NullString{}
	}

	return sql.NullString{String: value, Valid: true}
}
