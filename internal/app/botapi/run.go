package botapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/observability"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

// Run boots the Bot API service.
func Run(ctx context.Context) (err error) {
	cfg, err := config.Load("botapi")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger(cfg.Logging, cfg.Service)
	app, err := newApp(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize botapi app: %w", err)
	}
	defer func() {
		closeErr := closeBootstrap(ctx, app.bootstrap, app.cleanupTimeout)
		if closeErr == nil {
			return
		}
		if err != nil {
			err = errors.Join(err, fmt.Errorf("close botapi app: %w", closeErr))
			return
		}
		err = fmt.Errorf("close botapi app: %w", closeErr)
	}()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	workerErrCh := make(chan error, 1)
	go func() {
		workerErr := app.worker.Run(runCtx, logger)
		workerErrCh <- workerErr
		if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
			cancel()
		}
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
	)

	runErr := runtime.Run(
		runCtx,
		cfg.Runtime.ToRuntime(cfg.Service),
		logger,
		app.health,
		app.handler,
		nil,
	)
	cancel()

	workerErr := <-workerErrCh
	if runErr != nil {
		if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
			return errors.Join(runErr, workerErr)
		}
		return runErr
	}
	if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
		return workerErr
	}

	return nil
}
