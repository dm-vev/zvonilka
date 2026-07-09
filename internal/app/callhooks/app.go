package callhooks

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/callhook"
	callhookpg "github.com/dm-vev/zvonilka/internal/domain/callhook/pgstore"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
	mediapg "github.com/dm-vev/zvonilka/internal/domain/media/pgstore"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
	s3platform "github.com/dm-vev/zvonilka/internal/platform/storage/s3"
)

type app struct {
	bootstrap      bootstrapCloser
	executor       *callhook.Executor
	health         *runtime.Health
	handler        http.Handler
	cleanupTimeout time.Duration
}

type bootstrapCloser interface {
	Close(context.Context) error
}

type api struct {
	service      *callhook.Service
	hookSecret   string
	maxBodyBytes int64
}

type webhookEnvelope struct {
	Event domaincall.Event `json:"event"`
	Call  domaincall.Call  `json:"call"`
}

type multiCloser struct {
	closers []bootstrapCloser
}

var newCallhookStore = func(db *sql.DB, schema string) (callhook.Store, error) {
	return callhookpg.New(db, schema)
}

var newMediaStore = func(db *sql.DB, schema string) (domainmedia.Store, error) {
	return mediapg.New(db, schema)
}

var newMediaService = func(
	store domainmedia.Store,
	blob domainstorage.BlobStore,
	opts ...domainmedia.Option,
) (*domainmedia.Service, error) {
	return domainmedia.NewService(store, blob, opts...)
}

func newApp(ctx context.Context, cfg config.Configuration) (*app, error) {
	if !cfg.Infrastructure.Postgres.Enabled || !cfg.Infrastructure.ObjectStore.Enabled {
		return nil, callhook.ErrInvalidInput
	}

	postgresBootstrap := postgresplatform.NewBootstrap(cfg)
	objectBootstrap := s3platform.NewBootstrap(cfg)
	bootstrap := &multiCloser{closers: []bootstrapCloser{objectBootstrap, postgresBootstrap}}

	db, err := postgresBootstrap.Open(ctx)
	if err != nil {
		return nil, err
	}
	blob, err := objectBootstrap.Open(ctx)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}

	store, err := newCallhookStore(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}
	service, err := callhook.NewService(store)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}
	mediaStore, err := newMediaStore(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}
	mediaService, err := newMediaService(mediaStore, blob, domainmedia.WithSettings(cfg.Media.ToSettings()))
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}
	executor, err := callhook.NewExecutor(store, mediaService, callhook.ExecutorSettings{
		PollInterval:        cfg.Call.WorkerPollInterval,
		BatchSize:           cfg.Call.WorkerBatchSize,
		LeaseTTL:            cfg.Call.HookLeaseTTL,
		RetryInitialBackoff: cfg.Call.HookRetryInitialBackoff,
		RetryMaxBackoff:     cfg.Call.HookRetryMaxBackoff,
	})
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}

	return &app{
		bootstrap: bootstrap,
		executor:  executor,
		health:    runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date),
		handler: (&api{
			service:      service,
			hookSecret:   cfg.Call.HookSecret,
			maxBodyBytes: cfg.Call.HookMaxBodyBytes,
		}).routes(),
		cleanupTimeout: cfg.Runtime.ShutdownTimeout,
	}, nil
}

func (c *multiCloser) Close(ctx context.Context) error {
	if c == nil {
		return nil
	}

	var closeErr error
	for i := len(c.closers) - 1; i >= 0; i-- {
		if c.closers[i] == nil {
			continue
		}
		if err := c.closers[i].Close(ctx); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}

	return closeErr
}

func (a *api) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/recording", a.recording)
	mux.HandleFunc("/transcription", a.transcription)
	return mux
}

func (a *api) recording(writer http.ResponseWriter, request *http.Request) {
	a.handleHook(writer, request, func(ctx context.Context, payload domaincall.HookPayload) error {
		_, err := a.service.ApplyRecordingHook(ctx, payload)
		return err
	})
}

func (a *api) transcription(writer http.ResponseWriter, request *http.Request) {
	a.handleHook(writer, request, func(ctx context.Context, payload domaincall.HookPayload) error {
		_, err := a.service.ApplyTranscriptionHook(ctx, payload)
		return err
	})
}

func (a *api) handleHook(
	writer http.ResponseWriter,
	request *http.Request,
	apply func(context.Context, domaincall.HookPayload) error,
) {
	if request.Method != http.MethodPost {
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	body, status, err := a.readBody(writer, request)
	if err != nil {
		http.Error(writer, http.StatusText(status), status)
		return
	}
	if !callhook.VerifySignature(a.hookSecret, body, request.Header.Get(callhook.SignatureHeader)) {
		http.Error(writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	payload, err := decodePayload(body)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if err := apply(request.Context(), payload); err != nil {
		if errors.Is(err, callhook.ErrInvalidInput) {
			http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (a *api) readBody(writer http.ResponseWriter, request *http.Request) ([]byte, int, error) {
	maxBodyBytes := a.maxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = 1 << 20
	}

	request.Body = http.MaxBytesReader(writer, request.Body, maxBodyBytes)
	defer request.Body.Close()

	body, err := io.ReadAll(request.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, http.StatusRequestEntityTooLarge, err
		}

		return nil, http.StatusBadRequest, err
	}

	return body, http.StatusOK, nil
}

func decodePayload(body []byte) (domaincall.HookPayload, error) {
	var envelope webhookEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&envelope); err != nil {
		return domaincall.HookPayload{}, fmt.Errorf("decode call hook payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return domaincall.HookPayload{}, callhook.ErrInvalidInput
	}

	return domaincall.HookPayload{
		Event: envelope.Event,
		Call:  envelope.Call,
	}, nil
}

func closeBootstrap(ctx context.Context, bootstrap bootstrapCloser, cleanupTimeout time.Duration) error {
	if bootstrap == nil {
		return nil
	}

	timeout := cleanupTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return bootstrap.Close(cleanupCtx)
}
