package media

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

// Service coordinates media metadata and object-storage lifecycle.
type Service struct {
	store    Store
	blob     domainstorage.BlobStore
	now      func() time.Time
	settings Settings
	indexer  domainsearch.Indexer
}

// NewService constructs a media service backed by the provided store and blob store.
func NewService(store Store, blob domainstorage.BlobStore, opts ...Option) (*Service, error) {
	if store == nil || blob == nil {
		return nil, ErrInvalidInput
	}

	service := &Service{
		store:    store,
		blob:     blob,
		now:      func() time.Time { return time.Now().UTC() },
		settings: DefaultSettings(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}

	service.settings = normalizeSettings(service.settings)
	return service, nil
}

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}

	return s.now().UTC()
}

func (s *Service) validateContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

func normalizeAssetKind(kind MediaKind) MediaKind {
	kind = MediaKind(strings.ToLower(strings.TrimSpace(string(kind))))
	switch kind {
	case MediaKindImage, MediaKindVideo, MediaKindDocument, MediaKindVoice, MediaKindSticker, MediaKindGIF, MediaKindAvatar, MediaKindFile:
		return kind
	default:
		return MediaKindUnspecified
	}
}

func normalizeAssetStatus(status MediaStatus) MediaStatus {
	status = MediaStatus(strings.ToLower(strings.TrimSpace(string(status))))
	switch status {
	case MediaStatusReserved, MediaStatusReady, MediaStatusFailed, MediaStatusDeleted:
		return status
	default:
		return MediaStatusUnspecified
	}
}

func normalizeMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}

	return result
}

func mediaObjectKey(ownerAccountID string, mediaID string) string {
	return strings.Join([]string{"media", ownerAccountID, mediaID}, "/")
}

func ensureUploadSize(settings Settings, sizeBytes uint64) error {
	if settings.MaxUploadSize <= 0 {
		return nil
	}
	if sizeBytes > uint64(settings.MaxUploadSize) {
		return ErrConflict
	}

	return nil
}
