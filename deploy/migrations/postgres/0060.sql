CREATE TABLE IF NOT EXISTS {{schema}}.emoji_reaction_catalog (
	emoji TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	active BOOLEAN NOT NULL DEFAULT TRUE,
	sort_order INTEGER NOT NULL DEFAULT 0,
	static_icon_media_id TEXT NOT NULL REFERENCES {{schema}}.media_assets(id) ON DELETE RESTRICT,
	appear_animation_media_id TEXT NOT NULL REFERENCES {{schema}}.media_assets(id) ON DELETE RESTRICT,
	select_animation_media_id TEXT NOT NULL REFERENCES {{schema}}.media_assets(id) ON DELETE RESTRICT,
	activate_animation_media_id TEXT NOT NULL REFERENCES {{schema}}.media_assets(id) ON DELETE RESTRICT,
	effect_animation_media_id TEXT NOT NULL REFERENCES {{schema}}.media_assets(id) ON DELETE RESTRICT,
	around_animation_media_id TEXT NULL REFERENCES {{schema}}.media_assets(id) ON DELETE RESTRICT,
	center_animation_media_id TEXT NULL REFERENCES {{schema}}.media_assets(id) ON DELETE RESTRICT,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL,
	CHECK (
		(around_animation_media_id IS NULL AND center_animation_media_id IS NULL)
		OR
		(around_animation_media_id IS NOT NULL AND center_animation_media_id IS NOT NULL)
	)
);

CREATE INDEX IF NOT EXISTS emoji_reaction_catalog_active_order_idx
	ON {{schema}}.emoji_reaction_catalog(active, sort_order, emoji);
