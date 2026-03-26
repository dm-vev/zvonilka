package pgstore

import "strings"

const conversationColumnList = `id, kind, title, description, avatar_media_id, owner_account_id, only_admins_can_write, only_admins_can_add_members, allow_reactions, allow_forwards, allow_threads, require_encrypted_messages, require_join_approval, pinned_messages_only_admins, slow_mode_interval_nanos, archived, muted, pinned, hidden, last_sequence, created_at, updated_at, last_message_at`

const topicColumnList = `conversation_id, id, root_message_id, title, created_by_account_id, is_general, archived, pinned, closed, last_sequence, message_count, created_at, updated_at, last_message_at, archived_at, closed_at`

const conversationMemberColumnList = `conversation_id, account_id, role, invited_by_account_id, muted, banned, joined_at, left_at`

const conversationInviteColumnList = `id, conversation_id, code, created_by_account_id, allowed_roles, expires_at, max_uses, use_count, revoked, revoked_at, created_at, updated_at`

const messageColumnList = `id, conversation_id, sender_account_id, sender_device_id, client_message_id, sequence, kind, status, payload_key_id, payload_algorithm, payload_nonce, payload_ciphertext, payload_aad, payload_metadata, reply_conversation_id, reply_message_id, reply_sender_account_id, reply_kind, reply_snippet, thread_id, silent, pinned, disable_link_previews, view_count, metadata, created_at, updated_at, edited_at, deleted_at`

const attachmentColumnList = `message_id, attachment_index, media_id, kind, file_name, mime_type, size_bytes, sha256_hex, width, height, duration_seconds, caption`

const reactionColumnList = `message_id, account_id, reaction, created_at, updated_at`

const readStateColumnList = `conversation_id, account_id, device_id, last_read_sequence, last_delivered_sequence, last_acked_sequence, updated_at`

const syncStateColumnList = `device_id, account_id, last_applied_sequence, last_acked_sequence, conversation_watermarks, server_time, updated_at`

const eventColumnList = `sequence, event_id, event_type, conversation_id, actor_account_id, actor_device_id, causation_id, correlation_id, message_id, payload_type, payload_key_id, payload_algorithm, payload_nonce, payload_ciphertext, payload_aad, payload_metadata, read_through_sequence, metadata, created_at`

const moderationPolicyColumnList = `target_kind, target_id, only_admins_can_write, only_admins_can_add_members, allow_reactions, allow_forwards, allow_threads, require_encrypted_messages, require_join_approval, pinned_messages_only_admins, slow_mode_interval_nanos, anti_spam_window_nanos, anti_spam_burst_limit, shadow_mode, created_at, updated_at`

const moderationReportColumnList = `id, target_kind, target_id, reporter_account_id, target_account_id, reason, details, status, reviewed_by_account_id, reviewed_at, resolution, created_at, updated_at`

const moderationActionColumnList = `id, target_kind, target_id, actor_account_id, target_account_id, type, duration_seconds, reason, metadata, created_at`

const moderationRestrictionColumnList = `target_kind, target_id, account_id, state, applied_by_account_id, reason, expires_at, created_at, updated_at`

const moderationRateStateColumnList = `target_kind, target_id, account_id, last_write_at, window_started_at, window_count, updated_at`

func qualifyColumns(alias string, columnList string) string {
	parts := strings.Split(columnList, ", ")
	for idx, column := range parts {
		parts[idx] = alias + "." + column
	}

	return strings.Join(parts, ", ")
}
