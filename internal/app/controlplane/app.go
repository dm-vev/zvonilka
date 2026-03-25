package controlplane

import (
	"context"
	"net/http"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/media"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

type app struct {
	health         *runtime.Health
	handler        http.Handler
	catalog        *domainstorage.Catalog
	conversation   *conversation.Service
	identity       *identity.Service
	media          *media.Service
	presence       *presence.Service
	cleanupTimeout time.Duration
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
	storageCatalog, identityService, conversationService, mediaService, presenceService, err := buildAppStorage(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &app{
		health:         health,
		handler:        http.NotFoundHandler(),
		catalog:        storageCatalog,
		conversation:   conversationService,
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
