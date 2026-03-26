ALTER TABLE {{schema}}.bot_updates
	DROP CONSTRAINT IF EXISTS bot_updates_update_type_check;

ALTER TABLE {{schema}}.bot_updates
	ADD CONSTRAINT bot_updates_update_type_check
	CHECK (update_type IN (
		'message',
		'edited_message',
		'channel_post',
		'edited_channel_post',
		'callback_query',
		'inline_query'
	));

CREATE TABLE IF NOT EXISTS {{schema}}.bot_inline_queries (
	id TEXT PRIMARY KEY,
	bot_account_id TEXT NOT NULL,
	from_account_id TEXT NOT NULL,
	query_text TEXT NOT NULL DEFAULT '',
	query_offset TEXT NOT NULL DEFAULT '',
	chat_type TEXT NOT NULL DEFAULT '',
	answered BOOLEAN NOT NULL DEFAULT FALSE,
	results_json JSONB NOT NULL DEFAULT '[]'::jsonb,
	cache_time_seconds INTEGER NOT NULL DEFAULT 0,
	is_personal BOOLEAN NOT NULL DEFAULT FALSE,
	next_offset TEXT NOT NULL DEFAULT '',
	switch_pm_text TEXT NOT NULL DEFAULT '',
	switch_pm_param TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	answered_at TIMESTAMPTZ NULL,
	FOREIGN KEY (bot_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (from_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (id <> ''),
	CHECK (bot_account_id <> ''),
	CHECK (from_account_id <> ''),
	CHECK (cache_time_seconds >= 0),
	CHECK ((answered = FALSE AND answered_at IS NULL)
		OR (answered = TRUE AND answered_at IS NOT NULL)),
	CHECK ((switch_pm_text = '' AND switch_pm_param = '')
		OR (switch_pm_text <> '' AND switch_pm_param <> ''))
);

CREATE INDEX IF NOT EXISTS bot_inline_queries_bot_idx
	ON {{schema}}.bot_inline_queries (bot_account_id, created_at DESC);
