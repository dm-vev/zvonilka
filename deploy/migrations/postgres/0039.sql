CREATE TABLE IF NOT EXISTS {{schema}}.call_recording_jobs (
  call_id TEXT PRIMARY KEY,
  last_event_id TEXT NOT NULL,
  state TEXT NOT NULL,
  output_media_id TEXT NULL,
  started_at TIMESTAMPTZ NULL,
  stopped_at TIMESTAMPTZ NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS {{schema}}.call_transcription_jobs (
  call_id TEXT PRIMARY KEY,
  last_event_id TEXT NOT NULL,
  state TEXT NOT NULL,
  transcript_media_id TEXT NULL,
  started_at TIMESTAMPTZ NULL,
  stopped_at TIMESTAMPTZ NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

ALTER TABLE {{schema}}.call_recording_jobs
  DROP CONSTRAINT IF EXISTS call_recording_jobs_call_id_check,
  DROP CONSTRAINT IF EXISTS call_recording_jobs_last_event_id_check,
  DROP CONSTRAINT IF EXISTS call_recording_jobs_state_check;

ALTER TABLE {{schema}}.call_recording_jobs
  ADD CONSTRAINT call_recording_jobs_call_id_check CHECK (btrim(call_id) <> ''),
  ADD CONSTRAINT call_recording_jobs_last_event_id_check CHECK (btrim(last_event_id) <> ''),
  ADD CONSTRAINT call_recording_jobs_state_check CHECK (state IN ('inactive', 'active', 'failed'));

ALTER TABLE {{schema}}.call_transcription_jobs
  DROP CONSTRAINT IF EXISTS call_transcription_jobs_call_id_check,
  DROP CONSTRAINT IF EXISTS call_transcription_jobs_last_event_id_check,
  DROP CONSTRAINT IF EXISTS call_transcription_jobs_state_check;

ALTER TABLE {{schema}}.call_transcription_jobs
  ADD CONSTRAINT call_transcription_jobs_call_id_check CHECK (btrim(call_id) <> ''),
  ADD CONSTRAINT call_transcription_jobs_last_event_id_check CHECK (btrim(last_event_id) <> ''),
  ADD CONSTRAINT call_transcription_jobs_state_check CHECK (state IN ('inactive', 'active', 'failed'));
