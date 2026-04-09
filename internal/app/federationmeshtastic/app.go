package federationmeshtastic

import (
	"context"
	"net/http"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/federation/bridgeclient"
	transport "github.com/dm-vev/zvonilka/internal/platform/federation/transport"
	meshtastictransport "github.com/dm-vev/zvonilka/internal/platform/federation/transport/meshtastic"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
)

type app struct {
	worker  *worker
	health  *runtime.Health
	handler http.Handler
}

func newApp(ctx context.Context, cfg config.Configuration) (*app, error) {
	bridge, err := bridgeclient.NewGRPC(ctx, bridgeclient.Config{
		Endpoint:     cfg.Federation.BridgeEndpoint,
		SharedSecret: cfg.Federation.BridgeSharedSecret,
		DialTimeout:  cfg.Federation.DialTimeout,
	})
	if err != nil {
		return nil, err
	}

	adapter, err := meshtastictransport.New(meshtastictransport.Config{
		InterfaceKind:    cfg.Meshtastic.InterfaceKind,
		Device:           cfg.Meshtastic.Device,
		HelperPython:     cfg.Meshtastic.HelperPython,
		HelperScriptPath: cfg.Meshtastic.HelperScriptPath,
		ReceiveTimeout:   cfg.Meshtastic.ReceiveTimeout,
		TextPrefix:       cfg.Meshtastic.TextPrefix,
	})
	if err != nil {
		_ = bridge.Close()
		return nil, err
	}

	worker, err := newWorker(bridge, adapter, settings{
		PeerServerName: cfg.Federation.BridgePeerServer,
		LinkName:       cfg.Federation.BridgeLinkName,
		PollInterval:   cfg.Federation.BridgePollInterval,
		BatchSize:      cfg.Federation.BridgeBatchSize,
	})
	if err != nil {
		_ = adapter.Close()
		_ = bridge.Close()
		return nil, err
	}

	return &app{
		worker:  worker,
		health:  runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date),
		handler: http.NotFoundHandler(),
	}, nil
}

func closeApp(app *app) error {
	if app == nil || app.worker == nil {
		return nil
	}
	return app.worker.Close()
}

type bridgeClient interface {
	PullFragments(
		ctx context.Context,
		peerServerName string,
		linkName string,
		limit int,
	) (*federationv1.PullBridgeFragmentsResponse, error)
	SubmitFragments(
		ctx context.Context,
		peerServerName string,
		linkName string,
		fragments []*federationv1.BundleFragment,
	) (*federationv1.SubmitBridgeFragmentsResponse, error)
	AcknowledgeFragments(
		ctx context.Context,
		peerServerName string,
		linkName string,
		fragmentIDs []string,
		leaseToken string,
	) (*federationv1.AcknowledgeBridgeFragmentsResponse, error)
	Close() error
}

var (
	_ bridgeClient      = (*bridgeclient.Client)(nil)
	_ transport.Adapter = (*meshtastictransport.Adapter)(nil)
)
