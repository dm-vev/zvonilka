package identity

import "time"

// Settings captures the tunable lifecycle limits for the identity service.
//
// The values mirror the current defaults but stay explicit so application
// configuration can drive them without hidden literals in the domain layer.
type Settings struct {
	JoinRequestTTL  time.Duration
	ChallengeTTL    time.Duration
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	LoginCodeLength int
}

// DefaultSettings returns the production-safe identity defaults.
func DefaultSettings() Settings {
	return Settings{
		JoinRequestTTL:  72 * time.Hour,
		ChallengeTTL:    10 * time.Minute,
		AccessTokenTTL:  30 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		LoginCodeLength: 6,
	}
}

// normalize fills zero values with the package defaults.
func (s Settings) normalize() Settings {
	defaults := DefaultSettings()

	if s.JoinRequestTTL <= 0 {
		s.JoinRequestTTL = defaults.JoinRequestTTL
	}
	if s.ChallengeTTL <= 0 {
		s.ChallengeTTL = defaults.ChallengeTTL
	}
	if s.AccessTokenTTL <= 0 {
		s.AccessTokenTTL = defaults.AccessTokenTTL
	}
	if s.RefreshTokenTTL <= 0 {
		s.RefreshTokenTTL = defaults.RefreshTokenTTL
	}
	if s.LoginCodeLength <= 0 {
		s.LoginCodeLength = defaults.LoginCodeLength
	}

	return s
}

// WithSettings overrides the tunable identity lifecycle settings.
func WithSettings(settings Settings) Option {
	settings = settings.normalize()

	return func(service *Service) {
		service.joinRequestTTL = settings.JoinRequestTTL
		service.challengeTTL = settings.ChallengeTTL
		service.accessTokenTTL = settings.AccessTokenTTL
		service.refreshTokenTTL = settings.RefreshTokenTTL
		service.loginCodeLength = settings.LoginCodeLength
	}
}
