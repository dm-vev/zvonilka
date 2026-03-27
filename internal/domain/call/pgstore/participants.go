package pgstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

const participantColumnList = `
call_id, account_id, device_id, state, audio_muted, video_muted, camera_enabled, screen_share_enabled,
hand_raised, raised_hand_at, host_muted_audio, host_muted_video, joined_at, left_at, updated_at
`

func (s *Store) SaveParticipant(ctx context.Context, value call.Participant) (call.Participant, error) {
	if err := s.requireStore(); err != nil {
		return call.Participant{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Participant{}, err
	}
	if s.tx != nil {
		return s.saveParticipant(ctx, value)
	}

	var saved call.Participant
	err := s.WithinTx(ctx, func(tx call.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveParticipant(ctx, value)
		return saveErr
	})
	if err != nil {
		return call.Participant{}, err
	}
	return saved, nil
}

func (s *Store) saveParticipant(ctx context.Context, value call.Participant) (call.Participant, error) {
	value.CallID = strings.TrimSpace(value.CallID)
	value.AccountID = strings.TrimSpace(value.AccountID)
	value.DeviceID = strings.TrimSpace(value.DeviceID)
	if value.CallID == "" || value.AccountID == "" || value.DeviceID == "" || value.State == call.ParticipantStateUnspecified || value.JoinedAt.IsZero() || value.UpdatedAt.IsZero() {
		return call.Participant{}, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	call_id, account_id, device_id, state, audio_muted, video_muted, camera_enabled, screen_share_enabled,
	hand_raised, raised_hand_at, host_muted_audio, host_muted_video, joined_at, left_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
)
ON CONFLICT (call_id, device_id) DO UPDATE SET
	account_id = EXCLUDED.account_id,
	state = EXCLUDED.state,
	audio_muted = EXCLUDED.audio_muted,
	video_muted = EXCLUDED.video_muted,
	camera_enabled = EXCLUDED.camera_enabled,
	screen_share_enabled = EXCLUDED.screen_share_enabled,
	hand_raised = EXCLUDED.hand_raised,
	raised_hand_at = EXCLUDED.raised_hand_at,
	host_muted_audio = EXCLUDED.host_muted_audio,
	host_muted_video = EXCLUDED.host_muted_video,
	joined_at = EXCLUDED.joined_at,
	left_at = EXCLUDED.left_at,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("call_participants"), participantColumnList)
	row := s.conn().QueryRowContext(
		ctx,
		query,
		value.CallID,
		value.AccountID,
		value.DeviceID,
		value.State,
		value.MediaState.AudioMuted,
		value.MediaState.VideoMuted,
		value.MediaState.CameraEnabled,
		value.MediaState.ScreenShareEnabled,
		value.HandRaised,
		nullTime(value.RaisedHandAt),
		value.HostMutedAudio,
		value.HostMutedVideo,
		value.JoinedAt.UTC(),
		nullTime(value.LeftAt),
		value.UpdatedAt.UTC(),
	)
	saved, err := scanParticipant(row)
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return call.Participant{}, mapped
		}
		return call.Participant{}, fmt.Errorf("save participant %s/%s: %w", value.CallID, value.DeviceID, err)
	}
	return saved, nil
}

func (s *Store) ParticipantByCallAndDevice(ctx context.Context, callID string, deviceID string) (call.Participant, error) {
	if err := s.requireStore(); err != nil {
		return call.Participant{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Participant{}, err
	}
	callID = strings.TrimSpace(callID)
	deviceID = strings.TrimSpace(deviceID)
	if callID == "" || deviceID == "" {
		return call.Participant{}, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE call_id = $1 AND device_id = $2`, participantColumnList, s.table("call_participants"))
	row := s.conn().QueryRowContext(ctx, query, callID, deviceID)
	value, err := scanParticipant(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return call.Participant{}, call.ErrNotFound
		}
		return call.Participant{}, fmt.Errorf("load participant %s/%s: %w", callID, deviceID, err)
	}
	return value, nil
}

func (s *Store) ParticipantsByCall(ctx context.Context, callID string) ([]call.Participant, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE call_id = $1 ORDER BY device_id ASC`, participantColumnList, s.table("call_participants"))
	rows, err := s.conn().QueryContext(ctx, query, callID)
	if err != nil {
		return nil, fmt.Errorf("list participants for call %s: %w", callID, err)
	}
	defer rows.Close()

	result := make([]call.Participant, 0)
	for rows.Next() {
		value, scanErr := scanParticipant(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan participant for call %s: %w", callID, scanErr)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate participants for call %s: %w", callID, err)
	}
	return result, nil
}
