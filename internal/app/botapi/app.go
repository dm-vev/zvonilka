package botapi

import (
	"context"
	"net/http"
	"time"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

type app struct {
	bootstrap      bootstrapCloser
	catalog        *domainstorage.Catalog
	worker         *domainbot.Worker
	health         *runtime.Health
	handler        http.Handler
	cleanupTimeout time.Duration
}

type api struct {
	bot         *domainbot.Service
	media       mediaUploader
	uploadLimit int64
}

type mediaUploader interface {
	Upload(ctx context.Context, params domainmedia.UploadParams) (domainmedia.MediaAsset, error)
}

func newApp(ctx context.Context, cfg config.Configuration) (*app, error) {
	catalog, bootstrap, service, mediaService, worker, err := buildAppStorage(ctx, cfg)
	if err != nil {
		return nil, err
	}

	boundary := &api{bot: service, media: mediaService, uploadLimit: cfg.Media.MaxUploadSize}
	return &app{
		bootstrap:      bootstrap,
		catalog:        catalog,
		worker:         worker,
		health:         runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date),
		handler:        boundary.routes(),
		cleanupTimeout: cfg.Runtime.ShutdownTimeout,
	}, nil
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

func closeBootstrap(ctx context.Context, bootstrap bootstrapCloser, cleanupTimeout time.Duration) error {
	if bootstrap == nil {
		return nil
	}

	cleanupCtx, cancel := cleanupContext(ctx, cleanupTimeout)
	defer cancel()

	return bootstrap.Close(cleanupCtx)
}
