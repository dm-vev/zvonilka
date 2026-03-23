package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

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

	cfg := Config{
		ServiceName: serviceName,
		Env:         envOrDefault(serviceEnvKey(serviceName, "ENV"), envOrDefault("ZVONILKA_ENV", "development")),
		HTTPAddr:    envOrDefault(serviceEnvKey(serviceName, "HTTP_ADDR"), envOrDefault("ZVONILKA_HTTP_ADDR", ":8080")),
		GRPCAddr:    envOrDefault(serviceEnvKey(serviceName, "GRPC_ADDR"), envOrDefault("ZVONILKA_GRPC_ADDR", ":9090")),
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
