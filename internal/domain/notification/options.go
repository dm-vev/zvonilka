package notification

import "time"

// Option configures a notification service instance.
type Option func(*Service)

// WithNow overrides the service clock for tests.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if service != nil && now != nil {
			service.now = now
		}
	}
}

// WithSettings overrides the service settings.
func WithSettings(settings Settings) Option {
	return func(service *Service) {
		if service != nil {
			service.settings = settings.normalize()
		}
	}
}
