package config

import (
	"fmt"
	"os"
)

// Config contains the shared runtime settings for service skeletons.
type Config struct {
	ServiceName string
	Env         string
	HTTPAddr    string
	GRPCAddr    string
}

// FromEnv loads configuration for a named service from environment variables.
func FromEnv(serviceName string) (Config, error) {
	if serviceName == "" {
		return Config{}, fmt.Errorf("service name is required")
	}

	cfg := Config{
		ServiceName: serviceName,
		Env:         envOrDefault("ZVONILKA_ENV", "development"),
		HTTPAddr:    envOrDefault("ZVONILKA_HTTP_ADDR", ":8080"),
		GRPCAddr:    envOrDefault("ZVONILKA_GRPC_ADDR", ":9090"),
	}

	return cfg, nil
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
