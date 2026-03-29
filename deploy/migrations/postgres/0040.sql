ALTER TABLE {{schema}}.call_recording_jobs
  ADD COLUMN IF NOT EXISTS owner_account_id TEXT NULL,
  ADD COLUMN IF NOT EXISTS conversation_id TEXT NULL;

ALTER TABLE {{schema}}.call_transcription_jobs
  ADD COLUMN IF NOT EXISTS owner_account_id TEXT NULL,
  ADD COLUMN IF NOT EXISTS conversation_id TEXT NULL;

ALTER TABLE {{schema}}.call_recording_jobs
  DROP CONSTRAINT IF EXISTS call_recording_jobs_owner_account_id_check,
  DROP CONSTRAINT IF EXISTS call_recording_jobs_conversation_id_check;

ALTER TABLE {{schema}}.call_recording_jobs
  ADD CONSTRAINT call_recording_jobs_owner_account_id_check CHECK (owner_account_id IS NULL OR btrim(owner_account_id) <> ''),
  ADD CONSTRAINT call_recording_jobs_conversation_id_check CHECK (conversation_id IS NULL OR btrim(conversation_id) <> '');

ALTER TABLE {{schema}}.call_transcription_jobs
  DROP CONSTRAINT IF EXISTS call_transcription_jobs_owner_account_id_check,
  DROP CONSTRAINT IF EXISTS call_transcription_jobs_conversation_id_check;

ALTER TABLE {{schema}}.call_transcription_jobs
  ADD CONSTRAINT call_transcription_jobs_owner_account_id_check CHECK (owner_account_id IS NULL OR btrim(owner_account_id) <> ''),
  ADD CONSTRAINT call_transcription_jobs_conversation_id_check CHECK (conversation_id IS NULL OR btrim(conversation_id) <> '');

CREATE INDEX IF NOT EXISTS call_recording_jobs_pending_idx
  ON {{schema}}.call_recording_jobs (updated_at, call_id)
  WHERE state = 'inactive' AND stopped_at IS NOT NULL AND output_media_id IS NULL;

CREATE INDEX IF NOT EXISTS call_transcription_jobs_pending_idx
  ON {{schema}}.call_transcription_jobs (updated_at, call_id)
  WHERE state = 'inactive' AND stopped_at IS NOT NULL AND transcript_media_id IS NULL;
