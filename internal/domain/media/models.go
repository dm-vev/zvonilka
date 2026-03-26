package media

import (
	"strings"
	"time"
)

// MediaKind identifies the family of stored media.
type MediaKind string

// Supported media kinds.
const (
	MediaKindUnspecified MediaKind = ""
	MediaKindImage       MediaKind = "image"
	MediaKindVideo       MediaKind = "video"
	MediaKindDocument    MediaKind = "document"
	MediaKindVoice       MediaKind = "voice"
	MediaKindSticker     MediaKind = "sticker"
	MediaKindGIF         MediaKind = "gif"
	MediaKindAvatar      MediaKind = "avatar"
	MediaKindFile        MediaKind = "file"
)

// MediaStatus tracks the lifecycle of a media asset.
type MediaStatus string

// Supported media statuses.
const (
	MediaStatusUnspecified MediaStatus = ""
	MediaStatusReserved    MediaStatus = "reserved"
	MediaStatusReady       MediaStatus = "ready"
	MediaStatusFailed      MediaStatus = "failed"
	MediaStatusDeleted     MediaStatus = "deleted"
)

const (
	MetadataPurposeKey        = "purpose"
	MetadataConversationIDKey = "conversation_id"
)

const metadataVariantObjectKeyPrefix = "variant_object_key."

// MediaAsset describes a stored media record.
type MediaAsset struct {
	ID              string
	OwnerAccountID  string
	Kind            MediaKind
	Status          MediaStatus
	StorageProvider string
	Bucket          string
	ObjectKey       string
	FileName        string
	ContentType     string
	SizeBytes       uint64
	SHA256Hex       string
	Width           uint32
	Height          uint32
	Duration        time.Duration
	Metadata        map[string]string
	UploadExpiresAt time.Time
	ReadyAt         time.Time
	DeletedAt       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UploadTarget describes a presigned upload request.
type UploadTarget struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
	ObjectKey string
	Bucket    string
}

// DownloadTarget describes a presigned download target.
type DownloadTarget struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

func variantObjectKeyMetadataKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	return metadataVariantObjectKeyPrefix + name
}
