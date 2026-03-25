package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SaveConversation inserts or updates a conversation row.
func (s *Store) SaveConversation(ctx context.Context, conversationRow conversation.Conversation) (conversation.Conversation, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.Conversation{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.Conversation{}, err
	}
	if conversationRow.ID == "" {
		return conversation.Conversation{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveConversation(ctx, conversationRow)
	}

	var saved conversation.Conversation
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveConversation(ctx, conversationRow)
		return saveErr
	})
	if err != nil {
		return conversation.Conversation{}, err
	}

	return saved, nil
}

func (s *Store) saveConversation(ctx context.Context, conversationRow conversation.Conversation) (conversation.Conversation, error) {
	conversationRow.ID = strings.TrimSpace(conversationRow.ID)
	conversationRow.Kind = conversation.ConversationKind(strings.TrimSpace(string(conversationRow.Kind)))
	conversationRow.Title = strings.TrimSpace(conversationRow.Title)
	conversationRow.Description = strings.TrimSpace(conversationRow.Description)
	conversationRow.AvatarMediaID = strings.TrimSpace(conversationRow.AvatarMediaID)
	conversationRow.OwnerAccountID = strings.TrimSpace(conversationRow.OwnerAccountID)
	if conversationRow.ID == "" || conversationRow.Kind == conversation.ConversationKindUnspecified || conversationRow.OwnerAccountID == "" {
		return conversation.Conversation{}, conversation.ErrInvalidInput
	}

	settings := conversationRow.Settings

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, kind, title, description, avatar_media_id, owner_account_id,
	only_admins_can_write, only_admins_can_add_members, allow_reactions, allow_forwards,
	allow_threads, require_encrypted_messages, require_join_approval, pinned_messages_only_admins,
	slow_mode_interval_nanos, archived, muted, pinned, hidden, last_sequence, created_at, updated_at,
	last_message_at
) VALUES (
	$1, $2, $3, $4, $5, $6,
	$7, $8, $9, $10,
	$11, $12, $13, $14,
	$15, $16, $17, $18, $19, $20, $21, $22, $23
)
ON CONFLICT (id) DO UPDATE SET
	kind = EXCLUDED.kind,
	title = EXCLUDED.title,
	description = EXCLUDED.description,
	avatar_media_id = EXCLUDED.avatar_media_id,
	owner_account_id = EXCLUDED.owner_account_id,
	only_admins_can_write = EXCLUDED.only_admins_can_write,
	only_admins_can_add_members = EXCLUDED.only_admins_can_add_members,
	allow_reactions = EXCLUDED.allow_reactions,
	allow_forwards = EXCLUDED.allow_forwards,
	allow_threads = EXCLUDED.allow_threads,
	require_encrypted_messages = EXCLUDED.require_encrypted_messages,
	require_join_approval = EXCLUDED.require_join_approval,
	pinned_messages_only_admins = EXCLUDED.pinned_messages_only_admins,
	slow_mode_interval_nanos = EXCLUDED.slow_mode_interval_nanos,
	archived = EXCLUDED.archived,
	muted = EXCLUDED.muted,
	pinned = EXCLUDED.pinned,
	hidden = EXCLUDED.hidden,
	last_sequence = EXCLUDED.last_sequence,
	updated_at = EXCLUDED.updated_at,
	last_message_at = EXCLUDED.last_message_at
RETURNING %s
`, s.table("conversation_conversations"), conversationColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		conversationRow.ID,
		conversationRow.Kind,
		conversationRow.Title,
		conversationRow.Description,
		conversationRow.AvatarMediaID,
		conversationRow.OwnerAccountID,
		settings.OnlyAdminsCanWrite,
		settings.OnlyAdminsCanAddMembers,
		settings.AllowReactions,
		settings.AllowForwards,
		settings.AllowThreads,
		settings.RequireEncryptedMessages,
		settings.RequireJoinApproval,
		settings.PinnedMessagesOnlyAdmins,
		int64(settings.SlowModeInterval),
		conversationRow.Archived,
		conversationRow.Muted,
		conversationRow.Pinned,
		conversationRow.Hidden,
		conversationRow.LastSequence,
		conversationRow.CreatedAt.UTC(),
		conversationRow.UpdatedAt.UTC(),
		encodeTime(conversationRow.LastMessageAt),
	)

	saved, err := scanConversation(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.Conversation{}, mappedErr
		}
		return conversation.Conversation{}, fmt.Errorf("save conversation %s: %w", conversationRow.ID, err)
	}

	return saved, nil
}

// ConversationByID resolves a conversation by primary key.
func (s *Store) ConversationByID(ctx context.Context, conversationID string) (conversation.Conversation, error) {
	if err := s.requireStore(); err != nil {
		return conversation.Conversation{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.Conversation{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return conversation.Conversation{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, conversationColumnList, s.table("conversation_conversations"))
	row := s.conn().QueryRowContext(ctx, query, conversationID)
	conversationRow, err := scanConversation(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.Conversation{}, conversation.ErrNotFound
		}
		return conversation.Conversation{}, fmt.Errorf("load conversation %s: %w", conversationID, err)
	}

	return conversationRow, nil
}

// ConversationsByAccountID lists the conversations a member belongs to.
func (s *Store) ConversationsByAccountID(ctx context.Context, accountID string) ([]conversation.Conversation, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s AS c
INNER JOIN %s AS m ON m.conversation_id = c.id
WHERE m.account_id = $1 AND m.left_at IS NULL AND NOT m.banned
ORDER BY c.last_sequence DESC, c.updated_at DESC, c.id ASC
`, qualifyColumns("c", conversationColumnList), s.table("conversation_conversations"), s.table("conversation_members"))

	rows, err := s.conn().QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("list conversations for account %s: %w", accountID, err)
	}
	defer rows.Close()

	conversations := make([]conversation.Conversation, 0)
	for rows.Next() {
		rowConversation, scanErr := scanConversation(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan conversation for account %s: %w", accountID, scanErr)
		}
		conversations = append(conversations, rowConversation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations for account %s: %w", accountID, err)
	}

	return conversations, nil
}
