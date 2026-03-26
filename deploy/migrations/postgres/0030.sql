CREATE TABLE IF NOT EXISTS {{schema}}.bot_profiles (
    bot_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
    profile_kind TEXT NOT NULL,
    language_code TEXT NOT NULL DEFAULT '',
    value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (bot_account_id, profile_kind, language_code),
    CHECK (profile_kind IN ('name', 'description', 'short_description')),
    CHECK (btrim(language_code) = language_code),
    CHECK (btrim(value) <> '')
);

CREATE TABLE IF NOT EXISTS {{schema}}.bot_default_admin_rights (
    bot_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
    for_channels BOOLEAN NOT NULL,
    rights JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (bot_account_id, for_channels),
    CHECK (jsonb_typeof(rights) = 'object')
);
