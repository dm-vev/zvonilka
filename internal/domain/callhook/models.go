package callhook

import (
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

// RecordingJob tracks one persisted recording job derived from call hook events.
type RecordingJob struct {
	CallID        string
	LastEventID   string
	State         domaincall.RecordingState
	OutputMediaID string
	StartedAt     time.Time
	StoppedAt     time.Time
	UpdatedAt     time.Time
}

// TranscriptionJob tracks one persisted transcription job derived from call hook events.
type TranscriptionJob struct {
	CallID            string
	LastEventID       string
	State             domaincall.TranscriptionState
	TranscriptMediaID string
	StartedAt         time.Time
	StoppedAt         time.Time
	UpdatedAt         time.Time
}
