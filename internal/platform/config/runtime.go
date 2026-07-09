package config

import (
	"time"

	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

// RuntimeConfig defines listener addresses and server-level shutdown policy.
type RuntimeConfig struct {
	ShutdownTimeout time.Duration
	HTTP            HTTPConfig
	GRPC            GRPCConfig
}

// HTTPConfig defines HTTP listener and timeout settings.
type HTTPConfig struct {
	Address           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

// GRPCConfig defines gRPC listener settings.
type GRPCConfig struct {
	Address           string
	ReflectionEnabled bool
}

// ToRuntime converts config runtime settings into the runtime package shape.
func (r RuntimeConfig) ToRuntime(service ServiceConfig) runtime.Config {
	return runtime.Config{
		ServiceName:     service.Name,
		Env:             service.Environment,
		HTTPAddr:        r.HTTP.Address,
		GRPCAddr:        r.GRPC.Address,
		ShutdownTimeout: r.ShutdownTimeout,
		HTTP: runtime.HTTPConfig{
			ReadHeaderTimeout: r.HTTP.ReadHeaderTimeout,
			ReadTimeout:       r.HTTP.ReadTimeout,
			WriteTimeout:      r.HTTP.WriteTimeout,
			IdleTimeout:       r.HTTP.IdleTimeout,
			MaxHeaderBytes:    r.HTTP.MaxHeaderBytes,
		},
		GRPC: runtime.GRPCConfig{
			ReflectionEnabled: r.GRPC.ReflectionEnabled,
		},
	}
}
