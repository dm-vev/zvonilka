package gateway

import (
	"strings"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	mediav1 "github.com/dm-vev/zvonilka/gen/proto/contracts/media/v1"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
)

func mediaUploadMetadata(req *mediav1.InitiateUploadRequest) map[string]string {
	metadata := make(map[string]string, 2)

	if purposeValue := mediaPurposeMetadataValue(req.GetPurpose()); purposeValue != "" {
		metadata[domainmedia.MetadataPurposeKey] = purposeValue
	}

	if conversationID := strings.TrimSpace(req.GetConversationId()); conversationID != "" {
		metadata[domainmedia.MetadataConversationIDKey] = conversationID
	}

	if len(metadata) == 0 {
		return nil
	}

	return metadata
}

func mediaMatchesListFilters(
	asset domainmedia.MediaAsset,
	purposes []commonv1.MediaPurpose,
	conversationID string,
) bool {
	if len(purposes) > 0 {
		matchesPurpose := false
		assetPurpose := mediaPurposeFromAsset(asset)
		for _, purpose := range purposes {
			if purpose == assetPurpose {
				matchesPurpose = true
				break
			}
		}
		if !matchesPurpose {
			return false
		}
	}

	if conversationID = strings.TrimSpace(conversationID); conversationID != "" {
		if mediaConversationID(asset) != conversationID {
			return false
		}
	}

	return true
}

func mediaPurposeFromAsset(asset domainmedia.MediaAsset) commonv1.MediaPurpose {
	if metadataPurpose, ok := mediaPurposeFromMetadataValue(asset.Metadata[domainmedia.MetadataPurposeKey]); ok {
		return metadataPurpose
	}

	return mediaPurposeFromKind(asset.Kind)
}

func mediaConversationID(asset domainmedia.MediaAsset) string {
	return strings.TrimSpace(asset.Metadata[domainmedia.MetadataConversationIDKey])
}

func mediaPurposeMetadataValue(purpose commonv1.MediaPurpose) string {
	switch purpose {
	case commonv1.MediaPurpose_MEDIA_PURPOSE_MESSAGE_ATTACHMENT:
		return "message_attachment"
	case commonv1.MediaPurpose_MEDIA_PURPOSE_PROFILE_AVATAR:
		return "profile_avatar"
	case commonv1.MediaPurpose_MEDIA_PURPOSE_CHAT_AVATAR:
		return "chat_avatar"
	case commonv1.MediaPurpose_MEDIA_PURPOSE_BOT_AVATAR:
		return "bot_avatar"
	case commonv1.MediaPurpose_MEDIA_PURPOSE_STICKER_ASSET:
		return "sticker_asset"
	case commonv1.MediaPurpose_MEDIA_PURPOSE_EXPORT:
		return "export"
	case commonv1.MediaPurpose_MEDIA_PURPOSE_TEMPORARY:
		return "temporary"
	default:
		return ""
	}
}

func mediaPurposeFromMetadataValue(raw string) (commonv1.MediaPurpose, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "message_attachment":
		return commonv1.MediaPurpose_MEDIA_PURPOSE_MESSAGE_ATTACHMENT, true
	case "profile_avatar":
		return commonv1.MediaPurpose_MEDIA_PURPOSE_PROFILE_AVATAR, true
	case "chat_avatar":
		return commonv1.MediaPurpose_MEDIA_PURPOSE_CHAT_AVATAR, true
	case "bot_avatar":
		return commonv1.MediaPurpose_MEDIA_PURPOSE_BOT_AVATAR, true
	case "sticker_asset":
		return commonv1.MediaPurpose_MEDIA_PURPOSE_STICKER_ASSET, true
	case "export":
		return commonv1.MediaPurpose_MEDIA_PURPOSE_EXPORT, true
	case "temporary":
		return commonv1.MediaPurpose_MEDIA_PURPOSE_TEMPORARY, true
	default:
		return commonv1.MediaPurpose_MEDIA_PURPOSE_UNSPECIFIED, false
	}
}
