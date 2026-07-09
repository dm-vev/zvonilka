package media

import (
	"context"
	"fmt"
	"strings"
)

// GetMedia resolves one media asset visible to the owner.
func (s *Service) GetMedia(ctx context.Context, ownerAccountID string, mediaID string) (MediaAsset, error) {
	if err := s.validateContext(ctx, "get media"); err != nil {
		return MediaAsset{}, err
	}

	ownerAccountID = strings.TrimSpace(ownerAccountID)
	mediaID = strings.TrimSpace(mediaID)
	if ownerAccountID == "" || mediaID == "" {
		return MediaAsset{}, ErrInvalidInput
	}

	asset, err := s.store.MediaAssetByID(ctx, mediaID)
	if err != nil {
		return MediaAsset{}, fmt.Errorf("load media asset %s: %w", mediaID, err)
	}
	if asset.OwnerAccountID != ownerAccountID {
		return MediaAsset{}, ErrForbidden
	}

	return asset, nil
}
