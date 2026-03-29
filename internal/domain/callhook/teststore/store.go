package teststore

import (
	"context"
	"strings"
	"sync"

	"github.com/dm-vev/zvonilka/internal/domain/callhook"
	"github.com/dm-vev/zvonilka/internal/domain/storage"
)

// NewMemoryStore builds an in-memory callhook store for tests.
func NewMemoryStore() callhook.Store {
	return &memoryStore{
		recordingByCallID:     make(map[string]callhook.RecordingJob),
		transcriptionByCallID: make(map[string]callhook.TranscriptionJob),
	}
}

type memoryStore struct {
	mu sync.RWMutex

	recordingByCallID     map[string]callhook.RecordingJob
	transcriptionByCallID map[string]callhook.TranscriptionJob
}

func (s *memoryStore) WithinTx(ctx context.Context, fn func(callhook.Store) error) error {
	if ctx == nil || fn == nil {
		return callhook.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx := &memoryStore{
		recordingByCallID:     cloneRecordingJobs(s.recordingByCallID),
		transcriptionByCallID: cloneTranscriptionJobs(s.transcriptionByCallID),
	}
	err := fn(tx)
	if err == nil {
		s.recordingByCallID = cloneRecordingJobs(tx.recordingByCallID)
		s.transcriptionByCallID = cloneTranscriptionJobs(tx.transcriptionByCallID)
		return nil
	}
	if storage.IsCommit(err) {
		s.recordingByCallID = cloneRecordingJobs(tx.recordingByCallID)
		s.transcriptionByCallID = cloneTranscriptionJobs(tx.transcriptionByCallID)
		return storage.UnwrapCommit(err)
	}

	return err
}

func (s *memoryStore) SaveRecordingJob(_ context.Context, job callhook.RecordingJob) (callhook.RecordingJob, error) {
	job.CallID = strings.TrimSpace(job.CallID)
	job.LastEventID = strings.TrimSpace(job.LastEventID)
	if job.CallID == "" || job.LastEventID == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}
	s.recordingByCallID[job.CallID] = job
	return job, nil
}

func (s *memoryStore) RecordingJobByCallID(_ context.Context, callID string) (callhook.RecordingJob, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}
	job, ok := s.recordingByCallID[callID]
	if !ok {
		return callhook.RecordingJob{}, callhook.ErrNotFound
	}
	return job, nil
}

func (s *memoryStore) SaveTranscriptionJob(_ context.Context, job callhook.TranscriptionJob) (callhook.TranscriptionJob, error) {
	job.CallID = strings.TrimSpace(job.CallID)
	job.LastEventID = strings.TrimSpace(job.LastEventID)
	if job.CallID == "" || job.LastEventID == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}
	s.transcriptionByCallID[job.CallID] = job
	return job, nil
}

func (s *memoryStore) TranscriptionJobByCallID(_ context.Context, callID string) (callhook.TranscriptionJob, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}
	job, ok := s.transcriptionByCallID[callID]
	if !ok {
		return callhook.TranscriptionJob{}, callhook.ErrNotFound
	}
	return job, nil
}

func cloneRecordingJobs(src map[string]callhook.RecordingJob) map[string]callhook.RecordingJob {
	dst := make(map[string]callhook.RecordingJob, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneTranscriptionJobs(src map[string]callhook.TranscriptionJob) map[string]callhook.TranscriptionJob {
	dst := make(map[string]callhook.TranscriptionJob, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
