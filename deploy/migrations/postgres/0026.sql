ALTER TABLE {{schema}}.bot_updates
	DROP CONSTRAINT IF EXISTS bot_updates_update_type_check;

ALTER TABLE {{schema}}.bot_updates
	ADD CONSTRAINT bot_updates_update_type_check
	CHECK (update_type IN (
		'message',
		'edited_message',
		'channel_post',
		'edited_channel_post',
		'callback_query',
		'inline_query',
		'chat_member',
		'my_chat_member'
	));
