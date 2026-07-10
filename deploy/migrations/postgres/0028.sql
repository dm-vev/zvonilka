CREATE TABLE IF NOT EXISTS {{schema}}.bot_public_ids (
	public_id BIGSERIAL PRIMARY KEY,
	entity_kind TEXT NOT NULL,
	internal_id TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (entity_kind, internal_id),
	CHECK (entity_kind IN ('account', 'chat', 'message', 'topic')),
	CHECK (btrim(internal_id) <> '')
);
