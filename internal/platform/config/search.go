package config

import domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"

// SearchConfig defines the tunable search query and presentation settings.
type SearchConfig struct {
	DefaultLimit   int
	MaxLimit       int
	MinQueryLength int
	SnippetLength  int
}

// ToSettings converts configuration into domain search settings.
func (c SearchConfig) ToSettings() domainsearch.Settings {
	return domainsearch.Settings{
		DefaultLimit:   c.DefaultLimit,
		MaxLimit:       c.MaxLimit,
		MinQueryLength: c.MinQueryLength,
		SnippetLength:  c.SnippetLength,
	}
}

func (c SearchConfig) normalize() SearchConfig {
	settings := domainsearch.DefaultSettings()
	if c.DefaultLimit <= 0 {
		c.DefaultLimit = settings.DefaultLimit
	}
	if c.MaxLimit <= 0 {
		c.MaxLimit = settings.MaxLimit
	}
	if c.MinQueryLength <= 0 {
		c.MinQueryLength = settings.MinQueryLength
	}
	if c.SnippetLength <= 0 {
		c.SnippetLength = settings.SnippetLength
	}

	return c
}
