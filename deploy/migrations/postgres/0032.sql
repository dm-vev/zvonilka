CREATE TABLE IF NOT EXISTS {{schema}}.call_calls (
	call_id TEXT PRIMARY KEY,
	conversation_id TEXT NOT NULL REFERENCES {{schema}}.conversation_conversations(id) ON DELETE CASCADE,
	initiator_account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
	active_session_id TEXT NOT NULL DEFAULT '',
	requested_video BOOLEAN NOT NULL,
	state TEXT NOT NULL,
	end_reason TEXT NOT NULL DEFAULT '',
	started_at TIMESTAMPTZ NOT NULL,
	answered_at TIMESTAMPTZ NULL,
	ended_at TIMESTAMPTZ NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	CHECK (btrim(call_id) <> ''),
	CHECK (btrim(conversation_id) <> ''),
	CHECK (btrim(initiator_account_id) <> ''),
	CHECK (state IN ('ringing', 'active', 'ended')),
	CHECK (end_reason IN ('', 'cancelled', 'declined', 'missed', 'ended', 'failed')),
	CHECK ((state = 'ended' AND ended_at IS NOT NULL) OR (state <> 'ended')),
	CHECK ((state = 'active' AND answered_at IS NOT NULL) OR (state <> 'active'))
);

CREATE UNIQUE INDEX IF NOT EXISTS call_calls_active_conversation_idx
	ON {{schema}}.call_calls (conversation_id)
	WHERE state IN ('ringing', 'active');

CREATE TABLE IF NOT EXISTS {{schema}}.call_invites (
	call_id TEXT NOT NULL REFERENCES {{schema}}.call_calls(call_id) ON DELETE CASCADE,
	account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
	state TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	answered_at TIMESTAMPTZ NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (call_id, account_id),
	CHECK (btrim(account_id) <> ''),
	CHECK (state IN ('pending', 'accepted', 'declined', 'cancelled', 'expired'))
);

CREATE TABLE IF NOT EXISTS {{schema}}.call_participants (
	call_id TEXT NOT NULL REFERENCES {{schema}}.call_calls(call_id) ON DELETE CASCADE,
	account_id TEXT NOT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE CASCADE,
	device_id TEXT NOT NULL REFERENCES {{schema}}.identity_devices(id) ON DELETE CASCADE,
	state TEXT NOT NULL,
	audio_muted BOOLEAN NOT NULL,
	video_muted BOOLEAN NOT NULL,
	camera_enabled BOOLEAN NOT NULL,
	joined_at TIMESTAMPTZ NOT NULL,
	left_at TIMESTAMPTZ NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (call_id, device_id),
	CHECK (btrim(account_id) <> ''),
	CHECK (btrim(device_id) <> ''),
	CHECK (state IN ('joined', 'left'))
);

CREATE TABLE IF NOT EXISTS {{schema}}.call_events (
	event_id TEXT PRIMARY KEY,
	call_id TEXT NOT NULL REFERENCES {{schema}}.call_calls(call_id) ON DELETE CASCADE,
	conversation_id TEXT NOT NULL REFERENCES {{schema}}.conversation_conversations(id) ON DELETE CASCADE,
	event_type TEXT NOT NULL,
	actor_account_id TEXT NULL REFERENCES {{schema}}.identity_accounts(id) ON DELETE SET NULL,
	actor_device_id TEXT NULL REFERENCES {{schema}}.identity_devices(id) ON DELETE SET NULL,
	sequence BIGSERIAL NOT NULL UNIQUE,
	metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at TIMESTAMPTZ NOT NULL,
	CHECK (btrim(event_id) <> ''),
	CHECK (event_type IN (
		'call.started',
		'call.invited',
		'call.accepted',
		'call.declined',
		'call.joined',
		'call.left',
		'call.media_updated',
		'call.ended'
	)),
	CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX IF NOT EXISTS call_events_call_sequence_idx
	ON {{schema}}.call_events (call_id, sequence ASC);

CREATE INDEX IF NOT EXISTS call_events_conversation_sequence_idx
	ON {{schema}}.call_events (conversation_id, sequence ASC);
