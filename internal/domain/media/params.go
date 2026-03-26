package media

import "time"

// ReserveUploadParams describes a new media upload reservation.
type ReserveUploadParams struct {
	OwnerAccountID string
	Kind           MediaKind
	FileName       string
	ContentType    string
	SizeBytes      uint64
	SHA256Hex      string
	Width          uint32
	Height         uint32
	Duration       time.Duration
	Metadata       map[string]string
	CreatedAt      time.Time
}

// FinalizeUploadParams resolves an upload reservation after the object exists.
type FinalizeUploadParams struct {
	OwnerAccountID string
	MediaID        string
	CreatedAt      time.Time
}

// GetDownloadParams identifies a media asset for download URL generation.
type GetDownloadParams struct {
	OwnerAccountID string
	MediaID        string
	Variant        string
}

// DeleteParams identifies a media asset for deletion.
type DeleteParams struct {
	OwnerAccountID string
	MediaID        string
	HardDelete     bool
	DeletedAt      time.Time
}

// ListParams filters the asset list for an owner.
type ListParams struct {
	OwnerAccountID string
	Limit          int
	IncludeDeleted bool
}
