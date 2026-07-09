package user

import "time"

// Option mutates service configuration during construction.
type Option func(*Service)

// WithNow overrides the service clock.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if service != nil && now != nil {
			service.now = now
		}
	}
}
