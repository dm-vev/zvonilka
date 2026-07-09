package pgstore

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/dm-vev/zvonilka/internal/domain/media"
)

func TestSaveMediaAssetRejectsInvalidTimestamps(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	base := media.MediaAsset{
		ID:              "media_1",
		OwnerAccountID:  "acc-owner",
		Kind:            media.MediaKindImage,
		Status:          media.MediaStatusReserved,
		StorageProvider: "object",
		Bucket:          "media-bucket",
		ObjectKey:       "media/acc-owner/media_1",
		FileName:        "photo.jpg",
		ContentType:     "image/jpeg",
		SizeBytes:       1024,
		SHA256Hex:       "abc123",
		Metadata:        map[string]string{"album": "vacation"},
		UploadExpiresAt: time.Date(2026, time.March, 24, 12, 15, 0, 0, time.UTC),
		CreatedAt:       time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
	}

	cases := []struct {
		name   string
		mutate func(*media.MediaAsset)
	}{
		{
			name: "zero created at",
			mutate: func(asset *media.MediaAsset) {
				asset.CreatedAt = time.Time{}
			},
		},
		{
			name: "zero updated at",
			mutate: func(asset *media.MediaAsset) {
				asset.UpdatedAt = time.Time{}
			},
		},
		{
			name: "updated before created",
			mutate: func(asset *media.MediaAsset) {
				asset.UpdatedAt = asset.CreatedAt.Add(-time.Second)
			},
		},
		{
			name: "upload expires before created",
			mutate: func(asset *media.MediaAsset) {
				asset.UploadExpiresAt = asset.CreatedAt.Add(-time.Second)
			},
		},
		{
			name: "zero upload expires at",
			mutate: func(asset *media.MediaAsset) {
				asset.UploadExpiresAt = time.Time{}
			},
		},
		{
			name: "ready without ready at",
			mutate: func(asset *media.MediaAsset) {
				asset.Status = media.MediaStatusReady
				asset.ReadyAt = time.Time{}
			},
		},
		{
			name: "deleted without deleted at",
			mutate: func(asset *media.MediaAsset) {
				asset.Status = media.MediaStatusDeleted
				asset.DeletedAt = time.Time{}
			},
		},
		{
			name: "reserved with ready at",
			mutate: func(asset *media.MediaAsset) {
				asset.ReadyAt = asset.CreatedAt
			},
		},
		{
			name: "failed with deleted at",
			mutate: func(asset *media.MediaAsset) {
				asset.Status = media.MediaStatusFailed
				asset.DeletedAt = asset.CreatedAt
			},
		},
		{
			name: "ready at after updated at",
			mutate: func(asset *media.MediaAsset) {
				asset.Status = media.MediaStatusReady
				asset.ReadyAt = asset.UpdatedAt.Add(time.Second)
			},
		},
		{
			name: "deleted at after updated at",
			mutate: func(asset *media.MediaAsset) {
				asset.Status = media.MediaStatusDeleted
				asset.DeletedAt = asset.UpdatedAt.Add(time.Second)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asset := base
			tc.mutate(&asset)

			_, err := store.SaveMediaAsset(context.Background(), asset)
			if err == nil {
				t.Fatalf("expected %s to fail", tc.name)
			}
			if !errors.Is(err, media.ErrInvalidInput) {
				t.Fatalf("expected invalid input for %s, got %v", tc.name, err)
			}
		})
	}
}
