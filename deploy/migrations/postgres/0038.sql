CREATE TABLE IF NOT EXISTS {{schema}}.call_worker_cursors (
  name TEXT PRIMARY KEY,
  last_sequence BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE {{schema}}.call_worker_cursors
  DROP CONSTRAINT IF EXISTS call_worker_cursors_name_check,
  DROP CONSTRAINT IF EXISTS call_worker_cursors_last_sequence_check,
  DROP CONSTRAINT IF EXISTS call_worker_cursors_name_lower_check;

ALTER TABLE {{schema}}.call_worker_cursors
  ADD CONSTRAINT call_worker_cursors_name_check CHECK (btrim(name) <> ''),
  ADD CONSTRAINT call_worker_cursors_last_sequence_check CHECK (last_sequence >= 0),
  ADD CONSTRAINT call_worker_cursors_name_lower_check CHECK (name = lower(name));
