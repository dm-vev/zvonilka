package call_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	calltest "github.com/dm-vev/zvonilka/internal/domain/call/teststore"
)

type recordingHookRecorder struct {
	recording     []domaincall.HookPayload
	transcription []domaincall.HookPayload
	failEventID   string
}

type rehomeEventRecorder struct {
	events []domaincall.Event
}

func (r *rehomeEventRecorder) HandleCallEvents(_ context.Context, events []domaincall.Event) error {
	r.events = append(r.events, events...)
	return nil
}

func (r *recordingHookRecorder) HandleRecording(_ context.Context, payload domaincall.HookPayload) error {
	if payload.Event.EventID == r.failEventID {
		return errors.New("boom")
	}
	r.recording = append(r.recording, payload)
	return nil
}

func (r *recordingHookRecorder) HandleTranscription(_ context.Context, payload domaincall.HookPayload) error {
	if payload.Event.EventID == r.failEventID {
		return errors.New("boom")
	}
	r.transcription = append(r.transcription, payload)
	return nil
}

func TestCallWorkerProcessesRecordingAndTranscriptionEvents(t *testing.T) {
	t.Parallel()

	store := calltest.NewMemoryStore()
	now := time.Date(2026, time.March, 29, 16, 0, 0, 0, time.UTC)

	_, err := store.SaveCall(context.Background(), domaincall.Call{
		ID:                 "call-1",
		ConversationID:     "conv-1",
		InitiatorAccountID: "acc-a",
		HostAccountID:      "acc-a",
		State:              domaincall.StateActive,
		RecordingState:     domaincall.RecordingStateActive,
		TranscriptionState: domaincall.TranscriptionStateActive,
		StartedAt:          now,
		UpdatedAt:          now,
	})
	require.NoError(t, err)

	_, err = store.SaveEvent(context.Background(), domaincall.Event{
		EventID:        "evt-recording",
		CallID:         "call-1",
		ConversationID: "conv-1",
		EventType:      domaincall.EventTypeRecordingUpdated,
		ActorAccountID: "acc-a",
		ActorDeviceID:  "dev-a",
		CreatedAt:      now,
	})
	require.NoError(t, err)

	_, err = store.SaveEvent(context.Background(), domaincall.Event{
		EventID:        "evt-transcription",
		CallID:         "call-1",
		ConversationID: "conv-1",
		EventType:      domaincall.EventTypeTranscriptionUpdated,
		ActorAccountID: "acc-a",
		ActorDeviceID:  "dev-a",
		CreatedAt:      now,
	})
	require.NoError(t, err)

	hooks := &recordingHookRecorder{}
	worker, err := domaincall.NewWorker(store, hooks, domaincall.WorkerSettings{
		PollInterval: time.Hour,
		BatchSize:    10,
	})
	require.NoError(t, err)

	require.NoError(t, worker.ProcessOnceForTests(context.Background()))
	require.Len(t, hooks.recording, 1)
	require.Len(t, hooks.transcription, 1)

	cursor, err := store.WorkerCursorByName(context.Background(), "call_hooks")
	require.NoError(t, err)
	require.EqualValues(t, 2, cursor.LastSequence)
}

func TestCallWorkerDoesNotAdvanceCursorOnHandlerFailure(t *testing.T) {
	t.Parallel()

	store := calltest.NewMemoryStore()
	now := time.Date(2026, time.March, 29, 16, 10, 0, 0, time.UTC)

	_, err := store.SaveCall(context.Background(), domaincall.Call{
		ID:                 "call-1",
		ConversationID:     "conv-1",
		InitiatorAccountID: "acc-a",
		HostAccountID:      "acc-a",
		State:              domaincall.StateActive,
		RecordingState:     domaincall.RecordingStateActive,
		StartedAt:          now,
		UpdatedAt:          now,
	})
	require.NoError(t, err)

	_, err = store.SaveEvent(context.Background(), domaincall.Event{
		EventID:        "evt-recording",
		CallID:         "call-1",
		ConversationID: "conv-1",
		EventType:      domaincall.EventTypeRecordingUpdated,
		ActorAccountID: "acc-a",
		ActorDeviceID:  "dev-a",
		CreatedAt:      now,
	})
	require.NoError(t, err)

	hooks := &recordingHookRecorder{failEventID: "evt-recording"}
	worker, err := domaincall.NewWorker(store, hooks, domaincall.WorkerSettings{
		PollInterval: time.Hour,
		BatchSize:    10,
	})
	require.NoError(t, err)

	err = worker.ProcessOnceForTests(context.Background())
	require.Error(t, err)
	_, err = store.WorkerCursorByName(context.Background(), "call_hooks")
	require.ErrorIs(t, err, domaincall.ErrNotFound)
}

func TestRehomeWorkerMigratesUnavailableActiveCalls(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 29, 17, 0, 0, 0, time.UTC)
	runtime := newFakeRuntime(now)
	service := newTestService(t, now, runtime)

	started, _, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
		WithVideo:      true,
	})
	require.NoError(t, err)

	_, _, err = service.AcceptCall(context.Background(), domaincall.AcceptParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)

	_, _, _, err = service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	runtime.statsErr["sess-1"] = status.Error(codes.Unavailable, "rtc unavailable")

	recorder := &rehomeEventRecorder{}
	worker, err := domaincall.NewRehomeWorker(service, recorder, domaincall.RehomeWorkerSettings{
		PollInterval: time.Hour,
		BatchSize:    10,
	})
	require.NoError(t, err)

	require.NoError(t, worker.ProcessOnceForTests(context.Background()))
	require.NotEmpty(t, recorder.events)

	callRow, err := service.GetCall(context.Background(), domaincall.GetParams{
		CallID:    started.ID,
		AccountID: "acc-a",
	})
	require.NoError(t, err)
	require.Equal(t, "sess-2", callRow.ActiveSessionID)
}
