package config

import (
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/media"
)

// MediaConfig defines upload and download access settings for the media subsystem.
type MediaConfig struct {
	UploadURLTTL   time.Duration
	DownloadURLTTL time.Duration
	MaxUploadSize  int64
}

// ToSettings converts configuration into domain media settings.
func (c MediaConfig) ToSettings() media.Settings {
	return media.Settings{
		UploadURLTTL:   c.UploadURLTTL,
		DownloadURLTTL: c.DownloadURLTTL,
		MaxUploadSize:  c.MaxUploadSize,
	}
}
