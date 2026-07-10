ALTER TABLE {{schema}}.bot_profiles
    DROP CONSTRAINT IF EXISTS bot_profiles_profile_kind_check;

ALTER TABLE {{schema}}.bot_profiles
    ADD CONSTRAINT bot_profiles_profile_kind_check
    CHECK (profile_kind IN ('name', 'description', 'short_description', 'avatar'));
