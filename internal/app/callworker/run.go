package callworker

import (
	"context"
	"errors"
	"fmt"

	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/observability"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

// Run boots the call worker service.
func Run(ctx context.Context) (err error) {
	cfg, err := config.Load("callworker")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger(cfg.Logging, cfg.Service)
	app, err := newApp(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initialize call worker app: %w", err)
	}
	defer func() {
		err = finalizeRun(ctx, app, err)
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
		"grpc_addr",
		cfg.Runtime.GRPC.Address,
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

func finalizeRun(ctx context.Context, app *app, runErr error) error {
	if app == nil {
		return runErr
	}

	closeErr := closeBootstrap(ctx, app.bootstrap, app.cleanupTimeout)
	if closeErr == nil {
		return runErr
	}

	wrappedCloseErr := fmt.Errorf("close call worker app: %w", closeErr)
	if runErr != nil {
		return errors.Join(runErr, wrappedCloseErr)
	}

	return wrappedCloseErr
}
