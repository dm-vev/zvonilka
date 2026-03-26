package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/observability"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

// Run boots the public gRPC gateway.
func Run(ctx context.Context) error {
	cfg, err := config.Load("gateway")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger(cfg.Logging, cfg.Service)
	app, err := newApp(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = app.close(context.Background())
	}()

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

	if err := runtime.Run(
		ctx,
		cfg.Runtime.ToRuntime(cfg.Service),
		logger,
		app.health,
		app.handler,
		app.registerGRPC,
	); err != nil {
		closeErr := app.close(ctx)
		if closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}

	return app.close(ctx)
}
