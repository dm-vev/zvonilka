package call

import "time"

// Settings controls call lifecycle timing.
type Settings struct {
	InviteTimeout  time.Duration
	RingingTimeout time.Duration
	ReconnectGrace time.Duration
	MaxDuration    time.Duration
}

// DefaultSettings returns the default call settings.
func DefaultSettings() Settings {
	return Settings{
		InviteTimeout:  45 * time.Second,
		RingingTimeout: 45 * time.Second,
		ReconnectGrace: 20 * time.Second,
		MaxDuration:    2 * time.Hour,
	}
}

func (s Settings) normalize() Settings {
	defaults := DefaultSettings()
	if s.InviteTimeout <= 0 {
		s.InviteTimeout = defaults.InviteTimeout
	}
	if s.RingingTimeout <= 0 {
		s.RingingTimeout = defaults.RingingTimeout
	}
	if s.ReconnectGrace <= 0 {
		s.ReconnectGrace = defaults.ReconnectGrace
	}
	if s.MaxDuration <= 0 {
		s.MaxDuration = defaults.MaxDuration
	}

	return s
}
