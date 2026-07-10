ALTER TABLE {{schema}}.media_assets
	ADD COLUMN IF NOT EXISTS public_access BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS media_assets_ready_sha256_idx
	ON {{schema}}.media_assets(sha256_hex)
	WHERE status = 'ready';
