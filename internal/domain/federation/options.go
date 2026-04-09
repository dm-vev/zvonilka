package federation

import "time"

// Option customizes federation service construction.
type Option func(*Service)

// WithNow injects the service clock used by tests.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if service != nil && now != nil {
			service.now = now
		}
	}
}
