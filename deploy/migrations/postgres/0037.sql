ALTER TABLE {{schema}}.call_calls
  ADD COLUMN IF NOT EXISTS recording_state TEXT NOT NULL DEFAULT 'inactive',
  ADD COLUMN IF NOT EXISTS recording_started_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS recording_stopped_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS transcription_state TEXT NOT NULL DEFAULT 'inactive',
  ADD COLUMN IF NOT EXISTS transcription_started_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS transcription_stopped_at TIMESTAMPTZ NULL;

ALTER TABLE {{schema}}.call_calls
  DROP CONSTRAINT IF EXISTS call_calls_recording_state_check,
  DROP CONSTRAINT IF EXISTS call_calls_transcription_state_check;

ALTER TABLE {{schema}}.call_calls
  ADD CONSTRAINT call_calls_recording_state_check CHECK (
    recording_state IN ('inactive', 'active', 'failed')
  ),
  ADD CONSTRAINT call_calls_transcription_state_check CHECK (
    transcription_state IN ('inactive', 'active', 'failed')
  );
