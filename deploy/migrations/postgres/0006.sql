ALTER TABLE {{schema}}.conversation_conversations
	ADD CONSTRAINT conversation_conversations_kind_check
		CHECK (kind IN ('direct', 'group', 'channel', 'saved_messages')),
	ADD CONSTRAINT conversation_conversations_slow_mode_check
		CHECK (slow_mode_interval_nanos >= 0);

ALTER TABLE {{schema}}.conversation_members
	ADD CONSTRAINT conversation_members_role_check
		CHECK (role IN ('owner', 'admin', 'member', 'guest'));

ALTER TABLE {{schema}}.conversation_messages
	ADD CONSTRAINT conversation_messages_kind_check
		CHECK (kind IN ('text', 'image', 'video', 'document', 'voice', 'sticker', 'gif', 'system')),
	ADD CONSTRAINT conversation_messages_status_check
		CHECK (status IN ('pending', 'sent', 'delivered', 'read', 'failed', 'deleted'));

ALTER TABLE {{schema}}.conversation_message_attachments
	ADD CONSTRAINT conversation_message_attachments_kind_check
		CHECK (kind IN ('image', 'video', 'document', 'voice', 'sticker', 'gif', 'avatar', 'file'));

ALTER TABLE {{schema}}.conversation_events
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
			'sync.acknowledged',
			'topic.created',
			'topic.updated',
			'topic.archived',
			'topic.pinned',
			'topic.closed'
		)),
	ADD CONSTRAINT conversation_events_actor_device_check
		CHECK (actor_device_id IS NULL OR actor_device_id <> '');
