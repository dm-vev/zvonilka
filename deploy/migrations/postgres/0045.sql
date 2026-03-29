ALTER TABLE {{schema}}.conversation_conversations
ADD COLUMN IF NOT EXISTS require_trusted_devices boolean NOT NULL DEFAULT false;

ALTER TABLE {{schema}}.conversation_moderation_policies
ADD COLUMN IF NOT EXISTS require_trusted_devices boolean NOT NULL DEFAULT false;
