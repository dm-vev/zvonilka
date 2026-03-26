package gateway

import (
	"context"
	"strings"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	mediav1 "github.com/dm-vev/zvonilka/gen/proto/contracts/media/v1"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
)

// InitiateUpload reserves a media asset and returns a presigned upload target.
func (a *api) InitiateUpload(
	ctx context.Context,
	req *mediav1.InitiateUploadRequest,
) (*mediav1.InitiateUploadResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if owner := strings.TrimSpace(req.GetOwnerUserId()); owner != "" && owner != authContext.Account.ID {
		return nil, grpcError(domainmedia.ErrForbidden)
	}

	asset, upload, err := a.media.ReserveUpload(ctx, domainmedia.ReserveUploadParams{
		OwnerAccountID: authContext.Account.ID,
		Kind:           mediaKind(req.GetPurpose(), req.GetMimeType()),
		FileName:       req.GetFileName(),
		ContentType:    req.GetMimeType(),
		SizeBytes:      req.GetSizeBytes(),
		SHA256Hex:      req.GetSha256Hex(),
		Metadata:       mediaUploadMetadata(req),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &mediav1.InitiateUploadResponse{
		Media: mediaObject(asset),
		Upload: &mediav1.UploadSession{
			UploadId:       asset.ID,
			MediaId:        asset.ID,
			UploadUrl:      upload.URL,
			Headers:        upload.Headers,
			ExpiresAt:      protoTime(upload.ExpiresAt),
			ChecksumSha256: asset.SHA256Hex,
		},
	}, nil
}

func mediaKind(purpose commonv1.MediaPurpose, mimeType string) domainmedia.MediaKind {
	switch purpose {
	case commonv1.MediaPurpose_MEDIA_PURPOSE_PROFILE_AVATAR,
		commonv1.MediaPurpose_MEDIA_PURPOSE_CHAT_AVATAR,
		commonv1.MediaPurpose_MEDIA_PURPOSE_BOT_AVATAR:
		return domainmedia.MediaKindAvatar
	case commonv1.MediaPurpose_MEDIA_PURPOSE_STICKER_ASSET:
		return domainmedia.MediaKindSticker
	}

	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return domainmedia.MediaKindImage
	case strings.HasPrefix(mimeType, "video/"):
		return domainmedia.MediaKindVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return domainmedia.MediaKindVoice
	default:
		return domainmedia.MediaKindFile
	}
}

// CompleteUpload finalizes an uploaded media asset.
func (a *api) CompleteUpload(
	ctx context.Context,
	req *mediav1.CompleteUploadRequest,
) (*mediav1.CompleteUploadResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	mediaID := strings.TrimSpace(req.GetMediaId())
	if mediaID == "" {
		mediaID = strings.TrimSpace(req.GetUploadId())
	}

	asset, err := a.media.FinalizeUpload(ctx, domainmedia.FinalizeUploadParams{
		OwnerAccountID: authContext.Account.ID,
		MediaID:        mediaID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &mediav1.CompleteUploadResponse{Media: mediaObject(asset)}, nil
}

// GetMedia returns one owned media asset.
func (a *api) GetMedia(
	ctx context.Context,
	req *mediav1.GetMediaRequest,
) (*mediav1.GetMediaResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	asset, err := a.media.GetMedia(ctx, authContext.Account.ID, req.GetMediaId())
	if err != nil {
		return nil, grpcError(err)
	}

	return &mediav1.GetMediaResponse{Media: mediaObject(asset)}, nil
}

// GetDownloadUrl resolves a presigned download target.
func (a *api) GetDownloadUrl(
	ctx context.Context,
	req *mediav1.GetDownloadUrlRequest,
) (*mediav1.GetDownloadUrlResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	_, target, err := a.media.GetDownloadURL(ctx, domainmedia.GetDownloadParams{
		OwnerAccountID: authContext.Account.ID,
		MediaID:        req.GetMediaId(),
		Variant:        req.GetVariant(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &mediav1.GetDownloadUrlResponse{
		Url:       target.URL,
		ExpiresAt: protoTime(target.ExpiresAt),
		Headers:   target.Headers,
	}, nil
}

// ListMedia returns the media assets owned by the authenticated account.
func (a *api) ListMedia(
	ctx context.Context,
	req *mediav1.ListMediaRequest,
) (*mediav1.ListMediaResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if owner := strings.TrimSpace(req.GetOwnerUserId()); owner != "" && owner != authContext.Account.ID {
		return nil, grpcError(domainmedia.ErrForbidden)
	}

	assets, err := a.media.ListMedia(ctx, domainmedia.ListParams{
		OwnerAccountID: authContext.Account.ID,
		Limit:          500,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	filteredAssets := make([]domainmedia.MediaAsset, 0, len(assets))
	for _, asset := range assets {
		if !mediaMatchesListFilters(asset, req.GetPurposes(), req.GetConversationId()) {
			continue
		}
		filteredAssets = append(filteredAssets, asset)
	}
	assets = filteredAssets

	offset, err := decodeOffset(req.GetPage(), "media")
	if err != nil {
		return nil, grpcError(domainmedia.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(assets) {
		end = len(assets)
	}

	page := assets
	if offset < len(assets) {
		page = assets[offset:end]
	} else {
		page = nil
	}

	result := make([]*mediav1.MediaObject, 0, len(page))
	for _, asset := range page {
		result = append(result, mediaObject(asset))
	}

	nextToken := ""
	if end < len(assets) {
		nextToken = offsetToken("media", end)
	}

	return &mediav1.ListMediaResponse{
		Media: result,
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
			TotalSize:     uint64(len(assets)),
		},
	}, nil
}

// DeleteMedia deletes one media asset owned by the authenticated account.
func (a *api) DeleteMedia(
	ctx context.Context,
	req *mediav1.DeleteMediaRequest,
) (*mediav1.DeleteMediaResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	asset, err := a.media.DeleteMedia(ctx, domainmedia.DeleteParams{
		OwnerAccountID: authContext.Account.ID,
		MediaID:        req.GetMediaId(),
		HardDelete:     req.GetHardDelete(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &mediav1.DeleteMediaResponse{Media: mediaObject(asset)}, nil
}
