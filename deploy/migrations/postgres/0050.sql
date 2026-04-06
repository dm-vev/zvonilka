CREATE TABLE IF NOT EXISTS {{schema}}.conversation_message_translations (
	message_id TEXT NOT NULL REFERENCES {{schema}}.conversation_messages(id) ON DELETE CASCADE,
	target_language TEXT NOT NULL,
	source_language TEXT NOT NULL DEFAULT '',
	provider TEXT NOT NULL DEFAULT '',
	translated_text TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (message_id, target_language),
	CHECK (target_language <> ''),
	CHECK (translated_text <> '')
);

CREATE INDEX IF NOT EXISTS conversation_message_translations_message_idx
	ON {{schema}}.conversation_message_translations (message_id, updated_at DESC);
