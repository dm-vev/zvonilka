ALTER TABLE {{schema}}.federation_links
	ADD COLUMN IF NOT EXISTS allowed_message_kinds TEXT NOT NULL DEFAULT '[]';
