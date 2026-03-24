package controlplane

import (
	"context"
	"net/http"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/media"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

type app struct {
	health       *runtime.Health
	handler      http.Handler
	catalog      *domainstorage.Catalog
	conversation *conversation.Service
	identity     *identity.Service
	media        *media.Service
}

func (a *app) close(ctx context.Context) error {
	if a == nil || a.catalog == nil {
		return nil
	}

	return a.catalog.Close(cleanupContext(ctx))
}

func newApp(ctx context.Context, cfg config.Configuration) (*app, error) {
	health := runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date)
	storageCatalog, identityService, conversationService, mediaService, err := buildAppStorage(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &app{
		health:       health,
		handler:      http.NotFoundHandler(),
		catalog:      storageCatalog,
		conversation: conversationService,
		identity:     identityService,
		media:        mediaService,
	}, nil
}

// cleanupContext returns a context detached from runtime cancellation.
func cleanupContext(ctx context.Context) context.Context {
	return context.Background()
}
