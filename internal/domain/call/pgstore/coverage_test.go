package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

type scanRow struct {
	values []any
}

func (r scanRow) Scan(dest ...any) error {
	if len(dest) != len(r.values) {
		return errors.New("scan arity mismatch")
	}
	for i := range dest {
		switch target := dest[i].(type) {
		case *sql.NullTime:
			if r.values[i] == nil {
				*target = sql.NullTime{}
				continue
			}
			*target = sql.NullTime{Time: r.values[i].(time.Time), Valid: true}
		case *sql.NullString:
			if r.values[i] == nil {
				*target = sql.NullString{}
				continue
			}
			*target = sql.NullString{String: r.values[i].(string), Valid: true}
		default:
			value := reflect.ValueOf(dest[i])
			if value.Kind() != reflect.Ptr {
				return errors.New("destination is not pointer")
			}
			elem := value.Elem()
			if r.values[i] == nil {
				elem.Set(reflect.Zero(elem.Type()))
				continue
			}
			elem.Set(reflect.ValueOf(r.values[i]))
		}
	}
	return nil
}

func TestCodecHelpers(t *testing.T) {
	t.Parallel()

	raw, err := encodeMetadata(map[string]string{"a": "b"})
	require.NoError(t, err)
	require.JSONEq(t, `{"a":"b"}`, string(raw))

	raw, err = encodeMetadata(nil)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(raw))

	decoded, err := decodeMetadata([]byte(`{"a":"b"}`))
	require.NoError(t, err)
	require.Equal(t, "b", decoded["a"])

	decoded, err = decodeMetadata(nil)
	require.NoError(t, err)
	require.Nil(t, decoded)

	_, err = decodeMetadata([]byte(`{`))
	require.Error(t, err)

	require.False(t, nullTime(time.Time{}).Valid)
	require.True(t, nullTime(time.Date(2026, time.March, 27, 23, 45, 0, 0, time.UTC)).Valid)
	require.True(t, decodeTime(sql.NullTime{Valid: false}).IsZero())
}

func TestScanHelpers(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 23, 50, 0, 0, time.UTC)

	callRow, err := scanCall(scanRow{values: []any{
		"call-1", "conv-1", "acc-a", "acc-a", true, "acc-b", "dev-b", "sess-1", true, call.StateActive, call.EndReasonEnded,
		"active", now, nil, "inactive", nil, now,
		now, now, now, now,
	}})
	require.NoError(t, err)
	require.Equal(t, "call-1", callRow.ID)
	require.True(t, callRow.StageModeEnabled)
	require.Equal(t, "dev-b", callRow.PinnedSpeakerDeviceID)
	require.Equal(t, call.RecordingStateActive, callRow.RecordingState)
	require.Equal(t, call.TranscriptionStateInactive, callRow.TranscriptionState)

	inviteRow, err := scanInvite(scanRow{values: []any{"call-1", "acc-b", call.InviteStateAccepted, now, now, now}})
	require.NoError(t, err)
	require.Equal(t, call.InviteStateAccepted, inviteRow.State)

	participantRow, err := scanParticipant(scanRow{values: []any{
		"call-1", "acc-b", "dev-b", call.ParticipantStateJoined, false, true, false, true,
		true, now, false, true, now, nil, now,
	}})
	require.NoError(t, err)
	require.Equal(t, "dev-b", participantRow.DeviceID)
	require.True(t, participantRow.MediaState.ScreenShareEnabled)
	require.True(t, participantRow.HandRaised)
	require.True(t, participantRow.HostMutedVideo)

	eventRow, err := scanEvent(scanRow{values: []any{
		"evt-1", "call-1", "conv-1", call.EventTypeStarted, "acc-a", "dev-a", uint64(1), []byte(`{"with_video":"true"}`), now,
	}})
	require.NoError(t, err)
	require.Equal(t, "true", eventRow.Metadata["with_video"])
}

func TestCallInviteParticipantQueries(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 28, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_calls" WHERE call_id = \$1`).
		WithArgs("call-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"call_id", "conversation_id", "initiator_account_id", "host_account_id", "stage_mode_enabled", "pinned_speaker_account_id", "pinned_speaker_device_id", "active_session_id", "requested_video",
			"state", "end_reason", "recording_state", "recording_started_at", "recording_stopped_at", "transcription_state", "transcription_started_at", "transcription_stopped_at", "started_at", "answered_at", "ended_at", "updated_at",
		}).AddRow(
			"call-1", "conv-1", "acc-a", "acc-a", false, nil, nil, "sess-1", true, call.StateActive, "", "inactive", nil, nil, "inactive", nil, nil, now, nil, nil, now,
		))

	row, err := store.CallByID(context.Background(), "call-1")
	require.NoError(t, err)
	require.Equal(t, "call-1", row.ID)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_calls".*state IN \('ringing', 'active'\)`).
		WithArgs("conv-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"call_id", "conversation_id", "initiator_account_id", "host_account_id", "stage_mode_enabled", "pinned_speaker_account_id", "pinned_speaker_device_id", "active_session_id", "requested_video",
			"state", "end_reason", "recording_state", "recording_started_at", "recording_stopped_at", "transcription_state", "transcription_started_at", "transcription_stopped_at", "started_at", "answered_at", "ended_at", "updated_at",
		}).AddRow(
			"call-1", "conv-1", "acc-a", "acc-a", false, nil, nil, "sess-1", true, call.StateActive, "", "inactive", nil, nil, "inactive", nil, nil, now, nil, nil, now,
		))

	row, err = store.ActiveCallByConversation(context.Background(), "conv-1")
	require.NoError(t, err)
	require.Equal(t, "call-1", row.ID)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_calls".*WHERE conversation_id = \$1.*ORDER BY started_at DESC, call_id ASC`).
		WithArgs("conv-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"call_id", "conversation_id", "initiator_account_id", "host_account_id", "stage_mode_enabled", "pinned_speaker_account_id", "pinned_speaker_device_id", "active_session_id", "requested_video",
			"state", "end_reason", "recording_state", "recording_started_at", "recording_stopped_at", "transcription_state", "transcription_started_at", "transcription_stopped_at", "started_at", "answered_at", "ended_at", "updated_at",
		}).AddRow(
			"call-1", "conv-1", "acc-a", "acc-a", false, nil, nil, "", true, call.StateActive, "", "inactive", nil, nil, "inactive", nil, nil, now, nil, nil, now,
		))

	rows, err := store.CallsByConversation(context.Background(), "conv-1", false)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "call"\."call_invites".*RETURNING`).
		WithArgs("call-1", "acc-b", call.InviteStatePending, now, nil, now).
		WillReturnRows(sqlmock.NewRows([]string{
			"call_id", "account_id", "state", "expires_at", "answered_at", "updated_at",
		}).AddRow(
			"call-1", "acc-b", call.InviteStatePending, now, nil, now,
		))
	mock.ExpectCommit()

	_, err = store.SaveInvite(context.Background(), call.Invite{
		CallID:    "call-1",
		AccountID: "acc-b",
		State:     call.InviteStatePending,
		ExpiresAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_invites" WHERE call_id = \$1 AND account_id = \$2`).
		WithArgs("call-1", "acc-b").
		WillReturnRows(sqlmock.NewRows([]string{
			"call_id", "account_id", "state", "expires_at", "answered_at", "updated_at",
		}).AddRow(
			"call-1", "acc-b", call.InviteStatePending, now, nil, now,
		))

	invite, err := store.InviteByCallAndAccount(context.Background(), "call-1", "acc-b")
	require.NoError(t, err)
	require.Equal(t, "acc-b", invite.AccountID)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_invites" WHERE call_id = \$1 ORDER BY account_id ASC`).
		WithArgs("call-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"call_id", "account_id", "state", "expires_at", "answered_at", "updated_at",
		}).AddRow(
			"call-1", "acc-b", call.InviteStatePending, now, nil, now,
		))

	invites, err := store.InvitesByCall(context.Background(), "call-1")
	require.NoError(t, err)
	require.Len(t, invites, 1)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "call"\."call_participants".*RETURNING`).
		WithArgs("call-1", "acc-b", "dev-b", call.ParticipantStateJoined, false, false, true, false, false, nil, false, false, now, nil, now).
		WillReturnRows(sqlmock.NewRows([]string{
			"call_id", "account_id", "device_id", "state", "audio_muted", "video_muted", "camera_enabled", "screen_share_enabled",
			"hand_raised", "raised_hand_at", "host_muted_audio", "host_muted_video", "joined_at", "left_at", "updated_at",
		}).AddRow(
			"call-1", "acc-b", "dev-b", call.ParticipantStateJoined, false, false, true, false, false, nil, false, false, now, nil, now,
		))
	mock.ExpectCommit()

	_, err = store.SaveParticipant(context.Background(), call.Participant{
		CallID:    "call-1",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		State:     call.ParticipantStateJoined,
		MediaState: call.MediaState{
			CameraEnabled: true,
		},
		JoinedAt:  now,
		UpdatedAt: now,
	})
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_participants" WHERE call_id = \$1 AND device_id = \$2`).
		WithArgs("call-1", "dev-b").
		WillReturnRows(sqlmock.NewRows([]string{
			"call_id", "account_id", "device_id", "state", "audio_muted", "video_muted", "camera_enabled", "screen_share_enabled",
			"hand_raised", "raised_hand_at", "host_muted_audio", "host_muted_video", "joined_at", "left_at", "updated_at",
		}).AddRow(
			"call-1", "acc-b", "dev-b", call.ParticipantStateJoined, false, false, true, false, false, nil, false, false, now, nil, now,
		))

	participant, err := store.ParticipantByCallAndDevice(context.Background(), "call-1", "dev-b")
	require.NoError(t, err)
	require.Equal(t, "dev-b", participant.DeviceID)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_participants" WHERE call_id = \$1 ORDER BY device_id ASC`).
		WithArgs("call-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"call_id", "account_id", "device_id", "state", "audio_muted", "video_muted", "camera_enabled", "screen_share_enabled",
			"hand_raised", "raised_hand_at", "host_muted_audio", "host_muted_video", "joined_at", "left_at", "updated_at",
		}).AddRow(
			"call-1", "acc-b", "dev-b", call.ParticipantStateJoined, false, false, true, false, false, nil, false, false, now, nil, now,
		))

	participants, err := store.ParticipantsByCall(context.Background(), "call-1")
	require.NoError(t, err)
	require.Len(t, participants, 1)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_events" WHERE sequence > \$1 AND call_id = \$2 ORDER BY sequence ASC LIMIT 2`).
		WithArgs(uint64(0), "call-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_id", "call_id", "conversation_id", "event_type", "actor_account_id", "actor_device_id", "sequence", "metadata", "created_at",
		}).AddRow(
			"evt-1", "call-1", "conv-1", call.EventTypeStarted, "acc-a", "dev-a", uint64(1), []byte(`{"with_video":"true"}`), now,
		))

	events, err := store.EventsAfterSequence(context.Background(), 0, "call-1", "", 2)
	require.NoError(t, err)
	require.Len(t, events, 1)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPgstoreValidationAndNotFoundBranches(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	_, err := store.saveCall(context.Background(), call.Call{})
	require.ErrorIs(t, err, call.ErrInvalidInput)

	_, err = store.CallByID(context.Background(), "")
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.ActiveCallByConversation(context.Background(), "")
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.CallsByConversation(context.Background(), "", false)
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.saveInvite(context.Background(), call.Invite{})
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.InviteByCallAndAccount(context.Background(), "", "")
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.InvitesByCall(context.Background(), "")
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.saveParticipant(context.Background(), call.Participant{})
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.ParticipantByCallAndDevice(context.Background(), "", "")
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.ParticipantsByCall(context.Background(), "")
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.saveEvent(context.Background(), call.Event{})
	require.ErrorIs(t, err, call.ErrInvalidInput)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_calls" WHERE call_id = \$1`).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)
	_, err = store.CallByID(context.Background(), "missing")
	require.ErrorIs(t, err, call.ErrNotFound)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_invites" WHERE call_id = \$1 AND account_id = \$2`).
		WithArgs("call-1", "acc-b").
		WillReturnError(sql.ErrNoRows)
	_, err = store.InviteByCallAndAccount(context.Background(), "call-1", "acc-b")
	require.ErrorIs(t, err, call.ErrNotFound)

	mock.ExpectQuery(`SELECT .* FROM "call"\."call_participants" WHERE call_id = \$1 AND device_id = \$2`).
		WithArgs("call-1", "dev-b").
		WillReturnError(sql.ErrNoRows)
	_, err = store.ParticipantByCallAndDevice(context.Background(), "call-1", "dev-b")
	require.ErrorIs(t, err, call.ErrNotFound)

	require.Nil(t, mapConstraintError(errors.New("plain")))
	require.True(t, errors.Is(mapConstraintError(&pgconn.PgError{Code: "23514"}), call.ErrInvalidInput))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreNewAndWithinTxBranches(t *testing.T) {
	t.Parallel()

	_, err := New(nil, "call")
	require.ErrorIs(t, err, call.ErrInvalidInput)

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	store, err := New(db, "call")
	require.NoError(t, err)

	require.ErrorIs(t, store.requireContext(nil), call.ErrInvalidInput)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, store.requireContext(ctx), context.Canceled)
	require.Equal(t, `"call"."tbl"`, store.table("tbl"))

	mock.ExpectBegin()
	mock.ExpectCommit()
	err = store.WithinTx(context.Background(), func(call.Store) error { return nil })
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectRollback()
	errBoom := errors.New("boom")
	err = store.WithinTx(context.Background(), func(call.Store) error { return errBoom })
	require.ErrorIs(t, err, errBoom)

	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(&pgconn.PgError{Code: "23514"})
	err = store.WithinTx(context.Background(), func(call.Store) error { return nil })
	require.ErrorIs(t, err, call.ErrInvalidInput)

	txStore := &Store{db: db, tx: &sql.Tx{}, schema: "call"}
	called := false
	err = txStore.WithinTx(context.Background(), func(call.Store) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, called)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreRequireStoreBranches(t *testing.T) {
	t.Parallel()

	store := &Store{}
	require.ErrorIs(t, store.requireStore(), call.ErrInvalidInput)
	_, err := store.SaveCall(context.Background(), call.Call{})
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.SaveEvent(context.Background(), call.Event{})
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.SaveInvite(context.Background(), call.Invite{})
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.SaveParticipant(context.Background(), call.Participant{})
	require.ErrorIs(t, err, call.ErrInvalidInput)
}
