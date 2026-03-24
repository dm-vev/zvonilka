CREATE TABLE IF NOT EXISTS {{schema}}.conversation_conversations (
	id TEXT PRIMARY KEY,
	kind TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	avatar_media_id TEXT NOT NULL DEFAULT '',
	owner_account_id TEXT NOT NULL,
	only_admins_can_write BOOLEAN NOT NULL DEFAULT FALSE,
	only_admins_can_add_members BOOLEAN NOT NULL DEFAULT FALSE,
	allow_reactions BOOLEAN NOT NULL DEFAULT TRUE,
	allow_forwards BOOLEAN NOT NULL DEFAULT TRUE,
	allow_threads BOOLEAN NOT NULL DEFAULT TRUE,
	require_join_approval BOOLEAN NOT NULL DEFAULT FALSE,
	pinned_messages_only_admins BOOLEAN NOT NULL DEFAULT FALSE,
	slow_mode_interval_nanos BIGINT NOT NULL DEFAULT 0,
	archived BOOLEAN NOT NULL DEFAULT FALSE,
	muted BOOLEAN NOT NULL DEFAULT FALSE,
	pinned BOOLEAN NOT NULL DEFAULT FALSE,
	hidden BOOLEAN NOT NULL DEFAULT FALSE,
	last_sequence BIGINT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_message_at TIMESTAMPTZ NULL,
	FOREIGN KEY (owner_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS conversation_conversations_owner_account_id_idx
	ON {{schema}}.conversation_conversations (owner_account_id, updated_at DESC, id ASC);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_members (
	conversation_id TEXT NOT NULL,
	account_id TEXT NOT NULL,
	role TEXT NOT NULL,
	invited_by_account_id TEXT NULL,
	muted BOOLEAN NOT NULL DEFAULT FALSE,
	banned BOOLEAN NOT NULL DEFAULT FALSE,
	joined_at TIMESTAMPTZ NOT NULL,
	left_at TIMESTAMPTZ NULL,
	PRIMARY KEY (conversation_id, account_id),
	FOREIGN KEY (conversation_id) REFERENCES {{schema}}.conversation_conversations (id) ON DELETE CASCADE,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (invited_by_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS conversation_members_account_id_idx
	ON {{schema}}.conversation_members (account_id, joined_at DESC, conversation_id ASC);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_messages (
	id TEXT PRIMARY KEY,
	conversation_id TEXT NOT NULL,
	sender_account_id TEXT NOT NULL,
	sender_device_id TEXT NOT NULL,
	client_message_id TEXT NOT NULL DEFAULT '',
	sequence BIGINT NOT NULL DEFAULT 0,
	kind TEXT NOT NULL,
	status TEXT NOT NULL,
	payload_key_id TEXT NOT NULL DEFAULT '',
	payload_algorithm TEXT NOT NULL DEFAULT '',
	payload_nonce BYTEA NOT NULL DEFAULT '\x',
	payload_ciphertext BYTEA NOT NULL DEFAULT '\x',
	payload_aad BYTEA NOT NULL DEFAULT '\x',
	payload_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	reply_conversation_id TEXT NOT NULL DEFAULT '',
	reply_message_id TEXT NOT NULL DEFAULT '',
	reply_sender_account_id TEXT NOT NULL DEFAULT '',
	reply_kind TEXT NOT NULL DEFAULT '',
	reply_snippet TEXT NOT NULL DEFAULT '',
	thread_id TEXT NOT NULL DEFAULT '',
	silent BOOLEAN NOT NULL DEFAULT FALSE,
	pinned BOOLEAN NOT NULL DEFAULT FALSE,
	disable_link_previews BOOLEAN NOT NULL DEFAULT FALSE,
	view_count BIGINT NOT NULL DEFAULT 0,
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	edited_at TIMESTAMPTZ NULL,
	deleted_at TIMESTAMPTZ NULL,
	FOREIGN KEY (conversation_id) REFERENCES {{schema}}.conversation_conversations (id) ON DELETE CASCADE,
	FOREIGN KEY (sender_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (sender_account_id, sender_device_id) REFERENCES {{schema}}.identity_devices (account_id, id)
		DEFERRABLE INITIALLY DEFERRED
);

CREATE UNIQUE INDEX IF NOT EXISTS conversation_messages_client_message_key
	ON {{schema}}.conversation_messages (conversation_id, sender_account_id, client_message_id)
	WHERE client_message_id <> '';

CREATE INDEX IF NOT EXISTS conversation_messages_conversation_sequence_idx
	ON {{schema}}.conversation_messages (conversation_id, sequence, created_at, id);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_message_attachments (
	message_id TEXT NOT NULL,
	attachment_index INTEGER NOT NULL,
	media_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	file_name TEXT NOT NULL DEFAULT '',
	mime_type TEXT NOT NULL DEFAULT '',
	size_bytes BIGINT NOT NULL DEFAULT 0,
	sha256_hex TEXT NOT NULL DEFAULT '',
	width INTEGER NOT NULL DEFAULT 0,
	height INTEGER NOT NULL DEFAULT 0,
	duration_seconds BIGINT NOT NULL DEFAULT 0,
	caption TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (message_id, attachment_index),
	FOREIGN KEY (message_id) REFERENCES {{schema}}.conversation_messages (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_read_states (
	conversation_id TEXT NOT NULL,
	account_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	last_read_sequence BIGINT NOT NULL DEFAULT 0,
	last_delivered_sequence BIGINT NOT NULL DEFAULT 0,
	last_acked_sequence BIGINT NOT NULL DEFAULT 0,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (conversation_id, account_id, device_id),
	FOREIGN KEY (conversation_id) REFERENCES {{schema}}.conversation_conversations (id) ON DELETE CASCADE,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (account_id, device_id) REFERENCES {{schema}}.identity_devices (account_id, id)
		DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX IF NOT EXISTS conversation_read_states_device_idx
	ON {{schema}}.conversation_read_states (device_id, conversation_id);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_sync_states (
	device_id TEXT PRIMARY KEY,
	account_id TEXT NOT NULL,
	last_applied_sequence BIGINT NOT NULL DEFAULT 0,
	last_acked_sequence BIGINT NOT NULL DEFAULT 0,
	conversation_watermarks JSONB NOT NULL DEFAULT '{}'::jsonb,
	server_time TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (account_id, device_id) REFERENCES {{schema}}.identity_devices (account_id, id)
		DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX IF NOT EXISTS conversation_sync_states_account_idx
	ON {{schema}}.conversation_sync_states (account_id, updated_at DESC, device_id ASC);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_events (
	sequence BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	event_id TEXT NOT NULL UNIQUE,
	event_type TEXT NOT NULL,
	conversation_id TEXT NOT NULL,
	actor_account_id TEXT NOT NULL,
	actor_device_id TEXT NULL,
	causation_id TEXT NOT NULL DEFAULT '',
	correlation_id TEXT NOT NULL DEFAULT '',
	message_id TEXT NOT NULL DEFAULT '',
	payload_type TEXT NOT NULL DEFAULT '',
	payload_key_id TEXT NOT NULL DEFAULT '',
	payload_algorithm TEXT NOT NULL DEFAULT '',
	payload_nonce BYTEA NOT NULL DEFAULT '\x',
	payload_ciphertext BYTEA NOT NULL DEFAULT '\x',
	payload_aad BYTEA NOT NULL DEFAULT '\x',
	payload_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	read_through_sequence BIGINT NOT NULL DEFAULT 0,
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (conversation_id) REFERENCES {{schema}}.conversation_conversations (id) ON DELETE CASCADE,
	FOREIGN KEY (actor_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (actor_account_id, actor_device_id) REFERENCES {{schema}}.identity_devices (account_id, id)
		DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX IF NOT EXISTS conversation_events_conversation_sequence_idx
	ON {{schema}}.conversation_events (conversation_id, sequence, created_at, event_id);
