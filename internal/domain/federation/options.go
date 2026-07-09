package federation

import "time"

// Option customizes federation service construction.
type Option func(*Service)

const defaultBridgeFragmentLeaseTTL = 30 * time.Second

// WithNow injects the service clock used by tests.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if service != nil && now != nil {
			service.now = now
		}
	}
}

// WithBridgeFragmentLeaseTTL overrides the default bridge fragment lease TTL.
func WithBridgeFragmentLeaseTTL(ttl time.Duration) Option {
	return func(service *Service) {
		if service != nil && ttl > 0 {
			service.bridgeFragmentLeaseTTL = ttl
		}
	}
}
