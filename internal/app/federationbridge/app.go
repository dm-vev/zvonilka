package federationbridge

import (
	"context"
	"net/http"
	"time"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	"github.com/dm-vev/zvonilka/internal/domain/federation"
	federationpg "github.com/dm-vev/zvonilka/internal/domain/federation/pgstore"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
	"google.golang.org/grpc"
)

type app struct {
	bootstrap      bootstrapCloser
	health         *runtime.Health
	handler        http.Handler
	api            *api
	cleanupTimeout time.Duration
}

type api struct {
	federationv1.UnimplementedFederationBridgeServiceServer

	federation   *federation.Service
	sharedSecret string
}

type bootstrapCloser interface {
	Close(context.Context) error
}

func (a *app) registerGRPC(server *grpc.Server) {
	if a == nil || a.api == nil || server == nil {
		return
	}

	federationv1.RegisterFederationBridgeServiceServer(server, a.api)
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

	federationStore, err := federationpg.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}
	federationService, err := federation.NewService(federationStore)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}

	return &app{
		bootstrap: bootstrap,
		health:    runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date),
		handler:   http.NotFoundHandler(),
		api: &api{
			federation:   federationService,
			sharedSecret: cfg.Federation.BridgeSharedSecret,
		},
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
