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

	var saved MediaAsset
	err := s.store.WithinTx(ctx, func(tx Store) error {
		asset, loadErr := tx.MediaAssetByID(ctx, params.MediaID)
		if loadErr != nil {
			return fmt.Errorf("load media asset %s: %w", params.MediaID, loadErr)
		}
		if asset.OwnerAccountID != params.OwnerAccountID {
			return ErrForbidden
		}
		switch asset.Status {
		case MediaStatusReady:
			saved = asset
			return nil
		case MediaStatusDeleted:
			return ErrNotFound
		case MediaStatusReserved, MediaStatusFailed:
		default:
			return ErrConflict
		}
		if !now.Before(asset.UploadExpiresAt) {
			return ErrConflict
		}

		head, headErr := s.blob.HeadObject(ctx, asset.ObjectKey)
		if headErr != nil {
			if errors.Is(headErr, domainstorage.ErrNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("head media object %s: %w", asset.ObjectKey, headErr)
		}
		if asset.SizeBytes > 0 && head.ContentLength != int64(asset.SizeBytes) {
			return ErrConflict
		}
		if asset.SHA256Hex != "" {
			if head.Metadata == nil || head.Metadata["sha256"] != asset.SHA256Hex {
				return ErrConflict
			}
		}

		asset.Status = MediaStatusReady
		asset.ContentType = firstNonEmpty(asset.ContentType, head.ContentType)
		asset.Metadata = mergeMetadata(asset.Metadata, head.Metadata)
		asset.UpdatedAt = now
		asset.ReadyAt = now

		savedAsset, saveErr := tx.SaveMediaAsset(ctx, asset)
		if saveErr != nil {
			return fmt.Errorf("save media asset %s: %w", asset.ID, saveErr)
		}

		saved = savedAsset
		return nil
	})
	if err != nil {
		return MediaAsset{}, err
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
