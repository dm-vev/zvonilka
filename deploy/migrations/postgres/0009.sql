CREATE TABLE IF NOT EXISTS {{schema}}.conversation_topics (
	conversation_id TEXT NOT NULL,
	id TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	created_by_account_id TEXT NOT NULL,
	is_general BOOLEAN NOT NULL DEFAULT FALSE,
	archived BOOLEAN NOT NULL DEFAULT FALSE,
	pinned BOOLEAN NOT NULL DEFAULT FALSE,
	closed BOOLEAN NOT NULL DEFAULT FALSE,
	last_sequence BIGINT NOT NULL DEFAULT 0,
	message_count BIGINT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_message_at TIMESTAMPTZ NULL,
	archived_at TIMESTAMPTZ NULL,
	closed_at TIMESTAMPTZ NULL,
	PRIMARY KEY (conversation_id, id),
	FOREIGN KEY (conversation_id) REFERENCES {{schema}}.conversation_conversations (id) ON DELETE CASCADE,
	FOREIGN KEY (created_by_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS conversation_topics_conversation_idx
	ON {{schema}}.conversation_topics (conversation_id, is_general DESC, pinned DESC, last_sequence DESC, updated_at DESC, id ASC);

INSERT INTO {{schema}}.conversation_topics (
	conversation_id, id, title, created_by_account_id, is_general, archived, pinned, closed,
	last_sequence, message_count, created_at, updated_at, last_message_at, archived_at, closed_at
)
SELECT
	c.id,
	'',
	'General',
	c.owner_account_id,
	TRUE,
	FALSE,
	FALSE,
	FALSE,
	COALESCE(MAX(m.sequence), 0),
	COUNT(m.id),
	c.created_at,
	c.updated_at,
	MAX(m.created_at),
	NULL,
	NULL
FROM {{schema}}.conversation_conversations AS c
LEFT JOIN {{schema}}.conversation_messages AS m
	ON m.conversation_id = c.id
	AND m.thread_id = ''
GROUP BY c.id, c.owner_account_id, c.created_at, c.updated_at
ON CONFLICT (conversation_id, id) DO UPDATE SET
	title = EXCLUDED.title,
	created_by_account_id = EXCLUDED.created_by_account_id,
	is_general = EXCLUDED.is_general,
	archived = EXCLUDED.archived,
	pinned = EXCLUDED.pinned,
	closed = EXCLUDED.closed,
	last_sequence = EXCLUDED.last_sequence,
	message_count = EXCLUDED.message_count,
	updated_at = EXCLUDED.updated_at,
	last_message_at = EXCLUDED.last_message_at,
	archived_at = EXCLUDED.archived_at,
	closed_at = EXCLUDED.closed_at;

ALTER TABLE {{schema}}.conversation_messages
	ADD CONSTRAINT conversation_messages_thread_fk
	FOREIGN KEY (conversation_id, thread_id) REFERENCES {{schema}}.conversation_topics (conversation_id, id)
		ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS conversation_messages_thread_sequence_idx
	ON {{schema}}.conversation_messages (conversation_id, thread_id, sequence, created_at, id);

ALTER TABLE {{schema}}.conversation_topics
	ADD CONSTRAINT conversation_topics_is_general_check
		CHECK (is_general = (id = '')),
	ADD CONSTRAINT conversation_topics_title_check
		CHECK (title <> '' OR is_general),
	ADD CONSTRAINT conversation_topics_sequence_check
		CHECK (last_sequence >= 0 AND message_count >= 0),
	ADD CONSTRAINT conversation_topics_state_check
		CHECK (
			(archived_at IS NULL OR archived)
			AND (closed_at IS NULL OR closed)
			AND (last_message_at IS NULL OR last_message_at >= created_at)
		);
