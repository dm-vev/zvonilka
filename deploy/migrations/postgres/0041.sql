CREATE TABLE IF NOT EXISTS "{{schema}}"."e2ee_signed_prekeys" (
  account_id TEXT NOT NULL REFERENCES "{{schema}}"."identity_accounts"(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL REFERENCES "{{schema}}"."identity_devices"(id) ON DELETE CASCADE,
  key_id TEXT NOT NULL,
  algorithm TEXT NOT NULL,
  public_key BYTEA NOT NULL,
  signature BYTEA NOT NULL,
  created_at TIMESTAMPTZ NULL,
  rotated_at TIMESTAMPTZ NULL,
  expires_at TIMESTAMPTZ NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (account_id, device_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS e2ee_signed_prekeys_key_id_idx
  ON "{{schema}}"."e2ee_signed_prekeys"(account_id, device_id, key_id);

CREATE TABLE IF NOT EXISTS "{{schema}}"."e2ee_one_time_prekeys" (
  account_id TEXT NOT NULL REFERENCES "{{schema}}"."identity_accounts"(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL REFERENCES "{{schema}}"."identity_devices"(id) ON DELETE CASCADE,
  key_id TEXT NOT NULL,
  algorithm TEXT NOT NULL,
  public_key BYTEA NOT NULL,
  created_at TIMESTAMPTZ NULL,
  rotated_at TIMESTAMPTZ NULL,
  expires_at TIMESTAMPTZ NULL,
  claimed_at TIMESTAMPTZ NULL,
  claimed_by_account_id TEXT NULL,
  claimed_by_device_id TEXT NULL,
  PRIMARY KEY (account_id, device_id, key_id)
);

CREATE INDEX IF NOT EXISTS e2ee_one_time_prekeys_available_idx
  ON "{{schema}}"."e2ee_one_time_prekeys"(account_id, device_id, claimed_at, created_at);
