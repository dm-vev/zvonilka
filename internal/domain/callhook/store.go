package callhook

import "context"

// Store persists recording/transcription jobs.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	SaveRecordingJob(ctx context.Context, job RecordingJob) (RecordingJob, error)
	RecordingJobByCallID(ctx context.Context, callID string) (RecordingJob, error)

	SaveTranscriptionJob(ctx context.Context, job TranscriptionJob) (TranscriptionJob, error)
	TranscriptionJobByCallID(ctx context.Context, callID string) (TranscriptionJob, error)
}
