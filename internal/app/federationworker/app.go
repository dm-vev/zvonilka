package federationworker

import (
	"context"
	"net/http"
	"time"

	conversationpg "github.com/dm-vev/zvonilka/internal/domain/conversation/pgstore"
	"github.com/dm-vev/zvonilka/internal/domain/federation"
	federationpg "github.com/dm-vev/zvonilka/internal/domain/federation/pgstore"
	identitypg "github.com/dm-vev/zvonilka/internal/domain/identity/pgstore"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

type app struct {
	bootstrap      bootstrapCloser
	worker         *federation.Worker
	health         *runtime.Health
	handler        http.Handler
	cleanupTimeout time.Duration
}

type bootstrapCloser interface {
	Close(context.Context) error
}

func cleanupContext(ctx context.Context, fallback ...time.Duration) (context.Context, context.CancelFunc) {
	timeout := 30 * time.Second
	if len(fallback) > 0 && fallback[0] > 0 {
		timeout = fallback[0]
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
				timeout = remaining
			}
		}
	}

	return context.WithTimeout(context.Background(), timeout)
}

func newApp(ctx context.Context, cfg config.Configuration) (*app, error) {
	if !cfg.Features.FederationEnabled || !cfg.Infrastructure.Postgres.Enabled {
		return nil, federation.ErrInvalidInput
	}

	bootstrap := postgresplatform.NewBootstrap(cfg)
	db, err := bootstrap.Open(ctx)
	if err != nil {
		return nil, err
	}

	conversationStore, err := conversationpg.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}
	federationStore, err := federationpg.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}
	identityStore, err := identitypg.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}

	federationService, err := federation.NewService(federationStore)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}

	worker, err := federation.NewWorker(
		federationService,
		identityStore,
		conversationStore,
		newGRPCClientFactory(cfg.Federation),
		federation.WorkerSettings{
			LocalServerName: cfg.Federation.LocalServerName,
			PollInterval:    cfg.Federation.WorkerPollInterval,
			BatchSize:       cfg.Federation.WorkerBatchSize,
		},
	)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}

	return &app{
		bootstrap:      bootstrap,
		worker:         worker,
		health:         runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date),
		handler:        http.NotFoundHandler(),
		cleanupTimeout: cfg.Runtime.ShutdownTimeout,
	}, nil
}

func closeBootstrap(ctx context.Context, bootstrap bootstrapCloser, cleanupTimeout time.Duration) error {
	if bootstrap == nil {
		return nil
	}

	cleanupCtx, cancel := cleanupContext(ctx, cleanupTimeout)
	defer cancel()

	return bootstrap.Close(cleanupCtx)
}
