package call

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// RehomeWorkerSettings control proactive session rehome cadence.
type RehomeWorkerSettings struct {
	PollInterval time.Duration
	BatchSize    int
}

// RehomeHandler consumes call events produced by proactive session migration.
type RehomeHandler interface {
	HandleCallEvents(ctx context.Context, events []Event) error
}

// RehomeWorker proactively migrates active calls away from unavailable runtime nodes.
type RehomeWorker struct {
	service  *Service
	handler  RehomeHandler
	settings RehomeWorkerSettings
}

// NewRehomeWorker constructs a proactive session rehome worker.
func NewRehomeWorker(service *Service, handler RehomeHandler, settings RehomeWorkerSettings) (*RehomeWorker, error) {
	if service == nil || handler == nil {
		return nil, ErrInvalidInput
	}
	if settings.PollInterval <= 0 {
		settings.PollInterval = 3 * time.Second
	}
	if settings.BatchSize <= 0 {
		settings.BatchSize = 64
	}

	return &RehomeWorker{
		service:  service,
		handler:  handler,
		settings: settings,
	}, nil
}

// Run executes the proactive rehome loop until ctx is canceled.
func (w *RehomeWorker) Run(ctx context.Context, logger *slog.Logger) error {
	if ctx == nil || logger == nil {
		return ErrInvalidInput
	}

	ticker := time.NewTicker(w.settings.PollInterval)
	defer ticker.Stop()

	for {
		if err := w.processOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(ctx, "process call rehome batch", "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ProcessOnceForTests runs one proactive rehome batch. It is intended for tests.
func (w *RehomeWorker) ProcessOnceForTests(ctx context.Context) error {
	return w.processOnce(ctx)
}

func (w *RehomeWorker) processOnce(ctx context.Context) error {
	events, err := w.service.RehomeActiveCalls(ctx, w.settings.BatchSize)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	return w.handler.HandleCallEvents(ctx, events)
}
