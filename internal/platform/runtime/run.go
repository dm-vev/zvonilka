package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

// Run starts the HTTP and gRPC listeners for a service.
func Run(
	ctx context.Context,
	cfg Config,
	logger *slog.Logger,
	healthState *Health,
	httpHandler http.Handler,
	registerGRPC func(*grpc.Server),
) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if logger == nil {
		return errors.New("logger is required")
	}
	if healthState == nil {
		return errors.New("health state is required")
	}
	if cfg.HTTPAddr == "" && cfg.GRPCAddr == "" {
		return errors.New("at least one listen address is required")
	}

	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	httpServer, httpListener, err := startHTTPServer(cfg, httpHandler, healthState)
	if err != nil {
		return err
	}

	grpcServer, grpcListener, err := startGRPCServer(cfg, registerGRPC, healthState)
	if err != nil {
		_ = httpServer.Close()
		return err
	}

	healthState.SetReady()
	logger.InfoContext(
		ctx,
		"service started",
		"service",
		cfg.ServiceName,
		"env",
		cfg.Env,
		"http_addr",
		cfg.HTTPAddr,
		"grpc_addr",
		cfg.GRPCAddr,
	)

	httpErr := make(chan error, 1)
	grpcErr := make(chan error, 1)

	if httpServer != nil {
		go func() {
			httpErr <- serveHTTP(httpServer, httpListener)
		}()
	}

	if grpcServer != nil {
		go func() {
			grpcErr <- serveGRPC(grpcServer, grpcListener)
		}()
	}

	runErr := waitForRunError(ctx, cancel, httpErr, grpcErr)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	healthState.SetNotReady()

	var shutdownErr error
	if httpServer != nil {
		if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shutdown HTTP server: %w", err))
		}
	}

	if grpcServer != nil {
		done := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
		case <-shutdownCtx.Done():
			grpcServer.Stop()
		}
	}

	if runErr != nil {
		return runErr
	}

	return shutdownErr
}

func waitForRunError(
	ctx context.Context,
	cancel context.CancelFunc,
	httpErr <-chan error,
	grpcErr <-chan error,
) error {
	select {
	case err := <-httpErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			cancel()
			return fmt.Errorf("http server: %w", err)
		}
	case err := <-grpcErr:
		if err != nil {
			cancel()
			return fmt.Errorf("grpc server: %w", err)
		}
	case <-ctx.Done():
		return nil
	}

	return nil
}

func startHTTPServer(
	cfg Config,
	handler http.Handler,
	healthState *Health,
) (*http.Server, net.Listener, error) {
	if cfg.HTTPAddr == "" {
		return nil, nil, nil
	}

	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen HTTP on %s: %w", cfg.HTTPAddr, err)
	}

	if handler == nil {
		handler = http.NotFoundHandler()
	}

	server := &http.Server{
		Handler:           healthState.Handler(handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return server, listener, nil
}

func startGRPCServer(
	cfg Config,
	registerGRPC func(*grpc.Server),
	healthState *Health,
) (*grpc.Server, net.Listener, error) {
	if cfg.GRPCAddr == "" {
		return nil, nil, nil
	}

	listener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen gRPC on %s: %w", cfg.GRPCAddr, err)
	}

	grpcServer := grpc.NewServer()
	grpcHealth := health.NewServer()
	healthgrpc.RegisterHealthServer(grpcServer, grpcHealth)
	grpcHealth.SetServingStatus(healthState.serviceName, healthgrpc.HealthCheckResponse_SERVING)
	grpcHealth.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)

	if registerGRPC != nil {
		registerGRPC(grpcServer)
	}

	return grpcServer, listener, nil
}

func serveHTTP(server *http.Server, listener net.Listener) error {
	if server == nil || listener == nil {
		return nil
	}

	err := server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

func serveGRPC(server *grpc.Server, listener net.Listener) error {
	if server == nil || listener == nil {
		return nil
	}

	err := server.Serve(listener)
	if errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}

	return err
}
