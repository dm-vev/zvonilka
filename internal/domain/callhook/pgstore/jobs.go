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
	job.CallID = strings.TrimSpace(job.CallID)
	job.LastEventID = strings.TrimSpace(job.LastEventID)
	job.OutputMediaID = strings.TrimSpace(job.OutputMediaID)
	if job.CallID == "" || job.LastEventID == "" {
		return callhook.RecordingJob{}, callhook.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	call_id, last_event_id, state, output_media_id, started_at, stopped_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (call_id) DO UPDATE SET
	last_event_id = EXCLUDED.last_event_id,
	state = EXCLUDED.state,
	output_media_id = EXCLUDED.output_media_id,
	started_at = EXCLUDED.started_at,
	stopped_at = EXCLUDED.stopped_at,
	updated_at = EXCLUDED.updated_at
RETURNING call_id, last_event_id, state, output_media_id, started_at, stopped_at, updated_at
`, s.table("call_recording_jobs"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
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
SELECT call_id, last_event_id, state, output_media_id, started_at, stopped_at, updated_at
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
	job.CallID = strings.TrimSpace(job.CallID)
	job.LastEventID = strings.TrimSpace(job.LastEventID)
	job.TranscriptMediaID = strings.TrimSpace(job.TranscriptMediaID)
	if job.CallID == "" || job.LastEventID == "" {
		return callhook.TranscriptionJob{}, callhook.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	call_id, last_event_id, state, transcript_media_id, started_at, stopped_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (call_id) DO UPDATE SET
	last_event_id = EXCLUDED.last_event_id,
	state = EXCLUDED.state,
	transcript_media_id = EXCLUDED.transcript_media_id,
	started_at = EXCLUDED.started_at,
	stopped_at = EXCLUDED.stopped_at,
	updated_at = EXCLUDED.updated_at
RETURNING call_id, last_event_id, state, transcript_media_id, started_at, stopped_at, updated_at
`, s.table("call_transcription_jobs"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
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
SELECT call_id, last_event_id, state, transcript_media_id, started_at, stopped_at, updated_at
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
		job       callhook.RecordingJob
		state     sql.NullString
		outputID  sql.NullString
		startedAt sql.NullTime
		stoppedAt sql.NullTime
	)
	if err := row.Scan(&job.CallID, &job.LastEventID, &state, &outputID, &startedAt, &stoppedAt, &job.UpdatedAt); err != nil {
		return callhook.RecordingJob{}, err
	}
	job.State = domaincall.RecordingState(state.String)
	job.OutputMediaID = outputID.String
	job.StartedAt = decodeTime(startedAt)
	job.StoppedAt = decodeTime(stoppedAt)
	job.UpdatedAt = job.UpdatedAt.UTC()
	return job, nil
}

func scanTranscriptionJob(row *sql.Row) (callhook.TranscriptionJob, error) {
	var (
		job       callhook.TranscriptionJob
		state     sql.NullString
		mediaID   sql.NullString
		startedAt sql.NullTime
		stoppedAt sql.NullTime
	)
	if err := row.Scan(&job.CallID, &job.LastEventID, &state, &mediaID, &startedAt, &stoppedAt, &job.UpdatedAt); err != nil {
		return callhook.TranscriptionJob{}, err
	}
	job.State = domaincall.TranscriptionState(state.String)
	job.TranscriptMediaID = mediaID.String
	job.StartedAt = decodeTime(startedAt)
	job.StoppedAt = decodeTime(stoppedAt)
	job.UpdatedAt = job.UpdatedAt.UTC()
	return job, nil
}
