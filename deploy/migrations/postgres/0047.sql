ALTER TABLE {{schema}}.notification_deliveries
	ADD COLUMN IF NOT EXISTS lease_token TEXT NOT NULL DEFAULT '',
	ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ NULL;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'notification_deliveries_lease_state_check'
	) THEN
		ALTER TABLE {{schema}}.notification_deliveries
			ADD CONSTRAINT notification_deliveries_lease_state_check
			CHECK (
				(lease_token = '' AND lease_expires_at IS NULL)
				OR (lease_token <> '' AND lease_expires_at IS NOT NULL)
			);
	END IF;
END $$;

CREATE INDEX IF NOT EXISTS notification_deliveries_claim_idx
	ON {{schema}}.notification_deliveries (
		state,
		next_attempt_at,
		lease_expires_at,
		priority DESC,
		created_at ASC,
		id ASC
	)
	WHERE state = 'queued';
