package config

// LoggingConfig defines structured logging behavior.
type LoggingConfig struct {
	Level     string
	Format    string
	AddSource bool
}
