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
	if download.Method != http.MethodGet || download.URL != "https://example.invalid/download/"+asset.ObjectKey {
		t.Fatalf("unexpected download target: %+v", download)
	}
	if download.ExpiresAt.Before(fixedNow) {
		t.Fatalf("expected future download expiry, got %v", download.ExpiresAt)
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

func TestServerSideUpload(t *testing.T) {
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

	asset, err := svc.Upload(ctx, media.UploadParams{
		OwnerAccountID: "acc-owner",
		Kind:           media.MediaKindVoice,
		FileName:       "voice.ogg",
		ContentType:    "audio/ogg",
		SizeBytes:      uint64(len([]byte("voice"))),
		Body:           bytes.NewReader([]byte("voice")),
	})
	if err != nil {
		t.Fatalf("upload media: %v", err)
	}
	if asset.Status != media.MediaStatusReady {
		t.Fatalf("expected ready media, got %s", asset.Status)
	}
	if asset.ObjectKey == "" {
		t.Fatal("expected object key to be set")
	}
	if _, err := store.MediaAssetByID(ctx, asset.ID); err != nil {
		t.Fatalf("load uploaded asset: %v", err)
	}
}

func TestDeleteMediaRetriesBlobCleanupAfterFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemoryStore()
	blob := &flakyDeleteBlobStore{
		memoryBlobStore: newMemoryBlobStore("media-bucket"),
		deleteErrs: []error{
			errors.New("temporary delete failure"),
			nil,
		},
	}

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

	_, err = svc.DeleteMedia(ctx, media.DeleteParams{
		OwnerAccountID: "acc-owner",
		MediaID:        asset.ID,
	})
	if err == nil {
		t.Fatal("expected first delete to fail")
	}
	if blob.deleteCalls != 1 {
		t.Fatalf("expected one delete attempt, got %d", blob.deleteCalls)
	}

	deleted, err := svc.DeleteMedia(ctx, media.DeleteParams{
		OwnerAccountID: "acc-owner",
		MediaID:        asset.ID,
	})
	if err != nil {
		t.Fatalf("retry delete media: %v", err)
	}
	if deleted.Status != media.MediaStatusDeleted {
		t.Fatalf("expected deleted asset, got %s", deleted.Status)
	}
	if blob.deleteCalls != 2 {
		t.Fatalf("expected two delete attempts, got %d", blob.deleteCalls)
	}
}

func TestDeleteMediaHardDeleteRemovesRecord(t *testing.T) {
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

	deleted, err := svc.DeleteMedia(ctx, media.DeleteParams{
		OwnerAccountID: "acc-owner",
		MediaID:        asset.ID,
		HardDelete:     true,
	})
	if err != nil {
		t.Fatalf("hard delete media: %v", err)
	}
	if deleted.Status != media.MediaStatusDeleted {
		t.Fatalf("expected deleted status, got %s", deleted.Status)
	}
	if _, err := store.MediaAssetByID(ctx, asset.ID); !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("expected media row to be removed, got %v", err)
	}
}

func TestGetDownloadURLResolvesStoredVariant(t *testing.T) {
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

	now := time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC)
	asset := media.MediaAsset{
		ID:              "media-variant",
		OwnerAccountID:  "acc-owner",
		Kind:            media.MediaKindImage,
		Status:          media.MediaStatusReady,
		StorageProvider: "object",
		Bucket:          "media-bucket",
		ObjectKey:       "media/acc-owner/media-variant",
		FileName:        "photo.jpg",
		ContentType:     "image/jpeg",
		SizeBytes:       1024,
		Metadata: map[string]string{
			"variant_object_key.thumb": "media/acc-owner/media-variant-thumb",
		},
		UploadExpiresAt: now.Add(time.Minute),
		ReadyAt:         now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if _, err := store.SaveMediaAsset(ctx, asset); err != nil {
		t.Fatalf("save asset: %v", err)
	}

	_, target, err := svc.GetDownloadURL(ctx, media.GetDownloadParams{
		OwnerAccountID: "acc-owner",
		MediaID:        asset.ID,
		Variant:        "thumb",
	})
	if err != nil {
		t.Fatalf("get variant download url: %v", err)
	}
	if target.URL != "https://example.invalid/download/media/acc-owner/media-variant-thumb" {
		t.Fatalf("unexpected variant download url: %s", target.URL)
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

func TestFinalizeUploadRejectsExpiredReservation(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	blob := newMemoryBlobStore("media-bucket")

	svc, err := media.NewService(store, blob, media.WithNow(func() time.Time { return fixedNow }), media.WithSettings(media.Settings{
		UploadURLTTL:   time.Minute,
		DownloadURLTTL: time.Minute,
		MaxUploadSize:  10 << 20,
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	asset, _, err := svc.ReserveUpload(context.Background(), media.ReserveUploadParams{
		OwnerAccountID: "acc-owner",
		Kind:           media.MediaKindFile,
		FileName:       "archive.zip",
		SizeBytes:      1024,
		CreatedAt:      fixedNow,
	})
	if err != nil {
		t.Fatalf("reserve upload: %v", err)
	}

	blob.seedObject(asset.ObjectKey, domainstorage.BlobObject{
		Bucket:        "media-bucket",
		Key:           asset.ObjectKey,
		ContentLength: 1024,
	}, nil)

	_, err = svc.FinalizeUpload(context.Background(), media.FinalizeUploadParams{
		OwnerAccountID: "acc-owner",
		MediaID:        asset.ID,
		CreatedAt:      asset.UploadExpiresAt,
	})
	if !errors.Is(err, media.ErrConflict) {
		t.Fatalf("expected conflict for expired reservation boundary, got %v", err)
	}
}

func TestListMediaSkipsDeletedRowsWithoutTruncatingActiveResults(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newMemoryStore()
	blob := newMemoryBlobStore("media-bucket")
	fixedNow := time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC)

	svc, err := media.NewService(store, blob, media.WithNow(func() time.Time { return fixedNow }), media.WithSettings(media.Settings{
		UploadURLTTL:   time.Minute,
		DownloadURLTTL: time.Minute,
		MaxUploadSize:  10 << 20,
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	assets := []media.MediaAsset{
		{
			ID:              "media_deleted_1",
			OwnerAccountID:  "acc-owner",
			Kind:            media.MediaKindFile,
			Status:          media.MediaStatusDeleted,
			StorageProvider: "object",
			Bucket:          "media-bucket",
			ObjectKey:       "media/acc-owner/media_deleted_1",
			FileName:        "deleted-1.txt",
			UploadExpiresAt: fixedNow.Add(time.Minute),
			CreatedAt:       fixedNow.Add(-5 * time.Minute),
			UpdatedAt:       fixedNow.Add(-1 * time.Minute),
			DeletedAt:       fixedNow.Add(-1 * time.Minute),
		},
		{
			ID:              "media_deleted_2",
			OwnerAccountID:  "acc-owner",
			Kind:            media.MediaKindFile,
			Status:          media.MediaStatusDeleted,
			StorageProvider: "object",
			Bucket:          "media-bucket",
			ObjectKey:       "media/acc-owner/media_deleted_2",
			FileName:        "deleted-2.txt",
			UploadExpiresAt: fixedNow.Add(time.Minute),
			CreatedAt:       fixedNow.Add(-6 * time.Minute),
			UpdatedAt:       fixedNow.Add(-2 * time.Minute),
			DeletedAt:       fixedNow.Add(-2 * time.Minute),
		},
		{
			ID:              "media_ready_1",
			OwnerAccountID:  "acc-owner",
			Kind:            media.MediaKindFile,
			Status:          media.MediaStatusReady,
			StorageProvider: "object",
			Bucket:          "media-bucket",
			ObjectKey:       "media/acc-owner/media_ready_1",
			FileName:        "ready-1.txt",
			UploadExpiresAt: fixedNow.Add(time.Minute),
			ReadyAt:         fixedNow.Add(-3 * time.Minute),
			CreatedAt:       fixedNow.Add(-7 * time.Minute),
			UpdatedAt:       fixedNow.Add(-3 * time.Minute),
		},
		{
			ID:              "media_ready_2",
			OwnerAccountID:  "acc-owner",
			Kind:            media.MediaKindFile,
			Status:          media.MediaStatusReady,
			StorageProvider: "object",
			Bucket:          "media-bucket",
			ObjectKey:       "media/acc-owner/media_ready_2",
			FileName:        "ready-2.txt",
			UploadExpiresAt: fixedNow.Add(time.Minute),
			ReadyAt:         fixedNow.Add(-4 * time.Minute),
			CreatedAt:       fixedNow.Add(-8 * time.Minute),
			UpdatedAt:       fixedNow.Add(-4 * time.Minute),
		},
	}
	for _, asset := range assets {
		if _, err := store.SaveMediaAsset(ctx, asset); err != nil {
			t.Fatalf("save asset %s: %v", asset.ID, err)
		}
	}

	listed, err := svc.ListMedia(ctx, media.ListParams{
		OwnerAccountID: "acc-owner",
		Limit:          1,
	})
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected one active asset, got %d", len(listed))
	}
	if listed[0].Status != media.MediaStatusReady {
		t.Fatalf("expected ready asset, got %s", listed[0].Status)
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

func (s *memoryStore) MediaActiveAssetsByOwner(ctx context.Context, ownerAccountID string, limit int) ([]media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	assets := make([]media.MediaAsset, 0, len(s.assets))
	for _, asset := range s.assets {
		if asset.OwnerAccountID == ownerAccountID && asset.Status != media.MediaStatusDeleted {
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

func (s *memoryStore) DeleteMediaAsset(ctx context.Context, mediaID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.assets[mediaID]; !ok {
		return media.ErrNotFound
	}
	delete(s.assets, mediaID)
	return nil
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

type flakyDeleteBlobStore struct {
	*memoryBlobStore
	deleteErrs  []error
	deleteCalls int
}

func (s *flakyDeleteBlobStore) DeleteObject(ctx context.Context, key string) error {
	s.deleteCalls++
	if idx := s.deleteCalls - 1; idx < len(s.deleteErrs) && s.deleteErrs[idx] != nil {
		return s.deleteErrs[idx]
	}

	return s.memoryBlobStore.DeleteObject(ctx, key)
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
