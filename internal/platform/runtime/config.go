package runtime

import "time"

// Config contains the shared runtime settings for runnable services.
type Config struct {
	ServiceName     string
	Env             string
	HTTPAddr        string
	GRPCAddr        string
	ShutdownTimeout time.Duration
}
