ALTER TABLE {{schema}}.federation_links
	ADD COLUMN IF NOT EXISTS allowed_event_families TEXT NOT NULL DEFAULT '[]';
