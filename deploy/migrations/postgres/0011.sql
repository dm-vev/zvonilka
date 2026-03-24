ALTER TABLE {{schema}}.conversation_conversations
	ADD COLUMN IF NOT EXISTS require_encrypted_messages BOOLEAN NOT NULL DEFAULT TRUE;

UPDATE {{schema}}.conversation_messages
	SET reply_snippet = ''
	WHERE reply_snippet <> '';

UPDATE {{schema}}.conversation_message_attachments
	SET caption = ''
	WHERE caption <> '';

ALTER TABLE {{schema}}.conversation_conversations
	ADD CONSTRAINT conversation_conversations_encryption_check
		CHECK (require_encrypted_messages);

ALTER TABLE {{schema}}.conversation_messages
	ADD CONSTRAINT conversation_messages_payload_check
		CHECK (
			payload_key_id <> ''
			AND payload_algorithm <> ''
			AND payload_nonce <> '\x'
			AND payload_ciphertext <> '\x'
		),
	ADD CONSTRAINT conversation_messages_reply_snippet_check
		CHECK (reply_snippet = '');

ALTER TABLE {{schema}}.conversation_message_attachments
	ADD CONSTRAINT conversation_message_attachments_caption_check
		CHECK (caption = '');

ALTER TABLE {{schema}}.conversation_events
	ADD CONSTRAINT conversation_events_message_payload_check
		CHECK (
			payload_type <> 'message'
			OR (
				payload_key_id <> ''
				AND payload_algorithm <> ''
				AND payload_nonce <> '\x'
				AND payload_ciphertext <> '\x'
			)
		);
