CREATE TABLE IF NOT EXISTS {{schema}}.bot_game_scores (
	bot_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
	message_id TEXT NOT NULL REFERENCES {{schema}}.conversation_messages(id) ON DELETE CASCADE,
	account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
	score INTEGER NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (bot_account_id, message_id, account_id),
	CHECK (btrim(bot_account_id) <> ''),
	CHECK (btrim(message_id) <> ''),
	CHECK (btrim(account_id) <> ''),
	CHECK (score >= 0)
);

CREATE INDEX IF NOT EXISTS bot_game_scores_message_idx
	ON {{schema}}.bot_game_scores (bot_account_id, message_id, score DESC, updated_at ASC, account_id ASC);
