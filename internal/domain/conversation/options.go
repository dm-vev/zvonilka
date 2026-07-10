package conversation

import (
	"context"
	"time"

	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
)

// Option configures a Service at construction time.
type Option func(*Service)

// ReactionValidator resolves an active reaction to its canonical Unicode form.
type ReactionValidator func(context.Context, string) (string, error)

// WithReactionValidator configures the catalog validator used for new reactions.
func WithReactionValidator(validator ReactionValidator) Option {
	return func(service *Service) {
		if service != nil {
			service.reactionValidator = validator
		}
	}
}

// WithNow overrides the service clock for tests and deterministic flows.
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
