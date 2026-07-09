ALTER TABLE {{schema}}.call_participants
	ADD COLUMN IF NOT EXISTS screen_share_enabled BOOLEAN NOT NULL DEFAULT FALSE;
