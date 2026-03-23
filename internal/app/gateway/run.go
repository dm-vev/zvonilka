package gateway

import (
	"context"
	"fmt"

	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/observability"
)

// Run boots the gateway skeleton.
func Run(ctx context.Context) error {
	cfg, err := config.FromEnv("gateway")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger(cfg.Env, "gateway")
	logger.InfoContext(
		ctx,
		"service initialized",
		"version",
		buildinfo.Version,
		"http_addr",
		cfg.HTTPAddr,
		"grpc_addr",
		cfg.GRPCAddr,
	)

	return nil
}
