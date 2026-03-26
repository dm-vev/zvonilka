package bot

import "time"

// Option mutates bot service construction.
type Option func(*Service)

// WithSettings overrides the default bot settings.
func WithSettings(settings Settings) Option {
	return func(service *Service) {
		if service == nil {
			return
		}
		service.settings = settings
	}
}

// WithNow overrides the service clock.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if service == nil || now == nil {
			return
		}
		service.now = now
	}
}
