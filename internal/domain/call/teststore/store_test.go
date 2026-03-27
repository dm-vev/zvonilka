package teststore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/storage"
)

func TestMemoryStoreCallInviteParticipantAndEventRoundTrip(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Date(2026, time.March, 27, 23, 0, 0, 0, time.UTC)

	savedCall, err := store.SaveCall(ctx, call.Call{
		ID:                 "call-1",
		ConversationID:     "conv-1",
		InitiatorAccountID: "acc-a",
		State:              call.StateActive,
		StartedAt:          now,
		UpdatedAt:          now,
	})
	require.NoError(t, err)
	require.Equal(t, "call-1", savedCall.ID)

	loadedCall, err := store.CallByID(ctx, "call-1")
	require.NoError(t, err)
	require.Equal(t, "conv-1", loadedCall.ConversationID)

	activeCall, err := store.ActiveCallByConversation(ctx, "conv-1")
	require.NoError(t, err)
	require.Equal(t, "call-1", activeCall.ID)

	rows, err := store.CallsByConversation(ctx, "conv-1", false)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	savedInvite, err := store.SaveInvite(ctx, call.Invite{
		CallID:    "call-1",
		AccountID: "acc-b",
		State:     call.InviteStatePending,
		UpdatedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, "acc-b", savedInvite.AccountID)

	invite, err := store.InviteByCallAndAccount(ctx, "call-1", "acc-b")
	require.NoError(t, err)
	require.Equal(t, call.InviteStatePending, invite.State)

	invites, err := store.InvitesByCall(ctx, "call-1")
	require.NoError(t, err)
	require.Len(t, invites, 1)

	savedParticipant, err := store.SaveParticipant(ctx, call.Participant{
		CallID:    "call-1",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		State:     call.ParticipantStateJoined,
		JoinedAt:  now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, "dev-b", savedParticipant.DeviceID)

	participant, err := store.ParticipantByCallAndDevice(ctx, "call-1", "dev-b")
	require.NoError(t, err)
	require.Equal(t, call.ParticipantStateJoined, participant.State)

	participants, err := store.ParticipantsByCall(ctx, "call-1")
	require.NoError(t, err)
	require.Len(t, participants, 1)

	event, err := store.SaveEvent(ctx, call.Event{
		EventID:        "evt-1",
		CallID:         "call-1",
		ConversationID: "conv-1",
		EventType:      call.EventTypeStarted,
		ActorAccountID: "acc-a",
		ActorDeviceID:  "dev-a",
		Metadata: map[string]string{
			"with_video": "true",
		},
		CreatedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), event.Sequence)

	events, err := store.EventsAfterSequence(ctx, 0, "call-1", "", 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "true", events[0].Metadata["with_video"])
}

func TestMemoryStoreWithinTxRollbackAndCommitSemantics(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Date(2026, time.March, 27, 23, 30, 0, 0, time.UTC)

	errBoom := errors.New("boom")
	err := store.WithinTx(ctx, func(tx call.Store) error {
		_, saveErr := tx.SaveCall(ctx, call.Call{
			ID:                 "call-rollback",
			ConversationID:     "conv-1",
			InitiatorAccountID: "acc-a",
			State:              call.StateRinging,
			StartedAt:          now,
			UpdatedAt:          now,
		})
		require.NoError(t, saveErr)
		return errBoom
	})
	require.ErrorIs(t, err, errBoom)

	_, err = store.CallByID(ctx, "call-rollback")
	require.ErrorIs(t, err, call.ErrNotFound)

	err = store.WithinTx(ctx, func(tx call.Store) error {
		_, saveErr := tx.SaveCall(ctx, call.Call{
			ID:                 "call-commit",
			ConversationID:     "conv-1",
			InitiatorAccountID: "acc-a",
			State:              call.StateRinging,
			StartedAt:          now,
			UpdatedAt:          now,
		})
		require.NoError(t, saveErr)
		return storage.Commit(errors.New("commit"))
	})
	require.ErrorContains(t, err, "commit")

	committed, err := store.CallByID(ctx, "call-commit")
	require.NoError(t, err)
	require.Equal(t, "call-commit", committed.ID)
}

func TestMemoryStoreRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ctx := context.Background()

	_, err := store.SaveCall(ctx, call.Call{})
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.CallByID(ctx, "")
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.ActiveCallByConversation(ctx, "")
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.CallsByConversation(ctx, "", true)
	require.ErrorIs(t, err, call.ErrInvalidInput)

	_, err = store.SaveInvite(ctx, call.Invite{})
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.InviteByCallAndAccount(ctx, "", "")
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.InvitesByCall(ctx, "")
	require.ErrorIs(t, err, call.ErrInvalidInput)

	_, err = store.SaveParticipant(ctx, call.Participant{})
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.ParticipantByCallAndDevice(ctx, "", "")
	require.ErrorIs(t, err, call.ErrInvalidInput)
	_, err = store.ParticipantsByCall(ctx, "")
	require.ErrorIs(t, err, call.ErrInvalidInput)

	_, err = store.SaveEvent(ctx, call.Event{})
	require.ErrorIs(t, err, call.ErrInvalidInput)

	err = store.WithinTx(nil, func(call.Store) error { return nil })
	require.ErrorIs(t, err, call.ErrInvalidInput)
	err = store.WithinTx(ctx, nil)
	require.ErrorIs(t, err, call.ErrInvalidInput)
}
