package controlplane

import (
	"context"
	"net/http"
	"time"

	adminv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/admin/v1"
	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/federation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/media"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
	"google.golang.org/grpc"
)

type app struct {
	health         *runtime.Health
	handler        http.Handler
	catalog        *domainstorage.Catalog
	api            *api
	conversation   *conversation.Service
	federation     *federation.Service
	identity       *identity.Service
	media          *media.Service
	presence       *presence.Service
	cleanupTimeout time.Duration
}

type api struct {
	adminv1.UnimplementedAdminServiceServer
	federationv1.UnimplementedFederationServiceServer

	federation *federation.Service
	identity   *identity.Service
	presence   *presence.Service
}

func (a *app) registerGRPC(server *grpc.Server) {
	if a == nil || a.api == nil || server == nil {
		return
	}

	adminv1.RegisterAdminServiceServer(server, a.api)
	if a.federation != nil && a.api.federation != nil {
		federationv1.RegisterFederationServiceServer(server, a.api)
	}
}

func (a *app) close(ctx context.Context) error {
	if a == nil || a.catalog == nil {
		return nil
	}

	cleanupCtx, cancel := cleanupContext(ctx, a.cleanupTimeout)
	defer cancel()

	return a.catalog.Close(cleanupCtx)
}

func newApp(ctx context.Context, cfg config.Configuration) (*app, error) {
	health := runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date)
	storageCatalog, identityService, conversationService, federationService, mediaService, presenceService, err := buildAppStorage(ctx, cfg)
	if err != nil {
		return nil, err
	}

	var apiFederation *federation.Service
	if cfg.Features.FederationEnabled {
		apiFederation = federationService
	}

	return &app{
		health:         health,
		handler:        http.NotFoundHandler(),
		catalog:        storageCatalog,
		api:            &api{federation: apiFederation, identity: identityService, presence: presenceService},
		conversation:   conversationService,
		federation:     apiFederation,
		identity:       identityService,
		media:          mediaService,
		presence:       presenceService,
		cleanupTimeout: cfg.Runtime.ShutdownTimeout,
	}, nil
}

// cleanupContext returns a shutdown context that ignores cancellation but preserves the deadline budget.
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
