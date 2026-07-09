package notification

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
)

func TestRouteDecisionPrioritizesMentionAndReply(t *testing.T) {
	t.Parallel()

	conv := conversation.Conversation{Kind: conversation.ConversationKindDirect}
	message := conversation.Message{
		SenderAccountID:   "acc-sender",
		MentionAccountIDs: []string{"acc-mention"},
		ReplyTo:           conversation.MessageReference{SenderAccountID: "acc-reply"},
	}

	kind, reason, priority, ok := routeDecision(conv, message, "acc-mention")
	require.True(t, ok)
	require.Equal(t, NotificationKindMention, kind)
	require.Equal(t, "mention", reason)
	require.Equal(t, 100, priority)

	kind, reason, priority, ok = routeDecision(conv, message, "acc-reply")
	require.True(t, ok)
	require.Equal(t, NotificationKindReply, kind)
	require.Equal(t, "reply", reason)
	require.Equal(t, 90, priority)

	kind, reason, priority, ok = routeDecision(conv, message, "acc-other")
	require.True(t, ok)
	require.Equal(t, NotificationKindDirect, kind)
	require.Equal(t, "direct", reason)
	require.Equal(t, 80, priority)
}

func TestBuildDeliveriesAppliesRoutingAndOverrides(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	conv := conversation.Conversation{ID: "conv-1", Kind: conversation.ConversationKindGroup}
	message := conversation.Message{
		ID:                "msg-1",
		ConversationID:    "conv-1",
		SenderAccountID:   "acc-sender",
		MentionAccountIDs: []string{"acc-mention"},
		ReplyTo: conversation.MessageReference{
			SenderAccountID: "acc-reply",
		},
	}
	members := []conversation.ConversationMember{
		{ConversationID: "conv-1", AccountID: "acc-sender", Role: conversation.MemberRoleMember},
		{ConversationID: "conv-1", AccountID: "acc-mention", Role: conversation.MemberRoleMember},
		{ConversationID: "conv-1", AccountID: "acc-reply", Role: conversation.MemberRoleMember},
		{ConversationID: "conv-1", AccountID: "acc-muted", Role: conversation.MemberRoleMember},
	}

	preferences := map[string]Preference{
		"acc-mention": defaultPreference("acc-mention", now),
		"acc-reply":   defaultPreference("acc-reply", now),
		"acc-muted":   defaultPreference("acc-muted", now),
	}
	presences := map[string]presence.Snapshot{
		"acc-mention": {AccountID: "acc-mention", State: presence.PresenceStateOffline},
		"acc-reply":   {AccountID: "acc-reply", State: presence.PresenceStateOnline},
		"acc-muted":   {AccountID: "acc-muted", State: presence.PresenceStateOffline},
	}
	overrides := map[string]ConversationOverride{
		"acc-muted": {
			ConversationID: "conv-1",
			AccountID:      "acc-muted",
			MentionsOnly:   true,
			UpdatedAt:      now,
		},
	}
	tokens := map[string][]PushToken{
		"acc-mention": {
			{
				ID:        "tok-1",
				AccountID: "acc-mention",
				DeviceID:  "dev-1",
				Provider:  "apns",
				Token:     "token-1",
				Platform:  identity.DevicePlatformIOS,
				Enabled:   true,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	deliveries := buildDeliveries(now, "evt-1", conv, message, members, preferences, overrides, presences, tokens)
	require.Len(t, deliveries, 3)

	mentionDelivery := deliveryByAccountID(t, deliveries, "acc-mention")
	require.Equal(t, NotificationKindMention, mentionDelivery.Kind)
	require.Equal(t, "mention", mentionDelivery.Reason)
	require.Equal(t, DeliveryStateQueued, mentionDelivery.State)
	require.Equal(t, DeliveryModePush, mentionDelivery.Mode)

	replyDelivery := deliveryByAccountID(t, deliveries, "acc-reply")
	require.Equal(t, NotificationKindReply, replyDelivery.Kind)
	require.Equal(t, "reply", replyDelivery.Reason)
	require.Equal(t, DeliveryStateQueued, replyDelivery.State)
	require.Equal(t, DeliveryModeInApp, replyDelivery.Mode)

	mutedDelivery := deliveryByAccountID(t, deliveries, "acc-muted")
	require.Equal(t, NotificationKindGroup, mutedDelivery.Kind)
	require.Equal(t, "group", mutedDelivery.Reason)
	require.Equal(t, DeliveryStateSuppressed, mutedDelivery.State)
}

func TestQuietHoursActiveWrapsMidnight(t *testing.T) {
	t.Parallel()

	quietHours := QuietHours{
		Enabled:     true,
		StartMinute: 22 * 60,
		EndMinute:   7 * 60,
		Timezone:    "UTC",
	}

	require.True(t, quietHoursActive(time.Date(2026, time.March, 25, 23, 0, 0, 0, time.UTC), quietHours))
	require.True(t, quietHoursActive(time.Date(2026, time.March, 25, 6, 59, 0, 0, time.UTC), quietHours))
	require.False(t, quietHoursActive(time.Date(2026, time.March, 25, 8, 0, 0, 0, time.UTC), quietHours))
}

func deliveryByAccountID(t *testing.T, deliveries []Delivery, accountID string) Delivery {
	t.Helper()

	for _, delivery := range deliveries {
		if delivery.AccountID == accountID {
			return delivery
		}
	}

	t.Fatalf("delivery for account %s not found", accountID)
	return Delivery{}
}
