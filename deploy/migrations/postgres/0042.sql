CREATE TABLE IF NOT EXISTS {{schema}}.e2ee_direct_sessions (
    id TEXT PRIMARY KEY,
    initiator_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
    initiator_device_id TEXT NOT NULL REFERENCES {{schema}}.identity_devices(id) ON DELETE CASCADE,
    recipient_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
    recipient_device_id TEXT NOT NULL REFERENCES {{schema}}.identity_devices(id) ON DELETE CASCADE,
    initiator_ephemeral_key_id TEXT NOT NULL,
    initiator_ephemeral_algorithm TEXT NOT NULL,
    initiator_ephemeral_public_key BYTEA NOT NULL,
    identity_key_id TEXT NOT NULL,
    identity_key_algorithm TEXT NOT NULL,
    identity_key_public_key BYTEA NOT NULL,
    signed_prekey_id TEXT NOT NULL,
    signed_prekey_algorithm TEXT NOT NULL,
    signed_prekey_public_key BYTEA NOT NULL,
    signed_prekey_signature BYTEA NOT NULL,
    one_time_prekey_id TEXT,
    one_time_prekey_algorithm TEXT,
    one_time_prekey_public_key BYTEA,
    bootstrap_algorithm TEXT NOT NULL,
    bootstrap_nonce BYTEA,
    bootstrap_ciphertext BYTEA NOT NULL,
    bootstrap_metadata JSONB,
    state TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    acknowledged_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT e2ee_direct_sessions_state_check
        CHECK (state IN ('pending', 'acknowledged'))
);

CREATE INDEX IF NOT EXISTS e2ee_direct_sessions_recipient_idx
    ON {{schema}}.e2ee_direct_sessions (recipient_account_id, recipient_device_id, created_at DESC);
