package media

import "context"

// Store persists media metadata.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	SaveMediaAsset(ctx context.Context, asset MediaAsset) (MediaAsset, error)
	MediaAssetByID(ctx context.Context, mediaID string) (MediaAsset, error)
	MediaAssetsByOwner(ctx context.Context, ownerAccountID string, limit int) ([]MediaAsset, error)
	MediaActiveAssetsByOwner(ctx context.Context, ownerAccountID string, limit int) ([]MediaAsset, error)
	MediaAssetByObjectKey(ctx context.Context, objectKey string) (MediaAsset, error)
	DeleteMediaAsset(ctx context.Context, mediaID string) error
}
