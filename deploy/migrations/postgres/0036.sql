ALTER TABLE {{schema}}.call_calls
  ADD COLUMN IF NOT EXISTS stage_mode_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS pinned_speaker_account_id TEXT NULL,
  ADD COLUMN IF NOT EXISTS pinned_speaker_device_id TEXT NULL;

