ALTER TABLE {{schema}}.identity_accounts
	ADD CONSTRAINT identity_accounts_kind_check
		CHECK (kind IN ('user', 'bot')),
	ADD CONSTRAINT identity_accounts_status_check
		CHECK (status IN ('active', 'suspended', 'revoked')),
	ADD CONSTRAINT identity_accounts_bot_token_hash_check
		CHECK (
			(kind = 'bot' AND bot_token_hash <> '')
			OR (kind = 'user' AND bot_token_hash = '')
		);

ALTER TABLE {{schema}}.identity_join_requests
	ADD CONSTRAINT identity_join_requests_status_check
		CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled', 'expired'));

ALTER TABLE {{schema}}.identity_login_challenges
	ADD CONSTRAINT identity_login_challenges_account_kind_check
		CHECK (account_kind IN ('user', 'bot')),
	ADD CONSTRAINT identity_login_challenges_delivery_channel_check
		CHECK (delivery_channel IN ('sms', 'email', 'push', 'manual')),
	ADD CONSTRAINT identity_login_challenges_used_pair_check
		CHECK ((used AND used_at IS NOT NULL) OR (NOT used AND used_at IS NULL));

ALTER TABLE {{schema}}.identity_devices
	ADD CONSTRAINT identity_devices_platform_check
		CHECK (platform IN ('ios', 'android', 'web', 'desktop', 'server')),
	ADD CONSTRAINT identity_devices_status_check
		CHECK (status IN ('active', 'suspended', 'revoked', 'unverified')),
	-- Deferred so the service can write the device before the session inside one tx.
	ADD CONSTRAINT identity_devices_session_id_fkey
		FOREIGN KEY (session_id) REFERENCES {{schema}}.identity_sessions (id)
		DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE {{schema}}.identity_sessions
	ADD CONSTRAINT identity_sessions_status_check
		CHECK (status IN ('active', 'revoked'));

CREATE INDEX IF NOT EXISTS identity_join_requests_pending_expires_at_idx
	ON {{schema}}.identity_join_requests (expires_at, id)
	WHERE status = 'pending';
