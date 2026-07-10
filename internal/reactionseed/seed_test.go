package reactionseed

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/media"
	"github.com/dm-vev/zvonilka/internal/domain/reaction"
	"github.com/dm-vev/zvonilka/internal/domain/reaction/teststore"
)

func TestLoadManifestRejectsDuplicateSortOrder(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeReactionAsset(t, directory)
	manifest := testManifest()
	manifest.Reactions = append(manifest.Reactions, manifest.Reactions[0])
	manifest.Reactions[1].Emoji = "❤️"
	manifest.Reactions[1].SortOrder = manifest.Reactions[0].SortOrder
	path := writeManifest(t, directory, manifest)

	if _, err := LoadManifest(path); !errors.Is(err, reaction.ErrInvalidInput) {
		t.Fatalf("manifest error = %v", err)
	}
}

func TestSeedReusesAssetOnSecondRun(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeReactionAsset(t, directory)
	manifestPath := writeManifest(t, directory, testManifest())
	assets := newFakeAssetStore()
	uploader := &fakeUploader{assets: assets}
	store := teststore.New()
	seeder, err := New(uploader, assets, store, "seed-owner")
	if err != nil {
		t.Fatal(err)
	}
	seeder.now = func() time.Time { return time.Date(2026, time.July, 10, 10, 0, 0, 0, time.UTC) }

	first, err := seeder.Seed(context.Background(), manifestPath, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := seeder.Seed(context.Background(), manifestPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if uploader.uploads != 1 {
		t.Fatalf("uploads = %d, want 1", uploader.uploads)
	}
	if first.Version != second.Version || len(second.Reactions) != 1 {
		t.Fatalf("catalogs differ: first=%+v second=%+v", first, second)
	}
}

func TestLoadManifestValidatesTGSAnimation(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeReactionAsset(t, directory)
	tgsPath := filepath.Join(directory, "shared", "appear.tgs")
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(`{"w":512,"h":512,"fr":60,"ip":0,"op":180}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tgsPath, compressed.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := testManifest()
	manifest.Reactions[0].Assets.AppearAnimation = "shared/appear.tgs"
	manifest.Reactions[0].Assets.SelectAnimation = "shared/appear.tgs"
	manifest.Reactions[0].Assets.ActivateAnimation = "shared/appear.tgs"
	manifest.Reactions[0].Assets.EffectAnimation = "shared/appear.tgs"
	if _, err := LoadManifest(writeManifest(t, directory, manifest)); err != nil {
		t.Fatalf("valid TGS manifest: %v", err)
	}

	badManifest := manifest
	badManifest.Reactions[0].Assets.AppearAnimation = "shared/bad.tgs"
	if err := os.WriteFile(filepath.Join(directory, "shared", "bad.tgs"), []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(writeManifest(t, directory, badManifest)); err == nil {
		t.Fatal("expected invalid TGS to be rejected")
	}
}

func TestRepositoryReactionManifestAssets(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "assets", "reactions", "catalog.json")
	if _, err := LoadManifest(manifestPath); err != nil {
		t.Fatalf("repository reaction manifest: %v", err)
	}
}

func testManifest() Manifest {
	return Manifest{
		Version:      1,
		DefaultEmoji: "👍",
		Reactions: []ManifestReaction{{
			Emoji:     "👍",
			Slug:      "thumbs_up",
			Title:     "Like",
			Active:    true,
			SortOrder: 10,
			Assets: ManifestAssets{
				StaticIcon:        "shared/static.webp",
				AppearAnimation:   "shared/static.webp",
				SelectAnimation:   "shared/static.webp",
				ActivateAnimation: "shared/static.webp",
				EffectAnimation:   "shared/static.webp",
			},
		}},
	}
}

func writeReactionAsset(t *testing.T, directory string) {
	t.Helper()
	path := filepath.Join(directory, "shared", "static.webp")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 32)
	copy(data[:4], "RIFF")
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8X")
	data[24], data[27] = 99, 99
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeManifest(t *testing.T, directory string, manifest Manifest) string {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "catalog.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

type fakeAssetStore struct {
	assets map[string]media.MediaAsset
}

func newFakeAssetStore() *fakeAssetStore {
	return &fakeAssetStore{assets: make(map[string]media.MediaAsset)}
}

func (s *fakeAssetStore) MediaAssetByID(_ context.Context, mediaID string) (media.MediaAsset, error) {
	for _, asset := range s.assets {
		if asset.ID == mediaID {
			return asset, nil
		}
	}
	return media.MediaAsset{}, media.ErrNotFound
}

func (s *fakeAssetStore) MediaAssetBySHA256(_ context.Context, sha256Hex string) (media.MediaAsset, error) {
	for _, asset := range s.assets {
		if asset.SHA256Hex == sha256Hex && asset.Status == media.MediaStatusReady {
			return asset, nil
		}
	}
	return media.MediaAsset{}, media.ErrNotFound
}

func (s *fakeAssetStore) SaveMediaAsset(_ context.Context, asset media.MediaAsset) (media.MediaAsset, error) {
	s.assets[asset.ID] = asset
	return asset, nil
}

type fakeUploader struct {
	assets  *fakeAssetStore
	uploads int
}

func (u *fakeUploader) Upload(_ context.Context, params media.UploadParams) (media.MediaAsset, error) {
	u.uploads++
	asset := media.MediaAsset{
		ID:              "media-seeded",
		OwnerAccountID:  params.OwnerAccountID,
		Kind:            params.Kind,
		Status:          media.MediaStatusReady,
		ContentType:     params.ContentType,
		SizeBytes:       params.SizeBytes,
		SHA256Hex:       params.SHA256Hex,
		Width:           params.Width,
		Height:          params.Height,
		Metadata:        params.Metadata,
		PublicAccess:    params.PublicAccess,
		ReadyAt:         params.CreatedAt,
		UploadExpiresAt: params.CreatedAt.Add(time.Hour),
		CreatedAt:       params.CreatedAt,
		UpdatedAt:       params.CreatedAt,
	}
	u.assets.assets[asset.ID] = asset
	return asset, nil
}
