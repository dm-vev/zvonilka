CREATE TABLE IF NOT EXISTS {{schema}}.media_assets (
	id TEXT PRIMARY KEY,
	owner_account_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	status TEXT NOT NULL,
	storage_provider TEXT NOT NULL,
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	file_name TEXT NOT NULL DEFAULT '',
	content_type TEXT NOT NULL DEFAULT '',
	size_bytes BIGINT NOT NULL DEFAULT 0,
	sha256_hex TEXT NOT NULL DEFAULT '',
	width INTEGER NOT NULL DEFAULT 0,
	height INTEGER NOT NULL DEFAULT 0,
	duration_nanos BIGINT NOT NULL DEFAULT 0,
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	upload_expires_at TIMESTAMPTZ NOT NULL,
	ready_at TIMESTAMPTZ NULL,
	deleted_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (owner_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS media_assets_object_key_key
	ON {{schema}}.media_assets (object_key);

CREATE INDEX IF NOT EXISTS media_assets_owner_idx
	ON {{schema}}.media_assets (owner_account_id, updated_at DESC, id ASC);

CREATE INDEX IF NOT EXISTS media_assets_status_idx
	ON {{schema}}.media_assets (status, updated_at DESC, id ASC);
