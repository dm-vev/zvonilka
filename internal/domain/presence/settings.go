package presence

import "time"

// Settings controls presence derivation behavior.
type Settings struct {
	OnlineWindow time.Duration
}

// DefaultSettings returns a conservative baseline for presence derivation.
func DefaultSettings() Settings {
	return Settings{
		OnlineWindow: 5 * time.Minute,
	}
}

func (s Settings) normalize() Settings {
	if s.OnlineWindow <= 0 {
		return DefaultSettings()
	}

	return s
}
