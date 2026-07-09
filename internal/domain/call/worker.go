package call

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const defaultWorkerName = "call_hooks"

// WorkerCursor tracks the last processed call-event sequence for one worker.
type WorkerCursor struct {
	Name         string
	LastSequence uint64
	UpdatedAt    time.Time
}

// HookPayload is the durable payload delivered to external recording/transcription hooks.
type HookPayload struct {
	Event Event
	Call  Call
}

// HookHandler consumes call hook payloads.
type HookHandler interface {
	HandleRecording(ctx context.Context, payload HookPayload) error
	HandleTranscription(ctx context.Context, payload HookPayload) error
}

// WorkerSettings control worker cadence.
type WorkerSettings struct {
	PollInterval time.Duration
	BatchSize    int
}

// Worker fans out recording/transcription call events into external hooks.
type Worker struct {
	store    Store
	handler  HookHandler
	settings WorkerSettings
	name     string
	now      func() time.Time
}

// NewWorker constructs a call hook worker.
func NewWorker(store Store, handler HookHandler, settings WorkerSettings) (*Worker, error) {
	if store == nil || handler == nil {
		return nil, ErrInvalidInput
	}
	if settings.PollInterval <= 0 {
		settings.PollInterval = 2 * time.Second
	}
	if settings.BatchSize <= 0 {
		settings.BatchSize = 100
	}

	return &Worker{
		store:    store,
		handler:  handler,
		settings: settings,
		name:     defaultWorkerName,
		now:      func() time.Time { return time.Now().UTC() },
	}, nil
}

// Run polls call events and delivers matching hook payloads.
func (w *Worker) Run(ctx context.Context, logger *slog.Logger) error {
	if err := w.validateContext(ctx, "run call worker"); err != nil {
		return err
	}
	if logger == nil {
		return ErrInvalidInput
	}

	ticker := time.NewTicker(w.settings.PollInterval)
	defer ticker.Stop()

	for {
		if err := w.processOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(ctx, "process call hook batch", "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) processOnce(ctx context.Context) error {
	cursor, err := w.store.WorkerCursorByName(ctx, w.name)
	if err != nil {
		if err != ErrNotFound {
			return err
		}
		cursor = WorkerCursor{Name: w.name, UpdatedAt: w.currentTime()}
	}

	events, err := w.store.EventsAfterSequence(ctx, cursor.LastSequence, "", "", w.settings.BatchSize)
	if err != nil {
		return fmt.Errorf("load call events after %d: %w", cursor.LastSequence, err)
	}
	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		if !isHookEvent(event.EventType) {
			cursor.LastSequence = maxUint64(cursor.LastSequence, event.Sequence)
			cursor.UpdatedAt = w.currentTime()
			if _, err := w.store.SaveWorkerCursor(ctx, cursor); err != nil {
				return fmt.Errorf("save call worker cursor %s: %w", cursor.Name, err)
			}
			continue
		}

		callRow, err := w.store.CallByID(ctx, event.CallID)
		if err != nil {
			return fmt.Errorf("load call %s for hook event %s: %w", event.CallID, event.EventID, err)
		}
		payload := HookPayload{
			Event: cloneEvent(event),
			Call:  cloneCall(callRow),
		}

		switch event.EventType {
		case EventTypeRecordingUpdated:
			if err := w.handler.HandleRecording(ctx, payload); err != nil {
				return fmt.Errorf("deliver recording hook %s: %w", event.EventID, err)
			}
		case EventTypeTranscriptionUpdated:
			if err := w.handler.HandleTranscription(ctx, payload); err != nil {
				return fmt.Errorf("deliver transcription hook %s: %w", event.EventID, err)
			}
		}

		cursor.LastSequence = maxUint64(cursor.LastSequence, event.Sequence)
		cursor.UpdatedAt = w.currentTime()
		if _, err := w.store.SaveWorkerCursor(ctx, cursor); err != nil {
			return fmt.Errorf("save call worker cursor %s: %w", cursor.Name, err)
		}
	}

	return nil
}

// ProcessOnceForTests runs one worker batch. It is intended for tests.
func (w *Worker) ProcessOnceForTests(ctx context.Context) error {
	return w.processOnce(ctx)
}

// NormalizeWorkerCursor validates and normalizes one worker cursor.
func NormalizeWorkerCursor(cursor WorkerCursor, now time.Time) (WorkerCursor, error) {
	cursor.Name = strings.TrimSpace(strings.ToLower(cursor.Name))
	if cursor.Name == "" {
		return WorkerCursor{}, ErrInvalidInput
	}
	if cursor.UpdatedAt.IsZero() {
		cursor.UpdatedAt = now.UTC()
	} else {
		cursor.UpdatedAt = cursor.UpdatedAt.UTC()
	}

	return cursor, nil
}

func isHookEvent(eventType EventType) bool {
	return eventType == EventTypeRecordingUpdated || eventType == EventTypeTranscriptionUpdated
}

func (w *Worker) currentTime() time.Time {
	if w == nil || w.now == nil {
		return time.Now().UTC()
	}

	return w.now().UTC()
}

func (w *Worker) validateContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

func maxUint64(values ...uint64) uint64 {
	var max uint64
	for _, value := range values {
		if value > max {
			max = value
		}
	}

	return max
}
