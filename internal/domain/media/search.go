package media

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
)

func (s *Service) indexMedia(ctx context.Context, asset MediaAsset) {
	if s == nil || s.indexer == nil || asset.ID == "" {
		return
	}

	document := domainsearch.Document{
		Scope:      domainsearch.SearchScopeMedia,
		EntityType: "asset",
		TargetID:   asset.ID,
		Title:      mediaTitle(asset),
		Subtitle:   mediaSubtitle(asset),
		Snippet:    mediaSnippet(asset),
		UserID:     asset.OwnerAccountID,
		Metadata:   mediaMetadata(asset),
		UpdatedAt:  asset.UpdatedAt,
		CreatedAt:  asset.CreatedAt,
	}

	_, _ = s.indexer.UpsertDocument(ctx, document)
}

func (s *Service) deleteMediaIndex(ctx context.Context, asset MediaAsset) {
	if s == nil || s.indexer == nil || asset.ID == "" {
		return
	}

	_ = s.indexer.DeleteDocument(ctx, domainsearch.SearchScopeMedia, asset.ID)
}

func mediaTitle(asset MediaAsset) string {
	if strings.TrimSpace(asset.FileName) != "" {
		return strings.TrimSpace(asset.FileName)
	}
	return asset.ID
}

func mediaSubtitle(asset MediaAsset) string {
	parts := []string{
		string(asset.Kind),
		string(asset.Status),
		asset.ContentType,
	}
	return strings.Join(nonEmptyStrings(parts), " ")
}

func mediaSnippet(asset MediaAsset) string {
	parts := []string{
		asset.OwnerAccountID,
		asset.StorageProvider,
		asset.Bucket,
		asset.ObjectKey,
		asset.FileName,
		asset.ContentType,
		asset.SHA256Hex,
		fmt.Sprintf("%d", asset.SizeBytes),
		fmt.Sprintf("%dx%d", asset.Width, asset.Height),
		fmt.Sprintf("%d", asset.Duration.Nanoseconds()),
	}
	return strings.Join(nonEmptyStrings(parts), " ")
}

func mediaMetadata(asset MediaAsset) map[string]string {
	metadata := map[string]string{
		"owner_account_id": asset.OwnerAccountID,
		"kind":             strings.ToLower(string(asset.Kind)),
		"status":           strings.ToLower(string(asset.Status)),
		"storage_provider": asset.StorageProvider,
		"bucket":           asset.Bucket,
		"file_name":        asset.FileName,
		"content_type":     asset.ContentType,
		"size_bytes":       fmt.Sprintf("%d", asset.SizeBytes),
		"width":            fmt.Sprintf("%d", asset.Width),
		"height":           fmt.Sprintf("%d", asset.Height),
		"duration_nanos":   fmt.Sprintf("%d", asset.Duration.Nanoseconds()),
		"sha256":           asset.SHA256Hex,
	}
	if !asset.ReadyAt.IsZero() {
		metadata["ready_at"] = asset.ReadyAt.UTC().Format(time.RFC3339Nano)
	}
	if !asset.DeletedAt.IsZero() {
		metadata["deleted_at"] = asset.DeletedAt.UTC().Format(time.RFC3339Nano)
	}
	if !asset.UploadExpiresAt.IsZero() {
		metadata["upload_expires_at"] = asset.UploadExpiresAt.UTC().Format(time.RFC3339Nano)
	}

	return metadata
}

func nonEmptyStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		filtered = append(filtered, value)
	}

	return filtered
}
