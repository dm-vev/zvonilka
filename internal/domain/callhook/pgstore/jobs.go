package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/callhook"
)

const recordingColumnList = `
owner_account_id, conversation_id, call_id, last_event_id, last_event_sequence, state, output_media_id,
attempts, next_attempt_at, lease_token, lease_expires_at, last_attempt_at, last_error, started_at, stopped_at,
updated_at`

const transcriptionColumnList = `
owner_account_id, conversation_id, call_id, last_event_id, last_event_sequence, state, transcript_media_id,
attempts, next_attempt_at, lease_token, lease_expires_at, last_attempt_at, last_error, started_at, stopped_at,
updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Store) SaveRecordingJob(ctx context.Context, job callhook.RecordingJob) (callhook.RecordingJob, error) {
	if err := s.requireStore(); err != nil {
		return callhook.RecordingJob{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return callhook.RecordingJob{}, err
	}
	if s.tx != nil {
		return s.saveRecordingJob(ctx, job)
	}

	var saved callhook.RecordingJob
	err := s.WithinTx(ctx, func(tx callhook.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveRecordingJob(ctx, job)
		return saveErr
	})
	if err != nil {
		return callhook.RecordingJob{}, err
	}

	return saved, nil
}

func (s *Store) saveRecordingJob(ctx context.Context, job callhook.RecordingJob) (callhook.RecordingJob, error) {
	now := time.Now().UTC()
	job.OwnerAccountID = strings.TrimSpace(job.OwnerAccountID)
	job.ConversationID = strings.TrimSpace(job.ConversationID)
	job.CallID = strings.TrimSpace(job.CallID)
	job.LastEventID = strings.TrimSpace(job.LastEventID)
	job.OutputMediaID = strings.TrimSpace(job.OutputMediaID)
	job.LeaseToken = strings.TrimSpace(job.LeaseToken)
	job.LastError = strings.TrimSpace(job.LastError)
	if job.CallID == "" || job.LastEventID == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}
	if job.NextAttemptAt.IsZero() {
		job.NextAttemptAt = now
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = now
	}

	lastEventSequence, err := encodeSequence(job.LastEventSequence)
	if err != nil {
		return callhook.RecordingJob{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s AS existing (
	owner_account_id,
	conversation_id,
	call_id,
	last_event_id,
	last_event_sequence,
	state,
	output_media_id,
	attempts,
	next_attempt_at,
	lease_token,
	lease_expires_at,
	last_attempt_at,
	last_error,
	started_at,
	stopped_at,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
ON CONFLICT (call_id) DO UPDATE SET
	owner_account_id = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN COALESCE(NULLIF(EXCLUDED.owner_account_id, ''), existing.owner_account_id)
		ELSE existing.owner_account_id
	END,
	conversation_id = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN COALESCE(NULLIF(EXCLUDED.conversation_id, ''), existing.conversation_id)
		ELSE existing.conversation_id
	END,
	last_event_id = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.last_event_id
		ELSE existing.last_event_id
	END,
	last_event_sequence = GREATEST(existing.last_event_sequence, EXCLUDED.last_event_sequence),
	state = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.state
		ELSE existing.state
	END,
	output_media_id = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.output_media_id
		ELSE existing.output_media_id
	END,
	attempts = GREATEST(existing.attempts, EXCLUDED.attempts),
	next_attempt_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.next_attempt_at
		ELSE existing.next_attempt_at
	END,
	lease_token = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.lease_token
		ELSE existing.lease_token
	END,
	lease_expires_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.lease_expires_at
		ELSE existing.lease_expires_at
	END,
	last_attempt_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.last_attempt_at
		ELSE existing.last_attempt_at
	END,
	last_error = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.last_error
		ELSE existing.last_error
	END,
	started_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.started_at
		ELSE existing.started_at
	END,
	stopped_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.stopped_at
		ELSE existing.stopped_at
	END,
	updated_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.updated_at
		ELSE existing.updated_at
	END
RETURNING `+recordingColumnList+`
`, s.table("call_recording_jobs"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		nullString(job.OwnerAccountID),
		nullString(job.ConversationID),
		job.CallID,
		job.LastEventID,
		lastEventSequence,
		nullString(string(job.State)),
		nullString(job.OutputMediaID),
		job.Attempts,
		job.NextAttemptAt.UTC(),
		nullString(job.LeaseToken),
		nullTime(job.LeaseExpiresAt),
		nullTime(job.LastAttemptAt),
		job.LastError,
		nullTime(job.StartedAt),
		nullTime(job.StoppedAt),
		job.UpdatedAt.UTC(),
	)

	saved, err := scanRecordingJob(row)
	if err != nil {
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return callhook.RecordingJob{}, mappedErr
		}

		return callhook.RecordingJob{}, fmt.Errorf("save recording job %s: %w", job.CallID, err)
	}

	return saved, nil
}

func (s *Store) RecordingJobByCallID(ctx context.Context, callID string) (callhook.RecordingJob, error) {
	if err := s.requireStore(); err != nil {
		return callhook.RecordingJob{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return callhook.RecordingJob{}, err
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT `+recordingColumnList+`
FROM %s
WHERE call_id = $1
`, s.table("call_recording_jobs"))
	row := s.conn().QueryRowContext(ctx, query, callID)
	job, err := scanRecordingJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return callhook.RecordingJob{}, callhook.ErrNotFound
		}

		return callhook.RecordingJob{}, fmt.Errorf("load recording job %s: %w", callID, err)
	}

	return job, nil
}

func (s *Store) PendingRecordingJobs(ctx context.Context, limit int) ([]callhook.RecordingJob, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, callhook.ErrInvalidInput
	}

	before := time.Now().UTC()
	query := fmt.Sprintf(`
SELECT `+recordingColumnList+`
FROM %s
WHERE state = 'inactive'
	AND stopped_at IS NOT NULL
	AND COALESCE(output_media_id, '') = ''
	AND COALESCE(owner_account_id, '') <> ''
	AND COALESCE(conversation_id, '') <> ''
	AND next_attempt_at <= $1
	AND (lease_expires_at IS NULL OR lease_expires_at <= $1)
ORDER BY next_attempt_at ASC, updated_at ASC, call_id ASC
LIMIT $2
`, s.table("call_recording_jobs"))
	rows, err := s.conn().QueryContext(ctx, query, before, limit)
	if err != nil {
		return nil, fmt.Errorf("load pending recording jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]callhook.RecordingJob, 0, limit)
	for rows.Next() {
		job, scanErr := scanRecordingJob(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan pending recording job: %w", scanErr)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending recording jobs: %w", err)
	}

	return jobs, nil
}

func (s *Store) ClaimPendingRecordingJobs(
	ctx context.Context,
	params callhook.ClaimJobsParams,
) ([]callhook.RecordingJob, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	if params.Limit <= 0 {
		return nil, nil
	}
	if params.Before.IsZero() || params.LeaseDuration <= 0 || params.LeaseToken == "" {
		return nil, callhook.ErrInvalidInput
	}

	leaseUntil := params.Before.UTC().Add(params.LeaseDuration)
	query := fmt.Sprintf(`
WITH due AS (
	SELECT call_id
	FROM %s
	WHERE state = 'inactive'
		AND stopped_at IS NOT NULL
		AND COALESCE(output_media_id, '') = ''
		AND COALESCE(owner_account_id, '') <> ''
		AND COALESCE(conversation_id, '') <> ''
		AND next_attempt_at <= $1
		AND (lease_expires_at IS NULL OR lease_expires_at <= $1)
	ORDER BY next_attempt_at ASC, updated_at ASC, call_id ASC
	LIMIT $2
	FOR UPDATE SKIP LOCKED
)
UPDATE %s AS jobs
SET lease_token = $3,
	lease_expires_at = $4,
	updated_at = $1
FROM due
WHERE jobs.call_id = due.call_id
RETURNING `+recordingColumnList+`
`, s.table("call_recording_jobs"), s.table("call_recording_jobs"))
	rows, err := s.conn().QueryContext(ctx, query, params.Before.UTC(), params.Limit, params.LeaseToken, leaseUntil)
	if err != nil {
		return nil, fmt.Errorf("claim recording jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]callhook.RecordingJob, 0, params.Limit)
	for rows.Next() {
		job, scanErr := scanRecordingJob(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan claimed recording job: %w", scanErr)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed recording jobs: %w", err)
	}

	return jobs, nil
}

func (s *Store) CompleteRecordingJob(
	ctx context.Context,
	params callhook.CompleteRecordingJobParams,
) (callhook.RecordingJob, error) {
	if err := s.requireStore(); err != nil {
		return callhook.RecordingJob{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return callhook.RecordingJob{}, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	params.OutputMediaID = strings.TrimSpace(params.OutputMediaID)
	if params.CallID == "" || params.LeaseToken == "" || params.OutputMediaID == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}

	var saved callhook.RecordingJob
	err := s.WithinTx(ctx, func(tx callhook.Store) error {
		txStore := tx.(*Store)
		job, loadErr := txStore.recordingJobByCallIDForUpdate(ctx, params.CallID)
		if loadErr != nil {
			return loadErr
		}

		now := params.CompletedAt.UTC()
		if now.IsZero() {
			now = time.Now().UTC()
		}
		if leaseErr := validateRecordingLease(job, params.LeaseToken, now); leaseErr != nil {
			return leaseErr
		}

		job.Attempts++
		job.OutputMediaID = params.OutputMediaID
		job.LastAttemptAt = now
		job.LastError = ""
		job.NextAttemptAt = now
		job.LeaseToken = ""
		job.LeaseExpiresAt = time.Time{}
		job.UpdatedAt = now

		var saveErr error
		saved, saveErr = txStore.saveRecordingJob(ctx, job)
		return saveErr
	})
	if err != nil {
		return callhook.RecordingJob{}, err
	}

	return saved, nil
}

func (s *Store) RetryRecordingJob(ctx context.Context, params callhook.RetryJobParams) (callhook.RecordingJob, error) {
	if err := s.requireStore(); err != nil {
		return callhook.RecordingJob{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return callhook.RecordingJob{}, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	params.LastError = strings.TrimSpace(params.LastError)
	if params.CallID == "" || params.LeaseToken == "" || params.LastError == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}

	var saved callhook.RecordingJob
	err := s.WithinTx(ctx, func(tx callhook.Store) error {
		txStore := tx.(*Store)
		job, loadErr := txStore.recordingJobByCallIDForUpdate(ctx, params.CallID)
		if loadErr != nil {
			return loadErr
		}

		now := params.AttemptedAt.UTC()
		if now.IsZero() {
			now = time.Now().UTC()
		}
		if leaseErr := validateRecordingLease(job, params.LeaseToken, now); leaseErr != nil {
			return leaseErr
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

		var saveErr error
		saved, saveErr = txStore.saveRecordingJob(ctx, job)
		return saveErr
	})
	if err != nil {
		return callhook.RecordingJob{}, err
	}

	return saved, nil
}

func (s *Store) SaveTranscriptionJob(ctx context.Context, job callhook.TranscriptionJob) (callhook.TranscriptionJob, error) {
	if err := s.requireStore(); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	if s.tx != nil {
		return s.saveTranscriptionJob(ctx, job)
	}

	var saved callhook.TranscriptionJob
	err := s.WithinTx(ctx, func(tx callhook.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveTranscriptionJob(ctx, job)
		return saveErr
	})
	if err != nil {
		return callhook.TranscriptionJob{}, err
	}

	return saved, nil
}

func (s *Store) saveTranscriptionJob(ctx context.Context, job callhook.TranscriptionJob) (callhook.TranscriptionJob, error) {
	now := time.Now().UTC()
	job.OwnerAccountID = strings.TrimSpace(job.OwnerAccountID)
	job.ConversationID = strings.TrimSpace(job.ConversationID)
	job.CallID = strings.TrimSpace(job.CallID)
	job.LastEventID = strings.TrimSpace(job.LastEventID)
	job.TranscriptMediaID = strings.TrimSpace(job.TranscriptMediaID)
	job.LeaseToken = strings.TrimSpace(job.LeaseToken)
	job.LastError = strings.TrimSpace(job.LastError)
	if job.CallID == "" || job.LastEventID == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}
	if job.NextAttemptAt.IsZero() {
		job.NextAttemptAt = now
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = now
	}

	lastEventSequence, err := encodeSequence(job.LastEventSequence)
	if err != nil {
		return callhook.TranscriptionJob{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s AS existing (
	owner_account_id,
	conversation_id,
	call_id,
	last_event_id,
	last_event_sequence,
	state,
	transcript_media_id,
	attempts,
	next_attempt_at,
	lease_token,
	lease_expires_at,
	last_attempt_at,
	last_error,
	started_at,
	stopped_at,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
ON CONFLICT (call_id) DO UPDATE SET
	owner_account_id = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN COALESCE(NULLIF(EXCLUDED.owner_account_id, ''), existing.owner_account_id)
		ELSE existing.owner_account_id
	END,
	conversation_id = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN COALESCE(NULLIF(EXCLUDED.conversation_id, ''), existing.conversation_id)
		ELSE existing.conversation_id
	END,
	last_event_id = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.last_event_id
		ELSE existing.last_event_id
	END,
	last_event_sequence = GREATEST(existing.last_event_sequence, EXCLUDED.last_event_sequence),
	state = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.state
		ELSE existing.state
	END,
	transcript_media_id = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.transcript_media_id
		ELSE existing.transcript_media_id
	END,
	attempts = GREATEST(existing.attempts, EXCLUDED.attempts),
	next_attempt_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.next_attempt_at
		ELSE existing.next_attempt_at
	END,
	lease_token = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.lease_token
		ELSE existing.lease_token
	END,
	lease_expires_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.lease_expires_at
		ELSE existing.lease_expires_at
	END,
	last_attempt_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.last_attempt_at
		ELSE existing.last_attempt_at
	END,
	last_error = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.last_error
		ELSE existing.last_error
	END,
	started_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.started_at
		ELSE existing.started_at
	END,
	stopped_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.stopped_at
		ELSE existing.stopped_at
	END,
	updated_at = CASE
		WHEN EXCLUDED.last_event_sequence > existing.last_event_sequence
			OR (EXCLUDED.last_event_sequence = existing.last_event_sequence AND EXCLUDED.attempts > existing.attempts)
			THEN EXCLUDED.updated_at
		ELSE existing.updated_at
	END
RETURNING `+transcriptionColumnList+`
`, s.table("call_transcription_jobs"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		nullString(job.OwnerAccountID),
		nullString(job.ConversationID),
		job.CallID,
		job.LastEventID,
		lastEventSequence,
		nullString(string(job.State)),
		nullString(job.TranscriptMediaID),
		job.Attempts,
		job.NextAttemptAt.UTC(),
		nullString(job.LeaseToken),
		nullTime(job.LeaseExpiresAt),
		nullTime(job.LastAttemptAt),
		job.LastError,
		nullTime(job.StartedAt),
		nullTime(job.StoppedAt),
		job.UpdatedAt.UTC(),
	)

	saved, err := scanTranscriptionJob(row)
	if err != nil {
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return callhook.TranscriptionJob{}, mappedErr
		}

		return callhook.TranscriptionJob{}, fmt.Errorf("save transcription job %s: %w", job.CallID, err)
	}

	return saved, nil
}

func (s *Store) TranscriptionJobByCallID(ctx context.Context, callID string) (callhook.TranscriptionJob, error) {
	if err := s.requireStore(); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT `+transcriptionColumnList+`
FROM %s
WHERE call_id = $1
`, s.table("call_transcription_jobs"))
	row := s.conn().QueryRowContext(ctx, query, callID)
	job, err := scanTranscriptionJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return callhook.TranscriptionJob{}, callhook.ErrNotFound
		}

		return callhook.TranscriptionJob{}, fmt.Errorf("load transcription job %s: %w", callID, err)
	}

	return job, nil
}

func (s *Store) PendingTranscriptionJobs(ctx context.Context, limit int) ([]callhook.TranscriptionJob, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, callhook.ErrInvalidInput
	}

	before := time.Now().UTC()
	query := fmt.Sprintf(`
SELECT `+transcriptionColumnList+`
FROM %s
WHERE state = 'inactive'
	AND stopped_at IS NOT NULL
	AND COALESCE(transcript_media_id, '') = ''
	AND COALESCE(owner_account_id, '') <> ''
	AND COALESCE(conversation_id, '') <> ''
	AND next_attempt_at <= $1
	AND (lease_expires_at IS NULL OR lease_expires_at <= $1)
ORDER BY next_attempt_at ASC, updated_at ASC, call_id ASC
LIMIT $2
`, s.table("call_transcription_jobs"))
	rows, err := s.conn().QueryContext(ctx, query, before, limit)
	if err != nil {
		return nil, fmt.Errorf("load pending transcription jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]callhook.TranscriptionJob, 0, limit)
	for rows.Next() {
		job, scanErr := scanTranscriptionJob(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan pending transcription job: %w", scanErr)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending transcription jobs: %w", err)
	}

	return jobs, nil
}

func (s *Store) ClaimPendingTranscriptionJobs(
	ctx context.Context,
	params callhook.ClaimJobsParams,
) ([]callhook.TranscriptionJob, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	if params.Limit <= 0 {
		return nil, nil
	}
	if params.Before.IsZero() || params.LeaseDuration <= 0 || params.LeaseToken == "" {
		return nil, callhook.ErrInvalidInput
	}

	leaseUntil := params.Before.UTC().Add(params.LeaseDuration)
	query := fmt.Sprintf(`
WITH due AS (
	SELECT call_id
	FROM %s
	WHERE state = 'inactive'
		AND stopped_at IS NOT NULL
		AND COALESCE(transcript_media_id, '') = ''
		AND COALESCE(owner_account_id, '') <> ''
		AND COALESCE(conversation_id, '') <> ''
		AND next_attempt_at <= $1
		AND (lease_expires_at IS NULL OR lease_expires_at <= $1)
	ORDER BY next_attempt_at ASC, updated_at ASC, call_id ASC
	LIMIT $2
	FOR UPDATE SKIP LOCKED
)
UPDATE %s AS jobs
SET lease_token = $3,
	lease_expires_at = $4,
	updated_at = $1
FROM due
WHERE jobs.call_id = due.call_id
RETURNING `+transcriptionColumnList+`
`, s.table("call_transcription_jobs"), s.table("call_transcription_jobs"))
	rows, err := s.conn().QueryContext(ctx, query, params.Before.UTC(), params.Limit, params.LeaseToken, leaseUntil)
	if err != nil {
		return nil, fmt.Errorf("claim transcription jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]callhook.TranscriptionJob, 0, params.Limit)
	for rows.Next() {
		job, scanErr := scanTranscriptionJob(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan claimed transcription job: %w", scanErr)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed transcription jobs: %w", err)
	}

	return jobs, nil
}

func (s *Store) CompleteTranscriptionJob(
	ctx context.Context,
	params callhook.CompleteTranscriptionJobParams,
) (callhook.TranscriptionJob, error) {
	if err := s.requireStore(); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	params.TranscriptMediaID = strings.TrimSpace(params.TranscriptMediaID)
	if params.CallID == "" || params.LeaseToken == "" || params.TranscriptMediaID == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}

	var saved callhook.TranscriptionJob
	err := s.WithinTx(ctx, func(tx callhook.Store) error {
		txStore := tx.(*Store)
		job, loadErr := txStore.transcriptionJobByCallIDForUpdate(ctx, params.CallID)
		if loadErr != nil {
			return loadErr
		}

		now := params.CompletedAt.UTC()
		if now.IsZero() {
			now = time.Now().UTC()
		}
		if leaseErr := validateTranscriptionLease(job, params.LeaseToken, now); leaseErr != nil {
			return leaseErr
		}

		job.Attempts++
		job.TranscriptMediaID = params.TranscriptMediaID
		job.LastAttemptAt = now
		job.LastError = ""
		job.NextAttemptAt = now
		job.LeaseToken = ""
		job.LeaseExpiresAt = time.Time{}
		job.UpdatedAt = now

		var saveErr error
		saved, saveErr = txStore.saveTranscriptionJob(ctx, job)
		return saveErr
	})
	if err != nil {
		return callhook.TranscriptionJob{}, err
	}

	return saved, nil
}

func (s *Store) RetryTranscriptionJob(
	ctx context.Context,
	params callhook.RetryJobParams,
) (callhook.TranscriptionJob, error) {
	if err := s.requireStore(); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	params.CallID = strings.TrimSpace(params.CallID)
	params.LeaseToken = strings.TrimSpace(params.LeaseToken)
	params.LastError = strings.TrimSpace(params.LastError)
	if params.CallID == "" || params.LeaseToken == "" || params.LastError == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}

	var saved callhook.TranscriptionJob
	err := s.WithinTx(ctx, func(tx callhook.Store) error {
		txStore := tx.(*Store)
		job, loadErr := txStore.transcriptionJobByCallIDForUpdate(ctx, params.CallID)
		if loadErr != nil {
			return loadErr
		}

		now := params.AttemptedAt.UTC()
		if now.IsZero() {
			now = time.Now().UTC()
		}
		if leaseErr := validateTranscriptionLease(job, params.LeaseToken, now); leaseErr != nil {
			return leaseErr
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

		var saveErr error
		saved, saveErr = txStore.saveTranscriptionJob(ctx, job)
		return saveErr
	})
	if err != nil {
		return callhook.TranscriptionJob{}, err
	}

	return saved, nil
}

func (s *Store) recordingJobByCallIDForUpdate(ctx context.Context, callID string) (callhook.RecordingJob, error) {
	query := fmt.Sprintf(`
SELECT `+recordingColumnList+`
FROM %s
WHERE call_id = $1
FOR UPDATE
`, s.table("call_recording_jobs"))

	row := s.conn().QueryRowContext(ctx, query, callID)
	job, err := scanRecordingJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return callhook.RecordingJob{}, callhook.ErrNotFound
		}

		return callhook.RecordingJob{}, fmt.Errorf("load recording job %s for update: %w", callID, err)
	}

	return job, nil
}

func (s *Store) transcriptionJobByCallIDForUpdate(ctx context.Context, callID string) (callhook.TranscriptionJob, error) {
	query := fmt.Sprintf(`
SELECT `+transcriptionColumnList+`
FROM %s
WHERE call_id = $1
FOR UPDATE
`, s.table("call_transcription_jobs"))

	row := s.conn().QueryRowContext(ctx, query, callID)
	job, err := scanTranscriptionJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return callhook.TranscriptionJob{}, callhook.ErrNotFound
		}

		return callhook.TranscriptionJob{}, fmt.Errorf("load transcription job %s for update: %w", callID, err)
	}

	return job, nil
}

func validateRecordingLease(job callhook.RecordingJob, leaseToken string, now time.Time) error {
	if job.State != domaincall.RecordingStateInactive || job.StoppedAt.IsZero() || strings.TrimSpace(job.OutputMediaID) != "" {
		return callhook.ErrConflict
	}
	if strings.TrimSpace(leaseToken) == "" {
		return callhook.ErrInvalidInput
	}
	if job.LeaseToken != leaseToken {
		return callhook.ErrConflict
	}
	if job.LeaseExpiresAt.IsZero() || job.LeaseExpiresAt.Before(now) {
		return callhook.ErrConflict
	}

	return nil
}

func validateTranscriptionLease(job callhook.TranscriptionJob, leaseToken string, now time.Time) error {
	if job.State != domaincall.TranscriptionStateInactive || job.StoppedAt.IsZero() || strings.TrimSpace(job.TranscriptMediaID) != "" {
		return callhook.ErrConflict
	}
	if strings.TrimSpace(leaseToken) == "" {
		return callhook.ErrInvalidInput
	}
	if job.LeaseToken != leaseToken {
		return callhook.ErrConflict
	}
	if job.LeaseExpiresAt.IsZero() || job.LeaseExpiresAt.Before(now) {
		return callhook.ErrConflict
	}

	return nil
}

func nullTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func decodeTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return value.Time.UTC()
}

func nullString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	return value
}

func encodeSequence(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, callhook.ErrInvalidInput
	}

	return int64(value), nil
}

func scanRecordingJob(scanner rowScanner) (callhook.RecordingJob, error) {
	var (
		job               callhook.RecordingJob
		ownerAccountID    sql.NullString
		conversationID    sql.NullString
		lastEventSequence int64
		state             sql.NullString
		outputID          sql.NullString
		nextAttemptAt     time.Time
		leaseToken        sql.NullString
		leaseExpiresAt    sql.NullTime
		lastAttemptAt     sql.NullTime
		startedAt         sql.NullTime
		stoppedAt         sql.NullTime
	)
	if err := scanner.Scan(
		&ownerAccountID,
		&conversationID,
		&job.CallID,
		&job.LastEventID,
		&lastEventSequence,
		&state,
		&outputID,
		&job.Attempts,
		&nextAttemptAt,
		&leaseToken,
		&leaseExpiresAt,
		&lastAttemptAt,
		&job.LastError,
		&startedAt,
		&stoppedAt,
		&job.UpdatedAt,
	); err != nil {
		return callhook.RecordingJob{}, err
	}

	job.OwnerAccountID = ownerAccountID.String
	job.ConversationID = conversationID.String
	job.LastEventSequence = uint64(lastEventSequence)
	job.State = domaincall.RecordingState(state.String)
	job.OutputMediaID = outputID.String
	job.NextAttemptAt = nextAttemptAt.UTC()
	job.LeaseToken = leaseToken.String
	job.LeaseExpiresAt = decodeTime(leaseExpiresAt)
	job.LastAttemptAt = decodeTime(lastAttemptAt)
	job.StartedAt = decodeTime(startedAt)
	job.StoppedAt = decodeTime(stoppedAt)
	job.UpdatedAt = job.UpdatedAt.UTC()

	return job, nil
}

func scanTranscriptionJob(scanner rowScanner) (callhook.TranscriptionJob, error) {
	var (
		job               callhook.TranscriptionJob
		ownerAccountID    sql.NullString
		conversationID    sql.NullString
		lastEventSequence int64
		state             sql.NullString
		mediaID           sql.NullString
		nextAttemptAt     time.Time
		leaseToken        sql.NullString
		leaseExpiresAt    sql.NullTime
		lastAttemptAt     sql.NullTime
		startedAt         sql.NullTime
		stoppedAt         sql.NullTime
	)
	if err := scanner.Scan(
		&ownerAccountID,
		&conversationID,
		&job.CallID,
		&job.LastEventID,
		&lastEventSequence,
		&state,
		&mediaID,
		&job.Attempts,
		&nextAttemptAt,
		&leaseToken,
		&leaseExpiresAt,
		&lastAttemptAt,
		&job.LastError,
		&startedAt,
		&stoppedAt,
		&job.UpdatedAt,
	); err != nil {
		return callhook.TranscriptionJob{}, err
	}

	job.OwnerAccountID = ownerAccountID.String
	job.ConversationID = conversationID.String
	job.LastEventSequence = uint64(lastEventSequence)
	job.State = domaincall.TranscriptionState(state.String)
	job.TranscriptMediaID = mediaID.String
	job.NextAttemptAt = nextAttemptAt.UTC()
	job.LeaseToken = leaseToken.String
	job.LeaseExpiresAt = decodeTime(leaseExpiresAt)
	job.LastAttemptAt = decodeTime(lastAttemptAt)
	job.StartedAt = decodeTime(startedAt)
	job.StoppedAt = decodeTime(stoppedAt)
	job.UpdatedAt = job.UpdatedAt.UTC()

	return job, nil
}
