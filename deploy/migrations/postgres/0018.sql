ALTER TABLE {{schema}}.conversation_topics
	ADD COLUMN IF NOT EXISTS root_message_id TEXT NULL;

ALTER TABLE {{schema}}.conversation_topics
	DROP CONSTRAINT IF EXISTS conversation_topics_root_message_check;

ALTER TABLE {{schema}}.conversation_topics
	ADD CONSTRAINT conversation_topics_root_message_check
		CHECK ((is_general AND root_message_id IS NULL) OR NOT is_general);
