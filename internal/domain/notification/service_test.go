package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/notification"
	notificationtest "github.com/dm-vev/zvonilka/internal/domain/notification/teststore"
)

func TestPreferenceAndOverrideRegistry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-1")

	svc := mustNotificationService(t, notificationtest.NewMemoryStore(), identityStore, notification.WithNow(func() time.Time {
		return now
	}))

	loaded, err := svc.PreferenceByAccountID(ctx, "acc-1")
	require.NoError(t, err)
	require.True(t, loaded.Enabled)
	require.True(t, loaded.DirectEnabled)
	require.True(t, loaded.GroupEnabled)
	require.True(t, loaded.ChannelEnabled)
	require.True(t, loaded.MentionEnabled)
	require.True(t, loaded.ReplyEnabled)

	saved, err := svc.SetPreference(ctx, notification.SetPreferenceParams{
		AccountID:      "acc-1",
		Enabled:        true,
		DirectEnabled:  true,
		GroupEnabled:   false,
		ChannelEnabled: true,
		MentionEnabled: true,
		ReplyEnabled:   true,
		QuietHours: notification.QuietHours{
			Enabled:     true,
			StartMinute: 22 * 60,
			EndMinute:   7 * 60,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "UTC", saved.QuietHours.Timezone)
	require.False(t, saved.GroupEnabled)

	override, err := svc.SetConversationOverride(ctx, notification.SetOverrideParams{
		ConversationID: "conv-1",
		AccountID:      "acc-1",
		MentionsOnly:   true,
	})
	require.NoError(t, err)
	require.True(t, override.MentionsOnly)

	loadedOverride, err := svc.ConversationOverrideByConversationAndAccount(ctx, "conv-1", "acc-1")
	require.NoError(t, err)
	require.True(t, loadedOverride.MentionsOnly)
}

func TestPushTokenRegistryAndRevocation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-1")

	svc := mustNotificationService(t, notificationtest.NewMemoryStore(), identityStore, notification.WithNow(func() time.Time {
		return now
	}))

	saved, err := svc.RegisterPushToken(ctx, notification.RegisterPushTokenParams{
		AccountID: "acc-1",
		DeviceID:  "dev-1",
		Provider:  "APNS",
		Token:     "token-1",
		Platform:  identity.DevicePlatformIOS,
	})
	require.NoError(t, err)
	require.Equal(t, "apns", saved.Provider)

	tokens, err := svc.PushTokensByAccountID(ctx, "acc-1")
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, saved.ID, tokens[0].ID)

	revoked, err := svc.RevokePushToken(ctx, notification.RevokePushTokenParams{TokenID: saved.ID})
	require.NoError(t, err)
	require.False(t, revoked.Enabled)
	require.False(t, revoked.RevokedAt.IsZero())

	tokens, err = svc.PushTokensByAccountID(ctx, "acc-1")
	require.NoError(t, err)
	require.Empty(t, tokens)
}

func TestDeliveryRetrySemantics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-1")

	svc := mustNotificationService(t, notificationtest.NewMemoryStore(), identityStore, notification.WithNow(func() time.Time {
		return now
	}), notification.WithSettings(notification.Settings{
		WorkerPollInterval:  time.Second,
		RetryInitialBackoff: time.Second,
		RetryMaxBackoff:     4 * time.Second,
		MaxAttempts:         2,
		BatchSize:           10,
	}))

	queued, err := svc.QueueDelivery(ctx, notification.QueueDeliveryParams{
		DedupKey:       "evt-1:conv-1:msg-1:acc-1:dev-1:direct:direct",
		EventID:        "evt-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		AccountID:      "acc-1",
		DeviceID:       "dev-1",
		Kind:           notification.NotificationKindDirect,
		Reason:         "direct",
		Mode:           notification.DeliveryModeInApp,
		State:          notification.DeliveryStateQueued,
		Priority:       10,
	})
	require.NoError(t, err)

	duplicate, err := svc.QueueDelivery(ctx, notification.QueueDeliveryParams{
		DedupKey:       queued.DedupKey,
		EventID:        queued.EventID,
		ConversationID: queued.ConversationID,
		MessageID:      queued.MessageID,
		AccountID:      queued.AccountID,
		DeviceID:       queued.DeviceID,
		Kind:           queued.Kind,
		Reason:         queued.Reason,
		Mode:           queued.Mode,
		State:          notification.DeliveryStateQueued,
		Priority:       queued.Priority,
	})
	require.NoError(t, err)
	require.Equal(t, queued.ID, duplicate.ID)

	retried, err := svc.RetryDelivery(ctx, notification.RetryDeliveryParams{
		DeliveryID: queued.ID,
		LastError:  "push failed",
	})
	require.NoError(t, err)
	require.Equal(t, 1, retried.Attempts)
	require.Equal(t, notification.DeliveryStateQueued, retried.State)
	require.Equal(t, now.Add(time.Second), retried.NextAttemptAt)

	failed, err := svc.RetryDelivery(ctx, notification.RetryDeliveryParams{
		DeliveryID: queued.ID,
		LastError:  "push failed again",
	})
	require.NoError(t, err)
	require.Equal(t, 2, failed.Attempts)
	require.Equal(t, notification.DeliveryStateFailed, failed.State)
	require.False(t, failed.NextAttemptAt.IsZero())
}

func TestDeliveriesDueOrderingAndLimit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-1")

	svc := mustNotificationService(t, notificationtest.NewMemoryStore(), identityStore, notification.WithNow(func() time.Time {
		return now
	}))

	first, err := svc.QueueDelivery(ctx, notification.QueueDeliveryParams{
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
		NextAttemptAt:  now.Add(10 * time.Second),
	})
	require.NoError(t, err)

	_, err = svc.QueueDelivery(ctx, notification.QueueDeliveryParams{
		DedupKey:       "evt-2:conv-1:msg-2:acc-1::mention:mention",
		EventID:        "evt-2",
		ConversationID: "conv-1",
		MessageID:      "msg-2",
		AccountID:      "acc-1",
		Kind:           notification.NotificationKindMention,
		Reason:         "mention",
		Mode:           notification.DeliveryModeInApp,
		State:          notification.DeliveryStateQueued,
		Priority:       100,
		NextAttemptAt:  now.Add(20 * time.Second),
	})
	require.NoError(t, err)

	limited, err := svc.DeliveriesDue(ctx, now.Add(30*time.Second), 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	require.Equal(t, "evt-2:conv-1:msg-2:acc-1::mention:mention", limited[0].ID)

	all, err := svc.DeliveriesDue(ctx, now.Add(30*time.Second), 10)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Equal(t, "evt-2:conv-1:msg-2:acc-1::mention:mention", all[0].ID)
	require.Equal(t, first.ID, all[1].ID)
}

func TestQueueDeliveryRejectsPushWithoutRoutingFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-1")

	svc := mustNotificationService(t, notificationtest.NewMemoryStore(), identityStore, notification.WithNow(func() time.Time {
		return now
	}))

	_, err := svc.QueueDelivery(ctx, notification.QueueDeliveryParams{
		DedupKey:       "evt-1:conv-1:msg-1:acc-1::group:group",
		EventID:        "evt-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		AccountID:      "acc-1",
		Kind:           notification.NotificationKindGroup,
		Reason:         "group",
		Mode:           notification.DeliveryModePush,
		State:          notification.DeliveryStateQueued,
		Priority:       10,
	})
	require.ErrorIs(t, err, notification.ErrInvalidInput)
}

func TestDeliveryClaimAndCompletionLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-1")

	svc := mustNotificationService(t, notificationtest.NewMemoryStore(), identityStore, notification.WithNow(func() time.Time {
		return now
	}), notification.WithSettings(notification.Settings{
		WorkerPollInterval:  time.Second,
		RetryInitialBackoff: time.Second,
		RetryMaxBackoff:     4 * time.Second,
		DeliveryLeaseTTL:    30 * time.Second,
		MaxAttempts:         3,
		BatchSize:           10,
	}))

	queued, err := svc.QueueDelivery(ctx, notification.QueueDeliveryParams{
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
	require.Equal(t, queued.DedupKey, queued.ID)

	claimed, err := svc.ClaimDeliveries(ctx, notification.ClaimDeliveriesParams{
		Before:        now,
		Limit:         10,
		LeaseDuration: 30 * time.Second,
	})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.NotEmpty(t, claimed[0].LeaseToken)
	require.True(t, claimed[0].LeaseExpiresAt.Equal(now.Add(30*time.Second)))

	due, err := svc.DeliveriesDue(ctx, now, 10)
	require.NoError(t, err)
	require.Empty(t, due)

	delivered, err := svc.MarkDeliveryDelivered(ctx, notification.MarkDeliveryDeliveredParams{
		DeliveryID: claimed[0].ID,
		LeaseToken: claimed[0].LeaseToken,
	})
	require.NoError(t, err)
	require.Equal(t, notification.DeliveryStateDelivered, delivered.State)
	require.Equal(t, 1, delivered.Attempts)
	require.True(t, delivered.LeaseExpiresAt.IsZero())
	require.Empty(t, delivered.LeaseToken)
}

func TestDeliveryClaimFailurePaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	identityStore := identitytest.NewMemoryStore()
	seedActiveAccount(t, identityStore, "acc-1")

	svc := mustNotificationService(t, notificationtest.NewMemoryStore(), identityStore, notification.WithNow(func() time.Time {
		return now
	}), notification.WithSettings(notification.Settings{
		WorkerPollInterval:  time.Second,
		RetryInitialBackoff: time.Second,
		RetryMaxBackoff:     4 * time.Second,
		DeliveryLeaseTTL:    30 * time.Second,
		MaxAttempts:         2,
		BatchSize:           10,
	}))

	queued, err := svc.QueueDelivery(ctx, notification.QueueDeliveryParams{
		DedupKey:       "evt-1:conv-1:msg-1:acc-1::direct:direct",
		EventID:        "evt-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		AccountID:      "acc-1",
		Kind:           notification.NotificationKindDirect,
		Reason:         "direct",
		Mode:           notification.DeliveryModeInApp,
		State:          notification.DeliveryStateQueued,
		Priority:       10,
	})
	require.NoError(t, err)

	claimed, err := svc.ClaimDeliveries(ctx, notification.ClaimDeliveriesParams{
		Before:        now,
		Limit:         10,
		LeaseDuration: 30 * time.Second,
	})
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	retried, err := svc.RetryDelivery(ctx, notification.RetryDeliveryParams{
		DeliveryID: claimed[0].ID,
		LeaseToken: claimed[0].LeaseToken,
		LastError:  "temporary outage",
	})
	require.NoError(t, err)
	require.Equal(t, notification.DeliveryStateQueued, retried.State)
	require.Equal(t, 1, retried.Attempts)
	require.True(t, retried.LeaseExpiresAt.IsZero())
	require.True(t, retried.NextAttemptAt.Equal(now.Add(time.Second)))

	reclaimed, err := svc.ClaimDeliveries(ctx, notification.ClaimDeliveriesParams{
		Before:        now.Add(time.Second),
		Limit:         10,
		LeaseDuration: 30 * time.Second,
	})
	require.NoError(t, err)
	require.Len(t, reclaimed, 1)
	require.Equal(t, queued.ID, reclaimed[0].ID)

	failed, err := svc.FailDelivery(ctx, notification.FailDeliveryParams{
		DeliveryID: reclaimed[0].ID,
		LeaseToken: reclaimed[0].LeaseToken,
		LastError:  "permanent failure",
	})
	require.NoError(t, err)
	require.Equal(t, notification.DeliveryStateFailed, failed.State)
	require.Equal(t, 2, failed.Attempts)
}

func mustNotificationService(t *testing.T, store notification.Store, identityStore identity.Store, opts ...notification.Option) *notification.Service {
	t.Helper()

	svc, err := notification.NewService(store, identityStore, opts...)
	require.NoError(t, err)

	return svc
}

func seedActiveAccount(t *testing.T, store identity.Store, accountID string) {
	t.Helper()

	_, err := store.SaveAccount(context.Background(), identity.Account{
		ID:          accountID,
		Kind:        identity.AccountKindUser,
		Username:    accountID,
		DisplayName: "Test Account",
		Status:      identity.AccountStatusActive,
		CreatedAt:   time.Unix(1, 0).UTC(),
		UpdatedAt:   time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
}
