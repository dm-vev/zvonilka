ALTER TABLE {{schema}}.media_assets
	ADD CONSTRAINT media_assets_kind_check
		CHECK (kind IN ('image', 'video', 'document', 'voice', 'sticker', 'gif', 'avatar', 'file')),
	ADD CONSTRAINT media_assets_status_check
		CHECK (status IN ('reserved', 'ready', 'failed', 'deleted')),
	ADD CONSTRAINT media_assets_created_at_check
		CHECK (created_at > TIMESTAMPTZ '0001-01-01 00:00:00+00'),
	ADD CONSTRAINT media_assets_updated_at_check
		CHECK (updated_at > TIMESTAMPTZ '0001-01-01 00:00:00+00'),
	ADD CONSTRAINT media_assets_updated_after_created_check
		CHECK (updated_at >= created_at),
	ADD CONSTRAINT media_assets_size_check
		CHECK (size_bytes >= 0),
	ADD CONSTRAINT media_assets_width_check
		CHECK (width >= 0),
	ADD CONSTRAINT media_assets_height_check
		CHECK (height >= 0),
	ADD CONSTRAINT media_assets_duration_check
		CHECK (duration_nanos >= 0),
	ADD CONSTRAINT media_assets_storage_provider_check
		CHECK (storage_provider <> ''),
	ADD CONSTRAINT media_assets_bucket_check
		CHECK (bucket <> ''),
	ADD CONSTRAINT media_assets_object_key_check
		CHECK (object_key <> ''),
	ADD CONSTRAINT media_assets_upload_expires_check
		CHECK (
			upload_expires_at > TIMESTAMPTZ '0001-01-01 00:00:00+00'
			AND upload_expires_at >= created_at
		),
	ADD CONSTRAINT media_assets_ready_at_check
		CHECK (
			ready_at IS NULL OR (
				ready_at > TIMESTAMPTZ '0001-01-01 00:00:00+00'
				AND ready_at >= created_at
			)
		),
	ADD CONSTRAINT media_assets_ready_before_updated_check
		CHECK (
			ready_at IS NULL OR ready_at <= updated_at
		),
	ADD CONSTRAINT media_assets_deleted_at_check
		CHECK (
			deleted_at IS NULL OR (
				deleted_at > TIMESTAMPTZ '0001-01-01 00:00:00+00'
				AND deleted_at >= created_at
			)
		),
	ADD CONSTRAINT media_assets_deleted_before_updated_check
		CHECK (
			deleted_at IS NULL OR deleted_at <= updated_at
		),
	ADD CONSTRAINT media_assets_ready_state_check
		CHECK (
			status <> 'ready'
			OR ready_at IS NOT NULL
		),
	ADD CONSTRAINT media_assets_deleted_state_check
		CHECK (
			status <> 'deleted'
			OR deleted_at IS NOT NULL
		),
	ADD CONSTRAINT media_assets_ready_timestamp_state_check
		CHECK (
			status IN ('ready', 'deleted')
			OR ready_at IS NULL
		),
	ADD CONSTRAINT media_assets_deleted_timestamp_state_check
		CHECK (
			status = 'deleted'
			OR deleted_at IS NULL
		);
