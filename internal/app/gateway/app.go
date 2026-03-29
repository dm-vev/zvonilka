package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	callv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/call/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	e2eev1 "github.com/dm-vev/zvonilka/gen/proto/contracts/e2ee/v1"
	mediav1 "github.com/dm-vev/zvonilka/gen/proto/contracts/media/v1"
	searchv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/search/v1"
	syncv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/sync/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	domaine2ee "github.com/dm-vev/zvonilka/internal/domain/e2ee"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/media"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
	"github.com/dm-vev/zvonilka/internal/domain/search"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
	callruntimev1 "github.com/dm-vev/zvonilka/internal/genproto/callruntime/v1"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	platformrtc "github.com/dm-vev/zvonilka/internal/platform/rtc"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
	"google.golang.org/grpc"
)

type app struct {
	health         *runtime.Health
	handler        http.Handler
	catalog        *domainstorage.Catalog
	rtcCluster     *platformrtc.Cluster
	api            *api
	callRuntime    callruntimev1.CallRuntimeServiceServer
	cleanupTimeout time.Duration
	backgroundWG   sync.WaitGroup
}

type api struct {
	callv1.UnimplementedCallServiceServer
	authv1.UnimplementedAuthServiceServer
	e2eev1.UnimplementedE2EEServiceServer
	usersv1.UnimplementedUserServiceServer
	conversationv1.UnimplementedConversationServiceServer
	mediav1.UnimplementedMediaServiceServer
	searchv1.UnimplementedSearchServiceServer
	syncv1.UnimplementedSyncServiceServer

	call         *domaincall.Service
	e2ee         *domaine2ee.Service
	identity     *identity.Service
	conversation *conversation.Service
	media        *media.Service
	presence     *presence.Service
	search       *search.Service
	user         *domainuser.Service
	callNotifier *callNotifier
	syncNotifier *syncNotifier
}

func (a *app) registerGRPC(server *grpc.Server) {
	callv1.RegisterCallServiceServer(server, a.api)
	authv1.RegisterAuthServiceServer(server, a.api)
	e2eev1.RegisterE2EEServiceServer(server, a.api)
	usersv1.RegisterUserServiceServer(server, a.api)
	conversationv1.RegisterConversationServiceServer(server, a.api)
	mediav1.RegisterMediaServiceServer(server, a.api)
	searchv1.RegisterSearchServiceServer(server, a.api)
	syncv1.RegisterSyncServiceServer(server, a.api)
	callruntimev1.RegisterCallRuntimeServiceServer(server, a.callRuntime)
}

func (a *app) close(ctx context.Context) error {
	if a == nil || a.catalog == nil {
		if a == nil || a.rtcCluster == nil {
			return nil
		}
		return a.rtcCluster.Close(ctx)
	}

	cleanupCtx, cancel := cleanupContext(ctx, a.cleanupTimeout)
	defer cancel()

	clusterErr := error(nil)
	if a.rtcCluster != nil {
		clusterErr = a.rtcCluster.Close(cleanupCtx)
	}
	a.backgroundWG.Wait()

	return errors.Join(clusterErr, a.catalog.Close(cleanupCtx))
}

func newApp(ctx context.Context, cfg config.Configuration) (*app, error) {
	health := runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date)
	catalog, rtcCluster, localRTC, callService, e2eeService, identityService, conversationService, mediaService, presenceService, searchService, userService, err := buildAppStorage(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &app{
		health:     health,
		handler:    http.NotFoundHandler(),
		catalog:    catalog,
		rtcCluster: rtcCluster,
		api: &api{
			call:         callService,
			e2ee:         e2eeService,
			identity:     identityService,
			conversation: conversationService,
			media:        mediaService,
			presence:     presenceService,
			search:       searchService,
			user:         userService,
			callNotifier: newCallNotifier(),
			syncNotifier: newSyncNotifier(),
		},
		callRuntime:    platformrtc.NewGRPCRuntimeServer(localRTC),
		cleanupTimeout: cfg.Runtime.ShutdownTimeout,
	}, nil
}

func (a *app) startBackground(ctx context.Context, logger *slog.Logger, cfg config.Configuration) error {
	if a == nil || a.api == nil || a.api.call == nil || logger == nil {
		return nil
	}

	worker, err := domaincall.NewRehomeWorker(a.api.call, a.api, domaincall.RehomeWorkerSettings{
		PollInterval: cfg.Call.RehomePollInterval,
		BatchSize:    cfg.Call.RehomeBatchSize,
	})
	if err != nil {
		return err
	}

	a.backgroundWG.Add(1)
	go func() {
		defer a.backgroundWG.Done()
		_ = worker.Run(ctx, logger)
	}()

	return nil
}

// cleanupContext returns a shutdown context detached from runtime cancellation.
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
