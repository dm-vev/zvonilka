package reaction_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dm-vev/zvonilka/internal/domain/media"
	"github.com/dm-vev/zvonilka/internal/domain/reaction"
	"github.com/dm-vev/zvonilka/internal/domain/reaction/teststore"
)

func TestCatalogSortsAndSupportsNotModified(t *testing.T) {
	t.Parallel()

	store := teststore.New()
	assets := newMediaAssets("media-1")
	service, err := reaction.NewService(store, assets)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range []reaction.Definition{
		definitionFor("🔥", "Fire", true, 30),
		definitionFor("👍", "Like", true, 10),
		definitionFor("❤️", "Heart", false, 20),
	} {
		if _, err := store.SaveDefinition(context.Background(), definition); err != nil {
			t.Fatal(err)
		}
	}

	catalog, notModified, err := service.Catalog(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if notModified {
		t.Fatal("first catalog response must not be not_modified")
	}
	if catalog.DefaultEmoji != "👍" {
		t.Fatalf("default emoji = %q, want 👍", catalog.DefaultEmoji)
	}
	if got := []string{catalog.Reactions[0].Emoji, catalog.Reactions[1].Emoji, catalog.Reactions[2].Emoji}; got[0] != "👍" || got[1] != "❤️" || got[2] != "🔥" {
		t.Fatalf("unexpected order: %v", got)
	}

	unchanged, notModified, err := service.Catalog(context.Background(), catalog.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !notModified || len(unchanged.Reactions) != 0 || unchanged.Version != catalog.Version {
		t.Fatalf("unexpected not_modified response: %#v, %v", unchanged, notModified)
	}
}

func TestValidateReactionRejectsUnknownAndInactive(t *testing.T) {
	t.Parallel()

	store := teststore.New()
	assets := newMediaAssets("media-1")
	service, err := reaction.NewService(store, assets)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveDefinition(context.Background(), definitionFor("👍", "Like", true, 10)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveDefinition(context.Background(), definitionFor("🔥", "Fire", false, 20)); err != nil {
		t.Fatal(err)
	}

	canonical, err := service.ValidateReaction(context.Background(), " 👍 ")
	if err != nil || canonical != "👍" {
		t.Fatalf("canonical reaction = %q, err = %v", canonical, err)
	}
	if _, err := service.ValidateReaction(context.Background(), "👀"); !errors.Is(err, reaction.ErrInvalidInput) {
		t.Fatalf("unknown reaction error = %v", err)
	}
	if _, err := service.ValidateReaction(context.Background(), "🔥"); !errors.Is(err, reaction.ErrInactive) {
		t.Fatalf("inactive reaction error = %v", err)
	}
}

func TestCatalogRejectsIncompleteOptionalPair(t *testing.T) {
	t.Parallel()

	store := teststore.New()
	assets := newMediaAssets("media-1")
	service, err := reaction.NewService(store, assets)
	if err != nil {
		t.Fatal(err)
	}
	definition := definitionFor("👍", "Like", true, 10)
	definition.AroundAnimation = "media-1"
	if _, err := store.SaveDefinition(context.Background(), definition); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Catalog(context.Background(), ""); !errors.Is(err, reaction.ErrInvalidCatalog) {
		t.Fatalf("catalog error = %v", err)
	}
}

func definitionFor(emoji, title string, active bool, order uint32) reaction.Definition {
	return reaction.Definition{
		Emoji:             emoji,
		Title:             title,
		Active:            active,
		SortOrder:         order,
		StaticIcon:        "media-1",
		AppearAnimation:   "media-1",
		SelectAnimation:   "media-1",
		ActivateAnimation: "media-1",
		EffectAnimation:   "media-1",
	}
}

type mediaAssets map[string]media.MediaAsset

func newMediaAssets(ids ...string) mediaAssets {
	assets := make(mediaAssets, len(ids))
	for _, id := range ids {
		assets[id] = media.MediaAsset{
			ID:           id,
			Status:       media.MediaStatusReady,
			ContentType:  "image/webp",
			SizeBytes:    10,
			SHA256Hex:    "abc123",
			PublicAccess: true,
			Metadata:     map[string]string{media.MetadataPurposeKey: "sticker_asset"},
		}
	}
	return assets
}

func (m mediaAssets) MediaAssetByID(_ context.Context, mediaID string) (media.MediaAsset, error) {
	asset, ok := m[mediaID]
	if !ok {
		return media.MediaAsset{}, media.ErrNotFound
	}
	return asset, nil
}
