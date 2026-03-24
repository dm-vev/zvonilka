ALTER TABLE {{schema}}.conversation_messages
	ADD CONSTRAINT conversation_messages_edit_state_check
		CHECK (edited_at IS NULL OR edited_at >= created_at),
	ADD CONSTRAINT conversation_messages_delete_state_check
		CHECK (deleted_at IS NULL OR deleted_at >= created_at),
	ADD CONSTRAINT conversation_messages_deleted_status_check
		CHECK (deleted_at IS NULL OR status = 'deleted'),
	ADD CONSTRAINT conversation_messages_pinned_state_check
		CHECK (NOT pinned OR deleted_at IS NULL);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_message_reactions (
	message_id TEXT NOT NULL,
	account_id TEXT NOT NULL,
	reaction TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (message_id, account_id),
	FOREIGN KEY (message_id) REFERENCES {{schema}}.conversation_messages (id) ON DELETE CASCADE,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (reaction <> '')
);

CREATE INDEX IF NOT EXISTS conversation_message_reactions_message_idx
	ON {{schema}}.conversation_message_reactions (message_id, updated_at DESC, account_id ASC);

ALTER TABLE {{schema}}.conversation_events
	DROP CONSTRAINT conversation_events_type_check,
	ADD CONSTRAINT conversation_events_type_check
		CHECK (event_type IN (
			'conversation.created',
			'conversation.updated',
			'conversation.members_changed',
			'message.created',
			'message.delivered',
			'message.read',
			'message.edited',
			'message.deleted',
			'message.pinned',
			'message.reaction_added',
			'message.reaction_updated',
			'message.reaction_removed',
			'sync.acknowledged',
			'topic.created',
			'topic.updated',
			'topic.archived',
			'topic.pinned',
			'topic.closed'
		));
