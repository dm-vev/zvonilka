package observability

import (
	"log/slog"
	"os"
	"strings"

	"github.com/dm-vev/zvonilka/internal/platform/config"
)

// NewLogger builds a structured logger with stable service metadata.
func NewLogger(logging config.LoggingConfig, service config.ServiceConfig) *slog.Logger {
	handlerOptions := &slog.HandlerOptions{
		Level:     parseLevel(logging.Level),
		AddSource: logging.AddSource,
	}

	var handler slog.Handler
	switch strings.ToLower(logging.Format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, handlerOptions)
	default:
		handler = slog.NewTextHandler(os.Stdout, handlerOptions)
	}

	return slog.New(handler).With(
		"service",
		service.Name,
		"env",
		service.Environment,
	)
}

func parseLevel(level string) slog.Leveler {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
