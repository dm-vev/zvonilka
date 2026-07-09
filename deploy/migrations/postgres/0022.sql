CREATE TABLE IF NOT EXISTS {{schema}}.conversation_invites (
	id TEXT PRIMARY KEY,
	conversation_id TEXT NOT NULL REFERENCES {{schema}}.conversation_conversations (id) ON DELETE CASCADE,
	code TEXT NOT NULL UNIQUE,
	created_by_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts (id) ON DELETE RESTRICT,
	allowed_roles JSONB NOT NULL,
	expires_at TIMESTAMPTZ NULL,
	max_uses INTEGER NOT NULL DEFAULT 0 CHECK (max_uses >= 0),
	use_count INTEGER NOT NULL DEFAULT 0 CHECK (use_count >= 0),
	revoked BOOLEAN NOT NULL DEFAULT FALSE,
	revoked_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	CHECK (jsonb_typeof(allowed_roles) = 'array'),
	CHECK (jsonb_array_length(allowed_roles) > 0),
	CHECK ((NOT revoked AND revoked_at IS NULL) OR (revoked AND revoked_at IS NOT NULL)),
	CHECK (use_count <= max_uses OR max_uses = 0)
);

CREATE INDEX IF NOT EXISTS conversation_invites_conversation_created_idx
	ON {{schema}}.conversation_invites (conversation_id, created_at DESC, id ASC);
