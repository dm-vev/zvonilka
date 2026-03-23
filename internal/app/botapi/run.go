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
	cfg, err := config.FromEnv("botapi")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger(cfg.Env, "botapi")
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
		cfg.HTTPAddr,
		"grpc_addr",
		cfg.GRPCAddr,
	)

	return runtime.Run(
		ctx,
		runtime.Config{
			ServiceName:     cfg.ServiceName,
			Env:             cfg.Env,
			HTTPAddr:        cfg.HTTPAddr,
			GRPCAddr:        cfg.GRPCAddr,
			ShutdownTimeout: cfg.ShutdownTimeout,
		},
		logger,
		app.health,
		app.handler,
		nil,
	)
}
