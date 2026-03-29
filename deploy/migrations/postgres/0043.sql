CREATE TABLE IF NOT EXISTS {{schema}}.e2ee_group_sender_keys (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES {{schema}}.conversation_conversations(id) ON DELETE CASCADE,
    sender_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
    sender_device_id TEXT NOT NULL REFERENCES {{schema}}.identity_devices(id) ON DELETE CASCADE,
    recipient_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
    recipient_device_id TEXT NOT NULL REFERENCES {{schema}}.identity_devices(id) ON DELETE CASCADE,
    sender_key_id TEXT NOT NULL,
    payload_algorithm TEXT NOT NULL,
    payload_nonce BYTEA,
    payload_ciphertext BYTEA NOT NULL,
    payload_metadata JSONB,
    state TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    acknowledged_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT e2ee_group_sender_keys_state_check
        CHECK (state IN ('pending', 'acknowledged'))
);

CREATE INDEX IF NOT EXISTS e2ee_group_sender_keys_recipient_idx
    ON {{schema}}.e2ee_group_sender_keys (conversation_id, recipient_account_id, recipient_device_id, created_at DESC);
