package callhook

import "context"

// Store persists recording/transcription jobs.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	SaveRecordingJob(ctx context.Context, job RecordingJob) (RecordingJob, error)
	RecordingJobByCallID(ctx context.Context, callID string) (RecordingJob, error)
	PendingRecordingJobs(ctx context.Context, limit int) ([]RecordingJob, error)
	ClaimPendingRecordingJobs(ctx context.Context, params ClaimJobsParams) ([]RecordingJob, error)
	CompleteRecordingJob(ctx context.Context, params CompleteRecordingJobParams) (RecordingJob, error)
	RetryRecordingJob(ctx context.Context, params RetryJobParams) (RecordingJob, error)

	SaveTranscriptionJob(ctx context.Context, job TranscriptionJob) (TranscriptionJob, error)
	TranscriptionJobByCallID(ctx context.Context, callID string) (TranscriptionJob, error)
	PendingTranscriptionJobs(ctx context.Context, limit int) ([]TranscriptionJob, error)
	ClaimPendingTranscriptionJobs(ctx context.Context, params ClaimJobsParams) ([]TranscriptionJob, error)
	CompleteTranscriptionJob(ctx context.Context, params CompleteTranscriptionJobParams) (TranscriptionJob, error)
	RetryTranscriptionJob(ctx context.Context, params RetryJobParams) (TranscriptionJob, error)
}
