package observability

import (
	"log/slog"
	"os"
)

// NewLogger builds a structured logger with stable service metadata.
func NewLogger(env string, service string) *slog.Logger {
	level := slog.LevelInfo
	if env == "development" {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler).With(
		"service",
		service,
		"env",
		env,
	)
}
