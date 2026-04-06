CREATE TABLE IF NOT EXISTS {{schema}}.e2ee_device_link_transfers (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
    source_device_id TEXT NOT NULL REFERENCES {{schema}}.identity_devices(id) ON DELETE CASCADE,
    target_device_id TEXT NOT NULL REFERENCES {{schema}}.identity_devices(id) ON DELETE CASCADE,
    key_id TEXT,
    algorithm TEXT NOT NULL,
    nonce BYTEA,
    ciphertext BYTEA NOT NULL,
    aad BYTEA,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT e2ee_device_link_transfers_target_unique
        UNIQUE (account_id, target_device_id)
);

CREATE INDEX IF NOT EXISTS e2ee_device_link_transfers_target_idx
    ON {{schema}}.e2ee_device_link_transfers (account_id, target_device_id, created_at DESC);
