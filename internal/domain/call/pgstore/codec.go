package pgstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/jackc/pgx/v5/pgconn"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func encodeMetadata(metadata map[string]string) ([]byte, error) {
	if len(metadata) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(metadata)
}

func decodeMetadata(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var metadata map[string]string
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, err
	}
	return metadata, nil
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

func scanCall(row rowScanner) (call.Call, error) {
	var (
		value                call.Call
		activeSessionID      sql.NullString
		pinnedSpeakerAccount sql.NullString
		pinnedSpeakerDevice  sql.NullString
		recordingState       sql.NullString
		recordingStartedAt   sql.NullTime
		recordingStoppedAt   sql.NullTime
		transcriptionState   sql.NullString
		transcriptionStarted sql.NullTime
		transcriptionStopped sql.NullTime
		answeredAt           sql.NullTime
		endedAt              sql.NullTime
	)
	if err := row.Scan(
		&value.ID,
		&value.ConversationID,
		&value.InitiatorAccountID,
		&value.HostAccountID,
		&value.StageModeEnabled,
		&pinnedSpeakerAccount,
		&pinnedSpeakerDevice,
		&activeSessionID,
		&value.RequestedVideo,
		&value.State,
		&value.EndReason,
		&recordingState,
		&recordingStartedAt,
		&recordingStoppedAt,
		&transcriptionState,
		&transcriptionStarted,
		&transcriptionStopped,
		&value.StartedAt,
		&answeredAt,
		&endedAt,
		&value.UpdatedAt,
	); err != nil {
		return call.Call{}, err
	}
	value.StartedAt = value.StartedAt.UTC()
	value.ActiveSessionID = activeSessionID.String
	value.PinnedSpeakerAccountID = pinnedSpeakerAccount.String
	value.PinnedSpeakerDeviceID = pinnedSpeakerDevice.String
	value.RecordingState = call.RecordingState(recordingState.String)
	value.RecordingStartedAt = decodeTime(recordingStartedAt)
	value.RecordingStoppedAt = decodeTime(recordingStoppedAt)
	value.TranscriptionState = call.TranscriptionState(transcriptionState.String)
	value.TranscriptionStartedAt = decodeTime(transcriptionStarted)
	value.TranscriptionStoppedAt = decodeTime(transcriptionStopped)
	value.AnsweredAt = decodeTime(answeredAt)
	value.EndedAt = decodeTime(endedAt)
	value.UpdatedAt = value.UpdatedAt.UTC()
	return value, nil
}

func scanInvite(row rowScanner) (call.Invite, error) {
	var (
		value      call.Invite
		answeredAt sql.NullTime
	)
	if err := row.Scan(
		&value.CallID,
		&value.AccountID,
		&value.State,
		&value.ExpiresAt,
		&answeredAt,
		&value.UpdatedAt,
	); err != nil {
		return call.Invite{}, err
	}
	value.ExpiresAt = value.ExpiresAt.UTC()
	value.AnsweredAt = decodeTime(answeredAt)
	value.UpdatedAt = value.UpdatedAt.UTC()
	return value, nil
}

func scanParticipant(row rowScanner) (call.Participant, error) {
	var (
		value        call.Participant
		raisedHandAt sql.NullTime
		leftAt       sql.NullTime
	)
	if err := row.Scan(
		&value.CallID,
		&value.AccountID,
		&value.DeviceID,
		&value.State,
		&value.MediaState.AudioMuted,
		&value.MediaState.VideoMuted,
		&value.MediaState.CameraEnabled,
		&value.MediaState.ScreenShareEnabled,
		&value.HandRaised,
		&raisedHandAt,
		&value.HostMutedAudio,
		&value.HostMutedVideo,
		&value.JoinedAt,
		&leftAt,
		&value.UpdatedAt,
	); err != nil {
		return call.Participant{}, err
	}
	value.JoinedAt = value.JoinedAt.UTC()
	value.RaisedHandAt = decodeTime(raisedHandAt)
	value.LeftAt = decodeTime(leftAt)
	value.UpdatedAt = value.UpdatedAt.UTC()
	return value, nil
}

func scanEvent(row rowScanner) (call.Event, error) {
	var (
		value    call.Event
		metadata []byte
		deviceID sql.NullString
	)
	if err := row.Scan(
		&value.EventID,
		&value.CallID,
		&value.ConversationID,
		&value.EventType,
		&value.ActorAccountID,
		&deviceID,
		&value.Sequence,
		&metadata,
		&value.CreatedAt,
	); err != nil {
		return call.Event{}, err
	}

	decoded, err := decodeMetadata(metadata)
	if err != nil {
		return call.Event{}, err
	}
	value.ActorDeviceID = deviceID.String
	value.Metadata = decoded
	value.CreatedAt = value.CreatedAt.UTC()
	return value, nil
}

func mapConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return nil
	}

	switch pgErr.Code {
	case "23505":
		return call.ErrConflict
	case "23503":
		return call.ErrNotFound
	case "23514":
		return call.ErrInvalidInput
	default:
		return nil
	}
}
