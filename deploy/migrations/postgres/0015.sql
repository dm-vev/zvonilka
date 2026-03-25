CREATE TABLE IF NOT EXISTS {{schema}}.search_documents (
	scope TEXT NOT NULL,
	target_id TEXT NOT NULL,
	entity_type TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	subtitle TEXT NOT NULL DEFAULT '',
	snippet TEXT NOT NULL DEFAULT '',
	search_text TEXT NOT NULL DEFAULT '',
	conversation_id TEXT NOT NULL DEFAULT '',
	message_id TEXT NOT NULL DEFAULT '',
	media_id TEXT NOT NULL DEFAULT '',
	user_id TEXT NOT NULL DEFAULT '',
	account_kind TEXT NOT NULL DEFAULT '',
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (scope, target_id),
	CONSTRAINT search_documents_scope_allowed_check CHECK (scope IN ('users', 'conversations', 'messages', 'media')),
	CONSTRAINT search_documents_scope_lower_check CHECK (scope = lower(scope)),
	CONSTRAINT search_documents_target_id_check CHECK (target_id <> ''),
	CONSTRAINT search_documents_entity_type_check CHECK (entity_type <> ''),
	CONSTRAINT search_documents_entity_type_lower_check CHECK (entity_type = lower(entity_type)),
	CONSTRAINT search_documents_search_text_check CHECK (search_text <> ''),
	CONSTRAINT search_documents_account_kind_lower_check CHECK (account_kind = lower(account_kind)),
	CONSTRAINT search_documents_time_order_check CHECK (created_at <= updated_at),
	CONSTRAINT search_documents_scope_reference_check CHECK (
		(scope <> 'users' OR (user_id <> '' AND user_id = target_id))
		AND (scope <> 'conversations' OR (conversation_id <> '' AND conversation_id = target_id))
		AND (scope <> 'messages' OR (conversation_id <> '' AND message_id <> '' AND message_id = target_id))
		AND (scope <> 'media' OR (media_id <> '' AND user_id <> '' AND media_id = target_id))
	)
);

CREATE INDEX IF NOT EXISTS search_documents_updated_idx
	ON {{schema}}.search_documents (updated_at DESC, scope ASC, target_id ASC);

CREATE INDEX IF NOT EXISTS search_documents_conversation_idx
	ON {{schema}}.search_documents (conversation_id, updated_at DESC, target_id ASC)
	WHERE conversation_id <> '';

CREATE INDEX IF NOT EXISTS search_documents_user_idx
	ON {{schema}}.search_documents (user_id, updated_at DESC, target_id ASC)
	WHERE user_id <> '';

CREATE INDEX IF NOT EXISTS search_documents_media_idx
	ON {{schema}}.search_documents (media_id, updated_at DESC, target_id ASC)
	WHERE media_id <> '';

CREATE INDEX IF NOT EXISTS search_documents_search_text_idx
	ON {{schema}}.search_documents USING GIN (to_tsvector('simple', search_text));
