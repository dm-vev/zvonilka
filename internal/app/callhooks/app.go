package callhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/callhook"
	callhookpg "github.com/dm-vev/zvonilka/internal/domain/callhook/pgstore"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

type app struct {
	bootstrap      bootstrapCloser
	health         *runtime.Health
	handler        http.Handler
	cleanupTimeout time.Duration
}

type bootstrapCloser interface {
	Close(context.Context) error
}

type api struct {
	service *callhook.Service
}

type webhookEnvelope struct {
	Event domaincall.Event `json:"event"`
	Call  domaincall.Call  `json:"call"`
}

func newApp(ctx context.Context, cfg config.Configuration) (*app, error) {
	if !cfg.Infrastructure.Postgres.Enabled {
		return nil, callhook.ErrInvalidInput
	}

	bootstrap := postgresplatform.NewBootstrap(cfg)
	db, err := bootstrap.Open(ctx)
	if err != nil {
		return nil, err
	}

	store, err := callhookpg.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}
	service, err := callhook.NewService(store)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}

	return &app{
		bootstrap:      bootstrap,
		health:         runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date),
		handler:        (&api{service: service}).routes(),
		cleanupTimeout: cfg.Runtime.ShutdownTimeout,
	}, nil
}

func (a *api) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/recording", a.recording)
	mux.HandleFunc("/transcription", a.transcription)
	return mux
}

func (a *api) recording(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	payload, err := decodePayload(request)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if _, err := a.service.ApplyRecordingHook(request.Context(), payload); err != nil {
		if errors.Is(err, callhook.ErrInvalidInput) {
			http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (a *api) transcription(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	payload, err := decodePayload(request)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if _, err := a.service.ApplyTranscriptionHook(request.Context(), payload); err != nil {
		if errors.Is(err, callhook.ErrInvalidInput) {
			http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func decodePayload(request *http.Request) (domaincall.HookPayload, error) {
	defer request.Body.Close()

	var envelope webhookEnvelope
	if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
		return domaincall.HookPayload{}, fmt.Errorf("decode call hook payload: %w", err)
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
