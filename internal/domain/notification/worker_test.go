package notification_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/notification"
	notificationtest "github.com/dm-vev/zvonilka/internal/domain/notification/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
	presencetest "github.com/dm-vev/zvonilka/internal/domain/presence/teststore"
)

func TestWorkerProcessesMessageCreatedEvents(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-sender")
	seedActiveAccount(t, identityStore, "acc-online")
	seedActiveAccount(t, identityStore, "acc-push")

	conversations := conversationtest.NewMemoryStore()
	require.NoError(t, seedConversationEventFixture(ctx, conversations, now))

	presenceStore := presencetest.NewMemoryStore()
	presenceSvc, err := presence.NewService(
		presenceStore,
		identityStore,
		presence.WithNow(func() time.Time { return now }),
	)
	require.NoError(t, err)
	_, err = presenceSvc.SetPresence(ctx, presence.SetParams{
		AccountID: "acc-online",
		State:     presence.PresenceStateOnline,
	})
	require.NoError(t, err)

	notificationStore := notificationtest.NewMemoryStore()
	notificationSvc := mustNotificationService(t, notificationStore, identityStore, notification.WithNow(func() time.Time {
		return now
	}), notification.WithSettings(notification.Settings{
		WorkerPollInterval:  time.Hour,
		RetryInitialBackoff: time.Second,
		RetryMaxBackoff:     4 * time.Second,
		MaxAttempts:         3,
		BatchSize:           10,
	}))
	_, err = notificationSvc.RegisterPushToken(ctx, notification.RegisterPushTokenParams{
		AccountID: "acc-push",
		DeviceID:  "dev-push",
		Provider:  "apns",
		Token:     "token-push",
		Platform:  identity.DevicePlatformIOS,
	})
	require.NoError(t, err)

	worker, err := notification.NewWorker(notificationSvc, conversations, presenceSvc)
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()

	require.Eventually(t, func() bool {
		cursor, err := notificationSvc.WorkerCursorByName(ctx, "conversation_notifications")
		if err != nil {
			return false
		}
		return cursor.LastSequence == 2
	}, 2*time.Second, 10*time.Millisecond)

	onlineDelivery, err := notificationSvc.DeliveryByID(ctx, "evt-1:conv-1:msg-1:acc-online::group:group")
	require.NoError(t, err)
	require.Equal(t, notification.DeliveryStateQueued, onlineDelivery.State)
	require.Equal(t, notification.DeliveryModeInApp, onlineDelivery.Mode)

	pushDelivery, err := notificationSvc.DeliveryByID(ctx, "evt-1:conv-1:msg-1:acc-push:dev-push:group:group")
	require.NoError(t, err)
	require.Equal(t, notification.DeliveryStateQueued, pushDelivery.State)
	require.Equal(t, notification.DeliveryModePush, pushDelivery.Mode)

	cancel()
	require.NoError(t, <-errCh)
}

func TestWorkerDispatchesClaimedDeliveries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-1")

	presenceStore := presencetest.NewMemoryStore()
	presenceSvc, err := presence.NewService(
		presenceStore,
		identityStore,
		presence.WithNow(func() time.Time { return now }),
	)
	require.NoError(t, err)

	notificationSvc := mustNotificationService(t, notificationtest.NewMemoryStore(), identityStore, notification.WithNow(func() time.Time {
		return now
	}), notification.WithSettings(notification.Settings{
		WorkerPollInterval:  time.Second,
		RetryInitialBackoff: time.Second,
		RetryMaxBackoff:     4 * time.Second,
		DeliveryLeaseTTL:    30 * time.Second,
		MaxAttempts:         3,
		BatchSize:           10,
	}))

	queued, err := notificationSvc.QueueDelivery(ctx, notification.QueueDeliveryParams{
		DedupKey:       "evt-1:conv-1:msg-1:acc-1::group:group",
		EventID:        "evt-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		AccountID:      "acc-1",
		Kind:           notification.NotificationKindGroup,
		Reason:         "group",
		Mode:           notification.DeliveryModeInApp,
		State:          notification.DeliveryStateQueued,
		Priority:       10,
	})
	require.NoError(t, err)

	executor := &fakeExecutor{}
	worker, err := notification.NewWorker(
		notificationSvc,
		conversationtest.NewMemoryStore(),
		presenceSvc,
		notification.WithDeliveryExecutor(executor),
	)
	require.NoError(t, err)

	require.NoError(t, worker.ProcessOnceForTests(ctx))
	require.Len(t, executor.calls, 1)
	require.Equal(t, queued.ID, executor.calls[0].delivery.ID)
	require.Nil(t, executor.calls[0].target.PushToken)

	delivered, err := notificationSvc.DeliveryByID(ctx, queued.ID)
	require.NoError(t, err)
	require.Equal(t, notification.DeliveryStateDelivered, delivered.State)
	require.Equal(t, 1, delivered.Attempts)
}

func TestWorkerRetriesTransientDispatchFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-1")

	presenceStore := presencetest.NewMemoryStore()
	presenceSvc, err := presence.NewService(
		presenceStore,
		identityStore,
		presence.WithNow(func() time.Time { return now }),
	)
	require.NoError(t, err)

	notificationSvc := mustNotificationService(t, notificationtest.NewMemoryStore(), identityStore, notification.WithNow(func() time.Time {
		return now
	}), notification.WithSettings(notification.Settings{
		WorkerPollInterval:  time.Second,
		RetryInitialBackoff: time.Second,
		RetryMaxBackoff:     4 * time.Second,
		DeliveryLeaseTTL:    30 * time.Second,
		MaxAttempts:         3,
		BatchSize:           10,
	}))

	queued, err := notificationSvc.QueueDelivery(ctx, notification.QueueDeliveryParams{
		DedupKey:       "evt-1:conv-1:msg-1:acc-1::group:group",
		EventID:        "evt-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		AccountID:      "acc-1",
		Kind:           notification.NotificationKindGroup,
		Reason:         "group",
		Mode:           notification.DeliveryModeInApp,
		State:          notification.DeliveryStateQueued,
		Priority:       10,
	})
	require.NoError(t, err)

	worker, err := notification.NewWorker(
		notificationSvc,
		conversationtest.NewMemoryStore(),
		presenceSvc,
		notification.WithDeliveryExecutor(&fakeExecutor{err: errors.New("temporary outage")}),
	)
	require.NoError(t, err)

	require.NoError(t, worker.ProcessOnceForTests(ctx))

	retried, err := notificationSvc.DeliveryByID(ctx, queued.ID)
	require.NoError(t, err)
	require.Equal(t, notification.DeliveryStateQueued, retried.State)
	require.Equal(t, 1, retried.Attempts)
	require.True(t, retried.NextAttemptAt.Equal(now.Add(time.Second)))
}

func TestWorkerFailsPermanentDispatchFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-1")

	presenceStore := presencetest.NewMemoryStore()
	presenceSvc, err := presence.NewService(
		presenceStore,
		identityStore,
		presence.WithNow(func() time.Time { return now }),
	)
	require.NoError(t, err)

	notificationSvc := mustNotificationService(t, notificationtest.NewMemoryStore(), identityStore, notification.WithNow(func() time.Time {
		return now
	}), notification.WithSettings(notification.Settings{
		WorkerPollInterval:  time.Second,
		RetryInitialBackoff: time.Second,
		RetryMaxBackoff:     4 * time.Second,
		DeliveryLeaseTTL:    30 * time.Second,
		MaxAttempts:         3,
		BatchSize:           10,
	}))

	queued, err := notificationSvc.QueueDelivery(ctx, notification.QueueDeliveryParams{
		DedupKey:       "evt-1:conv-1:msg-1:acc-1::group:group",
		EventID:        "evt-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		AccountID:      "acc-1",
		Kind:           notification.NotificationKindGroup,
		Reason:         "group",
		Mode:           notification.DeliveryModeInApp,
		State:          notification.DeliveryStateQueued,
		Priority:       10,
	})
	require.NoError(t, err)

	worker, err := notification.NewWorker(
		notificationSvc,
		conversationtest.NewMemoryStore(),
		presenceSvc,
		notification.WithDeliveryExecutor(&fakeExecutor{
			err: notification.PermanentDeliveryError(errors.New("bad payload")),
		}),
	)
	require.NoError(t, err)

	require.NoError(t, worker.ProcessOnceForTests(ctx))

	failed, err := notificationSvc.DeliveryByID(ctx, queued.ID)
	require.NoError(t, err)
	require.Equal(t, notification.DeliveryStateFailed, failed.State)
	require.Equal(t, 1, failed.Attempts)
}

func seedConversationEventFixture(ctx context.Context, store conversation.Store, now time.Time) error {
	conversationID := "conv-1"
	senderID := "acc-sender"

	_, err := store.SaveConversation(ctx, conversation.Conversation{
		ID:                 conversationID,
		Kind:               conversation.ConversationKindGroup,
		Title:              "Group",
		OwnerAccountID:     senderID,
		Settings:           conversation.ConversationSettings{AllowReactions: true, AllowThreads: true},
		CreatedAt:          now,
		UpdatedAt:          now,
		LastMessageAt:      now,
		LastSequence:       0,
		UnreadCount:        0,
		UnreadMentionCount: 0,
	})
	if err != nil {
		return err
	}

	for _, accountID := range []string{senderID, "acc-online", "acc-push"} {
		if _, err := store.SaveConversationMember(ctx, conversation.ConversationMember{
			ConversationID: conversationID,
			AccountID:      accountID,
			Role:           conversation.MemberRoleMember,
			JoinedAt:       now,
		}); err != nil {
			return err
		}
	}

	message := conversation.Message{
		ID:              "msg-1",
		ConversationID:  conversationID,
		SenderAccountID: senderID,
		SenderDeviceID:  "dev-sender",
		Kind:            conversation.MessageKindText,
		Status:          conversation.MessageStatusSent,
		Payload: conversation.EncryptedPayload{
			Ciphertext: []byte("ciphertext"),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := store.SaveMessage(ctx, message); err != nil {
		return err
	}

	if _, err := store.SaveEvent(ctx, conversation.EventEnvelope{
		EventID:        "evt-1",
		EventType:      conversation.EventTypeMessageCreated,
		ConversationID: conversationID,
		ActorAccountID: senderID,
		ActorDeviceID:  "dev-sender",
		MessageID:      message.ID,
		PayloadType:    "message",
		Payload:        message.Payload,
		CreatedAt:      now,
	}); err != nil {
		return err
	}

	_, err = store.SaveEvent(ctx, conversation.EventEnvelope{
		EventID:        "evt-2",
		EventType:      conversation.EventTypeConversationUpdated,
		ConversationID: conversationID,
		ActorAccountID: senderID,
		CreatedAt:      now,
	})
	return err
}

type fakeExecutor struct {
	err   error
	calls []executorCall
}

type executorCall struct {
	delivery notification.Delivery
	target   notification.DeliveryTarget
}

func (e *fakeExecutor) Deliver(
	_ context.Context,
	delivery notification.Delivery,
	target notification.DeliveryTarget,
) error {
	e.calls = append(e.calls, executorCall{
		delivery: delivery,
		target:   target,
	})

	return e.err
}
