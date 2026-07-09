package media

import (
	"time"

	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
)

// Option configures a Service at construction time.
type Option func(*Service)

// WithNow overrides the service clock for tests and deterministic flows.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if now == nil {
			return
		}

		service.now = now
	}
}

// WithSettings overrides service settings for tests and wiring.
func WithSettings(settings Settings) Option {
	return func(service *Service) {
		service.settings = normalizeSettings(settings)
	}
}

// WithIndexer injects an optional search indexer.
func WithIndexer(indexer domainsearch.Indexer) Option {
	return func(service *Service) {
		if service != nil {
			service.indexer = indexer
		}
	}
}
