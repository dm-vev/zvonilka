CREATE TABLE IF NOT EXISTS {{schema}}.federation_peers (
	id TEXT PRIMARY KEY,
	server_name TEXT NOT NULL UNIQUE,
	base_url TEXT NOT NULL DEFAULT '',
	capabilities TEXT NOT NULL DEFAULT '[]',
	trusted BOOLEAN NOT NULL DEFAULT FALSE,
	state TEXT NOT NULL,
	verification_fingerprint TEXT NOT NULL DEFAULT '',
	shared_secret TEXT NOT NULL,
	shared_secret_hash TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_seen_at TIMESTAMPTZ NULL,
	CHECK (server_name <> ''),
	CHECK (state <> ''),
	CHECK (shared_secret <> ''),
	CHECK (shared_secret_hash <> '')
);

CREATE TABLE IF NOT EXISTS {{schema}}.federation_links (
	id TEXT PRIMARY KEY,
	peer_id TEXT NOT NULL REFERENCES {{schema}}.federation_peers(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	endpoint TEXT NOT NULL DEFAULT '',
	transport_kind TEXT NOT NULL,
	delivery_class TEXT NOT NULL,
	discovery_mode TEXT NOT NULL,
	media_policy TEXT NOT NULL,
	state TEXT NOT NULL,
	max_bundle_bytes INTEGER NOT NULL,
	max_fragment_bytes INTEGER NOT NULL,
	allowed_conversation_kinds TEXT NOT NULL DEFAULT '[]',
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	last_healthy_at TIMESTAMPTZ NULL,
	last_error TEXT NOT NULL DEFAULT '',
	UNIQUE (peer_id, name),
	CHECK (name <> ''),
	CHECK (transport_kind <> ''),
	CHECK (delivery_class <> ''),
	CHECK (discovery_mode <> ''),
	CHECK (media_policy <> ''),
	CHECK (state <> ''),
	CHECK (max_bundle_bytes > 0),
	CHECK (max_fragment_bytes > 0),
	CHECK (max_fragment_bytes <= max_bundle_bytes)
);

CREATE TABLE IF NOT EXISTS {{schema}}.federation_bundles (
	id TEXT PRIMARY KEY,
	peer_id TEXT NOT NULL REFERENCES {{schema}}.federation_peers(id) ON DELETE CASCADE,
	link_id TEXT NOT NULL REFERENCES {{schema}}.federation_links(id) ON DELETE CASCADE,
	dedup_key TEXT NOT NULL UNIQUE,
	direction TEXT NOT NULL,
	cursor_from BIGINT NOT NULL,
	cursor_to BIGINT NOT NULL,
	event_count INTEGER NOT NULL DEFAULT 0,
	payload_type TEXT NOT NULL DEFAULT '',
	payload BYTEA NOT NULL DEFAULT ''::bytea,
	compression TEXT NOT NULL,
	state TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	available_at TIMESTAMPTZ NOT NULL,
	expires_at TIMESTAMPTZ NULL,
	acked_at TIMESTAMPTZ NULL,
	CHECK (dedup_key <> ''),
	CHECK (direction <> ''),
	CHECK (cursor_from >= 0),
	CHECK (cursor_to >= cursor_from),
	CHECK (event_count >= 0),
	CHECK (compression <> ''),
	CHECK (state <> '')
);

CREATE INDEX IF NOT EXISTS federation_bundles_link_direction_cursor_idx
	ON {{schema}}.federation_bundles (peer_id, link_id, direction, cursor_to, created_at, id);

CREATE TABLE IF NOT EXISTS {{schema}}.federation_replication_cursors (
	peer_id TEXT NOT NULL REFERENCES {{schema}}.federation_peers(id) ON DELETE CASCADE,
	link_id TEXT NOT NULL REFERENCES {{schema}}.federation_links(id) ON DELETE CASCADE,
	last_received_cursor BIGINT NOT NULL DEFAULT 0,
	last_inbound_cursor BIGINT NOT NULL DEFAULT 0,
	last_outbound_cursor BIGINT NOT NULL DEFAULT 0,
	last_acked_cursor BIGINT NOT NULL DEFAULT 0,
	updated_at TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (peer_id, link_id),
	CHECK (last_received_cursor >= 0),
	CHECK (last_inbound_cursor >= 0),
	CHECK (last_outbound_cursor >= 0),
	CHECK (last_acked_cursor >= 0)
);
