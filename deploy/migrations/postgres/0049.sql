ALTER TABLE {{schema}}.conversation_messages
	ADD COLUMN IF NOT EXISTS deliver_at TIMESTAMPTZ NULL;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'conversation_messages_deliver_at_check'
			AND connamespace = '{{schema}}'::regnamespace
	) THEN
		ALTER TABLE {{schema}}.conversation_messages
			ADD CONSTRAINT conversation_messages_deliver_at_check
				CHECK (deliver_at IS NULL OR deliver_at >= created_at);
	END IF;
END $$;

CREATE INDEX IF NOT EXISTS conversation_messages_pending_delivery_idx
	ON {{schema}}.conversation_messages (status, deliver_at, created_at, id)
	WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS conversation_messages_sender_scheduled_idx
	ON {{schema}}.conversation_messages (conversation_id, sender_account_id, thread_id, status, deliver_at, created_at, id)
	WHERE status IN ('pending', 'failed');
