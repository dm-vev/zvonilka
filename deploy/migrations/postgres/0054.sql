ALTER TABLE {{schema}}.federation_peers
	ADD COLUMN IF NOT EXISTS signing_fingerprint TEXT NOT NULL DEFAULT '',
	ADD COLUMN IF NOT EXISTS signing_secret TEXT NOT NULL DEFAULT '',
	ADD COLUMN IF NOT EXISTS previous_signing_secret TEXT NOT NULL DEFAULT '',
	ADD COLUMN IF NOT EXISTS signing_key_version BIGINT NOT NULL DEFAULT 1;

UPDATE {{schema}}.federation_peers
SET signing_secret = shared_secret
WHERE signing_secret = '';

UPDATE {{schema}}.federation_peers
SET signing_fingerprint = verification_fingerprint
WHERE signing_fingerprint = '';

UPDATE {{schema}}.federation_peers
SET signing_key_version = 1
WHERE signing_key_version <= 0;
