ALTER TABLE {{schema}}.conversation_events
	DROP CONSTRAINT IF EXISTS conversation_events_type_check,
	ADD CONSTRAINT conversation_events_type_check
		CHECK (event_type IN (
			'conversation.created',
			'conversation.updated',
			'conversation.members_changed',
			'user.updated',
			'admin_action.recorded',
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
