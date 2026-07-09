ALTER TABLE {{schema}}.conversation_messages
	DROP CONSTRAINT IF EXISTS conversation_messages_reply_snippet_check;
