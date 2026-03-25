package notification_test

import (
	"context"
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
