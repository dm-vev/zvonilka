package teststore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

func TestSaveDeliveryPreservesHigherAttempts(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)

	existing, err := store.SaveDelivery(context.Background(), notification.Delivery{
		ID:             "del-1",
		DedupKey:       "evt-1:conv-1:msg-1:acc-1::group:group",
		EventID:        "evt-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		AccountID:      "acc-1",
		Kind:           notification.NotificationKindGroup,
		Reason:         "group",
		Mode:           notification.DeliveryModeInApp,
		State:          notification.DeliveryStateFailed,
		Priority:       10,
		Attempts:       2,
		NextAttemptAt:  now.Add(5 * time.Minute),
		LastAttemptAt:  now.Add(4 * time.Minute),
		LastError:      "initial failure",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	require.NoError(t, err)

	ignored, err := store.SaveDelivery(context.Background(), notification.Delivery{
		ID:             "del-2",
		DedupKey:       existing.DedupKey,
		EventID:        existing.EventID,
		ConversationID: existing.ConversationID,
		MessageID:      existing.MessageID,
		AccountID:      existing.AccountID,
		Kind:           existing.Kind,
		Reason:         existing.Reason,
		Mode:           notification.DeliveryModeInApp,
		State:          notification.DeliveryStateQueued,
		Priority:       1,
		Attempts:       1,
		NextAttemptAt:  now,
		CreatedAt:      now.Add(time.Minute),
		UpdatedAt:      now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, existing.ID, ignored.ID)
	require.Equal(t, 2, ignored.Attempts)
	require.Equal(t, notification.DeliveryStateFailed, ignored.State)
	require.True(t, ignored.NextAttemptAt.Equal(existing.NextAttemptAt))
	require.True(t, ignored.LastAttemptAt.Equal(existing.LastAttemptAt))
	require.Equal(t, "initial failure", ignored.LastError)
}

func TestSaveWorkerCursorPreservesHigherSequence(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)

	existing, err := store.SaveWorkerCursor(context.Background(), notification.WorkerCursor{
		Name:         "conversation_notifications",
		LastSequence: 10,
		UpdatedAt:    now,
	})
	require.NoError(t, err)

	ignored, err := store.SaveWorkerCursor(context.Background(), notification.WorkerCursor{
		Name:         existing.Name,
		LastSequence: 5,
		UpdatedAt:    now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, uint64(10), ignored.LastSequence)
	require.True(t, ignored.UpdatedAt.Equal(existing.UpdatedAt))
}
