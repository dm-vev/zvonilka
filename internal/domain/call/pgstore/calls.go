package pgstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

const callColumnList = `
call_id, conversation_id, initiator_account_id, host_account_id, stage_mode_enabled,
pinned_speaker_account_id, pinned_speaker_device_id, active_session_id, requested_video,
state, end_reason, recording_state, recording_started_at, recording_stopped_at,
transcription_state, transcription_started_at, transcription_stopped_at,
started_at, answered_at, ended_at, updated_at
`

func (s *Store) SaveCall(ctx context.Context, value call.Call) (call.Call, error) {
	if err := s.requireStore(); err != nil {
		return call.Call{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Call{}, err
	}
	if s.tx != nil {
		return s.saveCall(ctx, value)
	}

	var saved call.Call
	err := s.WithinTx(ctx, func(tx call.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveCall(ctx, value)
		return saveErr
	})
	if err != nil {
		return call.Call{}, err
	}
	return saved, nil
}

func (s *Store) saveCall(ctx context.Context, value call.Call) (call.Call, error) {
	value.ID = strings.TrimSpace(value.ID)
	value.ConversationID = strings.TrimSpace(value.ConversationID)
	value.InitiatorAccountID = strings.TrimSpace(value.InitiatorAccountID)
	value.HostAccountID = strings.TrimSpace(value.HostAccountID)
	value.PinnedSpeakerAccountID = strings.TrimSpace(value.PinnedSpeakerAccountID)
	value.PinnedSpeakerDeviceID = strings.TrimSpace(value.PinnedSpeakerDeviceID)
	value.ActiveSessionID = strings.TrimSpace(value.ActiveSessionID)
	if value.ID == "" || value.ConversationID == "" || value.InitiatorAccountID == "" || value.State == call.StateUnspecified {
		return call.Call{}, call.ErrInvalidInput
	}
	if value.HostAccountID == "" {
		value.HostAccountID = value.InitiatorAccountID
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	call_id, conversation_id, initiator_account_id, host_account_id, stage_mode_enabled,
	pinned_speaker_account_id, pinned_speaker_device_id, active_session_id, requested_video,
	state, end_reason, recording_state, recording_started_at, recording_stopped_at,
	transcription_state, transcription_started_at, transcription_stopped_at,
	started_at, answered_at, ended_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9,
	$10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
)
ON CONFLICT (call_id) DO UPDATE SET
	conversation_id = EXCLUDED.conversation_id,
	initiator_account_id = EXCLUDED.initiator_account_id,
	host_account_id = EXCLUDED.host_account_id,
	stage_mode_enabled = EXCLUDED.stage_mode_enabled,
	pinned_speaker_account_id = EXCLUDED.pinned_speaker_account_id,
	pinned_speaker_device_id = EXCLUDED.pinned_speaker_device_id,
	active_session_id = EXCLUDED.active_session_id,
	requested_video = EXCLUDED.requested_video,
	state = EXCLUDED.state,
	end_reason = EXCLUDED.end_reason,
	recording_state = EXCLUDED.recording_state,
	recording_started_at = EXCLUDED.recording_started_at,
	recording_stopped_at = EXCLUDED.recording_stopped_at,
	transcription_state = EXCLUDED.transcription_state,
	transcription_started_at = EXCLUDED.transcription_started_at,
	transcription_stopped_at = EXCLUDED.transcription_stopped_at,
	started_at = EXCLUDED.started_at,
	answered_at = EXCLUDED.answered_at,
	ended_at = EXCLUDED.ended_at,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("call_calls"), callColumnList)

	row := s.conn().QueryRowContext(
		ctx,
		query,
		value.ID,
		value.ConversationID,
		value.InitiatorAccountID,
		value.HostAccountID,
		value.StageModeEnabled,
		nullString(value.PinnedSpeakerAccountID),
		nullString(value.PinnedSpeakerDeviceID),
		nullString(value.ActiveSessionID),
		value.RequestedVideo,
		value.State,
		nullString(string(value.EndReason)),
		nullString(string(value.RecordingState)),
		nullTime(value.RecordingStartedAt),
		nullTime(value.RecordingStoppedAt),
		nullString(string(value.TranscriptionState)),
		nullTime(value.TranscriptionStartedAt),
		nullTime(value.TranscriptionStoppedAt),
		value.StartedAt.UTC(),
		nullTime(value.AnsweredAt),
		nullTime(value.EndedAt),
		value.UpdatedAt.UTC(),
	)
	saved, err := scanCall(row)
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return call.Call{}, mapped
		}
		return call.Call{}, fmt.Errorf("save call %s: %w", value.ID, err)
	}
	return saved, nil
}

func (s *Store) CallByID(ctx context.Context, callID string) (call.Call, error) {
	if err := s.requireStore(); err != nil {
		return call.Call{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Call{}, err
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return call.Call{}, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE call_id = $1`, callColumnList, s.table("call_calls"))
	row := s.conn().QueryRowContext(ctx, query, callID)
	value, err := scanCall(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return call.Call{}, call.ErrNotFound
		}
		return call.Call{}, fmt.Errorf("load call %s: %w", callID, err)
	}
	return value, nil
}

func (s *Store) ActiveCallByConversation(ctx context.Context, conversationID string) (call.Call, error) {
	if err := s.requireStore(); err != nil {
		return call.Call{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Call{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return call.Call{}, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE conversation_id = $1 AND state IN ('ringing', 'active')
ORDER BY started_at DESC
LIMIT 1
`, callColumnList, s.table("call_calls"))
	row := s.conn().QueryRowContext(ctx, query, conversationID)
	value, err := scanCall(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return call.Call{}, call.ErrNotFound
		}
		return call.Call{}, fmt.Errorf("load active call for conversation %s: %w", conversationID, err)
	}
	return value, nil
}

func (s *Store) CallsByConversation(ctx context.Context, conversationID string, includeEnded bool) ([]call.Call, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE conversation_id = $1
`, callColumnList, s.table("call_calls"))
	if !includeEnded {
		query += ` AND state <> 'ended'`
	}
	query += ` ORDER BY started_at DESC, call_id ASC`

	rows, err := s.conn().QueryContext(ctx, query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list calls for conversation %s: %w", conversationID, err)
	}
	defer rows.Close()

	result := make([]call.Call, 0)
	for rows.Next() {
		value, scanErr := scanCall(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan call for conversation %s: %w", conversationID, scanErr)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calls for conversation %s: %w", conversationID, err)
	}
	return result, nil
}

func nullString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
