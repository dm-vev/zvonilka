package identity

import (
	"time"

	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
)

// Option configures a Service at construction time.
type Option func(*Service)

// WithNow overrides the service clock for tests and deterministic flows.
//
// Passing nil leaves the default clock untouched.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if now == nil {
			return
		}

		service.now = now
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
