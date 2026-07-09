package media

import (
	"context"
	"fmt"
	"strings"
)

// GetDownloadURL resolves a presigned download target for a ready media asset.
func (s *Service) GetDownloadURL(ctx context.Context, params GetDownloadParams) (MediaAsset, DownloadTarget, error) {
	if err := s.validateContext(ctx, "get media download URL"); err != nil {
		return MediaAsset{}, DownloadTarget{}, err
	}
	params.OwnerAccountID = strings.TrimSpace(params.OwnerAccountID)
	params.MediaID = strings.TrimSpace(params.MediaID)
	params.Variant = strings.TrimSpace(params.Variant)
	if params.OwnerAccountID == "" || params.MediaID == "" {
		return MediaAsset{}, DownloadTarget{}, ErrInvalidInput
	}

	asset, err := s.store.MediaAssetByID(ctx, params.MediaID)
	if err != nil {
		return MediaAsset{}, DownloadTarget{}, fmt.Errorf("load media asset %s: %w", params.MediaID, err)
	}
	if asset.OwnerAccountID != params.OwnerAccountID {
		return MediaAsset{}, DownloadTarget{}, ErrForbidden
	}
	if asset.Status != MediaStatusReady {
		return MediaAsset{}, DownloadTarget{}, ErrNotFound
	}

	objectKey := asset.ObjectKey
	if params.Variant != "" && !strings.EqualFold(params.Variant, "original") {
		variantObjectKey := strings.TrimSpace(asset.Metadata[variantObjectKeyMetadataKey(params.Variant)])
		if variantObjectKey == "" {
			return MediaAsset{}, DownloadTarget{}, ErrNotFound
		}
		objectKey = variantObjectKey
	}

	target, err := s.blob.PresignGetObject(ctx, objectKey, s.settings.DownloadURLTTL)
	if err != nil {
		return MediaAsset{}, DownloadTarget{}, fmt.Errorf("presign download for media %s: %w", params.MediaID, err)
	}

	return asset, DownloadTarget{
		URL:       target.URL,
		Method:    target.Method,
		Headers:   target.Headers,
		ExpiresAt: target.ExpiresAt,
	}, nil
}

// ListMedia returns the assets created by one account.
func (s *Service) ListMedia(ctx context.Context, params ListParams) ([]MediaAsset, error) {
	if err := s.validateContext(ctx, "list media"); err != nil {
		return nil, err
	}
	params.OwnerAccountID = strings.TrimSpace(params.OwnerAccountID)
	if params.OwnerAccountID == "" {
		return nil, ErrInvalidInput
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	if params.IncludeDeleted {
		assets, err := s.store.MediaAssetsByOwner(ctx, params.OwnerAccountID, limit)
		if err != nil {
			return nil, fmt.Errorf("list media assets for account %s: %w", params.OwnerAccountID, err)
		}

		return assets, nil
	}

	assets, err := s.store.MediaActiveAssetsByOwner(ctx, params.OwnerAccountID, limit)
	if err != nil {
		return nil, fmt.Errorf("list active media assets for account %s: %w", params.OwnerAccountID, err)
	}

	return assets, nil
}
