package media_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/media"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

func TestMediaLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemoryStore()
	blob := newMemoryBlobStore("media-bucket")
	fixedNow := time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC)

	svc, err := media.NewService(
		store,
		blob,
		media.WithNow(func() time.Time { return fixedNow }),
		media.WithSettings(media.Settings{
			UploadURLTTL:   15 * time.Minute,
			DownloadURLTTL: 15 * time.Minute,
			MaxUploadSize:  10 << 20,
		}),
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	asset, upload, err := svc.ReserveUpload(ctx, media.ReserveUploadParams{
		OwnerAccountID: "acc-owner",
		Kind:           media.MediaKindImage,
		FileName:       "photo.jpg",
		ContentType:    "image/jpeg",
		SizeBytes:      1024,
		SHA256Hex:      "abc123",
		Metadata:       map[string]string{"album": "vacation"},
		CreatedAt:      fixedNow,
	})
	if err != nil {
		t.Fatalf("reserve upload: %v", err)
	}
	if upload.Method != http.MethodPut {
		t.Fatalf("expected PUT upload method, got %s", upload.Method)
	}
	if upload.ObjectKey == "" || upload.Bucket != "media-bucket" {
		t.Fatalf("unexpected upload target: %+v", upload)
	}

	blob.seedObject(upload.ObjectKey, domainstorage.BlobObject{
		Bucket:        upload.Bucket,
		Key:           upload.ObjectKey,
		ContentLength: 1024,
		ContentType:   "image/jpeg",
		Metadata: map[string]string{
			"sha256": "abc123",
		},
	}, []byte("hello"))

	finalized, err := svc.FinalizeUpload(ctx, media.FinalizeUploadParams{
		OwnerAccountID: "acc-owner",
		MediaID:        asset.ID,
		CreatedAt:      fixedNow.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("finalize upload: %v", err)
	}
	if finalized.Status != media.MediaStatusReady {
		t.Fatalf("expected ready media, got %s", finalized.Status)
	}

	_, download, err := svc.GetDownloadURL(ctx, media.GetDownloadParams{
		OwnerAccountID: "acc-owner",
		MediaID:        asset.ID,
	})
	if err != nil {
		t.Fatalf("get download url: %v", err)
	}
	if download.Method != http.MethodGet || download.URL == "" {
		t.Fatalf("unexpected download target: %+v", download)
	}

	listed, err := svc.ListMedia(ctx, media.ListParams{OwnerAccountID: "acc-owner"})
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one asset, got %d", len(listed))
	}

	deleted, err := svc.DeleteMedia(ctx, media.DeleteParams{
		OwnerAccountID: "acc-owner",
		MediaID:        asset.ID,
		DeletedAt:      fixedNow.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("delete media: %v", err)
	}
	if deleted.Status != media.MediaStatusDeleted {
		t.Fatalf("expected deleted asset, got %s", deleted.Status)
	}
}

func TestReserveUploadRejectsOversize(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, media.Settings{UploadURLTTL: time.Minute, DownloadURLTTL: time.Minute, MaxUploadSize: 10})

	_, _, err := svc.ReserveUpload(context.Background(), media.ReserveUploadParams{
		OwnerAccountID: "acc-owner",
		Kind:           media.MediaKindFile,
		FileName:       "archive.zip",
		SizeBytes:      11,
	})
	if !errors.Is(err, media.ErrConflict) {
		t.Fatalf("expected conflict for oversize upload, got %v", err)
	}
}

func TestFinalizeUploadRejectsMissingObject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemoryStore()
	blob := newMemoryBlobStore("media-bucket")

	svc, err := media.NewService(store, blob, media.WithSettings(media.Settings{
		UploadURLTTL:   time.Minute,
		DownloadURLTTL: time.Minute,
		MaxUploadSize:  10 << 20,
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	asset, _, err := svc.ReserveUpload(ctx, media.ReserveUploadParams{
		OwnerAccountID: "acc-owner",
		Kind:           media.MediaKindFile,
		FileName:       "archive.zip",
		SizeBytes:      1024,
	})
	if err != nil {
		t.Fatalf("reserve upload: %v", err)
	}

	_, err = svc.FinalizeUpload(ctx, media.FinalizeUploadParams{
		OwnerAccountID: "acc-owner",
		MediaID:        asset.ID,
	})
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("expected missing blob error, got %v", err)
	}
}

func TestFinalizeUploadRejectsSizeMismatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		headLength   int64
		expectedSize int64
	}{
		{
			name:         "zero_length_head",
			headLength:   0,
			expectedSize: 1024,
		},
		{
			name:         "positive_length_head",
			headLength:   2048,
			expectedSize: 1024,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			store := newMemoryStore()
			blob := newMemoryBlobStore("media-bucket")

			svc, err := media.NewService(store, blob, media.WithSettings(media.Settings{
				UploadURLTTL:   time.Minute,
				DownloadURLTTL: time.Minute,
				MaxUploadSize:  10 << 20,
			}))
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			asset, _, err := svc.ReserveUpload(ctx, media.ReserveUploadParams{
				OwnerAccountID: "acc-owner",
				Kind:           media.MediaKindFile,
				FileName:       "archive.zip",
				SizeBytes:      uint64(tc.expectedSize),
			})
			if err != nil {
				t.Fatalf("reserve upload: %v", err)
			}

			blob.seedObject(asset.ObjectKey, domainstorage.BlobObject{
				Bucket:        "media-bucket",
				Key:           asset.ObjectKey,
				ContentLength: tc.headLength,
			}, nil)

			_, err = svc.FinalizeUpload(ctx, media.FinalizeUploadParams{
				OwnerAccountID: "acc-owner",
				MediaID:        asset.ID,
			})
			if !errors.Is(err, media.ErrConflict) {
				t.Fatalf("expected conflict for size mismatch, got %v", err)
			}
		})
	}
}

func newTestService(t *testing.T, settings media.Settings) *media.Service {
	t.Helper()

	store := newMemoryStore()
	blob := newMemoryBlobStore("media-bucket")
	svc, err := media.NewService(store, blob, media.WithSettings(settings))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return svc
}

type memoryStore struct {
	mu     sync.Mutex
	assets map[string]media.MediaAsset
}

func newMemoryStore() *memoryStore {
	return &memoryStore{assets: make(map[string]media.MediaAsset)}
}

func (s *memoryStore) WithinTx(ctx context.Context, fn func(media.Store) error) error {
	if ctx == nil {
		return media.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx := s.clone()
	if err := fn(tx); err != nil {
		return err
	}

	s.assets = tx.assets
	return nil
}

func (s *memoryStore) SaveMediaAsset(ctx context.Context, asset media.MediaAsset) (media.MediaAsset, error) {
	if asset.ID == "" {
		return media.MediaAsset{}, media.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.assets == nil {
		s.assets = make(map[string]media.MediaAsset)
	}
	s.assets[asset.ID] = cloneAsset(asset)

	return cloneAsset(asset), nil
}

func (s *memoryStore) MediaAssetByID(ctx context.Context, mediaID string) (media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	asset, ok := s.assets[mediaID]
	if !ok {
		return media.MediaAsset{}, media.ErrNotFound
	}

	return cloneAsset(asset), nil
}

func (s *memoryStore) MediaAssetsByOwner(ctx context.Context, ownerAccountID string, limit int) ([]media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	assets := make([]media.MediaAsset, 0, len(s.assets))
	for _, asset := range s.assets {
		if asset.OwnerAccountID == ownerAccountID {
			assets = append(assets, cloneAsset(asset))
		}
	}
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].UpdatedAt.Equal(assets[j].UpdatedAt) {
			return assets[i].ID < assets[j].ID
		}
		return assets[i].UpdatedAt.After(assets[j].UpdatedAt)
	})
	if limit > 0 && len(assets) > limit {
		assets = assets[:limit]
	}

	return assets, nil
}

func (s *memoryStore) MediaAssetByObjectKey(ctx context.Context, objectKey string) (media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, asset := range s.assets {
		if asset.ObjectKey == objectKey {
			return cloneAsset(asset), nil
		}
	}

	return media.MediaAsset{}, media.ErrNotFound
}

func (s *memoryStore) clone() *memoryStore {
	clone := newMemoryStore()
	for id, asset := range s.assets {
		clone.assets[id] = cloneAsset(asset)
	}

	return clone
}

func cloneAsset(asset media.MediaAsset) media.MediaAsset {
	clone := asset
	clone.Metadata = cloneMetadata(asset.Metadata)

	return clone
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}

	return result
}

type memoryBlobStore struct {
	mu       sync.Mutex
	bucket   string
	objects  map[string]domainstorage.BlobObject
	payloads map[string][]byte
}

func newMemoryBlobStore(bucket string) *memoryBlobStore {
	return &memoryBlobStore{
		bucket:   bucket,
		objects:  make(map[string]domainstorage.BlobObject),
		payloads: make(map[string][]byte),
	}
}

func (s *memoryBlobStore) Name() string                   { return "object" }
func (s *memoryBlobStore) Kind() domainstorage.Kind       { return domainstorage.KindObject }
func (s *memoryBlobStore) Purpose() domainstorage.Purpose { return domainstorage.PurposeObject }
func (s *memoryBlobStore) Capabilities() domainstorage.Capability {
	return domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityBlob | domainstorage.CapabilityListing
}
func (s *memoryBlobStore) Close(context.Context) error { return nil }
func (s *memoryBlobStore) Bucket() string              { return s.bucket }

func (s *memoryBlobStore) PutObject(ctx context.Context, key string, body io.Reader, size int64, options domainstorage.PutObjectOptions) (domainstorage.BlobObject, error) {
	var payload []byte
	if body != nil {
		readPayload, _ := io.ReadAll(body)
		payload = readPayload
	}

	object := domainstorage.BlobObject{
		Bucket:        s.bucket,
		Key:           key,
		ContentLength: size,
		ContentType:   options.ContentType,
		Metadata:      cloneMetadata(options.Metadata),
		ETag:          "etag",
		LastModified:  time.Now().UTC(),
	}
	s.seedObject(key, object, payload)

	return object, nil
}

func (s *memoryBlobStore) GetObject(ctx context.Context, key string) (io.ReadCloser, domainstorage.BlobObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	object, ok := s.objects[key]
	if !ok {
		return nil, domainstorage.BlobObject{}, domainstorage.ErrNotFound
	}

	return io.NopCloser(bytes.NewReader(s.payloads[key])), object, nil
}

func (s *memoryBlobStore) HeadObject(ctx context.Context, key string) (domainstorage.BlobObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	object, ok := s.objects[key]
	if !ok {
		return domainstorage.BlobObject{}, domainstorage.ErrNotFound
	}

	return object, nil
}

func (s *memoryBlobStore) DeleteObject(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.objects[key]; !ok {
		return domainstorage.ErrNotFound
	}
	delete(s.objects, key)

	return nil
}

func (s *memoryBlobStore) PresignPutObject(ctx context.Context, key string, expires time.Duration, options domainstorage.PutObjectOptions) (domainstorage.PresignedRequest, error) {
	return domainstorage.PresignedRequest{
		URL:       fmt.Sprintf("https://example.invalid/upload/%s", key),
		Method:    http.MethodPut,
		Headers:   map[string]string{"content-type": options.ContentType},
		ExpiresAt: time.Now().UTC().Add(expires),
		Bucket:    s.bucket,
		ObjectKey: key,
	}, nil
}

func (s *memoryBlobStore) PresignGetObject(ctx context.Context, key string, expires time.Duration) (domainstorage.PresignedRequest, error) {
	return domainstorage.PresignedRequest{
		URL:       fmt.Sprintf("https://example.invalid/download/%s", key),
		Method:    http.MethodGet,
		ExpiresAt: time.Now().UTC().Add(expires),
		Bucket:    s.bucket,
		ObjectKey: key,
	}, nil
}

func (s *memoryBlobStore) seedObject(key string, object domainstorage.BlobObject, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.objects == nil {
		s.objects = make(map[string]domainstorage.BlobObject)
	}
	s.objects[key] = object
	if s.payloads == nil {
		s.payloads = make(map[string][]byte)
	}
	s.payloads[key] = append([]byte(nil), payload...)
}

func (s *memoryBlobStore) seedPayload(key string, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.payloads == nil {
		s.payloads = make(map[string][]byte)
	}
	s.payloads[key] = append([]byte(nil), payload...)
}
