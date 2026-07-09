package conversation

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// ScheduledMessageWorkerSettings control scheduled-message dispatch cadence.
type ScheduledMessageWorkerSettings struct {
	PollInterval time.Duration
	BatchSize    int
}

// ScheduledMessageHandler consumes sync events produced by scheduled dispatch.
type ScheduledMessageHandler interface {
	HandleScheduledMessageEvents(ctx context.Context, events []EventEnvelope) error
}

// ScheduledMessageWorker publishes due scheduled messages in the background.
type ScheduledMessageWorker struct {
	service  *Service
	handler  ScheduledMessageHandler
	settings ScheduledMessageWorkerSettings
}

// NewScheduledMessageWorker constructs a scheduled-message dispatcher.
func NewScheduledMessageWorker(
	service *Service,
	handler ScheduledMessageHandler,
	settings ScheduledMessageWorkerSettings,
) (*ScheduledMessageWorker, error) {
	if service == nil || handler == nil {
		return nil, ErrInvalidInput
	}
	if settings.PollInterval <= 0 {
		settings.PollInterval = 2 * time.Second
	}
	if settings.BatchSize <= 0 {
		settings.BatchSize = 100
	}

	return &ScheduledMessageWorker{
		service:  service,
		handler:  handler,
		settings: settings,
	}, nil
}

// Run executes the scheduled dispatch loop until ctx is canceled.
func (w *ScheduledMessageWorker) Run(ctx context.Context, logger *slog.Logger) error {
	if ctx == nil || logger == nil {
		return ErrInvalidInput
	}

	ticker := time.NewTicker(w.settings.PollInterval)
	defer ticker.Stop()

	for {
		if err := w.processOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(ctx, "process scheduled message batch", "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ProcessOnceForTests runs one scheduled-dispatch batch.
func (w *ScheduledMessageWorker) ProcessOnceForTests(ctx context.Context) error {
	return w.processOnce(ctx)
}

func (w *ScheduledMessageWorker) processOnce(ctx context.Context) error {
	events, err := w.service.DispatchDueMessages(ctx, time.Time{}, w.settings.BatchSize)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	return w.handler.HandleScheduledMessageEvents(ctx, events)
}
