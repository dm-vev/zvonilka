package translation

import "time"

// Option configures a translation service.
type Option func(*Service)

// WithNow overrides the translation clock.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if service != nil && now != nil {
			service.now = now
		}
	}
}
