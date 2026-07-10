ALTER TABLE {{schema}}.conversation_messages
	ADD COLUMN IF NOT EXISTS forward_conversation_id TEXT NULL,
	ADD COLUMN IF NOT EXISTS forward_message_id TEXT NULL,
	ADD COLUMN IF NOT EXISTS forward_sender_account_id TEXT NULL,
	ADD COLUMN IF NOT EXISTS forward_kind TEXT NOT NULL DEFAULT '',
	ADD COLUMN IF NOT EXISTS forward_snippet TEXT NULL;

UPDATE {{schema}}.conversation_messages AS forwarded
SET forward_conversation_id = NULLIF(forwarded.metadata->>'forward_from_conversation_id', ''),
	forward_message_id = NULLIF(forwarded.metadata->>'forward_from_message_id', ''),
	forward_sender_account_id = source.sender_account_id,
	forward_kind = source.kind
FROM {{schema}}.conversation_messages AS source
WHERE forwarded.forward_message_id IS NULL
	AND NULLIF(forwarded.metadata->>'forward_from_conversation_id', '') = source.conversation_id
	AND NULLIF(forwarded.metadata->>'forward_from_message_id', '') = source.id;
