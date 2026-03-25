CREATE TABLE IF NOT EXISTS {{schema}}.notification_preferences (
	account_id TEXT PRIMARY KEY,
	enabled BOOLEAN NOT NULL DEFAULT TRUE,
	direct_enabled BOOLEAN NOT NULL DEFAULT TRUE,
	group_enabled BOOLEAN NOT NULL DEFAULT TRUE,
	channel_enabled BOOLEAN NOT NULL DEFAULT TRUE,
	mention_enabled BOOLEAN NOT NULL DEFAULT TRUE,
	reply_enabled BOOLEAN NOT NULL DEFAULT TRUE,
	quiet_hours_enabled BOOLEAN NOT NULL DEFAULT FALSE,
	quiet_hours_start_minute INTEGER NOT NULL DEFAULT 0,
	quiet_hours_end_minute INTEGER NOT NULL DEFAULT 0,
	quiet_hours_timezone TEXT NOT NULL DEFAULT '',
	muted_until TIMESTAMPTZ NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (quiet_hours_start_minute >= 0 AND quiet_hours_start_minute < 24 * 60),
	CHECK (quiet_hours_end_minute >= 0 AND quiet_hours_end_minute < 24 * 60),
	CHECK (
		NOT quiet_hours_enabled
		OR quiet_hours_timezone <> ''
	)
);

CREATE TABLE IF NOT EXISTS {{schema}}.notification_conversation_overrides (
	conversation_id TEXT NOT NULL,
	account_id TEXT NOT NULL,
	muted BOOLEAN NOT NULL DEFAULT FALSE,
	mentions_only BOOLEAN NOT NULL DEFAULT FALSE,
	muted_until TIMESTAMPTZ NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (conversation_id, account_id),
	FOREIGN KEY (conversation_id) REFERENCES {{schema}}.conversation_conversations (id) ON DELETE CASCADE,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS {{schema}}.notification_push_tokens (
	id TEXT PRIMARY KEY,
	account_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	provider TEXT NOT NULL,
	token TEXT NOT NULL,
	platform TEXT NOT NULL,
	enabled BOOLEAN NOT NULL DEFAULT TRUE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	revoked_at TIMESTAMPTZ NULL,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	UNIQUE (device_id),
	UNIQUE (token),
	CHECK (platform IN ('ios', 'android', 'web', 'desktop', 'server')),
	CHECK (
		(enabled AND revoked_at IS NULL)
		OR (NOT enabled AND revoked_at IS NOT NULL)
	)
);

CREATE INDEX IF NOT EXISTS notification_push_tokens_account_idx
	ON {{schema}}.notification_push_tokens (account_id, enabled, updated_at DESC, id ASC);

CREATE TABLE IF NOT EXISTS {{schema}}.notification_deliveries (
	id TEXT PRIMARY KEY,
	dedup_key TEXT NOT NULL,
	event_id TEXT NOT NULL,
	conversation_id TEXT NOT NULL,
	message_id TEXT NOT NULL,
	account_id TEXT NOT NULL,
	device_id TEXT NOT NULL DEFAULT '',
	push_token_id TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL,
	reason TEXT NOT NULL DEFAULT '',
	mode TEXT NOT NULL,
	state TEXT NOT NULL,
	priority INTEGER NOT NULL DEFAULT 0,
	attempts INTEGER NOT NULL DEFAULT 0,
	next_attempt_at TIMESTAMPTZ NOT NULL,
	last_attempt_at TIMESTAMPTZ NULL,
	last_error TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (conversation_id) REFERENCES {{schema}}.conversation_conversations (id) ON DELETE CASCADE,
	FOREIGN KEY (message_id) REFERENCES {{schema}}.conversation_messages (id) ON DELETE CASCADE,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	UNIQUE (dedup_key),
	CHECK (kind IN ('direct', 'group', 'channel', 'mention', 'reply')),
	CHECK (mode IN ('in_app', 'push')),
	CHECK (state IN ('queued', 'suppressed', 'delivered', 'failed')),
	CHECK (priority >= 0),
	CHECK (attempts >= 0)
);

CREATE INDEX IF NOT EXISTS notification_deliveries_due_idx
	ON {{schema}}.notification_deliveries (state, next_attempt_at, priority DESC, created_at ASC, id ASC)
	WHERE state = 'queued';

CREATE INDEX IF NOT EXISTS notification_deliveries_account_idx
	ON {{schema}}.notification_deliveries (account_id, state, next_attempt_at DESC, id ASC);

CREATE TABLE IF NOT EXISTS {{schema}}.notification_worker_cursors (
	name TEXT PRIMARY KEY,
	last_sequence BIGINT NOT NULL DEFAULT 0,
	updated_at TIMESTAMPTZ NOT NULL
);
