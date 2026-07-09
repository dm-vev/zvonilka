package config

import "time"

// TranslationConfig defines the external translation provider integration.
type TranslationConfig struct {
	EndpointURL  string
	APIKey       string
	Timeout      time.Duration
	MaxTextBytes int
	ProviderName string
}
