package search

import "time"

// Settings controls query and presentation limits for the search domain.
type Settings struct {
	DefaultLimit   int
	MaxLimit       int
	MinQueryLength int
	SnippetLength  int
}

// DefaultSettings returns a conservative baseline for search queries.
func DefaultSettings() Settings {
	return Settings{
		DefaultLimit:   25,
		MaxLimit:       100,
		MinQueryLength: 2,
		SnippetLength:  160,
	}
}

func (s Settings) normalize() Settings {
	defaults := DefaultSettings()
	if s.DefaultLimit <= 0 {
		s.DefaultLimit = defaults.DefaultLimit
	}
	if s.MaxLimit <= 0 {
		s.MaxLimit = defaults.MaxLimit
	}
	if s.MaxLimit < s.DefaultLimit {
		s.MaxLimit = s.DefaultLimit
	}
	if s.MinQueryLength <= 0 {
		s.MinQueryLength = defaults.MinQueryLength
	}
	if s.SnippetLength <= 0 {
		s.SnippetLength = defaults.SnippetLength
	}

	return s
}

// WithNow overrides the search clock for tests and deterministic flows.
func WithNow(now func() time.Time) Option {
	return func(service *Service) {
		if service != nil && now != nil {
			service.now = now
		}
	}
}

// WithSettings overrides service settings for tests and wiring.
func WithSettings(settings Settings) Option {
	return func(service *Service) {
		if service != nil {
			service.settings = settings.normalize()
		}
	}
}
