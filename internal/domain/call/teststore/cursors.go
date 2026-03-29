package teststore

import (
	"context"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

func (s *memoryStore) SaveWorkerCursor(_ context.Context, cursor call.WorkerCursor) (call.WorkerCursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	value, err := call.NormalizeWorkerCursor(cursor, time.Now().UTC())
	if err != nil {
		return call.WorkerCursor{}, err
	}
	if existing, ok := s.workerCursors[value.Name]; ok && value.LastSequence <= existing.LastSequence {
		return existing, nil
	}
	s.workerCursors[value.Name] = value
	return value, nil
}

func (s *memoryStore) WorkerCursorByName(_ context.Context, name string) (call.WorkerCursor, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return call.WorkerCursor{}, call.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.workerCursors[name]
	if !ok {
		return call.WorkerCursor{}, call.ErrNotFound
	}

	return value, nil
}
