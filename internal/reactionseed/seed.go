package reactionseed

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/media"
	"github.com/dm-vev/zvonilka/internal/domain/reaction"
)

const (
	maxTGSFileSize       = 64 << 10
	maxTGSJSONSize       = 1 << 20
	maxWebMFileSize      = 256 << 10
	stickerAssetPurpose  = "sticker_asset"
	webPWidth            = 100
	webPHeight           = 100
	tgsCanvasWidth       = 512
	tgsCanvasHeight      = 512
	tgsFrameRate         = 60
	tgsMaxDurationSecond = 3
)

// Manifest describes a versioned reaction asset set.
type Manifest struct {
	Version      int                `json:"version"`
	DefaultEmoji string             `json:"default_emoji"`
	Reactions    []ManifestReaction `json:"reactions"`
}

// ManifestReaction describes one reaction and its source files.
type ManifestReaction struct {
	Emoji     string         `json:"emoji"`
	Slug      string         `json:"slug"`
	Title     string         `json:"title"`
	Active    bool           `json:"active"`
	SortOrder uint32         `json:"sort_order"`
	Assets    ManifestAssets `json:"assets"`
}

// ManifestAssets contains the source paths for the reaction media objects.
type ManifestAssets struct {
	StaticIcon        string `json:"static_icon"`
	AppearAnimation   string `json:"appear_animation"`
	SelectAnimation   string `json:"select_animation"`
	ActivateAnimation string `json:"activate_animation"`
	EffectAnimation   string `json:"effect_animation"`
	AroundAnimation   string `json:"around_animation"`
	CenterAnimation   string `json:"center_animation"`
}

// AssetStore persists and reads media records used by the catalog.
type AssetStore interface {
	MediaAssetByID(context.Context, string) (media.MediaAsset, error)
	MediaAssetBySHA256(context.Context, string) (media.MediaAsset, error)
	SaveMediaAsset(context.Context, media.MediaAsset) (media.MediaAsset, error)
}

// Uploader is the privileged server-side media upload operation.
type Uploader interface {
	Upload(context.Context, media.UploadParams) (media.MediaAsset, error)
}

// Seeder validates and installs a global reaction catalog.
type Seeder struct {
	uploader Uploader
	assets   AssetStore
	store    reaction.Store
	ownerID  string
	now      func() time.Time
}

// New constructs a reaction catalog seeder.
func New(uploader Uploader, assets AssetStore, store reaction.Store, ownerID string) (*Seeder, error) {
	if uploader == nil || assets == nil || store == nil || strings.TrimSpace(ownerID) == "" {
		return nil, reaction.ErrInvalidInput
	}

	return &Seeder{
		uploader: uploader,
		assets:   assets,
		store:    store,
		ownerID:  strings.TrimSpace(ownerID),
		now:      func() time.Time { return time.Now().UTC() },
	}, nil
}

// Seed validates a manifest, uploads missing assets, and upserts its definitions.
func (s *Seeder) Seed(ctx context.Context, manifestPath string, replace bool) (reaction.Catalog, error) {
	if s == nil || ctx == nil || strings.TrimSpace(manifestPath) == "" {
		return reaction.Catalog{}, reaction.ErrInvalidInput
	}
	prepared, err := LoadManifest(manifestPath)
	if err != nil {
		return reaction.Catalog{}, err
	}

	assetIDs := make(map[string]string, len(prepared.assets))
	for path, asset := range prepared.assets {
		stored, resolveErr := s.resolveAsset(ctx, asset)
		if resolveErr != nil {
			return reaction.Catalog{}, fmt.Errorf("resolve reaction asset %s: %w", path, resolveErr)
		}
		assetIDs[path] = stored.ID
	}

	definitions := make([]reaction.Definition, 0, len(prepared.manifest.Reactions))
	for _, entry := range prepared.manifest.Reactions {
		definitions = append(definitions, reaction.Definition{
			Emoji:             entry.Emoji,
			Title:             entry.Title,
			Active:            entry.Active,
			SortOrder:         entry.SortOrder,
			StaticIcon:        assetIDs[entry.Assets.StaticIcon],
			AppearAnimation:   assetIDs[entry.Assets.AppearAnimation],
			SelectAnimation:   assetIDs[entry.Assets.SelectAnimation],
			ActivateAnimation: assetIDs[entry.Assets.ActivateAnimation],
			EffectAnimation:   assetIDs[entry.Assets.EffectAnimation],
			AroundAnimation:   assetIDs[entry.Assets.AroundAnimation],
			CenterAnimation:   assetIDs[entry.Assets.CenterAnimation],
		})
	}

	if err := s.store.WithinTx(ctx, func(tx reaction.Store) error {
		if replace {
			if err := deactivateMissing(ctx, tx, definitions); err != nil {
				return err
			}
		}
		for _, definition := range definitions {
			if _, err := tx.SaveDefinition(ctx, definition); err != nil {
				return fmt.Errorf("save reaction %q: %w", definition.Emoji, err)
			}
		}
		return nil
	}); err != nil {
		return reaction.Catalog{}, fmt.Errorf("upsert reaction catalog: %w", err)
	}

	catalogService, err := reaction.NewService(s.store, s.assets)
	if err != nil {
		return reaction.Catalog{}, err
	}
	catalog, _, err := catalogService.Catalog(ctx, "")
	if err != nil {
		return reaction.Catalog{}, fmt.Errorf("validate seeded reaction catalog: %w", err)
	}
	if catalog.DefaultEmoji != prepared.manifest.DefaultEmoji {
		return reaction.Catalog{}, fmt.Errorf("default reaction %q is not the first active reaction %q", prepared.manifest.DefaultEmoji, catalog.DefaultEmoji)
	}

	return catalog, nil
}

func (s *Seeder) resolveAsset(ctx context.Context, source preparedAsset) (media.MediaAsset, error) {
	asset, err := s.assets.MediaAssetBySHA256(ctx, source.sha256)
	if err == nil {
		if err := validateStoredAsset(asset, source, false); err != nil {
			return media.MediaAsset{}, err
		}
		if !asset.PublicAccess {
			asset.PublicAccess = true
			asset.UpdatedAt = s.now().UTC()
			asset, err = s.assets.SaveMediaAsset(ctx, asset)
			if err != nil {
				return media.MediaAsset{}, fmt.Errorf("make existing asset public: %w", err)
			}
		}
		if err := validateStoredAsset(asset, source, true); err != nil {
			return media.MediaAsset{}, err
		}
		return asset, nil
	}
	if !errors.Is(err, media.ErrNotFound) {
		return media.MediaAsset{}, err
	}

	asset, err = s.uploader.Upload(ctx, media.UploadParams{
		OwnerAccountID: s.ownerID,
		Kind:           media.MediaKindSticker,
		FileName:       filepath.Base(source.path),
		ContentType:    source.mimeType,
		SizeBytes:      uint64(len(source.data)),
		SHA256Hex:      source.sha256,
		Width:          source.width,
		Height:         source.height,
		Metadata:       map[string]string{media.MetadataPurposeKey: stickerAssetPurpose},
		PublicAccess:   true,
		Body:           bytes.NewReader(source.data),
		CreatedAt:      s.now().UTC(),
	})
	if err != nil {
		return media.MediaAsset{}, fmt.Errorf("upload reaction asset: %w", err)
	}
	if err := validateStoredAsset(asset, source, true); err != nil {
		return media.MediaAsset{}, err
	}
	return asset, nil
}

func validateStoredAsset(asset media.MediaAsset, source preparedAsset, requirePublic bool) error {
	if asset.ID == "" || asset.Status != media.MediaStatusReady || (requirePublic && !asset.PublicAccess) {
		return reaction.ErrInvalidCatalog
	}
	if asset.Kind != media.MediaKindSticker || asset.SizeBytes != uint64(len(source.data)) || asset.SHA256Hex != source.sha256 {
		return reaction.ErrInvalidCatalog
	}
	if asset.ContentType != source.mimeType || asset.Metadata[media.MetadataPurposeKey] != stickerAssetPurpose {
		return reaction.ErrInvalidCatalog
	}
	return nil
}

func deactivateMissing(ctx context.Context, store reaction.Store, definitions []reaction.Definition) error {
	keep := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		keep[definition.Emoji] = struct{}{}
	}
	current, err := store.Definitions(ctx)
	if err != nil {
		return fmt.Errorf("list existing reactions: %w", err)
	}
	for _, definition := range current {
		if _, ok := keep[definition.Emoji]; ok || !definition.Active {
			continue
		}
		definition.Active = false
		if _, err := store.SaveDefinition(ctx, definition); err != nil {
			return fmt.Errorf("deactivate reaction %q: %w", definition.Emoji, err)
		}
	}
	return nil
}

// LoadManifest reads and validates a reaction manifest and its source files.
func LoadManifest(path string) (PreparedManifest, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	data, err := os.ReadFile(path)
	if err != nil {
		return PreparedManifest{}, fmt.Errorf("read reaction manifest: %w", err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return PreparedManifest{}, fmt.Errorf("decode reaction manifest: %w", err)
	}
	if err := validateManifest(&manifest); err != nil {
		return PreparedManifest{}, err
	}

	baseDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return PreparedManifest{}, fmt.Errorf("resolve reaction manifest directory: %w", err)
	}
	assets := make(map[string]preparedAsset)
	for _, entry := range manifest.Reactions {
		for _, assetPath := range assetPaths(entry.Assets) {
			if assetPath == "" {
				continue
			}
			if _, ok := assets[assetPath]; ok {
				continue
			}
			resolved, err := safeAssetPath(baseDir, assetPath)
			if err != nil {
				return PreparedManifest{}, fmt.Errorf("validate reaction asset %s: %w", assetPath, err)
			}
			asset, err := readAsset(resolved, assetPath)
			if err != nil {
				return PreparedManifest{}, fmt.Errorf("validate reaction asset %s: %w", assetPath, err)
			}
			assets[assetPath] = asset
		}
	}

	return PreparedManifest{manifest: manifest, assets: assets}, nil
}

// PreparedManifest is the validated manifest and its content-addressed assets.
type PreparedManifest struct {
	manifest Manifest
	assets   map[string]preparedAsset
}

func validateManifest(manifest *Manifest) error {
	if manifest == nil || len(manifest.Reactions) == 0 || strings.TrimSpace(manifest.DefaultEmoji) == "" {
		return reaction.ErrInvalidInput
	}
	seenEmoji := make(map[string]struct{}, len(manifest.Reactions))
	seenOrder := make(map[uint32]struct{}, len(manifest.Reactions))
	defaultFound := false
	defaultActive := false
	for _, entry := range manifest.Reactions {
		if entry.Emoji == "" || strings.TrimSpace(entry.Emoji) != entry.Emoji || entry.Title == "" || entry.SortOrder == 0 {
			return fmt.Errorf("%w: invalid reaction identity", reaction.ErrInvalidInput)
		}
		if _, ok := seenEmoji[entry.Emoji]; ok {
			return fmt.Errorf("%w: duplicate emoji %q", reaction.ErrInvalidInput, entry.Emoji)
		}
		if _, ok := seenOrder[entry.SortOrder]; ok {
			return fmt.Errorf("%w: duplicate sort_order %d", reaction.ErrInvalidInput, entry.SortOrder)
		}
		seenEmoji[entry.Emoji] = struct{}{}
		seenOrder[entry.SortOrder] = struct{}{}
		if entry.Emoji == manifest.DefaultEmoji {
			defaultFound = true
			defaultActive = entry.Active
		}
		if err := validateAssetPaths(entry.Assets); err != nil {
			return fmt.Errorf("reaction %q: %w", entry.Emoji, err)
		}
	}
	if !defaultFound || !defaultActive {
		return fmt.Errorf("%w: default reaction must exist and be active", reaction.ErrInvalidInput)
	}
	return nil
}

func validateAssetPaths(assets ManifestAssets) error {
	if strings.ToLower(filepath.Ext(assets.StaticIcon)) != ".webp" {
		return fmt.Errorf("%w: static_icon must be a WebP", reaction.ErrInvalidInput)
	}
	paths := []string{assets.StaticIcon, assets.AppearAnimation, assets.SelectAnimation, assets.ActivateAnimation, assets.EffectAnimation}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%w: required asset is missing", reaction.ErrInvalidInput)
		}
	}
	if (strings.TrimSpace(assets.AroundAnimation) == "") != (strings.TrimSpace(assets.CenterAnimation) == "") {
		return fmt.Errorf("%w: optional assets must be specified as a pair", reaction.ErrInvalidInput)
	}
	return nil
}

func safeAssetPath(baseDir, assetPath string) (string, error) {
	if filepath.IsAbs(assetPath) {
		return "", errors.New("asset path must be relative to the manifest")
	}
	resolved, err := filepath.Abs(filepath.Join(baseDir, filepath.Clean(assetPath)))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(baseDir, resolved)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("asset path escapes the manifest directory")
	}
	return resolved, nil
}

type preparedAsset struct {
	path     string
	data     []byte
	mimeType string
	width    uint32
	height   uint32
	sha256   string
}

func readAsset(path, manifestPath string) (preparedAsset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return preparedAsset{}, err
	}
	if len(data) == 0 {
		return preparedAsset{}, errors.New("asset is empty")
	}
	mimeType, err := mimeTypeForPath(manifestPath)
	if err != nil {
		return preparedAsset{}, err
	}
	digest := sha256.Sum256(data)
	asset := preparedAsset{
		path:     manifestPath,
		data:     data,
		mimeType: mimeType,
		sha256:   hex.EncodeToString(digest[:]),
	}
	switch mimeType {
	case "image/webp":
		if len(data) < 30 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" || string(data[12:16]) != "VP8X" {
			return preparedAsset{}, errors.New("invalid WebP header")
		}
		width := 1 + uint32(data[24]) + uint32(data[25])<<8 + uint32(data[26])<<16
		height := 1 + uint32(data[27]) + uint32(data[28])<<8 + uint32(data[29])<<16
		if width != webPWidth || height != webPHeight {
			return preparedAsset{}, fmt.Errorf("WebP must be %dx%d, got %dx%d", webPWidth, webPHeight, width, height)
		}
		asset.width, asset.height = webPWidth, webPHeight
	case "application/x-tgsticker":
		width, height, err := validateTGS(data)
		if err != nil {
			return preparedAsset{}, err
		}
		asset.width, asset.height = width, height
	case "video/webm":
		if len(data) > maxWebMFileSize || len(data) < 4 || string(data[:4]) != "\x1a\x45\xdf\xa3" {
			return preparedAsset{}, errors.New("invalid WebM asset")
		}
	}
	return asset, nil
}

func mimeTypeForPath(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".webp":
		return "image/webp", nil
	case ".tgs":
		return "application/x-tgsticker", nil
	case ".webm":
		return "video/webm", nil
	default:
		return "", fmt.Errorf("unsupported asset extension %q", filepath.Ext(path))
	}
}

func validateTGS(data []byte) (uint32, uint32, error) {
	if len(data) > maxTGSFileSize || len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return 0, 0, errors.New("invalid TGS gzip payload")
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("open TGS gzip payload: %w", err)
	}
	decompressed, err := io.ReadAll(io.LimitReader(reader, maxTGSJSONSize+1))
	closeErr := reader.Close()
	if err != nil {
		return 0, 0, fmt.Errorf("read TGS JSON: %w", err)
	}
	if closeErr != nil {
		return 0, 0, fmt.Errorf("close TGS gzip payload: %w", closeErr)
	}
	if len(decompressed) > maxTGSJSONSize {
		return 0, 0, errors.New("TGS JSON exceeds 1 MiB")
	}
	var animation struct {
		Width     float64 `json:"w"`
		Height    float64 `json:"h"`
		FrameRate float64 `json:"fr"`
		InPoint   float64 `json:"ip"`
		OutPoint  float64 `json:"op"`
	}
	if err := json.Unmarshal(decompressed, &animation); err != nil {
		return 0, 0, fmt.Errorf("decode TGS JSON: %w", err)
	}
	if animation.Width != tgsCanvasWidth || animation.Height != tgsCanvasHeight || animation.FrameRate != tgsFrameRate || animation.OutPoint < animation.InPoint || (animation.OutPoint-animation.InPoint)/animation.FrameRate > tgsMaxDurationSecond {
		return 0, 0, errors.New("TGS animation constraints are not satisfied")
	}
	return tgsCanvasWidth, tgsCanvasHeight, nil
}

func assetPaths(assets ManifestAssets) []string {
	return []string{
		assets.StaticIcon,
		assets.AppearAnimation,
		assets.SelectAnimation,
		assets.ActivateAnimation,
		assets.EffectAnimation,
		assets.AroundAnimation,
		assets.CenterAnimation,
	}
}
