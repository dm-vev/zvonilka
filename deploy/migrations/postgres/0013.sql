CREATE TABLE IF NOT EXISTS {{schema}}.user_presence (
	account_id TEXT PRIMARY KEY,
	state TEXT NOT NULL DEFAULT 'offline',
	custom_status TEXT NOT NULL DEFAULT '',
	hidden_until TIMESTAMPTZ NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CONSTRAINT user_presence_state_check CHECK (
		state IN ('offline', 'online', 'away', 'busy', 'invisible')
	)
);

CREATE INDEX IF NOT EXISTS user_presence_state_idx
	ON {{schema}}.user_presence (state, updated_at DESC, account_id ASC);

CREATE INDEX IF NOT EXISTS user_presence_hidden_until_idx
	ON {{schema}}.user_presence (hidden_until, account_id ASC);
