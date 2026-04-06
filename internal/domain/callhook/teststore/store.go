package teststore

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
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
	now := time.Now().UTC()
	job = normalizeRecordingJob(job, now)
	if strings.TrimSpace(job.CallID) == "" || strings.TrimSpace(job.LastEventID) == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}

	if existing, ok := s.recordingByCallID[job.CallID]; ok && !shouldReplaceRecording(existing, job) {
		return cloneRecordingJob(existing), nil
	}

	s.recordingByCallID[job.CallID] = cloneRecordingJob(job)
	return cloneRecordingJob(job), nil
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

	return cloneRecordingJob(job), nil
}

func (s *memoryStore) PendingRecordingJobs(_ context.Context, limit int) ([]callhook.RecordingJob, error) {
	if limit <= 0 {
		return nil, callhook.ErrInvalidInput
	}

	return s.collectPendingRecordingJobs(time.Now().UTC(), limit), nil
}

func (s *memoryStore) ClaimPendingRecordingJobs(
	_ context.Context,
	params callhook.ClaimJobsParams,
) ([]callhook.RecordingJob, error) {
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	if params.Limit <= 0 {
		return nil, nil
	}
	if params.Before.IsZero() || params.LeaseDuration <= 0 || params.LeaseToken == "" {
		return nil, callhook.ErrInvalidInput
	}

	jobs := s.collectPendingRecordingJobs(params.Before.UTC(), params.Limit)
	claimed := make([]callhook.RecordingJob, 0, len(jobs))
	for _, job := range jobs {
		stored := s.recordingByCallID[job.CallID]
		stored.LeaseToken = params.LeaseToken
		stored.LeaseExpiresAt = params.Before.UTC().Add(params.LeaseDuration)
		stored.UpdatedAt = params.Before.UTC()
		s.recordingByCallID[stored.CallID] = cloneRecordingJob(stored)
		claimed = append(claimed, cloneRecordingJob(stored))
	}

	return claimed, nil
}

func (s *memoryStore) CompleteRecordingJob(
	_ context.Context,
	params callhook.CompleteRecordingJobParams,
) (callhook.RecordingJob, error) {
	params.CallID = strings.TrimSpace(params.CallID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	params.OutputMediaID = strings.TrimSpace(params.OutputMediaID)
	if params.CallID == "" || params.LeaseToken == "" || params.OutputMediaID == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}

	now := params.CompletedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	job, err := s.recordingJobForLeaseAt(params.CallID, params.LeaseToken, now)
	if err != nil {
		return callhook.RecordingJob{}, err
	}

	job.Attempts++
	job.OutputMediaID = params.OutputMediaID
	job.LastAttemptAt = now
	job.LastError = ""
	job.NextAttemptAt = now
	job.LeaseToken = ""
	job.LeaseExpiresAt = time.Time{}
	job.UpdatedAt = now
	s.recordingByCallID[job.CallID] = cloneRecordingJob(job)

	return cloneRecordingJob(job), nil
}

func (s *memoryStore) RetryRecordingJob(_ context.Context, params callhook.RetryJobParams) (callhook.RecordingJob, error) {
	params.CallID = strings.TrimSpace(params.CallID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	params.LastError = strings.TrimSpace(params.LastError)
	if params.CallID == "" || params.LeaseToken == "" || params.LastError == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}

	now := params.AttemptedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	job, err := s.recordingJobForLeaseAt(params.CallID, params.LeaseToken, now)
	if err != nil {
		return callhook.RecordingJob{}, err
	}

	job.Attempts++
	job.LastAttemptAt = now
	job.LastError = params.LastError
	job.NextAttemptAt = params.RetryAt.UTC()
	if job.NextAttemptAt.IsZero() || job.NextAttemptAt.Before(now) {
		job.NextAttemptAt = now
	}
	job.LeaseToken = ""
	job.LeaseExpiresAt = time.Time{}
	job.UpdatedAt = now
	s.recordingByCallID[job.CallID] = cloneRecordingJob(job)

	return cloneRecordingJob(job), nil
}

func (s *memoryStore) SaveTranscriptionJob(_ context.Context, job callhook.TranscriptionJob) (callhook.TranscriptionJob, error) {
	now := time.Now().UTC()
	job = normalizeTranscriptionJob(job, now)
	if strings.TrimSpace(job.CallID) == "" || strings.TrimSpace(job.LastEventID) == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}

	if existing, ok := s.transcriptionByCallID[job.CallID]; ok && !shouldReplaceTranscription(existing, job) {
		return cloneTranscriptionJob(existing), nil
	}

	s.transcriptionByCallID[job.CallID] = cloneTranscriptionJob(job)
	return cloneTranscriptionJob(job), nil
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

	return cloneTranscriptionJob(job), nil
}

func (s *memoryStore) PendingTranscriptionJobs(_ context.Context, limit int) ([]callhook.TranscriptionJob, error) {
	if limit <= 0 {
		return nil, callhook.ErrInvalidInput
	}

	return s.collectPendingTranscriptionJobs(time.Now().UTC(), limit), nil
}

func (s *memoryStore) ClaimPendingTranscriptionJobs(
	_ context.Context,
	params callhook.ClaimJobsParams,
) ([]callhook.TranscriptionJob, error) {
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	if params.Limit <= 0 {
		return nil, nil
	}
	if params.Before.IsZero() || params.LeaseDuration <= 0 || params.LeaseToken == "" {
		return nil, callhook.ErrInvalidInput
	}

	jobs := s.collectPendingTranscriptionJobs(params.Before.UTC(), params.Limit)
	claimed := make([]callhook.TranscriptionJob, 0, len(jobs))
	for _, job := range jobs {
		stored := s.transcriptionByCallID[job.CallID]
		stored.LeaseToken = params.LeaseToken
		stored.LeaseExpiresAt = params.Before.UTC().Add(params.LeaseDuration)
		stored.UpdatedAt = params.Before.UTC()
		s.transcriptionByCallID[stored.CallID] = cloneTranscriptionJob(stored)
		claimed = append(claimed, cloneTranscriptionJob(stored))
	}

	return claimed, nil
}

func (s *memoryStore) CompleteTranscriptionJob(
	_ context.Context,
	params callhook.CompleteTranscriptionJobParams,
) (callhook.TranscriptionJob, error) {
	params.CallID = strings.TrimSpace(params.CallID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	params.TranscriptMediaID = strings.TrimSpace(params.TranscriptMediaID)
	if params.CallID == "" || params.LeaseToken == "" || params.TranscriptMediaID == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}

	now := params.CompletedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	job, err := s.transcriptionJobForLeaseAt(params.CallID, params.LeaseToken, now)
	if err != nil {
		return callhook.TranscriptionJob{}, err
	}

	job.Attempts++
	job.TranscriptMediaID = params.TranscriptMediaID
	job.LastAttemptAt = now
	job.LastError = ""
	job.NextAttemptAt = now
	job.LeaseToken = ""
	job.LeaseExpiresAt = time.Time{}
	job.UpdatedAt = now
	s.transcriptionByCallID[job.CallID] = cloneTranscriptionJob(job)

	return cloneTranscriptionJob(job), nil
}

func (s *memoryStore) RetryTranscriptionJob(
	_ context.Context,
	params callhook.RetryJobParams,
) (callhook.TranscriptionJob, error) {
	params.CallID = strings.TrimSpace(params.CallID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	params.LastError = strings.TrimSpace(params.LastError)
	if params.CallID == "" || params.LeaseToken == "" || params.LastError == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}

	now := params.AttemptedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	job, err := s.transcriptionJobForLeaseAt(params.CallID, params.LeaseToken, now)
	if err != nil {
		return callhook.TranscriptionJob{}, err
	}

	job.Attempts++
	job.LastAttemptAt = now
	job.LastError = params.LastError
	job.NextAttemptAt = params.RetryAt.UTC()
	if job.NextAttemptAt.IsZero() || job.NextAttemptAt.Before(now) {
		job.NextAttemptAt = now
	}
	job.LeaseToken = ""
	job.LeaseExpiresAt = time.Time{}
	job.UpdatedAt = now
	s.transcriptionByCallID[job.CallID] = cloneTranscriptionJob(job)

	return cloneTranscriptionJob(job), nil
}

func (s *memoryStore) collectPendingRecordingJobs(before time.Time, limit int) []callhook.RecordingJob {
	jobs := make([]callhook.RecordingJob, 0, limit)
	for _, job := range s.recordingByCallID {
		if !recordingJobDue(job, before) {
			continue
		}
		jobs = append(jobs, cloneRecordingJob(job))
	}

	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].NextAttemptAt.Equal(jobs[j].NextAttemptAt) {
			if jobs[i].UpdatedAt.Equal(jobs[j].UpdatedAt) {
				return jobs[i].CallID < jobs[j].CallID
			}
			return jobs[i].UpdatedAt.Before(jobs[j].UpdatedAt)
		}
		return jobs[i].NextAttemptAt.Before(jobs[j].NextAttemptAt)
	})
	if len(jobs) > limit {
		jobs = jobs[:limit]
	}

	return jobs
}

func (s *memoryStore) collectPendingTranscriptionJobs(before time.Time, limit int) []callhook.TranscriptionJob {
	jobs := make([]callhook.TranscriptionJob, 0, limit)
	for _, job := range s.transcriptionByCallID {
		if !transcriptionJobDue(job, before) {
			continue
		}
		jobs = append(jobs, cloneTranscriptionJob(job))
	}

	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].NextAttemptAt.Equal(jobs[j].NextAttemptAt) {
			if jobs[i].UpdatedAt.Equal(jobs[j].UpdatedAt) {
				return jobs[i].CallID < jobs[j].CallID
			}
			return jobs[i].UpdatedAt.Before(jobs[j].UpdatedAt)
		}
		return jobs[i].NextAttemptAt.Before(jobs[j].NextAttemptAt)
	})
	if len(jobs) > limit {
		jobs = jobs[:limit]
	}

	return jobs
}

func (s *memoryStore) recordingJobForLeaseAt(callID string, leaseToken string, now time.Time) (callhook.RecordingJob, error) {
	job, ok := s.recordingByCallID[callID]
	if !ok {
		return callhook.RecordingJob{}, callhook.ErrNotFound
	}
	if job.State != domaincall.RecordingStateInactive || job.StoppedAt.IsZero() || strings.TrimSpace(job.OutputMediaID) != "" {
		return callhook.RecordingJob{}, callhook.ErrConflict
	}
	if job.LeaseToken != leaseToken {
		return callhook.RecordingJob{}, callhook.ErrConflict
	}
	if job.LeaseExpiresAt.IsZero() || job.LeaseExpiresAt.Before(now) {
		return callhook.RecordingJob{}, callhook.ErrConflict
	}

	return cloneRecordingJob(job), nil
}

func (s *memoryStore) transcriptionJobForLeaseAt(callID string, leaseToken string, now time.Time) (callhook.TranscriptionJob, error) {
	job, ok := s.transcriptionByCallID[callID]
	if !ok {
		return callhook.TranscriptionJob{}, callhook.ErrNotFound
	}
	if job.State != domaincall.TranscriptionStateInactive || job.StoppedAt.IsZero() || strings.TrimSpace(job.TranscriptMediaID) != "" {
		return callhook.TranscriptionJob{}, callhook.ErrConflict
	}
	if job.LeaseToken != leaseToken {
		return callhook.TranscriptionJob{}, callhook.ErrConflict
	}
	if job.LeaseExpiresAt.IsZero() || job.LeaseExpiresAt.Before(now) {
		return callhook.TranscriptionJob{}, callhook.ErrConflict
	}

	return cloneTranscriptionJob(job), nil
}

func normalizeRecordingJob(job callhook.RecordingJob, now time.Time) callhook.RecordingJob {
	job.OwnerAccountID = strings.TrimSpace(job.OwnerAccountID)
	job.ConversationID = strings.TrimSpace(job.ConversationID)
	job.CallID = strings.TrimSpace(job.CallID)
	job.LastEventID = strings.TrimSpace(job.LastEventID)
	job.OutputMediaID = strings.TrimSpace(job.OutputMediaID)
	job.LeaseToken = strings.TrimSpace(job.LeaseToken)
	job.LastError = strings.TrimSpace(job.LastError)
	if job.NextAttemptAt.IsZero() {
		job.NextAttemptAt = now
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = now
	}

	job.NextAttemptAt = job.NextAttemptAt.UTC()
	job.LeaseExpiresAt = job.LeaseExpiresAt.UTC()
	job.LastAttemptAt = job.LastAttemptAt.UTC()
	job.StartedAt = job.StartedAt.UTC()
	job.StoppedAt = job.StoppedAt.UTC()
	job.UpdatedAt = job.UpdatedAt.UTC()

	return job
}

func normalizeTranscriptionJob(job callhook.TranscriptionJob, now time.Time) callhook.TranscriptionJob {
	job.OwnerAccountID = strings.TrimSpace(job.OwnerAccountID)
	job.ConversationID = strings.TrimSpace(job.ConversationID)
	job.CallID = strings.TrimSpace(job.CallID)
	job.LastEventID = strings.TrimSpace(job.LastEventID)
	job.TranscriptMediaID = strings.TrimSpace(job.TranscriptMediaID)
	job.LeaseToken = strings.TrimSpace(job.LeaseToken)
	job.LastError = strings.TrimSpace(job.LastError)
	if job.NextAttemptAt.IsZero() {
		job.NextAttemptAt = now
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = now
	}

	job.NextAttemptAt = job.NextAttemptAt.UTC()
	job.LeaseExpiresAt = job.LeaseExpiresAt.UTC()
	job.LastAttemptAt = job.LastAttemptAt.UTC()
	job.StartedAt = job.StartedAt.UTC()
	job.StoppedAt = job.StoppedAt.UTC()
	job.UpdatedAt = job.UpdatedAt.UTC()

	return job
}

func shouldReplaceRecording(existing callhook.RecordingJob, next callhook.RecordingJob) bool {
	return next.LastEventSequence > existing.LastEventSequence ||
		(next.LastEventSequence == existing.LastEventSequence && next.Attempts > existing.Attempts)
}

func shouldReplaceTranscription(existing callhook.TranscriptionJob, next callhook.TranscriptionJob) bool {
	return next.LastEventSequence > existing.LastEventSequence ||
		(next.LastEventSequence == existing.LastEventSequence && next.Attempts > existing.Attempts)
}

func recordingJobDue(job callhook.RecordingJob, before time.Time) bool {
	if strings.TrimSpace(job.OwnerAccountID) == "" || strings.TrimSpace(job.ConversationID) == "" {
		return false
	}
	if job.State != domaincall.RecordingStateInactive || job.StoppedAt.IsZero() || strings.TrimSpace(job.OutputMediaID) != "" {
		return false
	}
	if job.NextAttemptAt.After(before) {
		return false
	}
	if !job.LeaseExpiresAt.IsZero() && job.LeaseExpiresAt.After(before) {
		return false
	}

	return true
}

func transcriptionJobDue(job callhook.TranscriptionJob, before time.Time) bool {
	if strings.TrimSpace(job.OwnerAccountID) == "" || strings.TrimSpace(job.ConversationID) == "" {
		return false
	}
	if job.State != domaincall.TranscriptionStateInactive || job.StoppedAt.IsZero() || strings.TrimSpace(job.TranscriptMediaID) != "" {
		return false
	}
	if job.NextAttemptAt.After(before) {
		return false
	}
	if !job.LeaseExpiresAt.IsZero() && job.LeaseExpiresAt.After(before) {
		return false
	}

	return true
}

func cloneRecordingJobs(src map[string]callhook.RecordingJob) map[string]callhook.RecordingJob {
	dst := make(map[string]callhook.RecordingJob, len(src))
	for key, value := range src {
		dst[key] = cloneRecordingJob(value)
	}

	return dst
}

func cloneTranscriptionJobs(src map[string]callhook.TranscriptionJob) map[string]callhook.TranscriptionJob {
	dst := make(map[string]callhook.TranscriptionJob, len(src))
	for key, value := range src {
		dst[key] = cloneTranscriptionJob(value)
	}

	return dst
}

func cloneRecordingJob(job callhook.RecordingJob) callhook.RecordingJob {
	return job
}

func cloneTranscriptionJob(job callhook.TranscriptionJob) callhook.TranscriptionJob {
	return job
}
