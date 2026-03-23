package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type listenDefaults struct {
	http string
	grpc string
}

var serviceListenDefaults = map[string]listenDefaults{
	"controlplane": {
		http: ":8080",
		grpc: ":9090",
	},
	"gateway": {
		http: ":8081",
		grpc: ":9091",
	},
	"botapi": {
		http: ":8082",
		grpc: ":9092",
	},
}

// Config contains the shared runtime settings for service skeletons.
type Config struct {
	ServiceName     string
	Env             string
	HTTPAddr        string
	GRPCAddr        string
	ShutdownTimeout time.Duration
}

// FromEnv loads configuration for a named service from environment variables.
func FromEnv(serviceName string) (Config, error) {
	if serviceName == "" {
		return Config{}, fmt.Errorf("service name is required")
	}

	defaults := defaultsForService(serviceName)

	cfg := Config{
		ServiceName: serviceName,
		Env:         envOrDefault(serviceEnvKey(serviceName, "ENV"), envOrDefault("ZVONILKA_ENV", "development")),
		HTTPAddr:    envOrDefault(serviceEnvKey(serviceName, "HTTP_ADDR"), envOrDefault("ZVONILKA_HTTP_ADDR", defaults.http)),
		GRPCAddr:    envOrDefault(serviceEnvKey(serviceName, "GRPC_ADDR"), envOrDefault("ZVONILKA_GRPC_ADDR", defaults.grpc)),
	}

	shutdownTimeout, err := durationOrDefault(
		serviceEnvKey(serviceName, "SHUTDOWN_TIMEOUT"),
		envOrDefault("ZVONILKA_SHUTDOWN_TIMEOUT", "10s"),
	)
	if err != nil {
		return Config{}, err
	}

	cfg.ShutdownTimeout = shutdownTimeout

	return cfg, nil
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func defaultsForService(serviceName string) listenDefaults {
	defaults, ok := serviceListenDefaults[serviceName]
	if ok {
		return defaults
	}

	return listenDefaults{
		http: ":8080",
		grpc: ":9090",
	}
}

func serviceEnvKey(serviceName, suffix string) string {
	return "ZVONILKA_" + strings.ToUpper(serviceName) + "_" + suffix
}

func durationOrDefault(key string, fallback string) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		value = fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}

	return duration, nil
}
