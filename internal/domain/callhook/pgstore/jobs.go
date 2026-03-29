package pgstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/callhook"
)

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
	job.OwnerAccountID = strings.TrimSpace(job.OwnerAccountID)
	job.ConversationID = strings.TrimSpace(job.ConversationID)
	job.CallID = strings.TrimSpace(job.CallID)
	job.LastEventID = strings.TrimSpace(job.LastEventID)
	job.OutputMediaID = strings.TrimSpace(job.OutputMediaID)
	if job.CallID == "" || job.LastEventID == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	owner_account_id, conversation_id, call_id, last_event_id, state, output_media_id, started_at, stopped_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (call_id) DO UPDATE SET
	owner_account_id = COALESCE(NULLIF(EXCLUDED.owner_account_id, ''), %s.owner_account_id),
	conversation_id = COALESCE(NULLIF(EXCLUDED.conversation_id, ''), %s.conversation_id),
	last_event_id = EXCLUDED.last_event_id,
	state = EXCLUDED.state,
	output_media_id = EXCLUDED.output_media_id,
	started_at = EXCLUDED.started_at,
	stopped_at = EXCLUDED.stopped_at,
	updated_at = EXCLUDED.updated_at
RETURNING owner_account_id, conversation_id, call_id, last_event_id, state, output_media_id, started_at, stopped_at, updated_at
`, s.table("call_recording_jobs"), s.table("call_recording_jobs"), s.table("call_recording_jobs"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		nullString(job.OwnerAccountID),
		nullString(job.ConversationID),
		job.CallID,
		job.LastEventID,
		nullString(string(job.State)),
		nullString(job.OutputMediaID),
		nullTime(job.StartedAt),
		nullTime(job.StoppedAt),
		job.UpdatedAt.UTC(),
	)
	return scanRecordingJob(row)
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
SELECT owner_account_id, conversation_id, call_id, last_event_id, state, output_media_id, started_at, stopped_at, updated_at
FROM %s
WHERE call_id = $1
`, s.table("call_recording_jobs"))
	row := s.conn().QueryRowContext(ctx, query, callID)
	job, err := scanRecordingJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
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

	query := fmt.Sprintf(`
SELECT owner_account_id, conversation_id, call_id, last_event_id, state, output_media_id, started_at, stopped_at, updated_at
FROM %s
WHERE state = 'inactive'
  AND stopped_at IS NOT NULL
  AND COALESCE(output_media_id, '') = ''
  AND COALESCE(owner_account_id, '') <> ''
  AND COALESCE(conversation_id, '') <> ''
ORDER BY updated_at ASC, call_id ASC
LIMIT $1
`, s.table("call_recording_jobs"))
	rows, err := s.conn().QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("load pending recording jobs: %w", err)
	}
	defer rows.Close()

	var jobs []callhook.RecordingJob
	for rows.Next() {
		job, scanErr := scanRecordingJobRows(rows)
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
	job.OwnerAccountID = strings.TrimSpace(job.OwnerAccountID)
	job.ConversationID = strings.TrimSpace(job.ConversationID)
	job.CallID = strings.TrimSpace(job.CallID)
	job.LastEventID = strings.TrimSpace(job.LastEventID)
	job.TranscriptMediaID = strings.TrimSpace(job.TranscriptMediaID)
	if job.CallID == "" || job.LastEventID == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	owner_account_id, conversation_id, call_id, last_event_id, state, transcript_media_id, started_at, stopped_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (call_id) DO UPDATE SET
	owner_account_id = COALESCE(NULLIF(EXCLUDED.owner_account_id, ''), %s.owner_account_id),
	conversation_id = COALESCE(NULLIF(EXCLUDED.conversation_id, ''), %s.conversation_id),
	last_event_id = EXCLUDED.last_event_id,
	state = EXCLUDED.state,
	transcript_media_id = EXCLUDED.transcript_media_id,
	started_at = EXCLUDED.started_at,
	stopped_at = EXCLUDED.stopped_at,
	updated_at = EXCLUDED.updated_at
RETURNING owner_account_id, conversation_id, call_id, last_event_id, state, transcript_media_id, started_at, stopped_at, updated_at
`, s.table("call_transcription_jobs"), s.table("call_transcription_jobs"), s.table("call_transcription_jobs"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		nullString(job.OwnerAccountID),
		nullString(job.ConversationID),
		job.CallID,
		job.LastEventID,
		nullString(string(job.State)),
		nullString(job.TranscriptMediaID),
		nullTime(job.StartedAt),
		nullTime(job.StoppedAt),
		job.UpdatedAt.UTC(),
	)
	return scanTranscriptionJob(row)
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
SELECT owner_account_id, conversation_id, call_id, last_event_id, state, transcript_media_id, started_at, stopped_at, updated_at
FROM %s
WHERE call_id = $1
`, s.table("call_transcription_jobs"))
	row := s.conn().QueryRowContext(ctx, query, callID)
	job, err := scanTranscriptionJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
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

	query := fmt.Sprintf(`
SELECT owner_account_id, conversation_id, call_id, last_event_id, state, transcript_media_id, started_at, stopped_at, updated_at
FROM %s
WHERE state = 'inactive'
  AND stopped_at IS NOT NULL
  AND COALESCE(transcript_media_id, '') = ''
  AND COALESCE(owner_account_id, '') <> ''
  AND COALESCE(conversation_id, '') <> ''
ORDER BY updated_at ASC, call_id ASC
LIMIT $1
`, s.table("call_transcription_jobs"))
	rows, err := s.conn().QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("load pending transcription jobs: %w", err)
	}
	defer rows.Close()

	var jobs []callhook.TranscriptionJob
	for rows.Next() {
		job, scanErr := scanTranscriptionJobRows(rows)
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

func scanRecordingJob(row *sql.Row) (callhook.RecordingJob, error) {
	var (
		job            callhook.RecordingJob
		ownerAccountID sql.NullString
		conversationID sql.NullString
		state          sql.NullString
		outputID       sql.NullString
		startedAt      sql.NullTime
		stoppedAt      sql.NullTime
	)
	if err := row.Scan(&ownerAccountID, &conversationID, &job.CallID, &job.LastEventID, &state, &outputID, &startedAt, &stoppedAt, &job.UpdatedAt); err != nil {
		return callhook.RecordingJob{}, err
	}
	job.OwnerAccountID = ownerAccountID.String
	job.ConversationID = conversationID.String
	job.State = domaincall.RecordingState(state.String)
	job.OutputMediaID = outputID.String
	job.StartedAt = decodeTime(startedAt)
	job.StoppedAt = decodeTime(stoppedAt)
	job.UpdatedAt = job.UpdatedAt.UTC()
	return job, nil
}

func scanRecordingJobRows(rows *sql.Rows) (callhook.RecordingJob, error) {
	var (
		job            callhook.RecordingJob
		ownerAccountID sql.NullString
		conversationID sql.NullString
		state          sql.NullString
		outputID       sql.NullString
		startedAt      sql.NullTime
		stoppedAt      sql.NullTime
	)
	if err := rows.Scan(&ownerAccountID, &conversationID, &job.CallID, &job.LastEventID, &state, &outputID, &startedAt, &stoppedAt, &job.UpdatedAt); err != nil {
		return callhook.RecordingJob{}, err
	}
	job.OwnerAccountID = ownerAccountID.String
	job.ConversationID = conversationID.String
	job.State = domaincall.RecordingState(state.String)
	job.OutputMediaID = outputID.String
	job.StartedAt = decodeTime(startedAt)
	job.StoppedAt = decodeTime(stoppedAt)
	job.UpdatedAt = job.UpdatedAt.UTC()
	return job, nil
}

func scanTranscriptionJob(row *sql.Row) (callhook.TranscriptionJob, error) {
	var (
		job            callhook.TranscriptionJob
		ownerAccountID sql.NullString
		conversationID sql.NullString
		state          sql.NullString
		mediaID        sql.NullString
		startedAt      sql.NullTime
		stoppedAt      sql.NullTime
	)
	if err := row.Scan(&ownerAccountID, &conversationID, &job.CallID, &job.LastEventID, &state, &mediaID, &startedAt, &stoppedAt, &job.UpdatedAt); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	job.OwnerAccountID = ownerAccountID.String
	job.ConversationID = conversationID.String
	job.State = domaincall.TranscriptionState(state.String)
	job.TranscriptMediaID = mediaID.String
	job.StartedAt = decodeTime(startedAt)
	job.StoppedAt = decodeTime(stoppedAt)
	job.UpdatedAt = job.UpdatedAt.UTC()
	return job, nil
}

func scanTranscriptionJobRows(rows *sql.Rows) (callhook.TranscriptionJob, error) {
	var (
		job            callhook.TranscriptionJob
		ownerAccountID sql.NullString
		conversationID sql.NullString
		state          sql.NullString
		mediaID        sql.NullString
		startedAt      sql.NullTime
		stoppedAt      sql.NullTime
	)
	if err := rows.Scan(&ownerAccountID, &conversationID, &job.CallID, &job.LastEventID, &state, &mediaID, &startedAt, &stoppedAt, &job.UpdatedAt); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	job.OwnerAccountID = ownerAccountID.String
	job.ConversationID = conversationID.String
	job.State = domaincall.TranscriptionState(state.String)
	job.TranscriptMediaID = mediaID.String
	job.StartedAt = decodeTime(startedAt)
	job.StoppedAt = decodeTime(stoppedAt)
	job.UpdatedAt = job.UpdatedAt.UTC()
	return job, nil
}
