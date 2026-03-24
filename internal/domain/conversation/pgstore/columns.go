package pgstore

const conversationColumnList = `id, kind, title, description, avatar_media_id, owner_account_id, only_admins_can_write, only_admins_can_add_members, allow_reactions, allow_forwards, allow_threads, require_join_approval, pinned_messages_only_admins, slow_mode_interval_nanos, archived, muted, pinned, hidden, last_sequence, created_at, updated_at, last_message_at`

const conversationMemberColumnList = `conversation_id, account_id, role, invited_by_account_id, muted, banned, joined_at, left_at`

const messageColumnList = `id, conversation_id, sender_account_id, sender_device_id, client_message_id, sequence, kind, status, payload_key_id, payload_algorithm, payload_nonce, payload_ciphertext, payload_aad, payload_metadata, reply_conversation_id, reply_message_id, reply_sender_account_id, reply_kind, reply_snippet, thread_id, silent, pinned, disable_link_previews, view_count, metadata, created_at, updated_at, edited_at, deleted_at`

const attachmentColumnList = `message_id, attachment_index, media_id, kind, file_name, mime_type, size_bytes, sha256_hex, width, height, duration_seconds, caption`

const readStateColumnList = `conversation_id, account_id, device_id, last_read_sequence, last_delivered_sequence, last_acked_sequence, updated_at`

const syncStateColumnList = `device_id, account_id, last_applied_sequence, last_acked_sequence, conversation_watermarks, server_time, updated_at`

const eventColumnList = `sequence, event_id, event_type, conversation_id, actor_account_id, actor_device_id, causation_id, correlation_id, message_id, payload_type, payload_key_id, payload_algorithm, payload_nonce, payload_ciphertext, payload_aad, payload_metadata, read_through_sequence, metadata, created_at`
