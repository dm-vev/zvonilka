package config

import (
	"fmt"
	"strings"
)

// Configuration contains the full service configuration surface.
type Configuration struct {
	Service        ServiceConfig
	Logging        LoggingConfig
	Runtime        RuntimeConfig
	Identity       IdentityConfig
	Call           CallConfig
	RTC            RTCConfig
	Bot            BotConfig
	Media          MediaConfig
	Presence       PresenceConfig
	Notification   NotificationConfig
	Search         SearchConfig
	Infrastructure InfrastructureConfig
	Storage        StorageConfig
	Features       FeatureConfig
}

// Config is kept as a compatibility alias for older call sites.
type Config = Configuration

// Load builds a fully validated configuration for the requested service.
func Load(serviceName string) (Configuration, error) {
	serviceName = strings.ToLower(strings.TrimSpace(serviceName))
	if serviceName == "" {
		return Configuration{}, fmt.Errorf("service name is required")
	}
	if err := validateServiceName(serviceName); err != nil {
		return Configuration{}, err
	}

	cfg := defaultConfiguration(serviceName)
	if err := applyEnvOverrides(&cfg, serviceName); err != nil {
		return Configuration{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Configuration{}, err
	}

	return cfg, nil
}

// FromEnv is the historical loader entrypoint retained for compatibility.
func FromEnv(serviceName string) (Config, error) {
	return Load(serviceName)
}

// MustLoad returns configuration or panics when loading fails.
func MustLoad(serviceName string) Configuration {
	cfg, err := Load(serviceName)
	if err != nil {
		panic(err)
	}

	return cfg
}
