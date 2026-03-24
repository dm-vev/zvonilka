ALTER TABLE {{schema}}.identity_devices
	ADD CONSTRAINT identity_devices_account_session_key
		UNIQUE (account_id, id);

ALTER TABLE {{schema}}.identity_sessions
	ADD CONSTRAINT identity_sessions_account_device_key
		UNIQUE (account_id, id);

ALTER TABLE {{schema}}.identity_devices
	ADD CONSTRAINT identity_devices_account_session_fkey
		FOREIGN KEY (account_id, session_id)
		REFERENCES {{schema}}.identity_sessions (account_id, id)
		DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE {{schema}}.identity_sessions
	ADD CONSTRAINT identity_sessions_account_device_fkey
		FOREIGN KEY (account_id, device_id)
		REFERENCES {{schema}}.identity_devices (account_id, id)
		DEFERRABLE INITIALLY DEFERRED;
