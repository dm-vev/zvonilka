ALTER TABLE {{schema}}.notification_conversation_overrides
	ADD COLUMN IF NOT EXISTS show_preview BOOLEAN NOT NULL DEFAULT TRUE,
	ADD COLUMN IF NOT EXISTS sound_id BIGINT NOT NULL DEFAULT -1,
	ADD COLUMN IF NOT EXISTS mute_stories BOOLEAN NOT NULL DEFAULT FALSE,
	ADD COLUMN IF NOT EXISTS story_sound_id BIGINT NOT NULL DEFAULT -1,
	ADD COLUMN IF NOT EXISTS show_story_sender BOOLEAN NOT NULL DEFAULT TRUE,
	ADD COLUMN IF NOT EXISTS disable_pinned_message_notifications BOOLEAN NOT NULL DEFAULT FALSE,
	ADD COLUMN IF NOT EXISTS disable_mention_notifications BOOLEAN NOT NULL DEFAULT FALSE,
	ADD COLUMN IF NOT EXISTS use_default_mute_for BOOLEAN NOT NULL DEFAULT TRUE,
	ADD COLUMN IF NOT EXISTS use_default_sound BOOLEAN NOT NULL DEFAULT TRUE,
	ADD COLUMN IF NOT EXISTS use_default_show_preview BOOLEAN NOT NULL DEFAULT TRUE,
	ADD COLUMN IF NOT EXISTS use_default_mute_stories BOOLEAN NOT NULL DEFAULT TRUE,
	ADD COLUMN IF NOT EXISTS use_default_story_sound BOOLEAN NOT NULL DEFAULT TRUE,
	ADD COLUMN IF NOT EXISTS use_default_show_story_sender BOOLEAN NOT NULL DEFAULT TRUE,
	ADD COLUMN IF NOT EXISTS use_default_disable_pinned_message_notifications BOOLEAN NOT NULL DEFAULT TRUE,
	ADD COLUMN IF NOT EXISTS use_default_disable_mention_notifications BOOLEAN NOT NULL DEFAULT TRUE;

UPDATE {{schema}}.notification_conversation_overrides
SET use_default_mute_for = FALSE;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conname = 'notification_conversation_overrides_sound_id_check'
			AND conrelid = '{{schema}}.notification_conversation_overrides'::regclass
	) THEN
		ALTER TABLE {{schema}}.notification_conversation_overrides
			ADD CONSTRAINT notification_conversation_overrides_sound_id_check
			CHECK (sound_id >= -1 AND story_sound_id >= -1);
	END IF;
END $$;

CREATE TABLE IF NOT EXISTS {{schema}}.notification_scope_settings (
	account_id TEXT NOT NULL,
	scope TEXT NOT NULL,
	muted_until TIMESTAMPTZ NULL,
	show_preview BOOLEAN NOT NULL DEFAULT TRUE,
	sound_id BIGINT NOT NULL DEFAULT -1,
	mute_stories BOOLEAN NOT NULL DEFAULT FALSE,
	story_sound_id BIGINT NOT NULL DEFAULT -1,
	show_story_sender BOOLEAN NOT NULL DEFAULT TRUE,
	disable_pinned_message_notifications BOOLEAN NOT NULL DEFAULT FALSE,
	disable_mention_notifications BOOLEAN NOT NULL DEFAULT FALSE,
	use_default_mute_stories BOOLEAN NOT NULL DEFAULT TRUE,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (account_id, scope),
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (scope IN ('private_chats', 'group_chats', 'channel_chats')),
	CHECK (sound_id >= -1),
	CHECK (story_sound_id >= -1)
);

CREATE TABLE IF NOT EXISTS {{schema}}.notification_reaction_settings (
	account_id TEXT PRIMARY KEY,
	message_reaction_source TEXT NOT NULL DEFAULT 'contacts',
	story_reaction_source TEXT NOT NULL DEFAULT 'contacts',
	poll_vote_source TEXT NOT NULL DEFAULT 'contacts',
	sound_id BIGINT NOT NULL DEFAULT -1,
	show_preview BOOLEAN NOT NULL DEFAULT TRUE,
	updated_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CHECK (message_reaction_source IN ('none', 'contacts', 'all')),
	CHECK (story_reaction_source IN ('none', 'contacts', 'all')),
	CHECK (poll_vote_source IN ('none', 'contacts', 'all')),
	CHECK (sound_id >= -1)
);

CREATE TABLE IF NOT EXISTS {{schema}}.notification_saved_sounds (
	sound_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	account_id TEXT NOT NULL,
	media_id TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL,
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	FOREIGN KEY (media_id) REFERENCES {{schema}}.media_assets (id) ON DELETE CASCADE,
	UNIQUE (account_id, media_id),
	CHECK (sound_id > 0),
	CHECK (media_id <> '')
);

CREATE INDEX IF NOT EXISTS notification_saved_sounds_account_idx
	ON {{schema}}.notification_saved_sounds (account_id, created_at ASC, sound_id ASC);
