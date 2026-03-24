package media

import "time"

// Settings controls upload and download lifecycle limits.
type Settings struct {
	UploadURLTTL   time.Duration
	DownloadURLTTL time.Duration
	MaxUploadSize  int64
}

// DefaultSettings returns a conservative, production-safe baseline.
func DefaultSettings() Settings {
	return Settings{
		UploadURLTTL:   15 * time.Minute,
		DownloadURLTTL: 15 * time.Minute,
		MaxUploadSize:  100 << 20,
	}
}

func normalizeSettings(settings Settings) Settings {
	defaults := DefaultSettings()
	if settings.UploadURLTTL <= 0 {
		settings.UploadURLTTL = defaults.UploadURLTTL
	}
	if settings.DownloadURLTTL <= 0 {
		settings.DownloadURLTTL = defaults.DownloadURLTTL
	}
	if settings.MaxUploadSize <= 0 {
		settings.MaxUploadSize = defaults.MaxUploadSize
	}

	return settings
}
