ALTER TABLE {{schema}}.bot_updates
	DROP CONSTRAINT IF EXISTS bot_updates_update_type_check;

ALTER TABLE {{schema}}.bot_updates
	ADD CONSTRAINT bot_updates_update_type_check
	CHECK (update_type IN ('message', 'edited_message', 'channel_post', 'edited_channel_post', 'callback_query'));

CREATE TABLE IF NOT EXISTS {{schema}}.bot_callbacks (
	id TEXT PRIMARY KEY,
	bot_account_id TEXT NOT NULL,
	from_account_id TEXT NOT NULL,
	conversation_id TEXT NOT NULL,
	message_id TEXT NOT NULL,
	message_thread_id TEXT NOT NULL DEFAULT '',
	chat_instance TEXT NOT NULL,
	data TEXT NOT NULL DEFAULT '',
	answered_text TEXT NOT NULL DEFAULT '',
	answered_url TEXT NOT NULL DEFAULT '',
	show_alert BOOLEAN NOT NULL DEFAULT FALSE,
	cache_time_seconds INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	answered_at TIMESTAMPTZ NULL,
	FOREIGN KEY (bot_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (from_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (conversation_id) REFERENCES {{schema}}.conversation_conversations (id) ON DELETE CASCADE,
	FOREIGN KEY (message_id) REFERENCES {{schema}}.conversation_messages (id) ON DELETE CASCADE,
	CHECK (id <> ''),
	CHECK (bot_account_id <> ''),
	CHECK (from_account_id <> ''),
	CHECK (conversation_id <> ''),
	CHECK (message_id <> ''),
	CHECK (chat_instance <> ''),
	CHECK (cache_time_seconds >= 0),
	CHECK ((answered_at IS NULL AND answered_text = '' AND answered_url = '' AND show_alert = FALSE)
		OR answered_at IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS bot_callbacks_bot_idx
	ON {{schema}}.bot_callbacks (bot_account_id, created_at DESC);
