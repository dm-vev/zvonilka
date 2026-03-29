package callhook

import (
	"context"
	"fmt"
	"strings"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

// Service applies incoming call hook payloads to durable recorder/transcriber job state.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService constructs a call hook service.
func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}

	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}, nil
}

// ApplyRecordingHook persists one recording job update.
func (s *Service) ApplyRecordingHook(ctx context.Context, payload domaincall.HookPayload) (RecordingJob, error) {
	if err := s.validatePayload(ctx, payload, domaincall.EventTypeRecordingUpdated); err != nil {
		return RecordingJob{}, err
	}

	job := RecordingJob{
		OwnerAccountID: ownerAccountID(payload.Call),
		ConversationID: strings.TrimSpace(payload.Call.ConversationID),
		CallID:         payload.Call.ID,
		LastEventID:    payload.Event.EventID,
		State:          payload.Call.RecordingState,
		StartedAt:      payload.Call.RecordingStartedAt,
		StoppedAt:      payload.Call.RecordingStoppedAt,
		UpdatedAt:      s.currentTime(),
	}

	saved, err := s.store.SaveRecordingJob(ctx, job)
	if err != nil {
		return RecordingJob{}, fmt.Errorf("save recording job %s: %w", payload.Call.ID, err)
	}

	return saved, nil
}

// ApplyTranscriptionHook persists one transcription job update.
func (s *Service) ApplyTranscriptionHook(ctx context.Context, payload domaincall.HookPayload) (TranscriptionJob, error) {
	if err := s.validatePayload(ctx, payload, domaincall.EventTypeTranscriptionUpdated); err != nil {
		return TranscriptionJob{}, err
	}

	job := TranscriptionJob{
		OwnerAccountID: ownerAccountID(payload.Call),
		ConversationID: strings.TrimSpace(payload.Call.ConversationID),
		CallID:         payload.Call.ID,
		LastEventID:    payload.Event.EventID,
		State:          payload.Call.TranscriptionState,
		StartedAt:      payload.Call.TranscriptionStartedAt,
		StoppedAt:      payload.Call.TranscriptionStoppedAt,
		UpdatedAt:      s.currentTime(),
	}

	saved, err := s.store.SaveTranscriptionJob(ctx, job)
	if err != nil {
		return TranscriptionJob{}, fmt.Errorf("save transcription job %s: %w", payload.Call.ID, err)
	}

	return saved, nil
}

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}

	return s.now().UTC()
}

func (s *Service) validatePayload(ctx context.Context, payload domaincall.HookPayload, want domaincall.EventType) error {
	if ctx == nil {
		return ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if payload.Event.EventType != want {
		return ErrInvalidInput
	}
	if strings.TrimSpace(payload.Event.CallID) == "" || strings.TrimSpace(payload.Call.ID) == "" {
		return ErrInvalidInput
	}
	if strings.TrimSpace(payload.Event.CallID) != strings.TrimSpace(payload.Call.ID) {
		return ErrInvalidInput
	}

	return nil
}

func ownerAccountID(callRow domaincall.Call) string {
	hostAccountID := strings.TrimSpace(callRow.HostAccountID)
	if hostAccountID != "" {
		return hostAccountID
	}

	return strings.TrimSpace(callRow.InitiatorAccountID)
}
