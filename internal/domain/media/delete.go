package media

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

// DeleteMedia marks a media asset deleted and removes the object from storage.
func (s *Service) DeleteMedia(ctx context.Context, params DeleteParams) (MediaAsset, error) {
	if err := s.validateContext(ctx, "delete media"); err != nil {
		return MediaAsset{}, err
	}
	params.OwnerAccountID = strings.TrimSpace(params.OwnerAccountID)
	params.MediaID = strings.TrimSpace(params.MediaID)
	if params.OwnerAccountID == "" || params.MediaID == "" {
		return MediaAsset{}, ErrInvalidInput
	}

	now := params.DeletedAt
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
		if asset.Status == MediaStatusDeleted {
			saved = asset
			return nil
		}

		asset.Status = MediaStatusDeleted
		asset.DeletedAt = now
		asset.UpdatedAt = now

		savedAsset, saveErr := tx.SaveMediaAsset(ctx, asset)
		if saveErr != nil {
			return fmt.Errorf("save deleted media asset %s: %w", asset.ID, saveErr)
		}

		saved = savedAsset
		return nil
	})
	if err != nil {
		return MediaAsset{}, err
	}

	s.deleteMediaIndex(ctx, saved)
	if deleteErr := s.blob.DeleteObject(ctx, saved.ObjectKey); deleteErr != nil && !errors.Is(deleteErr, domainstorage.ErrNotFound) {
		return saved, fmt.Errorf("delete media object %s: %w", saved.ObjectKey, deleteErr)
	}

	return saved, nil
}
