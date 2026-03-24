package media

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

// FinalizeUpload validates the uploaded object and marks the asset ready.
func (s *Service) FinalizeUpload(ctx context.Context, params FinalizeUploadParams) (MediaAsset, error) {
	if err := s.validateContext(ctx, "finalize media upload"); err != nil {
		return MediaAsset{}, err
	}
	params.OwnerAccountID = strings.TrimSpace(params.OwnerAccountID)
	params.MediaID = strings.TrimSpace(params.MediaID)
	if params.OwnerAccountID == "" || params.MediaID == "" {
		return MediaAsset{}, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	asset, err := s.store.MediaAssetByID(ctx, params.MediaID)
	if err != nil {
		return MediaAsset{}, fmt.Errorf("load media asset %s: %w", params.MediaID, err)
	}
	if asset.OwnerAccountID != params.OwnerAccountID {
		return MediaAsset{}, ErrForbidden
	}
	switch asset.Status {
	case MediaStatusReady:
		return asset, nil
	case MediaStatusDeleted:
		return MediaAsset{}, ErrNotFound
	case MediaStatusReserved, MediaStatusFailed:
	default:
		return MediaAsset{}, ErrConflict
	}

	head, err := s.blob.HeadObject(ctx, asset.ObjectKey)
	if err != nil {
		if errors.Is(err, domainstorage.ErrNotFound) {
			return MediaAsset{}, ErrNotFound
		}
		return MediaAsset{}, fmt.Errorf("head media object %s: %w", asset.ObjectKey, err)
	}
	if asset.SizeBytes > 0 && head.ContentLength != int64(asset.SizeBytes) {
		return MediaAsset{}, ErrConflict
	}
	if asset.SHA256Hex != "" {
		if head.Metadata == nil || head.Metadata["sha256"] != asset.SHA256Hex {
			return MediaAsset{}, ErrConflict
		}
	}

	asset.Status = MediaStatusReady
	asset.ContentType = firstNonEmpty(asset.ContentType, head.ContentType)
	asset.Metadata = mergeMetadata(asset.Metadata, head.Metadata)
	asset.UpdatedAt = now
	asset.ReadyAt = now

	saved, err := s.store.SaveMediaAsset(ctx, asset)
	if err != nil {
		return MediaAsset{}, fmt.Errorf("save media asset %s: %w", asset.ID, err)
	}

	return saved, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func mergeMetadata(values ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, value := range values {
		for key, entry := range value {
			key = strings.TrimSpace(key)
			entry = strings.TrimSpace(entry)
			if key == "" || entry == "" {
				continue
			}
			merged[key] = entry
		}
	}
	if len(merged) == 0 {
		return nil
	}

	return merged
}
