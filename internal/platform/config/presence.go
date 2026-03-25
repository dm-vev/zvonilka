package config

import (
	"time"

	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
)

// PresenceConfig controls presence and last-seen derivation.
type PresenceConfig struct {
	OnlineWindow time.Duration
}

// ToSettings converts configuration into domain presence settings.
func (c PresenceConfig) ToSettings() domainpresence.Settings {
	settings := domainpresence.DefaultSettings()
	if c.OnlineWindow > 0 {
		settings.OnlineWindow = c.OnlineWindow
	}

	return settings
}
