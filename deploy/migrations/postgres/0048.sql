ALTER TABLE {{schema}}.call_recording_jobs
  ADD COLUMN IF NOT EXISTS last_event_sequence BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS lease_token TEXT NULL,
  ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE {{schema}}.call_transcription_jobs
  ADD COLUMN IF NOT EXISTS last_event_sequence BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS lease_token TEXT NULL,
  ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE {{schema}}.call_recording_jobs
  DROP CONSTRAINT IF EXISTS call_recording_jobs_last_event_sequence_check,
  DROP CONSTRAINT IF EXISTS call_recording_jobs_attempts_check,
  DROP CONSTRAINT IF EXISTS call_recording_jobs_lease_token_check;

ALTER TABLE {{schema}}.call_recording_jobs
  ADD CONSTRAINT call_recording_jobs_last_event_sequence_check CHECK (last_event_sequence >= 0),
  ADD CONSTRAINT call_recording_jobs_attempts_check CHECK (attempts >= 0),
  ADD CONSTRAINT call_recording_jobs_lease_token_check CHECK (lease_token IS NULL OR btrim(lease_token) <> '');

ALTER TABLE {{schema}}.call_transcription_jobs
  DROP CONSTRAINT IF EXISTS call_transcription_jobs_last_event_sequence_check,
  DROP CONSTRAINT IF EXISTS call_transcription_jobs_attempts_check,
  DROP CONSTRAINT IF EXISTS call_transcription_jobs_lease_token_check;

ALTER TABLE {{schema}}.call_transcription_jobs
  ADD CONSTRAINT call_transcription_jobs_last_event_sequence_check CHECK (last_event_sequence >= 0),
  ADD CONSTRAINT call_transcription_jobs_attempts_check CHECK (attempts >= 0),
  ADD CONSTRAINT call_transcription_jobs_lease_token_check CHECK (lease_token IS NULL OR btrim(lease_token) <> '');

CREATE INDEX IF NOT EXISTS call_recording_jobs_due_idx
  ON {{schema}}.call_recording_jobs (next_attempt_at, lease_expires_at, updated_at, call_id)
  WHERE state = 'inactive'
    AND stopped_at IS NOT NULL
    AND COALESCE(output_media_id, '') = ''
    AND COALESCE(owner_account_id, '') <> ''
    AND COALESCE(conversation_id, '') <> '';

CREATE INDEX IF NOT EXISTS call_transcription_jobs_due_idx
  ON {{schema}}.call_transcription_jobs (next_attempt_at, lease_expires_at, updated_at, call_id)
  WHERE state = 'inactive'
    AND stopped_at IS NOT NULL
    AND COALESCE(transcript_media_id, '') = ''
    AND COALESCE(owner_account_id, '') <> ''
    AND COALESCE(conversation_id, '') <> '';
