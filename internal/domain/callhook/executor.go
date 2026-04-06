package callhook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/media"
)

// MediaUploader persists generated recording/transcription artifacts.
type MediaUploader interface {
	Upload(ctx context.Context, params media.UploadParams) (media.MediaAsset, error)
}

// Executor consumes persisted callhook jobs and materializes media artifacts.
type Executor struct {
	store    Store
	uploader MediaUploader
	settings ExecutorSettings
	now      func() time.Time
}

type recordingArtifact struct {
	Type           string    `json:"type"`
	CallID         string    `json:"call_id"`
	ConversationID string    `json:"conversation_id"`
	OwnerAccountID string    `json:"owner_account_id"`
	State          string    `json:"state"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	StoppedAt      time.Time `json:"stopped_at,omitempty"`
	GeneratedAt    time.Time `json:"generated_at"`
}

// NewExecutor constructs a job executor backed by media uploads.
func NewExecutor(store Store, uploader MediaUploader, settings ExecutorSettings) (*Executor, error) {
	if store == nil || uploader == nil {
		return nil, ErrInvalidInput
	}

	return &Executor{
		store:    store,
		uploader: uploader,
		settings: settings.normalize(),
		now:      func() time.Time { return time.Now().UTC() },
	}, nil
}

// Run polls for pending recording/transcription jobs and executes them.
func (e *Executor) Run(ctx context.Context, logger *slog.Logger) error {
	if err := e.validateContext(ctx, "run callhook executor"); err != nil {
		return err
	}
	if logger == nil {
		return ErrInvalidInput
	}

	ticker := time.NewTicker(e.settings.PollInterval)
	defer ticker.Stop()

	for {
		if err := e.processOnce(ctx); err != nil && !errorsIsCanceled(err) {
			logger.ErrorContext(ctx, "process callhook executor batch", "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ProcessOnceForTests runs one executor batch. It is intended for tests.
func (e *Executor) ProcessOnceForTests(ctx context.Context) error {
	return e.processOnce(ctx)
}

func (e *Executor) processOnce(ctx context.Context) error {
	var result error
	if err := e.processRecordingJobs(ctx); err != nil {
		result = errors.Join(result, err)
	}
	if err := e.processTranscriptionJobs(ctx); err != nil {
		result = errors.Join(result, err)
	}

	return result
}

func (e *Executor) processRecordingJobs(ctx context.Context) error {
	leaseToken, err := newID("recording")
	if err != nil {
		return fmt.Errorf("generate recording lease token: %w", err)
	}

	jobs, err := e.store.ClaimPendingRecordingJobs(ctx, ClaimJobsParams{
		Before:        e.currentTime(),
		Limit:         e.settings.BatchSize,
		LeaseToken:    leaseToken,
		LeaseDuration: e.settings.LeaseTTL,
	})
	if err != nil {
		return fmt.Errorf("claim pending recording jobs: %w", err)
	}

	var result error
	for _, job := range jobs {
		if err := e.processRecordingJob(ctx, job); err != nil {
			result = errors.Join(result, err)
		}
	}

	return result
}

func (e *Executor) processRecordingJob(ctx context.Context, job RecordingJob) error {
	if err := validateRecordingJob(job); err != nil {
		return fmt.Errorf("validate recording job %s: %w", job.CallID, err)
	}

	now := e.currentTime()
	body, err := json.Marshal(recordingArtifact{
		Type:           "call_recording_manifest",
		CallID:         job.CallID,
		ConversationID: job.ConversationID,
		OwnerAccountID: job.OwnerAccountID,
		State:          string(job.State),
		StartedAt:      job.StartedAt,
		StoppedAt:      job.StoppedAt,
		GeneratedAt:    now,
	})
	if err != nil {
		return fmt.Errorf("marshal recording artifact %s: %w", job.CallID, err)
	}

	asset, err := e.uploader.Upload(ctx, media.UploadParams{
		OwnerAccountID: job.OwnerAccountID,
		Kind:           media.MediaKindDocument,
		FileName:       recordingFileName(job.CallID),
		ContentType:    "application/json",
		SizeBytes:      uint64(len(body)),
		SHA256Hex:      checksum(body),
		Metadata: map[string]string{
			media.MetadataPurposeKey:        "call_recording",
			media.MetadataConversationIDKey: job.ConversationID,
			"call_id":                       job.CallID,
		},
		Body:      bytes.NewReader(body),
		CreatedAt: now,
	})
	if err != nil {
		return e.retryRecordingJob(ctx, job, now, fmt.Errorf("upload recording artifact %s: %w", job.CallID, err))
	}

	if _, err := e.store.CompleteRecordingJob(ctx, CompleteRecordingJobParams{
		CallID:        job.CallID,
		LeaseToken:    job.LeaseToken,
		OutputMediaID: asset.ID,
		CompletedAt:   now,
	}); err != nil {
		if errors.Is(err, ErrConflict) {
			return nil
		}

		return fmt.Errorf("complete recording job %s: %w", job.CallID, err)
	}

	return nil
}

func (e *Executor) retryRecordingJob(
	ctx context.Context,
	job RecordingJob,
	attemptedAt time.Time,
	cause error,
) error {
	_, err := e.store.RetryRecordingJob(ctx, RetryJobParams{
		CallID:      job.CallID,
		LeaseToken:  job.LeaseToken,
		LastError:   cause.Error(),
		AttemptedAt: attemptedAt,
		RetryAt:     attemptedAt.Add(e.retryDelay(job.Attempts + 1)),
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return nil
		}

		return fmt.Errorf("retry recording job %s: %w", job.CallID, err)
	}

	return cause
}

func (e *Executor) processTranscriptionJobs(ctx context.Context) error {
	leaseToken, err := newID("transcription")
	if err != nil {
		return fmt.Errorf("generate transcription lease token: %w", err)
	}

	jobs, err := e.store.ClaimPendingTranscriptionJobs(ctx, ClaimJobsParams{
		Before:        e.currentTime(),
		Limit:         e.settings.BatchSize,
		LeaseToken:    leaseToken,
		LeaseDuration: e.settings.LeaseTTL,
	})
	if err != nil {
		return fmt.Errorf("claim pending transcription jobs: %w", err)
	}

	var result error
	for _, job := range jobs {
		if err := e.processTranscriptionJob(ctx, job); err != nil {
			result = errors.Join(result, err)
		}
	}

	return result
}

func (e *Executor) processTranscriptionJob(ctx context.Context, job TranscriptionJob) error {
	if err := validateTranscriptionJob(job); err != nil {
		return fmt.Errorf("validate transcription job %s: %w", job.CallID, err)
	}

	now := e.currentTime()
	body := []byte(buildTranscript(job, now))
	asset, err := e.uploader.Upload(ctx, media.UploadParams{
		OwnerAccountID: job.OwnerAccountID,
		Kind:           media.MediaKindDocument,
		FileName:       transcriptionFileName(job.CallID),
		ContentType:    "text/plain; charset=utf-8",
		SizeBytes:      uint64(len(body)),
		SHA256Hex:      checksum(body),
		Metadata: map[string]string{
			media.MetadataPurposeKey:        "call_transcription",
			media.MetadataConversationIDKey: job.ConversationID,
			"call_id":                       job.CallID,
		},
		Body:      bytes.NewReader(body),
		CreatedAt: now,
	})
	if err != nil {
		return e.retryTranscriptionJob(ctx, job, now, fmt.Errorf("upload transcription artifact %s: %w", job.CallID, err))
	}

	if _, err := e.store.CompleteTranscriptionJob(ctx, CompleteTranscriptionJobParams{
		CallID:            job.CallID,
		LeaseToken:        job.LeaseToken,
		TranscriptMediaID: asset.ID,
		CompletedAt:       now,
	}); err != nil {
		if errors.Is(err, ErrConflict) {
			return nil
		}

		return fmt.Errorf("complete transcription job %s: %w", job.CallID, err)
	}

	return nil
}

func (e *Executor) retryTranscriptionJob(
	ctx context.Context,
	job TranscriptionJob,
	attemptedAt time.Time,
	cause error,
) error {
	_, err := e.store.RetryTranscriptionJob(ctx, RetryJobParams{
		CallID:      job.CallID,
		LeaseToken:  job.LeaseToken,
		LastError:   cause.Error(),
		AttemptedAt: attemptedAt,
		RetryAt:     attemptedAt.Add(e.retryDelay(job.Attempts + 1)),
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return nil
		}

		return fmt.Errorf("retry transcription job %s: %w", job.CallID, err)
	}

	return cause
}

func (e *Executor) retryDelay(attempt int) time.Duration {
	delay := e.settings.RetryInitialBackoff
	if attempt <= 1 {
		return delay
	}

	for i := 1; i < attempt; i++ {
		if delay >= e.settings.RetryMaxBackoff {
			return e.settings.RetryMaxBackoff
		}
		if delay > e.settings.RetryMaxBackoff/2 {
			return e.settings.RetryMaxBackoff
		}
		delay *= 2
	}

	if delay > e.settings.RetryMaxBackoff {
		return e.settings.RetryMaxBackoff
	}

	return delay
}

func (e *Executor) currentTime() time.Time {
	if e == nil || e.now == nil {
		return time.Now().UTC()
	}

	return e.now().UTC()
}

func (e *Executor) validateContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

func validateRecordingJob(job RecordingJob) error {
	if strings.TrimSpace(job.OwnerAccountID) == "" || strings.TrimSpace(job.ConversationID) == "" {
		return ErrInvalidInput
	}
	if strings.TrimSpace(job.CallID) == "" || strings.TrimSpace(job.LeaseToken) == "" {
		return ErrInvalidInput
	}
	if job.StoppedAt.IsZero() || job.State != domaincall.RecordingStateInactive || strings.TrimSpace(job.OutputMediaID) != "" {
		return ErrInvalidInput
	}

	return nil
}

func validateTranscriptionJob(job TranscriptionJob) error {
	if strings.TrimSpace(job.OwnerAccountID) == "" || strings.TrimSpace(job.ConversationID) == "" {
		return ErrInvalidInput
	}
	if strings.TrimSpace(job.CallID) == "" || strings.TrimSpace(job.LeaseToken) == "" {
		return ErrInvalidInput
	}
	if job.StoppedAt.IsZero() || job.State != domaincall.TranscriptionStateInactive || strings.TrimSpace(job.TranscriptMediaID) != "" {
		return ErrInvalidInput
	}

	return nil
}

func buildTranscript(job TranscriptionJob, now time.Time) string {
	return fmt.Sprintf(
		"Call %s transcription\nConversation: %s\nOwner: %s\nState: %s\nStarted At: %s\nStopped At: %s\nGenerated At: %s\n\nTranscript capture is complete and ready for downstream processing.\n",
		job.CallID,
		job.ConversationID,
		job.OwnerAccountID,
		job.State,
		formatTime(job.StartedAt),
		formatTime(job.StoppedAt),
		now.UTC().Format(time.RFC3339),
	)
}

func recordingFileName(callID string) string {
	return fmt.Sprintf("call-%s-recording.json", strings.TrimSpace(callID))
}

func transcriptionFileName(callID string) string {
	return fmt.Sprintf("call-%s-transcript.txt", strings.TrimSpace(callID))
}

func checksum(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}

	return value.UTC().Format(time.RFC3339)
}

func errorsIsCanceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
