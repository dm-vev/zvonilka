package pgstore

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

func TestSaveCallRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 18, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "call"\."call_calls".*RETURNING`).
		WithArgs(
			"call-1",
			"conv-1",
			"acc-1",
			"acc-1",
			false,
			nil,
			nil,
			nil,
			true,
			call.StateRinging,
			nil,
			"inactive",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"inactive",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			now.UTC(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"call_id",
			"conversation_id",
			"initiator_account_id",
			"host_account_id",
			"stage_mode_enabled",
			"pinned_speaker_account_id",
			"pinned_speaker_device_id",
			"active_session_id",
			"requested_video",
			"state",
			"end_reason",
			"recording_state",
			"recording_started_at",
			"recording_stopped_at",
			"transcription_state",
			"transcription_started_at",
			"transcription_stopped_at",
			"started_at",
			"answered_at",
			"ended_at",
			"updated_at",
		}).AddRow(
			"call-1",
			"conv-1",
			"acc-1",
			"acc-1",
			false,
			nil,
			nil,
			"",
			true,
			call.StateRinging,
			"",
			"inactive",
			nil,
			nil,
			"inactive",
			nil,
			nil,
			now.UTC(),
			nil,
			nil,
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveCall(context.Background(), call.Call{
		ID:                 "call-1",
		ConversationID:     "conv-1",
		InitiatorAccountID: "acc-1",
		HostAccountID:      "acc-1",
		RequestedVideo:     true,
		State:              call.StateRinging,
		RecordingState:     call.RecordingStateInactive,
		TranscriptionState: call.TranscriptionStateInactive,
		StartedAt:          now,
		UpdatedAt:          now,
	})
	require.NoError(t, err)
	require.Equal(t, "call-1", saved.ID)
	require.Equal(t, call.StateRinging, saved.State)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveEventRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 18, 10, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "call"\."call_events".*RETURNING`).
		WithArgs(
			"evt-1",
			"call-1",
			"conv-1",
			call.EventTypeStarted,
			"acc-1",
			"dev-1",
			[]byte(`{"with_video":"true"}`),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"event_id",
			"call_id",
			"conversation_id",
			"event_type",
			"actor_account_id",
			"actor_device_id",
			"sequence",
			"metadata",
			"created_at",
		}).AddRow(
			"evt-1",
			"call-1",
			"conv-1",
			call.EventTypeStarted,
			"acc-1",
			"dev-1",
			uint64(7),
			[]byte(`{"with_video":"true"}`),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveEvent(context.Background(), call.Event{
		EventID:        "evt-1",
		CallID:         "call-1",
		ConversationID: "conv-1",
		EventType:      call.EventTypeStarted,
		ActorAccountID: "acc-1",
		ActorDeviceID:  "dev-1",
		Metadata:       map[string]string{"with_video": "true"},
		CreatedAt:      now,
	})
	require.NoError(t, err)
	require.EqualValues(t, 7, saved.Sequence)
	require.Equal(t, "true", saved.Metadata["with_video"])
	require.NoError(t, mock.ExpectationsWereMet())
}
