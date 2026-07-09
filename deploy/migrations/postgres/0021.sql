ALTER TABLE {{schema}}.identity_login_challenges
	ADD COLUMN IF NOT EXISTS purpose TEXT NOT NULL DEFAULT 'login';

ALTER TABLE {{schema}}.identity_login_challenges
	DROP CONSTRAINT IF EXISTS identity_login_challenges_purpose_check,
	ADD CONSTRAINT identity_login_challenges_purpose_check
		CHECK (purpose IN ('login', 'recovery'));

CREATE TABLE IF NOT EXISTS {{schema}}.identity_account_credentials (
	account_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	secret_hash TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (account_id, kind),
	FOREIGN KEY (account_id) REFERENCES {{schema}}.identity_accounts (id) ON DELETE CASCADE,
	CONSTRAINT identity_account_credentials_kind_check
		CHECK (kind IN ('password', 'recovery')),
	CONSTRAINT identity_account_credentials_secret_hash_check
		CHECK (secret_hash <> '')
);
