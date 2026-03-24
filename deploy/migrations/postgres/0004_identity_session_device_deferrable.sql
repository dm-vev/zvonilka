ALTER TABLE {{schema}}.identity_sessions
	ALTER CONSTRAINT identity_sessions_device_id_fkey
		DEFERRABLE INITIALLY DEFERRED;
