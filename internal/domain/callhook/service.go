package callhook

import (
	"context"
	"errors"
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
		OwnerAccountID:    ownerAccountID(payload.Call),
		ConversationID:    strings.TrimSpace(payload.Call.ConversationID),
		CallID:            payload.Call.ID,
		LastEventID:       payload.Event.EventID,
		LastEventSequence: payload.Event.Sequence,
		State:             payload.Call.RecordingState,
		StartedAt:         payload.Call.RecordingStartedAt,
		StoppedAt:         payload.Call.RecordingStoppedAt,
		UpdatedAt:         s.currentTime(),
	}

	var saved RecordingJob
	err := s.store.WithinTx(ctx, func(tx Store) error {
		existing, loadErr := tx.RecordingJobByCallID(ctx, job.CallID)
		switch {
		case loadErr == nil:
			if job.LastEventSequence <= existing.LastEventSequence {
				saved = existing
				return nil
			}
			job = mergeRecordingJob(existing, job, s.currentTime())
		case errors.Is(loadErr, ErrNotFound):
			job = prepareRecordingJob(job, s.currentTime())
		default:
			return fmt.Errorf("load recording job %s: %w", job.CallID, loadErr)
		}

		var saveErr error
		saved, saveErr = tx.SaveRecordingJob(ctx, job)
		if saveErr != nil {
			return fmt.Errorf("save recording job %s: %w", payload.Call.ID, saveErr)
		}

		return nil
	})
	if err != nil {
		return RecordingJob{}, err
	}

	return saved, nil
}

// ApplyTranscriptionHook persists one transcription job update.
func (s *Service) ApplyTranscriptionHook(ctx context.Context, payload domaincall.HookPayload) (TranscriptionJob, error) {
	if err := s.validatePayload(ctx, payload, domaincall.EventTypeTranscriptionUpdated); err != nil {
		return TranscriptionJob{}, err
	}

	job := TranscriptionJob{
		OwnerAccountID:    ownerAccountID(payload.Call),
		ConversationID:    strings.TrimSpace(payload.Call.ConversationID),
		CallID:            payload.Call.ID,
		LastEventID:       payload.Event.EventID,
		LastEventSequence: payload.Event.Sequence,
		State:             payload.Call.TranscriptionState,
		StartedAt:         payload.Call.TranscriptionStartedAt,
		StoppedAt:         payload.Call.TranscriptionStoppedAt,
		UpdatedAt:         s.currentTime(),
	}

	var saved TranscriptionJob
	err := s.store.WithinTx(ctx, func(tx Store) error {
		existing, loadErr := tx.TranscriptionJobByCallID(ctx, job.CallID)
		switch {
		case loadErr == nil:
			if job.LastEventSequence <= existing.LastEventSequence {
				saved = existing
				return nil
			}
			job = mergeTranscriptionJob(existing, job, s.currentTime())
		case errors.Is(loadErr, ErrNotFound):
			job = prepareTranscriptionJob(job, s.currentTime())
		default:
			return fmt.Errorf("load transcription job %s: %w", job.CallID, loadErr)
		}

		var saveErr error
		saved, saveErr = tx.SaveTranscriptionJob(ctx, job)
		if saveErr != nil {
			return fmt.Errorf("save transcription job %s: %w", payload.Call.ID, saveErr)
		}

		return nil
	})
	if err != nil {
		return TranscriptionJob{}, err
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
	if strings.TrimSpace(payload.Event.EventID) == "" || payload.Event.Sequence == 0 {
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

func prepareRecordingJob(job RecordingJob, now time.Time) RecordingJob {
	job.UpdatedAt = now.UTC()
	job.NextAttemptAt = now.UTC()
	job.LeaseToken = ""
	job.LeaseExpiresAt = time.Time{}
	job.LastAttemptAt = time.Time{}
	job.LastError = ""
	job.Attempts = 0

	return job
}

func mergeRecordingJob(existing RecordingJob, next RecordingJob, now time.Time) RecordingJob {
	next = prepareRecordingJob(next, now)
	if next.OwnerAccountID == "" {
		next.OwnerAccountID = existing.OwnerAccountID
	}
	if next.ConversationID == "" {
		next.ConversationID = existing.ConversationID
	}
	if sameRecordingCapture(existing, next) {
		next.OutputMediaID = existing.OutputMediaID
		next.Attempts = existing.Attempts
		next.NextAttemptAt = existing.NextAttemptAt
		next.LeaseToken = existing.LeaseToken
		next.LeaseExpiresAt = existing.LeaseExpiresAt
		next.LastAttemptAt = existing.LastAttemptAt
		next.LastError = existing.LastError
	}

	return next
}

func prepareTranscriptionJob(job TranscriptionJob, now time.Time) TranscriptionJob {
	job.UpdatedAt = now.UTC()
	job.NextAttemptAt = now.UTC()
	job.LeaseToken = ""
	job.LeaseExpiresAt = time.Time{}
	job.LastAttemptAt = time.Time{}
	job.LastError = ""
	job.Attempts = 0

	return job
}

func mergeTranscriptionJob(existing TranscriptionJob, next TranscriptionJob, now time.Time) TranscriptionJob {
	next = prepareTranscriptionJob(next, now)
	if next.OwnerAccountID == "" {
		next.OwnerAccountID = existing.OwnerAccountID
	}
	if next.ConversationID == "" {
		next.ConversationID = existing.ConversationID
	}
	if sameTranscriptionCapture(existing, next) {
		next.TranscriptMediaID = existing.TranscriptMediaID
		next.Attempts = existing.Attempts
		next.NextAttemptAt = existing.NextAttemptAt
		next.LeaseToken = existing.LeaseToken
		next.LeaseExpiresAt = existing.LeaseExpiresAt
		next.LastAttemptAt = existing.LastAttemptAt
		next.LastError = existing.LastError
	}

	return next
}

func sameRecordingCapture(left RecordingJob, right RecordingJob) bool {
	return left.State == right.State &&
		left.StartedAt.Equal(right.StartedAt) &&
		left.StoppedAt.Equal(right.StoppedAt)
}

func sameTranscriptionCapture(left TranscriptionJob, right TranscriptionJob) bool {
	return left.State == right.State &&
		left.StartedAt.Equal(right.StartedAt) &&
		left.StoppedAt.Equal(right.StoppedAt)
}
