CREATE TABLE IF NOT EXISTS {{schema}}.conversation_moderation_policies (
	target_kind TEXT NOT NULL,
	target_id TEXT NOT NULL,
	only_admins_can_write BOOLEAN NOT NULL DEFAULT FALSE,
	only_admins_can_add_members BOOLEAN NOT NULL DEFAULT FALSE,
	allow_reactions BOOLEAN NOT NULL DEFAULT TRUE,
	allow_forwards BOOLEAN NOT NULL DEFAULT TRUE,
	allow_threads BOOLEAN NOT NULL DEFAULT TRUE,
	require_encrypted_messages BOOLEAN NOT NULL DEFAULT FALSE,
	require_join_approval BOOLEAN NOT NULL DEFAULT FALSE,
	pinned_messages_only_admins BOOLEAN NOT NULL DEFAULT FALSE,
	slow_mode_interval_nanos BIGINT NOT NULL DEFAULT 0,
	anti_spam_window_nanos BIGINT NOT NULL DEFAULT 0,
	anti_spam_burst_limit INTEGER NOT NULL DEFAULT 0,
	shadow_mode BOOLEAN NOT NULL DEFAULT FALSE,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (target_kind, target_id),
	CHECK (target_kind IN ('conversation', 'topic', 'channel')),
	CHECK (target_kind = lower(target_kind)),
	CHECK (target_id <> ''),
	CHECK (slow_mode_interval_nanos >= 0),
	CHECK (anti_spam_window_nanos >= 0),
	CHECK (anti_spam_burst_limit >= 0),
	CHECK (created_at <= updated_at)
);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_moderation_reports (
	id TEXT PRIMARY KEY,
	target_kind TEXT NOT NULL,
	target_id TEXT NOT NULL,
	reporter_account_id TEXT NOT NULL,
	target_account_id TEXT NOT NULL DEFAULT '',
	reason TEXT NOT NULL,
	details TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	reviewed_by_account_id TEXT NOT NULL DEFAULT '',
	reviewed_at TIMESTAMPTZ NULL,
	resolution TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (reporter_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (status IN ('pending', 'resolved', 'rejected')),
	CHECK (target_kind IN ('conversation', 'topic', 'channel')),
	CHECK (target_kind = lower(target_kind)),
	CHECK (id <> ''),
	CHECK (target_id <> ''),
	CHECK (reporter_account_id <> ''),
	CHECK (reason <> ''),
	CHECK (details <> '' OR details = ''),
	CHECK (resolution <> '' OR resolution = ''),
	CHECK (
		(reviewed_by_account_id = '' AND reviewed_at IS NULL)
		OR (reviewed_by_account_id <> '' AND reviewed_at IS NOT NULL)
	),
	CHECK (reviewed_at IS NULL OR reviewed_at >= created_at),
	CHECK (created_at <= updated_at)
);

CREATE INDEX IF NOT EXISTS conversation_moderation_reports_target_idx
	ON {{schema}}.conversation_moderation_reports (target_kind, target_id, created_at DESC, id ASC);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_moderation_actions (
	id TEXT PRIMARY KEY,
	target_kind TEXT NOT NULL,
	target_id TEXT NOT NULL,
	actor_account_id TEXT NOT NULL,
	target_account_id TEXT NOT NULL DEFAULT '',
	type TEXT NOT NULL,
	duration_seconds BIGINT NOT NULL DEFAULT 0,
	reason TEXT NOT NULL DEFAULT '',
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (actor_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (target_kind IN ('conversation', 'topic', 'channel')),
	CHECK (target_kind = lower(target_kind)),
	CHECK (type IN (
		'ban',
		'kick',
		'mute',
		'unmute',
		'shadow_ban',
		'shadow_unban',
		'policy_set',
		'report_set',
		'report_resolve',
		'report_reject',
		'slow_mode_set',
		'anti_spam_set'
	)),
	CHECK (type = lower(type)),
	CHECK (id <> ''),
	CHECK (target_id <> ''),
	CHECK (actor_account_id <> ''),
	CHECK (duration_seconds >= 0),
	CHECK (reason <> '' OR reason = '')
);

CREATE INDEX IF NOT EXISTS conversation_moderation_actions_target_idx
	ON {{schema}}.conversation_moderation_actions (target_kind, target_id, created_at DESC, id ASC);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_moderation_restrictions (
	target_kind TEXT NOT NULL,
	target_id TEXT NOT NULL,
	account_id TEXT NOT NULL,
	state TEXT NOT NULL,
	applied_by_account_id TEXT NOT NULL,
	reason TEXT NOT NULL DEFAULT '',
	expires_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (target_kind, target_id, account_id),
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (applied_by_account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (target_kind IN ('conversation', 'topic', 'channel')),
	CHECK (target_kind = lower(target_kind)),
	CHECK (state IN ('muted', 'banned', 'shadowed')),
	CHECK (state = lower(state)),
	CHECK (target_id <> ''),
	CHECK (account_id <> ''),
	CHECK (applied_by_account_id <> ''),
	CHECK (reason <> '' OR reason = ''),
	CHECK (expires_at IS NULL OR expires_at >= created_at),
	CHECK (created_at <= updated_at)
);

CREATE INDEX IF NOT EXISTS conversation_moderation_restrictions_target_idx
	ON {{schema}}.conversation_moderation_restrictions (target_kind, target_id, state, updated_at DESC, account_id ASC);

CREATE TABLE IF NOT EXISTS {{schema}}.conversation_moderation_rate_states (
	target_kind TEXT NOT NULL,
	target_id TEXT NOT NULL,
	account_id TEXT NOT NULL,
	last_write_at TIMESTAMPTZ NULL,
	window_started_at TIMESTAMPTZ NULL,
	window_count BIGINT NOT NULL DEFAULT 0,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (target_kind, target_id, account_id),
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (target_kind IN ('conversation', 'topic', 'channel')),
	CHECK (target_kind = lower(target_kind)),
	CHECK (target_id <> ''),
	CHECK (account_id <> ''),
	CHECK (window_count >= 0)
);

CREATE INDEX IF NOT EXISTS conversation_moderation_rate_states_target_idx
	ON {{schema}}.conversation_moderation_rate_states (target_kind, target_id, account_id, updated_at DESC);
