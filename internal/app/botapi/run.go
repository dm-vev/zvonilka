package botapi

import (
	"context"
	"fmt"

	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/observability"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

// Run boots the bot API skeleton.
func Run(ctx context.Context) error {
	cfg, err := config.Load("botapi")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger(cfg.Logging, cfg.Service)
	app := newApp(cfg)

	logger.InfoContext(
		ctx,
		"service initialized",
		"version",
		buildinfo.Version,
		"commit",
		buildinfo.Commit,
		"date",
		buildinfo.Date,
		"http_addr",
		cfg.Runtime.HTTP.Address,
		"grpc_addr",
		cfg.Runtime.GRPC.Address,
	)

	return runtime.Run(
		ctx,
		cfg.Runtime.ToRuntime(cfg.Service),
		logger,
		app.health,
		app.handler,
		nil,
	)
}
