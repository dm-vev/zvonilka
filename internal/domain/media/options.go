package media

import (
	"context"
	"time"

	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
)

// Option configures a Service at construction time.
type Option func(*Service)

// ConversationAccessChecker authorizes an account to read conversation media.
type ConversationAccessChecker func(context.Context, string, string) (bool, error)

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

// WithConversationAccessChecker enables access checks for media attached to conversations.
func WithConversationAccessChecker(checker ConversationAccessChecker) Option {
	return func(service *Service) {
		if service != nil {
			service.conversationAccessChecker = checker
		}
	}
}
