CREATE TABLE IF NOT EXISTS {{schema}}.bot_public_ids (
	public_id BIGSERIAL PRIMARY KEY,
	entity_kind TEXT NOT NULL,
	internal_id TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (entity_kind, internal_id),
	CHECK (entity_kind IN ('account', 'chat', 'message', 'topic')),
	CHECK (btrim(internal_id) <> '')
);

DO $$
BEGIN
	IF to_regclass('bot.bot_public_ids') IS NOT NULL THEN
		EXECUTE 'INSERT INTO {{schema}}.bot_public_ids (public_id, entity_kind, internal_id, created_at) '
			|| 'SELECT public_id, entity_kind, internal_id, created_at FROM bot.bot_public_ids '
			|| 'ON CONFLICT (entity_kind, internal_id) DO NOTHING';
	END IF;
END
$$;

SELECT setval(
	pg_get_serial_sequence('{{schema}}.bot_public_ids', 'public_id'),
	GREATEST(COALESCE((SELECT MAX(public_id) FROM {{schema}}.bot_public_ids), 1), 1),
	true
);
