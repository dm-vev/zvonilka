CREATE TABLE IF NOT EXISTS {{schema}}.federation_bundle_fragments (
	id TEXT PRIMARY KEY,
	peer_id TEXT NOT NULL REFERENCES {{schema}}.federation_peers(id) ON DELETE CASCADE,
	link_id TEXT NOT NULL REFERENCES {{schema}}.federation_links(id) ON DELETE CASCADE,
	bundle_id TEXT NOT NULL,
	dedup_key TEXT NOT NULL UNIQUE,
	direction TEXT NOT NULL,
	cursor_from BIGINT NOT NULL,
	cursor_to BIGINT NOT NULL,
	event_count INTEGER NOT NULL DEFAULT 0,
	payload_type TEXT NOT NULL DEFAULT '',
	compression TEXT NOT NULL,
	fragment_index INTEGER NOT NULL,
	fragment_count INTEGER NOT NULL,
	payload BYTEA NOT NULL DEFAULT ''::bytea,
	state TEXT NOT NULL,
	lease_token TEXT NULL,
	lease_expires_at TIMESTAMPTZ NULL,
	attempt_count INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL,
	available_at TIMESTAMPTZ NOT NULL,
	acked_at TIMESTAMPTZ NULL,
	UNIQUE (bundle_id, direction, fragment_index),
	CHECK (bundle_id <> ''),
	CHECK (dedup_key <> ''),
	CHECK (direction <> ''),
	CHECK (cursor_from >= 0),
	CHECK (cursor_to >= cursor_from),
	CHECK (event_count >= 0),
	CHECK (compression <> ''),
	CHECK (fragment_index >= 0),
	CHECK (fragment_count > 0),
	CHECK (fragment_index < fragment_count),
	CHECK (attempt_count >= 0),
	CHECK (state <> '')
);

CREATE INDEX IF NOT EXISTS federation_bundle_fragments_link_state_idx
	ON {{schema}}.federation_bundle_fragments (peer_id, link_id, direction, state, available_at, lease_expires_at, cursor_to, bundle_id, fragment_index, id);

CREATE INDEX IF NOT EXISTS federation_bundle_fragments_bundle_idx
	ON {{schema}}.federation_bundle_fragments (bundle_id, direction, fragment_index, created_at, id);
