package controlplane

import (
	"fmt"
	"net/http"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

type app struct {
	identity *identity.Service
	health   *runtime.Health
	handler  http.Handler
}

func newApp(cfg config.Config) (*app, error) {
	store := identity.NewMemoryStore()
	service, err := identity.NewService(store, identity.NoopCodeSender{})
	if err != nil {
		return nil, fmt.Errorf("create identity service: %w", err)
	}

	health := runtime.NewHealth(cfg.ServiceName, buildinfo.Version, buildinfo.Commit, buildinfo.Date)

	return &app{
		identity: service,
		health:   health,
		handler:  http.NotFoundHandler(),
	}, nil
}
