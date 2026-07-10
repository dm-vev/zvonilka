ALTER TABLE {{schema}}.identity_accounts
    ADD COLUMN IF NOT EXISTS avatar_media_id TEXT NULL;

ALTER TABLE {{schema}}.identity_accounts
    ADD COLUMN IF NOT EXISTS username_changed_at TIMESTAMPTZ NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'identity_accounts_avatar_media_fk'
          AND conrelid = '{{schema}}.identity_accounts'::regclass
    ) THEN
        ALTER TABLE {{schema}}.identity_accounts
            ADD CONSTRAINT identity_accounts_avatar_media_fk
            FOREIGN KEY (avatar_media_id)
            REFERENCES {{schema}}.media_assets(id)
            ON DELETE RESTRICT;
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS identity_accounts_avatar_media_idx
    ON {{schema}}.identity_accounts(avatar_media_id)
    WHERE avatar_media_id IS NOT NULL;
