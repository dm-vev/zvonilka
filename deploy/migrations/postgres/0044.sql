CREATE TABLE IF NOT EXISTS {{schema}}.e2ee_device_trust (
    observer_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
    observer_device_id TEXT NOT NULL REFERENCES {{schema}}.identity_devices(id) ON DELETE CASCADE,
    target_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
    target_device_id TEXT NOT NULL REFERENCES {{schema}}.identity_devices(id) ON DELETE CASCADE,
    state TEXT NOT NULL,
    key_fingerprint TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (observer_account_id, observer_device_id, target_account_id, target_device_id),
    CONSTRAINT e2ee_device_trust_state_check
        CHECK (state IN ('trusted', 'untrusted', 'compromised'))
);

CREATE INDEX IF NOT EXISTS e2ee_device_trust_observer_idx
    ON {{schema}}.e2ee_device_trust (observer_account_id, observer_device_id, updated_at DESC);
