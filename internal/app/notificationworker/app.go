package notificationworker

import (
	"context"
	"net/http"

	conversationpg "github.com/dm-vev/zvonilka/internal/domain/conversation/pgstore"
	identitypg "github.com/dm-vev/zvonilka/internal/domain/identity/pgstore"
	"github.com/dm-vev/zvonilka/internal/domain/notification"
	notificationpg "github.com/dm-vev/zvonilka/internal/domain/notification/pgstore"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
	presencepg "github.com/dm-vev/zvonilka/internal/domain/presence/pgstore"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

type app struct {
	bootstrap *postgresplatform.Bootstrap
	worker    *notification.Worker
	health    *runtime.Health
	handler   http.Handler
}

func newApp(ctx context.Context, cfg config.Configuration) (*app, error) {
	if !cfg.Infrastructure.Postgres.Enabled {
		return nil, notification.ErrInvalidInput
	}

	bootstrap := postgresplatform.NewBootstrap(cfg)
	db, err := bootstrap.Open(ctx)
	if err != nil {
		return nil, err
	}

	identityStore, err := identitypg.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = bootstrap.Close(ctx)
		return nil, err
	}
	conversationStore, err := conversationpg.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = bootstrap.Close(ctx)
		return nil, err
	}
	presenceStore, err := presencepg.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = bootstrap.Close(ctx)
		return nil, err
	}
	notificationStore, err := notificationpg.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = bootstrap.Close(ctx)
		return nil, err
	}

	presenceService, err := presence.NewService(
		presenceStore,
		identityStore,
		presence.WithSettings(cfg.Presence.ToSettings()),
	)
	if err != nil {
		_ = bootstrap.Close(ctx)
		return nil, err
	}
	notificationService, err := notification.NewService(
		notificationStore,
		identityStore,
		notification.WithSettings(cfg.Notification.ToSettings()),
	)
	if err != nil {
		_ = bootstrap.Close(ctx)
		return nil, err
	}
	worker, err := notification.NewWorker(notificationService, conversationStore, presenceService)
	if err != nil {
		_ = bootstrap.Close(ctx)
		return nil, err
	}

	return &app{
		bootstrap: bootstrap,
		worker:    worker,
		health:    runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date),
		handler:   http.NotFoundHandler(),
	}, nil
}
