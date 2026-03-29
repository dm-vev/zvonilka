package callhook_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/callhook"
	callhooktest "github.com/dm-vev/zvonilka/internal/domain/callhook/teststore"
)

func TestApplyRecordingAndTranscriptionHooks(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	service, err := callhook.NewService(store)
	require.NoError(t, err)

	now := time.Date(2026, time.March, 29, 18, 0, 0, 0, time.UTC)
	payload := domaincall.HookPayload{
		Event: domaincall.Event{
			EventID:   "evt-recording",
			CallID:    "call-1",
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{
			ID:                 "call-1",
			RecordingState:     domaincall.RecordingStateActive,
			RecordingStartedAt: now,
		},
	}

	recording, err := service.ApplyRecordingHook(context.Background(), payload)
	require.NoError(t, err)
	require.Equal(t, domaincall.RecordingStateActive, recording.State)

	transcription, err := service.ApplyTranscriptionHook(context.Background(), domaincall.HookPayload{
		Event: domaincall.Event{
			EventID:   "evt-transcription",
			CallID:    "call-1",
			EventType: domaincall.EventTypeTranscriptionUpdated,
		},
		Call: domaincall.Call{
			ID:                     "call-1",
			TranscriptionState:     domaincall.TranscriptionStateActive,
			TranscriptionStartedAt: now,
		},
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.TranscriptionStateActive, transcription.State)
}

func TestApplyHookRejectsMismatchedCallIDs(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	service, err := callhook.NewService(store)
	require.NoError(t, err)

	_, err = service.ApplyRecordingHook(context.Background(), domaincall.HookPayload{
		Event: domaincall.Event{
			EventID:   "evt-recording",
			CallID:    "call-1",
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{
			ID: "call-2",
		},
	})
	require.ErrorIs(t, err, callhook.ErrInvalidInput)
}
