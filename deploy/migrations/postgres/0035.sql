ALTER TABLE {{schema}}.call_calls
  ADD COLUMN IF NOT EXISTS host_account_id TEXT;

UPDATE {{schema}}.call_calls
SET host_account_id = initiator_account_id
WHERE host_account_id IS NULL;

ALTER TABLE {{schema}}.call_calls
  ALTER COLUMN host_account_id SET NOT NULL;

