package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SaveMessage inserts or updates a message row and replaces its attachments.
func (s *Store) SaveMessage(ctx context.Context, message conversation.Message) (conversation.Message, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.Message{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.Message{}, err
	}
	if message.ID == "" {
		return conversation.Message{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveMessage(ctx, message)
	}

	var saved conversation.Message
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveMessage(ctx, message)
		return saveErr
	})
	if err != nil {
		return conversation.Message{}, err
	}

	return saved, nil
}

func (s *Store) saveMessage(ctx context.Context, message conversation.Message) (conversation.Message, error) {
	message.ID = strings.TrimSpace(message.ID)
	message.ConversationID = strings.TrimSpace(message.ConversationID)
	message.SenderAccountID = strings.TrimSpace(message.SenderAccountID)
	message.SenderDeviceID = strings.TrimSpace(message.SenderDeviceID)
	message.ClientMessageID = strings.TrimSpace(message.ClientMessageID)
	message.ThreadID = strings.TrimSpace(message.ThreadID)
	message.MentionAccountIDs = normalizeIDs(message.MentionAccountIDs)
	if message.ID == "" || message.ConversationID == "" || message.SenderAccountID == "" || message.SenderDeviceID == "" {
		return conversation.Message{}, conversation.ErrInvalidInput
	}
	if message.Kind == conversation.MessageKindUnspecified {
		return conversation.Message{}, conversation.ErrInvalidInput
	}
	if message.Status == conversation.MessageStatusUnspecified {
		return conversation.Message{}, conversation.ErrInvalidInput
	}
	if err := conversation.ValidateMessagePayload(message.Payload, false); err != nil {
		return conversation.Message{}, err
	}

	conversation.StripMessageHints(&message)

	payloadKeyID, payloadAlgorithm, payloadNonce, payloadCiphertext, payloadAAD, payloadMetadata, err := encodePayload(message.Payload)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("encode message payload: %w", err)
	}
	replyKind := string(message.ReplyTo.MessageKind)
	replyConversationID := nullString(message.ReplyTo.ConversationID)
	replyMessageID := nullString(message.ReplyTo.MessageID)
	replySenderAccountID := nullString(message.ReplyTo.SenderAccountID)
	replySnippet := nullString(message.ReplyTo.Snippet)
	metadata, err := encodeMetadata(message.Metadata)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("encode message metadata: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, conversation_id, sender_account_id, sender_device_id, client_message_id, sequence, kind, status,
	payload_key_id, payload_algorithm, payload_nonce, payload_ciphertext, payload_aad, payload_metadata,
	reply_conversation_id, reply_message_id, reply_sender_account_id, reply_kind, reply_snippet,
	thread_id, silent, pinned, disable_link_previews, view_count, deliver_at, metadata,
	created_at, updated_at, edited_at, deleted_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8,
	$9, $10, $11, $12, $13, $14,
	$15, $16, $17, $18, $19,
	$20, $21, $22, $23, $24, $25, $26,
	$27, $28, $29, $30
)
ON CONFLICT (id) DO UPDATE SET
	conversation_id = EXCLUDED.conversation_id,
	sender_account_id = EXCLUDED.sender_account_id,
	sender_device_id = EXCLUDED.sender_device_id,
	client_message_id = EXCLUDED.client_message_id,
	sequence = EXCLUDED.sequence,
	kind = EXCLUDED.kind,
	status = EXCLUDED.status,
	payload_key_id = EXCLUDED.payload_key_id,
	payload_algorithm = EXCLUDED.payload_algorithm,
	payload_nonce = EXCLUDED.payload_nonce,
	payload_ciphertext = EXCLUDED.payload_ciphertext,
	payload_aad = EXCLUDED.payload_aad,
	payload_metadata = EXCLUDED.payload_metadata,
	reply_conversation_id = EXCLUDED.reply_conversation_id,
	reply_message_id = EXCLUDED.reply_message_id,
	reply_sender_account_id = EXCLUDED.reply_sender_account_id,
	reply_kind = EXCLUDED.reply_kind,
	reply_snippet = EXCLUDED.reply_snippet,
	thread_id = EXCLUDED.thread_id,
	silent = EXCLUDED.silent,
	pinned = EXCLUDED.pinned,
	disable_link_previews = EXCLUDED.disable_link_previews,
	view_count = EXCLUDED.view_count,
	deliver_at = EXCLUDED.deliver_at,
	metadata = EXCLUDED.metadata,
	updated_at = EXCLUDED.updated_at,
	edited_at = EXCLUDED.edited_at,
	deleted_at = EXCLUDED.deleted_at
RETURNING %s
`, s.table("conversation_messages"), messageColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		message.ID,
		message.ConversationID,
		message.SenderAccountID,
		message.SenderDeviceID,
		message.ClientMessageID,
		message.Sequence,
		message.Kind,
		message.Status,
		payloadKeyID,
		payloadAlgorithm,
		payloadNonce,
		payloadCiphertext,
		payloadAAD,
		payloadMetadata,
		replyConversationID,
		replyMessageID,
		replySenderAccountID,
		replyKind,
		replySnippet,
		message.ThreadID,
		message.Silent,
		message.Pinned,
		message.DisableLinkPreviews,
		message.ViewCount,
		encodeTime(message.DeliverAt),
		metadata,
		message.CreatedAt.UTC(),
		message.UpdatedAt.UTC(),
		encodeTime(message.EditedAt),
		encodeTime(message.DeletedAt),
	)

	saved, err := scanMessage(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.Message{}, mappedErr
		}
		return conversation.Message{}, fmt.Errorf("save message %s: %w", message.ID, err)
	}

	if _, err := s.conn().ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE message_id = $1`, s.table("conversation_message_attachments")), saved.ID); err != nil {
		return conversation.Message{}, fmt.Errorf("replace attachments for message %s: %w", saved.ID, err)
	}

	for idx, attachment := range message.Attachments {
		if err := s.insertAttachment(ctx, saved.ID, idx, attachment); err != nil {
			return conversation.Message{}, err
		}
	}

	if len(message.Attachments) > 0 {
		saved.Attachments = append([]conversation.AttachmentRef(nil), message.Attachments...)
	}

	if err := s.replaceMentions(ctx, saved.ID, message.MentionAccountIDs); err != nil {
		return conversation.Message{}, err
	}
	saved.MentionAccountIDs = append([]string(nil), message.MentionAccountIDs...)

	reactions, err := s.reactionsByMessageIDs(ctx, []string{saved.ID})
	if err != nil {
		return conversation.Message{}, err
	}
	saved.Reactions = reactions[saved.ID]

	return saved, nil
}

func (s *Store) insertAttachment(ctx context.Context, messageID string, attachmentIndex int, attachment conversation.AttachmentRef) error {
	attachment.MediaID = strings.TrimSpace(attachment.MediaID)
	attachment.FileName = strings.TrimSpace(attachment.FileName)
	attachment.MimeType = strings.TrimSpace(attachment.MimeType)
	attachment.SHA256Hex = strings.TrimSpace(attachment.SHA256Hex)
	attachment.Caption = strings.TrimSpace(attachment.Caption)

	query := fmt.Sprintf(`
INSERT INTO %s (
	message_id, attachment_index, media_id, kind, file_name, mime_type, size_bytes, sha256_hex, width, height, duration_seconds, caption
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)`, s.table("conversation_message_attachments"))

	_, err := s.conn().ExecContext(ctx, query,
		messageID,
		attachmentIndex,
		attachment.MediaID,
		attachment.Kind,
		attachment.FileName,
		attachment.MimeType,
		attachment.SizeBytes,
		attachment.SHA256Hex,
		attachment.Width,
		attachment.Height,
		encodeAttachmentDuration(attachment.Duration),
		attachment.Caption,
	)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return mappedErr
		}
		return fmt.Errorf("insert attachment %s:%d: %w", messageID, attachmentIndex, err)
	}

	return nil
}

// MessageByID resolves a message by primary key.
func (s *Store) MessageByID(ctx context.Context, conversationID string, messageID string) (conversation.Message, error) {
	if err := s.requireStore(); err != nil {
		return conversation.Message{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.Message{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	messageID = strings.TrimSpace(messageID)
	if conversationID == "" || messageID == "" {
		return conversation.Message{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1 AND conversation_id = $2`, messageColumnList, s.table("conversation_messages"))
	row := s.conn().QueryRowContext(ctx, query, messageID, conversationID)
	message, err := scanMessage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.Message{}, conversation.ErrNotFound
		}
		return conversation.Message{}, fmt.Errorf("load message %s: %w", messageID, err)
	}

	attachments, err := s.attachmentsByMessageIDs(ctx, []string{message.ID})
	if err != nil {
		return conversation.Message{}, err
	}
	message.Attachments = attachments[message.ID]

	mentions, err := s.mentionsByMessageIDs(ctx, []string{message.ID})
	if err != nil {
		return conversation.Message{}, err
	}
	message.MentionAccountIDs = mentions[message.ID]

	reactions, err := s.reactionsByMessageIDs(ctx, []string{message.ID})
	if err != nil {
		return conversation.Message{}, err
	}
	message.Reactions = reactions[message.ID]

	return message, nil
}

// MessagesByConversationID lists the messages in a conversation and optional thread.
func (s *Store) MessagesByConversationID(ctx context.Context, conversationID string, threadID string, fromSequence uint64, limit int) ([]conversation.Message, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	conversationID = strings.TrimSpace(conversationID)
	threadID = strings.TrimSpace(threadID)
	if conversationID == "" {
		return nil, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE conversation_id = $1 AND thread_id = $2 AND sequence > $3
	AND status NOT IN ('pending', 'failed')
ORDER BY sequence ASC, created_at ASC, id ASC
LIMIT $4
`, messageColumnList, s.table("conversation_messages"))
	rows, err := s.conn().QueryContext(ctx, query, conversationID, threadID, fromSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages for conversation %s: %w", conversationID, err)
	}
	defer rows.Close()

	messages := make([]conversation.Message, 0)
	messageIDs := make([]string, 0)
	for rows.Next() {
		message, scanErr := scanMessage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan message for conversation %s: %w", conversationID, scanErr)
		}
		messages = append(messages, message)
		messageIDs = append(messageIDs, message.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages for conversation %s: %w", conversationID, err)
	}

	attachments, err := s.attachmentsByMessageIDs(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for idx := range messages {
		messages[idx].Attachments = attachments[messages[idx].ID]
	}

	mentions, err := s.mentionsByMessageIDs(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for idx := range messages {
		messages[idx].MentionAccountIDs = mentions[messages[idx].ID]
	}

	reactions, err := s.reactionsByMessageIDs(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for idx := range messages {
		messages[idx].Reactions = reactions[messages[idx].ID]
	}

	return messages, nil
}

func (s *Store) ScheduledMessagesByConversationAndSender(
	ctx context.Context,
	conversationID string,
	senderAccountID string,
	threadID string,
	includeFailed bool,
	limit int,
) ([]conversation.Message, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	conversationID = strings.TrimSpace(conversationID)
	senderAccountID = strings.TrimSpace(senderAccountID)
	threadID = strings.TrimSpace(threadID)
	if conversationID == "" || senderAccountID == "" {
		return nil, conversation.ErrInvalidInput
	}

	statusFilter := `status = 'pending'`
	if includeFailed {
		statusFilter = `status IN ('pending', 'failed')`
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE conversation_id = $1
	AND sender_account_id = $2
	AND thread_id = $3
	AND %s
ORDER BY CASE WHEN status = 'pending' THEN 0 ELSE 1 END ASC, deliver_at ASC NULLS LAST, created_at ASC, id ASC
LIMIT $4
`, messageColumnList, s.table("conversation_messages"), statusFilter)

	rows, err := s.conn().QueryContext(ctx, query, conversationID, senderAccountID, threadID, limit)
	if err != nil {
		return nil, fmt.Errorf("list scheduled messages for conversation %s: %w", conversationID, err)
	}
	defer rows.Close()

	return s.scanMessages(ctx, rows, conversationID)
}

func (s *Store) DueScheduledMessages(ctx context.Context, before time.Time, limit int) ([]conversation.Message, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	if before.IsZero() {
		before = time.Now().UTC()
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE status = 'pending'
	AND deliver_at IS NOT NULL
	AND deliver_at <= $1
ORDER BY deliver_at ASC, created_at ASC, id ASC
LIMIT $2
FOR UPDATE SKIP LOCKED
`, messageColumnList, s.table("conversation_messages"))
	rows, err := s.conn().QueryContext(ctx, query, before.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list due scheduled messages: %w", err)
	}
	defer rows.Close()

	return s.scanMessages(ctx, rows, "")
}

func (s *Store) scanMessages(ctx context.Context, rows *sql.Rows, conversationID string) ([]conversation.Message, error) {
	messages := make([]conversation.Message, 0)
	messageIDs := make([]string, 0)
	for rows.Next() {
		message, scanErr := scanMessage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan message for conversation %s: %w", conversationID, scanErr)
		}
		messages = append(messages, message)
		messageIDs = append(messageIDs, message.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages for conversation %s: %w", conversationID, err)
	}

	attachments, err := s.attachmentsByMessageIDs(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for idx := range messages {
		messages[idx].Attachments = attachments[messages[idx].ID]
	}

	mentions, err := s.mentionsByMessageIDs(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for idx := range messages {
		messages[idx].MentionAccountIDs = mentions[messages[idx].ID]
	}

	reactions, err := s.reactionsByMessageIDs(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for idx := range messages {
		messages[idx].Reactions = reactions[messages[idx].ID]
	}

	return messages, nil
}

func (s *Store) attachmentsByMessageIDs(ctx context.Context, messageIDs []string) (map[string][]conversation.AttachmentRef, error) {
	if len(messageIDs) == 0 {
		return make(map[string][]conversation.AttachmentRef), nil
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE message_id = ANY($1)
ORDER BY message_id ASC, attachment_index ASC
`, attachmentColumnList, s.table("conversation_message_attachments"))
	rows, err := s.conn().QueryContext(ctx, query, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()

	attachments := make(map[string][]conversation.AttachmentRef, len(messageIDs))
	for rows.Next() {
		var messageID string
		var attachment conversation.AttachmentRef
		var (
			attachmentIndex int
			durationSeconds int64
		)
		if err := rows.Scan(
			&messageID,
			&attachmentIndex,
			&attachment.MediaID,
			&attachment.Kind,
			&attachment.FileName,
			&attachment.MimeType,
			&attachment.SizeBytes,
			&attachment.SHA256Hex,
			&attachment.Width,
			&attachment.Height,
			&durationSeconds,
			&attachment.Caption,
		); err != nil {
			return nil, fmt.Errorf("scan attachment message id: %w", err)
		}
		attachment.Duration = decodeAttachmentDuration(durationSeconds)
		attachments[messageID] = append(attachments[messageID], attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachments: %w", err)
	}

	return attachments, nil
}
