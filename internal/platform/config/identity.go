package config

import (
	"time"

	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
)

// IdentityConfig defines the tunable identity lifecycle settings.
type IdentityConfig struct {
	JoinRequestTTL  time.Duration
	ChallengeTTL    time.Duration
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	LoginCodeLength int
}

// ToSettings converts the config-facing identity lifecycle values into the
// domain-layer settings shape used by the identity service.
func (c IdentityConfig) ToSettings() domainidentity.Settings {
	return domainidentity.Settings{
		JoinRequestTTL:  c.JoinRequestTTL,
		ChallengeTTL:    c.ChallengeTTL,
		AccessTokenTTL:  c.AccessTokenTTL,
		RefreshTokenTTL: c.RefreshTokenTTL,
		LoginCodeLength: c.LoginCodeLength,
	}
}
