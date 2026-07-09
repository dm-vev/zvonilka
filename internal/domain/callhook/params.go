package callhook

import "time"

// ClaimJobsParams acquires a lease on pending call hook jobs.
type ClaimJobsParams struct {
	Before        time.Time
	Limit         int
	LeaseToken    string
	LeaseDuration time.Duration
}

// CompleteRecordingJobParams marks a recording job as successfully materialized.
type CompleteRecordingJobParams struct {
	CallID        string
	LeaseToken    string
	OutputMediaID string
	CompletedAt   time.Time
}

// CompleteTranscriptionJobParams marks a transcription job as successfully materialized.
type CompleteTranscriptionJobParams struct {
	CallID            string
	LeaseToken        string
	TranscriptMediaID string
	CompletedAt       time.Time
}

// RetryJobParams schedules another executor attempt for a claimed job.
type RetryJobParams struct {
	CallID      string
	LeaseToken  string
	LastError   string
	AttemptedAt time.Time
	RetryAt     time.Time
}
