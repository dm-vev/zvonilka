package runtime

import "time"

// HTTPConfig contains HTTP server listener and timeout settings.
type HTTPConfig struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
}

// GRPCConfig contains gRPC server runtime settings.
type GRPCConfig struct {
	ReflectionEnabled bool
}

// Config contains the shared runtime settings for runnable services.
type Config struct {
	ServiceName     string
	Env             string
	HTTPAddr        string
	GRPCAddr        string
	HTTP            HTTPConfig
	GRPC            GRPCConfig
	ShutdownTimeout time.Duration
}
