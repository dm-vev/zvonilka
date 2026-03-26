package pgstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func encodeJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	return string(raw), nil
}

func decodeMetadata(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}

	var metadata map[string]string
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}

	return metadata, nil
}

func encodeMetadata(metadata map[string]string) (string, error) {
	if len(metadata) == 0 {
		return "{}", nil
	}

	return encodeJSON(metadata)
}

func decodeWatermarks(raw string) (map[string]uint64, error) {
	if raw == "" {
		return nil, nil
	}

	var watermarks map[string]uint64
	if err := json.Unmarshal([]byte(raw), &watermarks); err != nil {
		return nil, fmt.Errorf("decode watermarks: %w", err)
	}

	return watermarks, nil
}

func encodeWatermarks(watermarks map[string]uint64) (string, error) {
	if len(watermarks) == 0 {
		return "{}", nil
	}

	return encodeJSON(watermarks)
}

func encodePayload(payload conversation.EncryptedPayload) (string, string, []byte, []byte, []byte, string, error) {
	metadata, err := encodeMetadata(payload.Metadata)
	if err != nil {
		return "", "", nil, nil, nil, "", fmt.Errorf("encode payload metadata: %w", err)
	}

	return payload.KeyID, payload.Algorithm, append([]byte(nil), payload.Nonce...), append([]byte(nil), payload.Ciphertext...), append([]byte(nil), payload.AAD...), metadata, nil
}

func decodePayload(keyID, algorithm string, nonce, ciphertext, aad []byte, rawMetadata string) (conversation.EncryptedPayload, error) {
	metadata, err := decodeMetadata(rawMetadata)
	if err != nil {
		return conversation.EncryptedPayload{}, err
	}

	return conversation.EncryptedPayload{
		KeyID:      keyID,
		Algorithm:  algorithm,
		Nonce:      append([]byte(nil), nonce...),
		Ciphertext: append([]byte(nil), ciphertext...),
		AAD:        append([]byte(nil), aad...),
		Metadata:   metadata,
	}, nil
}

func encodeTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func decodeTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return value.Time.UTC()
}

func encodeAttachmentDuration(value time.Duration) int64 {
	return int64(value / time.Second)
}

func decodeAttachmentDuration(value int64) time.Duration {
	return time.Duration(value) * time.Second
}

func scanConversation(row rowScanner) (conversation.Conversation, error) {
	var (
		conversationRow conversation.Conversation
		avatarMediaID   string
		lastMessageAt   sql.NullTime
	)

	if err := row.Scan(
		&conversationRow.ID,
		&conversationRow.Kind,
		&conversationRow.Title,
		&conversationRow.Description,
		&avatarMediaID,
		&conversationRow.OwnerAccountID,
		&conversationRow.Settings.OnlyAdminsCanWrite,
		&conversationRow.Settings.OnlyAdminsCanAddMembers,
		&conversationRow.Settings.AllowReactions,
		&conversationRow.Settings.AllowForwards,
		&conversationRow.Settings.AllowThreads,
		&conversationRow.Settings.RequireEncryptedMessages,
		&conversationRow.Settings.RequireJoinApproval,
		&conversationRow.Settings.PinnedMessagesOnlyAdmins,
		&conversationRow.Settings.SlowModeInterval,
		&conversationRow.Archived,
		&conversationRow.Muted,
		&conversationRow.Pinned,
		&conversationRow.Hidden,
		&conversationRow.LastSequence,
		&conversationRow.CreatedAt,
		&conversationRow.UpdatedAt,
		&lastMessageAt,
	); err != nil {
		return conversation.Conversation{}, err
	}

	conversationRow.AvatarMediaID = avatarMediaID
	conversationRow.CreatedAt = conversationRow.CreatedAt.UTC()
	conversationRow.UpdatedAt = conversationRow.UpdatedAt.UTC()
	conversationRow.LastMessageAt = decodeTime(lastMessageAt)

	return conversationRow, nil
}

func scanTopic(row rowScanner) (conversation.ConversationTopic, error) {
	var (
		topic         conversation.ConversationTopic
		rootMessageID sql.NullString
		lastMessageAt sql.NullTime
		archivedAt    sql.NullTime
		closedAt      sql.NullTime
	)

	if err := row.Scan(
		&topic.ConversationID,
		&topic.ID,
		&rootMessageID,
		&topic.Title,
		&topic.CreatedByAccountID,
		&topic.IsGeneral,
		&topic.Archived,
		&topic.Pinned,
		&topic.Closed,
		&topic.LastSequence,
		&topic.MessageCount,
		&topic.CreatedAt,
		&topic.UpdatedAt,
		&lastMessageAt,
		&archivedAt,
		&closedAt,
	); err != nil {
		return conversation.ConversationTopic{}, err
	}

	topic.RootMessageID = rootMessageID.String
	topic.CreatedAt = topic.CreatedAt.UTC()
	topic.UpdatedAt = topic.UpdatedAt.UTC()
	topic.LastMessageAt = decodeTime(lastMessageAt)
	topic.ArchivedAt = decodeTime(archivedAt)
	topic.ClosedAt = decodeTime(closedAt)
	return topic, nil
}

func scanConversationMember(row rowScanner) (conversation.ConversationMember, error) {
	var (
		member    conversation.ConversationMember
		invitedBy sql.NullString
		leftAt    sql.NullTime
	)

	if err := row.Scan(
		&member.ConversationID,
		&member.AccountID,
		&member.Role,
		&invitedBy,
		&member.Muted,
		&member.Banned,
		&member.JoinedAt,
		&leftAt,
	); err != nil {
		return conversation.ConversationMember{}, err
	}

	member.InvitedByAccountID = invitedBy.String
	member.JoinedAt = member.JoinedAt.UTC()
	member.LeftAt = decodeTime(leftAt)

	return member, nil
}

func scanReaction(row rowScanner) (conversation.MessageReaction, error) {
	var reaction conversation.MessageReaction

	if err := row.Scan(
		&reaction.MessageID,
		&reaction.AccountID,
		&reaction.Reaction,
		&reaction.CreatedAt,
		&reaction.UpdatedAt,
	); err != nil {
		return conversation.MessageReaction{}, err
	}

	reaction.CreatedAt = reaction.CreatedAt.UTC()
	reaction.UpdatedAt = reaction.UpdatedAt.UTC()
	return reaction, nil
}

func scanMessage(row rowScanner) (conversation.Message, error) {
	var (
		message             conversation.Message
		keyID               string
		algorithm           string
		nonce               []byte
		ciphertext          []byte
		aad                 []byte
		rawPayloadMetadata  string
		rawMetadata         string
		replyConversationID sql.NullString
		replyMessageID      sql.NullString
		replySenderID       sql.NullString
		replyKind           sql.NullString
		replySnippet        sql.NullString
		editedAt            sql.NullTime
		deletedAt           sql.NullTime
	)

	if err := row.Scan(
		&message.ID,
		&message.ConversationID,
		&message.SenderAccountID,
		&message.SenderDeviceID,
		&message.ClientMessageID,
		&message.Sequence,
		&message.Kind,
		&message.Status,
		&keyID,
		&algorithm,
		&nonce,
		&ciphertext,
		&aad,
		&rawPayloadMetadata,
		&replyConversationID,
		&replyMessageID,
		&replySenderID,
		&replyKind,
		&replySnippet,
		&message.ThreadID,
		&message.Silent,
		&message.Pinned,
		&message.DisableLinkPreviews,
		&message.ViewCount,
		&rawMetadata,
		&message.CreatedAt,
		&message.UpdatedAt,
		&editedAt,
		&deletedAt,
	); err != nil {
		return conversation.Message{}, err
	}

	payload, err := decodePayload(keyID, algorithm, nonce, ciphertext, aad, rawPayloadMetadata)
	if err != nil {
		return conversation.Message{}, err
	}

	message.Payload = payload
	message.ReplyTo = conversation.MessageReference{
		ConversationID:  replyConversationID.String,
		MessageID:       replyMessageID.String,
		SenderAccountID: replySenderID.String,
		MessageKind:     conversation.MessageKind(replyKind.String),
		Snippet:         replySnippet.String,
	}
	message.CreatedAt = message.CreatedAt.UTC()
	message.UpdatedAt = message.UpdatedAt.UTC()
	message.EditedAt = decodeTime(editedAt)
	message.DeletedAt = decodeTime(deletedAt)
	message.Metadata, err = decodeMetadata(rawMetadata)
	if err != nil {
		return conversation.Message{}, err
	}
	return message, nil
}

func scanReadState(row rowScanner) (conversation.ReadState, error) {
	var (
		state     conversation.ReadState
		updatedAt sql.NullTime
	)

	if err := row.Scan(
		&state.ConversationID,
		&state.AccountID,
		&state.DeviceID,
		&state.LastReadSequence,
		&state.LastDeliveredSequence,
		&state.LastAckedSequence,
		&updatedAt,
	); err != nil {
		return conversation.ReadState{}, err
	}

	state.UpdatedAt = decodeTime(updatedAt)
	return state, nil
}

func scanSyncState(row rowScanner) (conversation.SyncState, error) {
	var (
		state         conversation.SyncState
		rawWatermarks string
		serverTime    sql.NullTime
		updatedAt     sql.NullTime
	)

	if err := row.Scan(
		&state.DeviceID,
		&state.AccountID,
		&state.LastAppliedSequence,
		&state.LastAckedSequence,
		&rawWatermarks,
		&serverTime,
		&updatedAt,
	); err != nil {
		return conversation.SyncState{}, err
	}

	watermarks, err := decodeWatermarks(rawWatermarks)
	if err != nil {
		return conversation.SyncState{}, err
	}

	state.ConversationWatermarks = watermarks
	state.ServerTime = decodeTime(serverTime)
	return state, nil
}

func scanEvent(row rowScanner) (conversation.EventEnvelope, error) {
	var (
		event              conversation.EventEnvelope
		actorDeviceID      sql.NullString
		messageID          sql.NullString
		keyID              string
		algorithm          string
		nonce              []byte
		ciphertext         []byte
		aad                []byte
		rawPayloadMetadata string
		rawMetadata        string
	)

	if err := row.Scan(
		&event.Sequence,
		&event.EventID,
		&event.EventType,
		&event.ConversationID,
		&event.ActorAccountID,
		&actorDeviceID,
		&event.CausationID,
		&event.CorrelationID,
		&messageID,
		&event.PayloadType,
		&keyID,
		&algorithm,
		&nonce,
		&ciphertext,
		&aad,
		&rawPayloadMetadata,
		&event.ReadThroughSequence,
		&rawMetadata,
		&event.CreatedAt,
	); err != nil {
		return conversation.EventEnvelope{}, err
	}

	event.ActorDeviceID = actorDeviceID.String
	event.MessageID = messageID.String
	payload, err := decodePayload(keyID, algorithm, nonce, ciphertext, aad, rawPayloadMetadata)
	if err != nil {
		return conversation.EventEnvelope{}, err
	}

	event.Payload = payload
	event.Metadata, err = decodeMetadata(rawMetadata)
	if err != nil {
		return conversation.EventEnvelope{}, err
	}
	event.CreatedAt = event.CreatedAt.UTC()
	return event, nil
}
