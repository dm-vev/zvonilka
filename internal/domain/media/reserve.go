package media

import (
	"context"
	"fmt"
	"strings"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

// ReserveUpload creates a media reservation and returns a presigned upload target.
func (s *Service) ReserveUpload(ctx context.Context, params ReserveUploadParams) (MediaAsset, UploadTarget, error) {
	if err := s.validateContext(ctx, "reserve media upload"); err != nil {
		return MediaAsset{}, UploadTarget{}, err
	}
	params.OwnerAccountID = strings.TrimSpace(params.OwnerAccountID)
	params.FileName = strings.TrimSpace(params.FileName)
	params.ContentType = strings.TrimSpace(params.ContentType)
	params.SHA256Hex = strings.TrimSpace(params.SHA256Hex)
	params.Metadata = normalizeMetadata(params.Metadata)
	params.Kind = normalizeAssetKind(params.Kind)
	if params.OwnerAccountID == "" || params.Kind == MediaKindUnspecified || params.FileName == "" {
		return MediaAsset{}, UploadTarget{}, ErrInvalidInput
	}
	if err := ensureUploadSize(s.settings, params.SizeBytes); err != nil {
		return MediaAsset{}, UploadTarget{}, err
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	mediaID, err := newID("media")
	if err != nil {
		return MediaAsset{}, UploadTarget{}, fmt.Errorf("generate media id: %w", err)
	}

	objectKey := mediaObjectKey(params.OwnerAccountID, mediaID)
	presignedUpload, err := s.blob.PresignPutObject(ctx, objectKey, s.settings.UploadURLTTL, domainstorage.PutObjectOptions{
		ContentType: params.ContentType,
		Metadata:    uploadMetadata(params),
	})
	if err != nil {
		return MediaAsset{}, UploadTarget{}, fmt.Errorf("presign upload for media %s: %w", mediaID, err)
	}

	var saved MediaAsset
	err = s.store.WithinTx(ctx, func(tx Store) error {
		asset := MediaAsset{
			ID:              mediaID,
			OwnerAccountID:  params.OwnerAccountID,
			Kind:            params.Kind,
			Status:          MediaStatusReserved,
			StorageProvider: s.blob.Name(),
			Bucket:          s.blob.Bucket(),
			ObjectKey:       objectKey,
			FileName:        params.FileName,
			ContentType:     params.ContentType,
			SizeBytes:       params.SizeBytes,
			SHA256Hex:       params.SHA256Hex,
			Width:           params.Width,
			Height:          params.Height,
			Duration:        params.Duration,
			Metadata:        params.Metadata,
			UploadExpiresAt: presignedUpload.ExpiresAt,
			CreatedAt:       now,
			UpdatedAt:       now,
		}

		var saveErr error
		saved, saveErr = tx.SaveMediaAsset(ctx, asset)
		if saveErr != nil {
			return fmt.Errorf("save media reservation: %w", saveErr)
		}

		return nil
	})
	if err != nil {
		return MediaAsset{}, UploadTarget{}, err
	}

	return saved, UploadTarget{
		URL:       presignedUpload.URL,
		Method:    presignedUpload.Method,
		Headers:   presignedUpload.Headers,
		ExpiresAt: presignedUpload.ExpiresAt,
		ObjectKey: saved.ObjectKey,
		Bucket:    saved.Bucket,
	}, nil
}

func uploadMetadata(params ReserveUploadParams) map[string]string {
	metadata := make(map[string]string, len(params.Metadata)+4)
	for key, value := range params.Metadata {
		metadata[key] = value
	}
	if params.SHA256Hex != "" {
		metadata["sha256"] = params.SHA256Hex
	}
	if params.FileName != "" {
		metadata["file_name"] = params.FileName
	}
	if params.ContentType != "" {
		metadata["content_type"] = params.ContentType
	}
	if params.Duration > 0 {
		metadata["duration_nanos"] = fmt.Sprintf("%d", params.Duration.Nanoseconds())
	}

	return metadata
}
