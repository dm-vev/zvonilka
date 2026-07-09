package teststore

import (
	"sync"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

// NewMemoryStore builds a concurrency-safe in-memory call store for tests.
func NewMemoryStore() call.Store {
	return &memoryStore{
		callsByID:         make(map[string]call.Call),
		invitesByKey:      make(map[string]call.Invite),
		participantsByKey: make(map[string]call.Participant),
		eventsByID:        make(map[string]call.Event),
		workerCursors:     make(map[string]call.WorkerCursor),
	}
}

type memoryStore struct {
	mu sync.RWMutex

	callsByID         map[string]call.Call
	invitesByKey      map[string]call.Invite
	participantsByKey map[string]call.Participant
	eventsByID        map[string]call.Event
	workerCursors     map[string]call.WorkerCursor
	eventOrder        []string
	nextSequence      uint64
}

func inviteKey(callID string, accountID string) string {
	return callID + "|" + accountID
}

func participantKey(callID string, deviceID string) string {
	return callID + "|" + deviceID
}

var _ call.Store = (*memoryStore)(nil)
