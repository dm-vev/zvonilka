CREATE TABLE IF NOT EXISTS {{schema}}.user_privacy (
	account_id TEXT PRIMARY KEY,
	phone_visibility TEXT NOT NULL DEFAULT 'contacts',
	last_seen_visibility TEXT NOT NULL DEFAULT 'contacts',
	message_privacy TEXT NOT NULL DEFAULT 'everyone',
	birthday_visibility TEXT NOT NULL DEFAULT 'nobody',
	allow_contact_sync BOOLEAN NOT NULL DEFAULT TRUE,
	allow_unknown_senders BOOLEAN NOT NULL DEFAULT TRUE,
	allow_username_search BOOLEAN NOT NULL DEFAULT TRUE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CONSTRAINT user_privacy_phone_visibility_check
		CHECK (phone_visibility IN ('everyone', 'contacts', 'nobody', 'custom')),
	CONSTRAINT user_privacy_last_seen_visibility_check
		CHECK (last_seen_visibility IN ('everyone', 'contacts', 'nobody', 'custom')),
	CONSTRAINT user_privacy_message_visibility_check
		CHECK (message_privacy IN ('everyone', 'contacts', 'nobody', 'custom')),
	CONSTRAINT user_privacy_birthday_visibility_check
		CHECK (birthday_visibility IN ('everyone', 'contacts', 'nobody', 'custom'))
);

CREATE TABLE IF NOT EXISTS {{schema}}.user_contacts (
	owner_account_id TEXT NOT NULL,
	contact_account_id TEXT NOT NULL,
	display_name TEXT NOT NULL DEFAULT '',
	username TEXT NOT NULL DEFAULT '',
	phone_hash TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL,
	starred BOOLEAN NOT NULL DEFAULT FALSE,
	raw_contact_id TEXT NOT NULL DEFAULT '',
	source_device_id TEXT NOT NULL DEFAULT '',
	sync_checksum TEXT NOT NULL DEFAULT '',
	added_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (owner_account_id, contact_account_id),
	FOREIGN KEY (owner_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (contact_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CONSTRAINT user_contacts_source_check
		CHECK (source IN ('manual', 'imported', 'synced', 'invited')),
	CONSTRAINT user_contacts_distinct_accounts_check
		CHECK (owner_account_id <> contact_account_id)
);

CREATE INDEX IF NOT EXISTS user_contacts_owner_idx
	ON {{schema}}.user_contacts (owner_account_id, starred DESC, display_name, contact_account_id);

CREATE INDEX IF NOT EXISTS user_contacts_sync_idx
	ON {{schema}}.user_contacts (owner_account_id, source, source_device_id, contact_account_id);

CREATE TABLE IF NOT EXISTS {{schema}}.user_blocks (
	owner_account_id TEXT NOT NULL,
	blocked_account_id TEXT NOT NULL,
	reason TEXT NOT NULL DEFAULT '',
	blocked_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (owner_account_id, blocked_account_id),
	FOREIGN KEY (owner_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (blocked_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CONSTRAINT user_blocks_distinct_accounts_check
		CHECK (owner_account_id <> blocked_account_id)
);

CREATE INDEX IF NOT EXISTS user_blocks_owner_idx
	ON {{schema}}.user_blocks (owner_account_id, blocked_at, blocked_account_id);
