package reaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/media"
)

// AssetReader loads media metadata without applying user-facing ownership rules.
type AssetReader interface {
	MediaAssetByID(context.Context, string) (media.MediaAsset, error)
}

// Service validates and serves the global reaction catalog.
type Service struct {
	store Store
	media AssetReader
}

// NewService constructs a reaction catalog service.
func NewService(store Store, mediaReader AssetReader) (*Service, error) {
	if store == nil || mediaReader == nil {
		return nil, ErrInvalidInput
	}

	return &Service{store: store, media: mediaReader}, nil
}

// ValidateReaction returns the canonical active emoji for a new reaction.
func (s *Service) ValidateReaction(ctx context.Context, emoji string) (string, error) {
	if s == nil || s.store == nil {
		return "", ErrInvalidInput
	}
	if err := validateContext(ctx); err != nil {
		return "", err
	}
	emoji = strings.TrimSpace(emoji)
	if emoji == "" {
		return "", ErrInvalidInput
	}

	definition, err := s.store.DefinitionByEmoji(ctx, emoji)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", ErrInvalidInput
		}
		return "", fmt.Errorf("load reaction %q: %w", emoji, err)
	}
	if !definition.Active {
		return "", ErrInactive
	}

	return definition.Emoji, nil
}

// Catalog returns a validated, deterministic catalog and its version state.
func (s *Service) Catalog(ctx context.Context, knownVersion string) (Catalog, bool, error) {
	if s == nil || s.store == nil || s.media == nil {
		return Catalog{}, false, ErrInvalidInput
	}
	if err := validateContext(ctx); err != nil {
		return Catalog{}, false, err
	}
	definitions, err := s.store.Definitions(ctx)
	if err != nil {
		return Catalog{}, false, fmt.Errorf("load reaction catalog: %w", err)
	}
	if len(definitions) == 0 {
		return Catalog{}, false, ErrEmptyCatalog
	}

	sortDefinitions(definitions)
	usableDefinitions := make([]Definition, 0, len(definitions))
	defaultEmoji := ""
	for index := range definitions {
		definition := definitions[index]
		if err := validateDefinition(definition); err != nil {
			return Catalog{}, false, err
		}
		if _, err := s.assetsForDefinition(ctx, definition); err != nil {
			if errors.Is(err, media.ErrNotFound) || errors.Is(err, ErrInvalidCatalog) {
				slog.Default().Error("skip invalid reaction catalog entry", "emoji", definition.Emoji, "error", err)
				continue
			}
			return Catalog{}, false, err
		}
		usableDefinitions = append(usableDefinitions, definition)
		if definition.Active && defaultEmoji == "" {
			defaultEmoji = definition.Emoji
		}
	}
	if len(usableDefinitions) == 0 {
		return Catalog{}, false, ErrInvalidCatalog
	}
	if defaultEmoji == "" {
		return Catalog{}, false, ErrInvalidCatalog
	}

	version, err := s.catalogVersion(ctx, usableDefinitions)
	if err != nil {
		return Catalog{}, false, err
	}
	if strings.TrimSpace(knownVersion) == version {
		return Catalog{Version: version, DefaultEmoji: defaultEmoji}, true, nil
	}

	return Catalog{
		Version:      version,
		DefaultEmoji: defaultEmoji,
		Reactions:    usableDefinitions,
	}, false, nil
}

// AssetsForDefinition resolves the validated media metadata for one catalog entry.
func (s *Service) AssetsForDefinition(ctx context.Context, definition Definition) (map[string]media.MediaAsset, error) {
	if s == nil || s.media == nil {
		return nil, ErrInvalidInput
	}
	if err := validateContext(ctx); err != nil {
		return nil, err
	}
	if err := validateDefinition(definition); err != nil {
		return nil, err
	}

	return s.assetsForDefinition(ctx, definition)
}

func (s *Service) assetsForDefinition(ctx context.Context, definition Definition) (map[string]media.MediaAsset, error) {
	assets := make(map[string]media.MediaAsset, len(assetIDs(definition)))
	for _, mediaID := range assetIDs(definition) {
		asset, err := s.media.MediaAssetByID(ctx, mediaID)
		if err != nil {
			return nil, fmt.Errorf("load reaction asset %s: %w", mediaID, err)
		}
		if err := validateAsset(asset); err != nil {
			return nil, fmt.Errorf("validate reaction asset %s: %w", mediaID, err)
		}
		assets[mediaID] = asset
	}

	return assets, nil
}

func (s *Service) catalogVersion(ctx context.Context, definitions []Definition) (string, error) {
	hash := sha256.New()
	for _, definition := range definitions {
		if _, err := hash.Write([]byte(definition.Emoji + "\x00" + definition.Title + "\x00")); err != nil {
			return "", fmt.Errorf("hash reaction catalog: %w", err)
		}
		active := "0"
		if definition.Active {
			active = "1"
		}
		if _, err := hash.Write([]byte(fmt.Sprintf("%s\x00%d\x00", active, definition.SortOrder))); err != nil {
			return "", fmt.Errorf("hash reaction catalog: %w", err)
		}
		for _, mediaID := range assetIDs(definition) {
			asset, err := s.media.MediaAssetByID(ctx, mediaID)
			if err != nil {
				return "", fmt.Errorf("load reaction asset %s: %w", mediaID, err)
			}
			if err := validateAsset(asset); err != nil {
				return "", fmt.Errorf("validate reaction asset %s: %w", mediaID, err)
			}
			if _, err := hash.Write([]byte(mediaID + "\x00" + asset.SHA256Hex + "\x00")); err != nil {
				return "", fmt.Errorf("hash reaction catalog: %w", err)
			}
		}
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func validateDefinition(definition Definition) error {
	if strings.TrimSpace(definition.Emoji) == "" || strings.TrimSpace(definition.Title) == "" {
		return fmt.Errorf("%w: reaction identity is incomplete", ErrInvalidCatalog)
	}
	if definition.StaticIcon == "" || definition.AppearAnimation == "" ||
		definition.SelectAnimation == "" || definition.ActivateAnimation == "" || definition.EffectAnimation == "" {
		return fmt.Errorf("%w: reaction %q has missing required assets", ErrInvalidCatalog, definition.Emoji)
	}
	if (definition.AroundAnimation == "") != (definition.CenterAnimation == "") {
		return fmt.Errorf("%w: reaction %q has an incomplete optional asset pair", ErrInvalidCatalog, definition.Emoji)
	}

	return nil
}

func validateAsset(asset media.MediaAsset) error {
	if asset.Status != media.MediaStatusReady || !asset.PublicAccess || asset.SizeBytes == 0 || asset.SHA256Hex == "" {
		return ErrInvalidCatalog
	}
	if asset.Metadata[media.MetadataPurposeKey] != "sticker_asset" {
		return ErrInvalidCatalog
	}
	switch strings.ToLower(asset.ContentType) {
	case "image/webp", "application/x-tgsticker", "video/webm":
		return nil
	default:
		return ErrInvalidCatalog
	}
}

func assetIDs(definition Definition) []string {
	ids := []string{
		definition.StaticIcon,
		definition.AppearAnimation,
		definition.SelectAnimation,
		definition.ActivateAnimation,
		definition.EffectAnimation,
	}
	if definition.AroundAnimation != "" {
		ids = append(ids, definition.AroundAnimation, definition.CenterAnimation)
	}
	return ids
}

func sortDefinitions(definitions []Definition) {
	sort.SliceStable(definitions, func(left, right int) bool {
		if definitions[left].SortOrder == definitions[right].SortOrder {
			return definitions[left].Emoji < definitions[right].Emoji
		}
		return definitions[left].SortOrder < definitions[right].SortOrder
	})
}

func validateContext(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidInput
	}
	return ctx.Err()
}
