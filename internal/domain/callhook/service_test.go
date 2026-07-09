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
			Sequence:  1,
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{
			ConversationID:     "conv-1",
			HostAccountID:      "acc-host",
			InitiatorAccountID: "acc-init",
			ID:                 "call-1",
			RecordingState:     domaincall.RecordingStateActive,
			RecordingStartedAt: now,
		},
	}

	recording, err := service.ApplyRecordingHook(context.Background(), payload)
	require.NoError(t, err)
	require.Equal(t, domaincall.RecordingStateActive, recording.State)
	require.Equal(t, "acc-host", recording.OwnerAccountID)
	require.Equal(t, "conv-1", recording.ConversationID)

	transcription, err := service.ApplyTranscriptionHook(context.Background(), domaincall.HookPayload{
		Event: domaincall.Event{
			EventID:   "evt-transcription",
			CallID:    "call-1",
			Sequence:  2,
			EventType: domaincall.EventTypeTranscriptionUpdated,
		},
		Call: domaincall.Call{
			ConversationID:         "conv-1",
			InitiatorAccountID:     "acc-init",
			ID:                     "call-1",
			TranscriptionState:     domaincall.TranscriptionStateActive,
			TranscriptionStartedAt: now,
		},
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.TranscriptionStateActive, transcription.State)
	require.Equal(t, "acc-init", transcription.OwnerAccountID)
	require.Equal(t, "conv-1", transcription.ConversationID)
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
			Sequence:  1,
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{
			ID: "call-2",
		},
	})
	require.ErrorIs(t, err, callhook.ErrInvalidInput)
}

func TestApplyRecordingHookIgnoresStaleSequence(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	service, err := callhook.NewService(store)
	require.NoError(t, err)

	now := time.Date(2026, time.March, 29, 18, 0, 0, 0, time.UTC)
	payload := domaincall.HookPayload{
		Event: domaincall.Event{
			EventID:   "evt-recording-new",
			CallID:    "call-1",
			Sequence:  10,
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{
			ConversationID:     "conv-1",
			HostAccountID:      "acc-host",
			InitiatorAccountID: "acc-init",
			ID:                 "call-1",
			RecordingState:     domaincall.RecordingStateActive,
			RecordingStartedAt: now,
		},
	}

	recording, err := service.ApplyRecordingHook(context.Background(), payload)
	require.NoError(t, err)
	require.Equal(t, uint64(10), recording.LastEventSequence)

	recording, err = service.ApplyRecordingHook(context.Background(), domaincall.HookPayload{
		Event: domaincall.Event{
			EventID:   "evt-recording-old",
			CallID:    "call-1",
			Sequence:  9,
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{
			ConversationID:     "conv-1",
			HostAccountID:      "acc-host",
			InitiatorAccountID: "acc-init",
			ID:                 "call-1",
			RecordingState:     domaincall.RecordingStateInactive,
			RecordingStartedAt: now,
			RecordingStoppedAt: now.Add(time.Minute),
		},
	})
	require.NoError(t, err)
	require.Equal(t, "evt-recording-new", recording.LastEventID)
	require.Equal(t, uint64(10), recording.LastEventSequence)
	require.Equal(t, domaincall.RecordingStateActive, recording.State)
}

func TestApplyRecordingHookPreservesLeaseForSameCapture(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	service, err := callhook.NewService(store)
	require.NoError(t, err)

	startedAt := time.Date(2026, time.March, 29, 18, 0, 0, 0, time.UTC)
	stoppedAt := startedAt.Add(5 * time.Minute)

	_, err = service.ApplyRecordingHook(context.Background(), domaincall.HookPayload{
		Event: domaincall.Event{
			EventID:   "evt-recording-1",
			CallID:    "call-1",
			Sequence:  10,
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{
			ConversationID:     "conv-1",
			HostAccountID:      "acc-host",
			InitiatorAccountID: "acc-init",
			ID:                 "call-1",
			RecordingState:     domaincall.RecordingStateInactive,
			RecordingStartedAt: startedAt,
			RecordingStoppedAt: stoppedAt,
		},
	})
	require.NoError(t, err)

	claimed, err := store.ClaimPendingRecordingJobs(context.Background(), callhook.ClaimJobsParams{
		Before:        time.Now().UTC().Add(time.Minute),
		Limit:         1,
		LeaseToken:    "lease-1",
		LeaseDuration: time.Minute,
	})
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	recording, err := service.ApplyRecordingHook(context.Background(), domaincall.HookPayload{
		Event: domaincall.Event{
			EventID:   "evt-recording-2",
			CallID:    "call-1",
			Sequence:  11,
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{
			ConversationID:     "conv-1",
			HostAccountID:      "acc-host",
			InitiatorAccountID: "acc-init",
			ID:                 "call-1",
			RecordingState:     domaincall.RecordingStateInactive,
			RecordingStartedAt: startedAt,
			RecordingStoppedAt: stoppedAt,
		},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(11), recording.LastEventSequence)
	require.Equal(t, "lease-1", recording.LeaseToken)
	require.False(t, recording.LeaseExpiresAt.IsZero())
}
