package pgstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/media"
)

// SaveMediaAsset inserts or updates a media asset row.
func (s *Store) SaveMediaAsset(ctx context.Context, asset media.MediaAsset) (media.MediaAsset, error) {
	if err := s.requireContext(ctx); err != nil {
		return media.MediaAsset{}, err
	}
	if err := s.requireStore(); err != nil {
		return media.MediaAsset{}, err
	}
	if asset.ID == "" {
		return media.MediaAsset{}, media.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveMediaAsset(ctx, asset)
	}

	var saved media.MediaAsset
	err := s.withTransaction(ctx, func(tx media.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveMediaAsset(ctx, asset)
		return saveErr
	})
	if err != nil {
		return media.MediaAsset{}, err
	}

	return saved, nil
}

func (s *Store) saveMediaAsset(ctx context.Context, asset media.MediaAsset) (media.MediaAsset, error) {
	asset.ID = strings.TrimSpace(asset.ID)
	asset.OwnerAccountID = strings.TrimSpace(asset.OwnerAccountID)
	asset.FileName = strings.TrimSpace(asset.FileName)
	asset.ContentType = strings.TrimSpace(asset.ContentType)
	asset.SHA256Hex = strings.TrimSpace(asset.SHA256Hex)
	asset.StorageProvider = strings.TrimSpace(asset.StorageProvider)
	asset.Bucket = strings.TrimSpace(asset.Bucket)
	asset.ObjectKey = strings.TrimSpace(asset.ObjectKey)
	if asset.ID == "" || asset.OwnerAccountID == "" || asset.ObjectKey == "" || asset.StorageProvider == "" || asset.Bucket == "" {
		return media.MediaAsset{}, media.ErrInvalidInput
	}
	if asset.Kind == media.MediaKindUnspecified || asset.Status == media.MediaStatusUnspecified {
		return media.MediaAsset{}, media.ErrInvalidInput
	}
	if asset.UploadExpiresAt.IsZero() {
		return media.MediaAsset{}, media.ErrInvalidInput
	}

	metadata, err := encodeMetadata(asset.Metadata)
	if err != nil {
		return media.MediaAsset{}, fmt.Errorf("encode media metadata: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, owner_account_id, kind, status, storage_provider, bucket, object_key, file_name,
	content_type, size_bytes, sha256_hex, width, height, duration_nanos, metadata,
	upload_expires_at, ready_at, deleted_at, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8,
	$9, $10, $11, $12, $13, $14, $15,
	$16, $17, $18, $19, $20
)
ON CONFLICT (id) DO UPDATE SET
	owner_account_id = EXCLUDED.owner_account_id,
	kind = EXCLUDED.kind,
	status = EXCLUDED.status,
	storage_provider = EXCLUDED.storage_provider,
	bucket = EXCLUDED.bucket,
	object_key = EXCLUDED.object_key,
	file_name = EXCLUDED.file_name,
	content_type = EXCLUDED.content_type,
	size_bytes = EXCLUDED.size_bytes,
	sha256_hex = EXCLUDED.sha256_hex,
	width = EXCLUDED.width,
	height = EXCLUDED.height,
	duration_nanos = EXCLUDED.duration_nanos,
	metadata = EXCLUDED.metadata,
	upload_expires_at = EXCLUDED.upload_expires_at,
	ready_at = EXCLUDED.ready_at,
	deleted_at = EXCLUDED.deleted_at,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("media_assets"), mediaColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		asset.ID,
		asset.OwnerAccountID,
		asset.Kind,
		asset.Status,
		asset.StorageProvider,
		asset.Bucket,
		asset.ObjectKey,
		asset.FileName,
		asset.ContentType,
		asset.SizeBytes,
		asset.SHA256Hex,
		asset.Width,
		asset.Height,
		encodeDuration(asset.Duration),
		metadata,
		encodeTime(asset.UploadExpiresAt),
		encodeTime(asset.ReadyAt),
		encodeTime(asset.DeletedAt),
		asset.CreatedAt.UTC(),
		asset.UpdatedAt.UTC(),
	)

	saved, err := scanAsset(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, media.ErrNotFound); mappedErr != nil {
			return media.MediaAsset{}, mappedErr
		}
		return media.MediaAsset{}, fmt.Errorf("save media asset %s: %w", asset.ID, err)
	}

	return saved, nil
}

// MediaAssetByID resolves a media asset by primary key.
func (s *Store) MediaAssetByID(ctx context.Context, mediaID string) (media.MediaAsset, error) {
	if err := s.requireStore(); err != nil {
		return media.MediaAsset{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return media.MediaAsset{}, err
	}
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return media.MediaAsset{}, media.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, mediaColumnList, s.table("media_assets"))
	row := s.conn().QueryRowContext(ctx, query, mediaID)
	asset, err := scanAsset(row)
	if err != nil {
		if isNotFound(err) {
			return media.MediaAsset{}, media.ErrNotFound
		}
		return media.MediaAsset{}, fmt.Errorf("load media asset %s: %w", mediaID, err)
	}

	return asset, nil
}

// MediaAssetsByOwner lists media assets for one account.
func (s *Store) MediaAssetsByOwner(ctx context.Context, ownerAccountID string, limit int) ([]media.MediaAsset, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	ownerAccountID = strings.TrimSpace(ownerAccountID)
	if ownerAccountID == "" {
		return nil, media.ErrInvalidInput
	}
	if limit <= 0 {
		limit = 100
	}

	query := fmt.Sprintf(`
SELECT %s FROM %s
WHERE owner_account_id = $1
ORDER BY updated_at DESC, id ASC
LIMIT $2
`, mediaColumnList, s.table("media_assets"))
	rows, err := s.conn().QueryContext(ctx, query, ownerAccountID, limit)
	if err != nil {
		return nil, fmt.Errorf("list media assets for owner %s: %w", ownerAccountID, err)
	}
	defer rows.Close()

	assets := make([]media.MediaAsset, 0)
	for rows.Next() {
		asset, scanErr := scanAsset(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan media asset for owner %s: %w", ownerAccountID, scanErr)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate media assets for owner %s: %w", ownerAccountID, err)
	}

	return assets, nil
}

// MediaAssetByObjectKey resolves a media asset by object key.
func (s *Store) MediaAssetByObjectKey(ctx context.Context, objectKey string) (media.MediaAsset, error) {
	if err := s.requireStore(); err != nil {
		return media.MediaAsset{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return media.MediaAsset{}, err
	}
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return media.MediaAsset{}, media.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE object_key = $1`, mediaColumnList, s.table("media_assets"))
	row := s.conn().QueryRowContext(ctx, query, objectKey)
	asset, err := scanAsset(row)
	if err != nil {
		if isNotFound(err) {
			return media.MediaAsset{}, media.ErrNotFound
		}
		return media.MediaAsset{}, fmt.Errorf("load media asset %s: %w", objectKey, err)
	}

	return asset, nil
}
