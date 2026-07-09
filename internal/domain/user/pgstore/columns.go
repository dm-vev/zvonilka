package pgstore

const privacyColumnList = `account_id, phone_visibility, last_seen_visibility, message_privacy, birthday_visibility, allow_contact_sync, allow_unknown_senders, allow_username_search, created_at, updated_at`

const contactColumnList = `owner_account_id, contact_account_id, display_name, username, phone_hash, source, starred, raw_contact_id, source_device_id, sync_checksum, added_at, updated_at`

const blockColumnList = `owner_account_id, blocked_account_id, reason, blocked_at, updated_at`
