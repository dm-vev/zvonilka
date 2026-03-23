CREATE SCHEMA IF NOT EXISTS {{schema}};

CREATE TABLE IF NOT EXISTS {{schema}}.identity_accounts (
	id TEXT PRIMARY KEY,
	kind TEXT NOT NULL,
	username TEXT NOT NULL,
	display_name TEXT NOT NULL,
	bio TEXT NOT NULL DEFAULT '',
	email TEXT NOT NULL DEFAULT '',
	phone TEXT NOT NULL DEFAULT '',
	roles TEXT NOT NULL DEFAULT '[]',
	status TEXT NOT NULL,
	bot_token_hash TEXT NOT NULL DEFAULT '',
	created_by TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	disabled_at TIMESTAMPTZ NULL,
	last_auth_at TIMESTAMPTZ NULL,
	custom_badge_emoji TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS identity_accounts_username_key
	ON {{schema}}.identity_accounts (username)
	WHERE username <> '';

CREATE UNIQUE INDEX IF NOT EXISTS identity_accounts_email_key
	ON {{schema}}.identity_accounts (email)
	WHERE email <> '';

CREATE UNIQUE INDEX IF NOT EXISTS identity_accounts_phone_key
	ON {{schema}}.identity_accounts (phone)
	WHERE phone <> '';

CREATE UNIQUE INDEX IF NOT EXISTS identity_accounts_bot_token_hash_key
	ON {{schema}}.identity_accounts (bot_token_hash)
	WHERE bot_token_hash <> '';

CREATE TABLE IF NOT EXISTS {{schema}}.identity_join_requests (
	id TEXT PRIMARY KEY,
	username TEXT NOT NULL,
	display_name TEXT NOT NULL,
	email TEXT NOT NULL DEFAULT '',
	phone TEXT NOT NULL DEFAULT '',
	note TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	requested_at TIMESTAMPTZ NOT NULL,
	reviewed_at TIMESTAMPTZ NULL,
	reviewed_by TEXT NOT NULL DEFAULT '',
	decision_reason TEXT NOT NULL DEFAULT '',
	expires_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS identity_join_requests_username_pending_key
	ON {{schema}}.identity_join_requests (username)
	WHERE status = 'pending' AND username <> '';

CREATE UNIQUE INDEX IF NOT EXISTS identity_join_requests_email_pending_key
	ON {{schema}}.identity_join_requests (email)
	WHERE status = 'pending' AND email <> '';

CREATE UNIQUE INDEX IF NOT EXISTS identity_join_requests_phone_pending_key
	ON {{schema}}.identity_join_requests (phone)
	WHERE status = 'pending' AND phone <> '';

CREATE INDEX IF NOT EXISTS identity_join_requests_status_requested_at_idx
	ON {{schema}}.identity_join_requests (status, requested_at, id);

CREATE TABLE IF NOT EXISTS {{schema}}.identity_login_challenges (
	id TEXT PRIMARY KEY,
	account_id TEXT NOT NULL,
	account_kind TEXT NOT NULL,
	code_hash TEXT NOT NULL,
	delivery_channel TEXT NOT NULL,
	targets TEXT NOT NULL DEFAULT '[]',
	expires_at TIMESTAMPTZ NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	used_at TIMESTAMPTZ NULL,
	used BOOLEAN NOT NULL DEFAULT FALSE,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS identity_login_challenges_account_id_idx
	ON {{schema}}.identity_login_challenges (account_id, created_at, id);

CREATE TABLE IF NOT EXISTS {{schema}}.identity_devices (
	id TEXT PRIMARY KEY,
	account_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	name TEXT NOT NULL,
	platform TEXT NOT NULL,
	status TEXT NOT NULL,
	public_key TEXT NOT NULL,
	push_token TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL,
	last_seen_at TIMESTAMPTZ NOT NULL,
	revoked_at TIMESTAMPTZ NULL,
	last_rotated_at TIMESTAMPTZ NULL,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS identity_devices_session_id_idx
	ON {{schema}}.identity_devices (session_id);

CREATE INDEX IF NOT EXISTS identity_devices_account_id_idx
	ON {{schema}}.identity_devices (account_id, created_at, id);

CREATE TABLE IF NOT EXISTS {{schema}}.identity_sessions (
	id TEXT PRIMARY KEY,
	account_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	device_name TEXT NOT NULL,
	device_platform TEXT NOT NULL,
	ip_address TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	current BOOLEAN NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	last_seen_at TIMESTAMPTZ NOT NULL,
	revoked_at TIMESTAMPTZ NULL,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (device_id) REFERENCES {{schema}}.identity_devices (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS identity_sessions_device_id_key
	ON {{schema}}.identity_sessions (device_id);

CREATE INDEX IF NOT EXISTS identity_sessions_account_id_idx
	ON {{schema}}.identity_sessions (account_id, created_at, id);
