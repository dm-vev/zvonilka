CREATE TABLE IF NOT EXISTS {{schema}}.bot_commands (
	bot_account_id TEXT NOT NULL,
	scope_type TEXT NOT NULL,
	scope_chat_id TEXT NOT NULL DEFAULT '',
	scope_user_id TEXT NOT NULL DEFAULT '',
	language_code TEXT NOT NULL DEFAULT '',
	commands JSONB NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (bot_account_id, scope_type, scope_chat_id, scope_user_id, language_code),
	FOREIGN KEY (bot_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (bot_account_id <> ''),
	CHECK (scope_type IN (
		'default',
		'all_private_chats',
		'all_group_chats',
		'all_chat_administrators',
		'chat',
		'chat_administrators',
		'chat_member'
	)),
	CHECK (
		(scope_type IN ('default', 'all_private_chats', 'all_group_chats', 'all_chat_administrators') AND scope_chat_id = '' AND scope_user_id = '')
		OR (scope_type IN ('chat', 'chat_administrators') AND scope_chat_id <> '' AND scope_user_id = '')
		OR (scope_type = 'chat_member' AND scope_chat_id <> '' AND scope_user_id <> '')
	),
	CHECK (language_code = lower(language_code)),
	CHECK (jsonb_typeof(commands) = 'array')
);

CREATE TABLE IF NOT EXISTS {{schema}}.bot_menu_buttons (
	bot_account_id TEXT NOT NULL,
	chat_id TEXT NOT NULL DEFAULT '',
	button JSONB NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (bot_account_id, chat_id),
	FOREIGN KEY (bot_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (bot_account_id <> ''),
	CHECK (jsonb_typeof(button) = 'object')
);
