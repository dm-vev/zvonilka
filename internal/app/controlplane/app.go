package controlplane

import (
	"net/http"

	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

type app struct {
	health  *runtime.Health
	handler http.Handler
}

func newApp(cfg config.Config) *app {
	health := runtime.NewHealth(cfg.ServiceName, buildinfo.Version, buildinfo.Commit, buildinfo.Date)

	return &app{
		health:  health,
		handler: http.NotFoundHandler(),
	}
}
