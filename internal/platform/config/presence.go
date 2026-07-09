package config

import (
	"time"

	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
)

// PresenceConfig defines the tunable presence derivation settings.
type PresenceConfig struct {
	OnlineWindow time.Duration
}

// ToSettings converts configuration into domain presence settings.
func (c PresenceConfig) ToSettings() domainpresence.Settings {
	return domainpresence.Settings{
		OnlineWindow: c.OnlineWindow,
	}
}
