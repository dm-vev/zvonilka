ALTER TABLE {{schema}}.federation_bundles
	ADD COLUMN IF NOT EXISTS integrity_hash TEXT NOT NULL DEFAULT '',
	ADD COLUMN IF NOT EXISTS auth_tag TEXT NOT NULL DEFAULT '';

ALTER TABLE {{schema}}.federation_bundle_fragments
	ADD COLUMN IF NOT EXISTS integrity_hash TEXT NOT NULL DEFAULT '',
	ADD COLUMN IF NOT EXISTS auth_tag TEXT NOT NULL DEFAULT '';
