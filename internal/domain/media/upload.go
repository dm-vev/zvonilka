package media

import (
	"context"
	"fmt"
	"strings"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

// Upload writes a blob directly through the server and persists a ready media asset.
func (s *Service) Upload(ctx context.Context, params UploadParams) (MediaAsset, error) {
	if err := s.validateContext(ctx, "upload media"); err != nil {
		return MediaAsset{}, err
	}
	params.OwnerAccountID = strings.TrimSpace(params.OwnerAccountID)
	params.FileName = strings.TrimSpace(params.FileName)
	params.ContentType = strings.TrimSpace(params.ContentType)
	params.SHA256Hex = strings.TrimSpace(params.SHA256Hex)
	params.Metadata = normalizeMetadata(params.Metadata)
	params.Kind = normalizeAssetKind(params.Kind)
	if params.OwnerAccountID == "" || params.Kind == MediaKindUnspecified || params.FileName == "" || params.Body == nil {
		return MediaAsset{}, ErrInvalidInput
	}
	if err := ensureUploadSize(s.settings, params.SizeBytes); err != nil {
		return MediaAsset{}, err
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	mediaID, err := newID("media")
	if err != nil {
		return MediaAsset{}, fmt.Errorf("generate media id: %w", err)
	}
	objectKey := mediaObjectKey(params.OwnerAccountID, mediaID)

	object, err := s.blob.PutObject(ctx, objectKey, params.Body, int64(params.SizeBytes), domainstorage.PutObjectOptions{
		ContentType: params.ContentType,
		Metadata: uploadMetadata(ReserveUploadParams{
			OwnerAccountID: params.OwnerAccountID,
			Kind:           params.Kind,
			FileName:       params.FileName,
			ContentType:    params.ContentType,
			SizeBytes:      params.SizeBytes,
			SHA256Hex:      params.SHA256Hex,
			Duration:       params.Duration,
			Metadata:       params.Metadata,
		}),
	})
	if err != nil {
		return MediaAsset{}, fmt.Errorf("put media object %s: %w", objectKey, err)
	}
	if params.SizeBytes > 0 && uint64(object.ContentLength) != params.SizeBytes {
		_ = s.blob.DeleteObject(ctx, objectKey)
		return MediaAsset{}, ErrConflict
	}
	if params.SHA256Hex != "" && strings.TrimSpace(object.Metadata["sha256"]) != params.SHA256Hex {
		_ = s.blob.DeleteObject(ctx, objectKey)
		return MediaAsset{}, ErrConflict
	}

	asset := MediaAsset{
		ID:              mediaID,
		OwnerAccountID:  params.OwnerAccountID,
		Kind:            params.Kind,
		Status:          MediaStatusReady,
		StorageProvider: s.blob.Name(),
		Bucket:          s.blob.Bucket(),
		ObjectKey:       objectKey,
		FileName:        params.FileName,
		ContentType:     firstNonEmpty(params.ContentType, object.ContentType),
		SizeBytes:       params.SizeBytes,
		SHA256Hex:       params.SHA256Hex,
		Width:           params.Width,
		Height:          params.Height,
		Duration:        params.Duration,
		Metadata:        mergeMetadata(params.Metadata, object.Metadata),
		UploadExpiresAt: now.Add(s.settings.UploadURLTTL),
		ReadyAt:         now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	saved, saveErr := asset, error(nil)
	if err := s.store.WithinTx(ctx, func(tx Store) error {
		saved, saveErr = tx.SaveMediaAsset(ctx, asset)
		if saveErr != nil {
			return fmt.Errorf("save uploaded media asset %s: %w", asset.ID, saveErr)
		}

		return nil
	}); err != nil {
		_ = s.blob.DeleteObject(ctx, objectKey)
		return MediaAsset{}, err
	}

	s.indexMedia(ctx, saved)
	return saved, nil
}
