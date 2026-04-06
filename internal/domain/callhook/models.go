package callhook

import (
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

// RecordingJob tracks one persisted recording job derived from call hook events.
type RecordingJob struct {
	OwnerAccountID    string
	ConversationID    string
	CallID            string
	LastEventID       string
	LastEventSequence uint64
	State             domaincall.RecordingState
	OutputMediaID     string
	Attempts          int
	NextAttemptAt     time.Time
	LeaseToken        string
	LeaseExpiresAt    time.Time
	LastAttemptAt     time.Time
	LastError         string
	StartedAt         time.Time
	StoppedAt         time.Time
	UpdatedAt         time.Time
}

// TranscriptionJob tracks one persisted transcription job derived from call hook events.
type TranscriptionJob struct {
	OwnerAccountID    string
	ConversationID    string
	CallID            string
	LastEventID       string
	LastEventSequence uint64
	State             domaincall.TranscriptionState
	TranscriptMediaID string
	Attempts          int
	NextAttemptAt     time.Time
	LeaseToken        string
	LeaseExpiresAt    time.Time
	LastAttemptAt     time.Time
	LastError         string
	StartedAt         time.Time
	StoppedAt         time.Time
	UpdatedAt         time.Time
}
