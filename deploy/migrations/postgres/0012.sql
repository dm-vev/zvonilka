CREATE TABLE IF NOT EXISTS {{schema}}.conversation_message_mentions (
	message_id TEXT NOT NULL,
	mention_index INTEGER NOT NULL,
	account_id TEXT NOT NULL,
	PRIMARY KEY (message_id, mention_index),
	UNIQUE (message_id, account_id),
	FOREIGN KEY (message_id) REFERENCES {{schema}}.conversation_messages (id) ON DELETE CASCADE,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS conversation_message_mentions_account_idx
	ON {{schema}}.conversation_message_mentions (account_id, message_id);

CREATE INDEX IF NOT EXISTS conversation_read_states_account_idx
	ON {{schema}}.conversation_read_states (account_id, conversation_id, last_read_sequence DESC);
