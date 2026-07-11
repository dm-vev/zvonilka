ALTER TABLE {{schema}}.user_privacy
	ADD COLUMN IF NOT EXISTS privacy_rules JSONB NOT NULL DEFAULT '{}',
	ADD COLUMN IF NOT EXISTS show_read_date BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE {{schema}}.user_privacy
	ADD CONSTRAINT user_privacy_rules_object_check
	CHECK (jsonb_typeof(privacy_rules) = 'object');
