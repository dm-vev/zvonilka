package controlplane

import (
	"context"
	"errors"
	"fmt"

	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/observability"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

// Run boots the controlplane skeleton.
func Run(ctx context.Context) (err error) {
	cfg, err := config.Load("controlplane")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger(cfg.Logging, cfg.Service)
	app, err := newApp(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize controlplane app: %w", err)
	}
	defer func() {
		err = finalizeRun(ctx, app, err)
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

	err = runtime.Run(
		ctx,
		cfg.Runtime.ToRuntime(cfg.Service),
		logger,
		app.health,
		app.handler,
		nil,
	)
	return err
}

func finalizeRun(ctx context.Context, app *app, runErr error) error {
	if app == nil {
		return runErr
	}

	closeErr := app.close(ctx)
	if closeErr == nil {
		return runErr
	}

	wrappedCloseErr := fmt.Errorf("close controlplane app: %w", closeErr)
	if runErr != nil {
		return errors.Join(runErr, wrappedCloseErr)
	}

	return wrappedCloseErr
}
