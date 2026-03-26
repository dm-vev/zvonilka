CREATE TABLE IF NOT EXISTS {{schema}}.bot_webhooks (
	bot_account_id TEXT PRIMARY KEY,
	url TEXT NOT NULL,
	secret_token TEXT NOT NULL DEFAULT '',
	allowed_updates JSONB NOT NULL DEFAULT '[]'::jsonb,
	max_connections INTEGER NOT NULL DEFAULT 40,
	last_error_message TEXT NOT NULL DEFAULT '',
	last_error_at TIMESTAMPTZ NULL,
	last_success_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (bot_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (bot_account_id <> ''),
	CHECK (url <> ''),
	CHECK (max_connections > 0),
	CHECK (jsonb_typeof(allowed_updates) = 'array')
);

CREATE TABLE IF NOT EXISTS {{schema}}.bot_updates (
	update_id BIGSERIAL PRIMARY KEY,
	bot_account_id TEXT NOT NULL,
	event_id TEXT NOT NULL,
	update_type TEXT NOT NULL,
	payload JSONB NOT NULL,
	attempts INTEGER NOT NULL DEFAULT 0,
	next_attempt_at TIMESTAMPTZ NOT NULL,
	last_error TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (bot_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	UNIQUE (bot_account_id, event_id, update_type),
	CHECK (bot_account_id <> ''),
	CHECK (event_id <> ''),
	CHECK (update_type IN ('message', 'edited_message', 'channel_post', 'edited_channel_post')),
	CHECK (attempts >= 0),
	CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX IF NOT EXISTS bot_updates_bot_idx
	ON {{schema}}.bot_updates (bot_account_id, update_id ASC);

CREATE INDEX IF NOT EXISTS bot_updates_due_idx
	ON {{schema}}.bot_updates (bot_account_id, next_attempt_at ASC, update_id ASC);

CREATE TABLE IF NOT EXISTS {{schema}}.bot_worker_cursors (
	name TEXT PRIMARY KEY,
	last_sequence BIGINT NOT NULL DEFAULT 0,
	updated_at TIMESTAMPTZ NOT NULL,
	CHECK (name <> ''),
	CHECK (name = lower(name)),
	CHECK (last_sequence >= 0)
);

