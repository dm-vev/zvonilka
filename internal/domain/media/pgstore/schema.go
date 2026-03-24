package pgstore

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/media"
)

const mediaColumnList = `
	id,
	owner_account_id,
	kind,
	status,
	storage_provider,
	bucket,
	object_key,
	file_name,
	content_type,
	size_bytes,
	sha256_hex,
	width,
	height,
	duration_nanos,
	metadata,
	upload_expires_at,
	ready_at,
	deleted_at,
	created_at,
	updated_at
`

func scanAsset(row rowScanner) (media.MediaAsset, error) {
	var (
		asset        media.MediaAsset
		metadata      string
		durationNanos int64
		uploadExpires sql.NullTime
		readyAt       sql.NullTime
		deletedAt     sql.NullTime
	)

	if err := row.Scan(
		&asset.ID,
		&asset.OwnerAccountID,
		&asset.Kind,
		&asset.Status,
		&asset.StorageProvider,
		&asset.Bucket,
		&asset.ObjectKey,
		&asset.FileName,
		&asset.ContentType,
		&asset.SizeBytes,
		&asset.SHA256Hex,
		&asset.Width,
		&asset.Height,
		&durationNanos,
		&metadata,
		&uploadExpires,
		&readyAt,
		&deletedAt,
		&asset.CreatedAt,
		&asset.UpdatedAt,
	); err != nil {
		return media.MediaAsset{}, err
	}

	decoded, err := decodeMetadata(metadata)
	if err != nil {
		return media.MediaAsset{}, err
	}

	asset.Metadata = decoded
	asset.Duration = time.Duration(durationNanos) * time.Nanosecond
	asset.UploadExpiresAt = decodeTime(uploadExpires)
	asset.ReadyAt = decodeTime(readyAt)
	asset.DeletedAt = decodeTime(deletedAt)
	asset.CreatedAt = asset.CreatedAt.UTC()
	asset.UpdatedAt = asset.UpdatedAt.UTC()

	return asset, nil
}

func encodeDuration(value time.Duration) int64 {
	return int64(value)
}

func decodeTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return value.Time.UTC()
}

func encodeTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func normalizeSchema(schema string) string {
	return strings.TrimSpace(schema)
}

func qualifiedName(schema string, name string) string {
	schema = normalizeSchema(schema)
	if schema == "" {
		schema = "public"
	}

	return fmt.Sprintf(`"%s"."%s"`, strings.ReplaceAll(schema, `"`, `""`), strings.ReplaceAll(name, `"`, `""`))
}
