CREATE TABLE IF NOT EXISTS {{schema}}.identity_session_credentials (
	session_id TEXT NOT NULL,
	account_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	token_hash TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (session_id, kind),
	UNIQUE (token_hash),
	FOREIGN KEY (session_id) REFERENCES {{schema}}.identity_sessions (id) ON DELETE CASCADE,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (device_id) REFERENCES {{schema}}.identity_devices (id) ON DELETE CASCADE,
	CONSTRAINT identity_session_credentials_kind_check
		CHECK (kind IN ('access', 'refresh')),
	CONSTRAINT identity_session_credentials_times_check
		CHECK (expires_at >= created_at AND updated_at >= created_at)
);

CREATE INDEX IF NOT EXISTS identity_session_credentials_account_expires_idx
	ON {{schema}}.identity_session_credentials (account_id, expires_at, kind);

CREATE INDEX IF NOT EXISTS identity_session_credentials_device_idx
	ON {{schema}}.identity_session_credentials (device_id, kind);
