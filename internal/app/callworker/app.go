package callworker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/call"
	callpg "github.com/dm-vev/zvonilka/internal/domain/call/pgstore"
	"github.com/dm-vev/zvonilka/internal/platform/buildinfo"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/platform/runtime"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

type app struct {
	bootstrap      bootstrapCloser
	worker         *call.Worker
	health         *runtime.Health
	handler        http.Handler
	cleanupTimeout time.Duration
}

type bootstrapCloser interface {
	Close(context.Context) error
}

type webhookHandler struct {
	client           *http.Client
	recordingURL     string
	transcriptionURL string
}

type webhookEnvelope struct {
	Event call.Event `json:"event"`
	Call  call.Call  `json:"call"`
}

func newApp(ctx context.Context, cfg config.Configuration) (*app, error) {
	if !cfg.Infrastructure.Postgres.Enabled {
		return nil, call.ErrInvalidInput
	}

	bootstrap := postgresplatform.NewBootstrap(cfg)
	db, err := bootstrap.Open(ctx)
	if err != nil {
		return nil, err
	}

	callStore, err := callpg.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}

	handler := &webhookHandler{
		client: &http.Client{
			Timeout: cfg.Call.HookTimeout,
		},
		recordingURL:     cfg.Call.RecordingHookURL,
		transcriptionURL: cfg.Call.TranscriptionHookURL,
	}

	worker, err := call.NewWorker(callStore, handler, call.WorkerSettings{
		PollInterval: cfg.Call.WorkerPollInterval,
		BatchSize:    cfg.Call.WorkerBatchSize,
	})
	if err != nil {
		_ = closeBootstrap(ctx, bootstrap, cfg.Runtime.ShutdownTimeout)
		return nil, err
	}

	return &app{
		bootstrap:      bootstrap,
		worker:         worker,
		health:         runtime.NewHealth(cfg.Service.Name, buildinfo.Version, buildinfo.Commit, buildinfo.Date),
		handler:        http.NotFoundHandler(),
		cleanupTimeout: cfg.Runtime.ShutdownTimeout,
	}, nil
}

func (h *webhookHandler) HandleRecording(ctx context.Context, payload call.HookPayload) error {
	if h == nil || strings.TrimSpace(h.recordingURL) == "" {
		return nil
	}
	return h.post(ctx, h.recordingURL, payload)
}

func (h *webhookHandler) HandleTranscription(ctx context.Context, payload call.HookPayload) error {
	if h == nil || strings.TrimSpace(h.transcriptionURL) == "" {
		return nil
	}
	return h.post(ctx, h.transcriptionURL, payload)
}

func (h *webhookHandler) post(ctx context.Context, url string, payload call.HookPayload) error {
	body, err := json.Marshal(webhookEnvelope{
		Event: payload.Event,
		Call:  payload.Call,
	})
	if err != nil {
		return fmt.Errorf("marshal call hook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build call hook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Zvonilka-Event-ID", payload.Event.EventID)
	req.Header.Set("X-Zvonilka-Event-Type", string(payload.Event.EventType))

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("deliver call hook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("deliver call hook: unexpected status %d", resp.StatusCode)
	}

	return nil
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
