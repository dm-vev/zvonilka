package pgstore

const accountColumnList = `id, kind, username, display_name, bio, email, phone, roles, status, bot_token_hash, created_by, created_at, updated_at, disabled_at, last_auth_at, custom_badge_emoji`

const joinRequestColumnList = `id, username, display_name, email, phone, note, status, requested_at, reviewed_at, reviewed_by, decision_reason, expires_at`

const loginChallengeColumnList = `id, account_id, account_kind, code_hash, delivery_channel, targets, expires_at, created_at, used_at, used`

const deviceColumnList = `id, account_id, session_id, name, platform, status, public_key, push_token, created_at, last_seen_at, revoked_at, last_rotated_at`

const sessionColumnList = `id, account_id, device_id, device_name, device_platform, ip_address, user_agent, status, current, created_at, last_seen_at, revoked_at`

const credentialColumnList = `session_id, account_id, device_id, kind, token_hash, expires_at, created_at, updated_at`
