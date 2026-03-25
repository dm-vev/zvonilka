package notification

import (
	"sort"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
)

func buildDeliveries(
	now time.Time,
	eventID string,
	conv conversation.Conversation,
	message conversation.Message,
	members []conversation.ConversationMember,
	preferenceByAccountID map[string]Preference,
	overrideByAccountID map[string]ConversationOverride,
	presenceByAccountID map[string]presence.Snapshot,
	pushTokensByAccountID map[string][]PushToken,
) []Delivery {
	recipients := recipientIDs(conv, message, members)
	if len(recipients) == 0 {
		return nil
	}

	deliveries := make([]Delivery, 0)
	for _, accountID := range recipients {
		preference := preferenceByAccountID[accountID]
		if preference.AccountID == "" {
			preference = defaultPreference(accountID, now)
		}

		override := overrideByAccountID[accountID]
		presenceSnapshot := presenceByAccountID[accountID]
		tokens := pushTokensByAccountID[accountID]

		kind, reason, priority, ok := routeDecision(conv, message, accountID)
		if !ok {
			continue
		}
		if !shouldDeliver(preference, override, membershipByAccountID(members, accountID), kind, reason, now) {
			deliveries = append(deliveries, suppressedDelivery(now, eventID, conv, message, accountID, kind, reason, priority))
			continue
		}

		mode := deliveryModeForPresence(presenceSnapshot, tokens)
		if len(tokens) == 0 {
			deliveries = append(deliveries, queuedDelivery(
				now,
				eventID,
				conv,
				message,
				accountID,
				"",
				"",
				kind,
				reason,
				mode,
				priority,
			))
			continue
		}

		for _, token := range tokens {
			deliveries = append(deliveries, queuedDelivery(
				now,
				eventID,
				conv,
				message,
				accountID,
				token.DeviceID,
				token.ID,
				kind,
				reason,
				mode,
				priority,
			))
		}
	}

	return deliveries
}

func recipientIDs(conv conversation.Conversation, message conversation.Message, members []conversation.ConversationMember) []string {
	switch conv.Kind {
	case conversation.ConversationKindSavedMessages:
		return nil
	}

	mentions := make(map[string]struct{}, len(message.MentionAccountIDs))
	for _, mentionAccountID := range message.MentionAccountIDs {
		mentions[mentionAccountID] = struct{}{}
	}

	recipients := make(map[string]struct{}, len(members))
	for _, member := range members {
		if !activeConversationMember(member) || member.AccountID == message.SenderAccountID {
			continue
		}
		recipients[member.AccountID] = struct{}{}
	}

	if len(mentions) > 0 {
		for mentionAccountID := range mentions {
			recipients[mentionAccountID] = struct{}{}
		}
	}
	if replyAccountID := strings.TrimSpace(message.ReplyTo.SenderAccountID); replyAccountID != "" {
		recipients[replyAccountID] = struct{}{}
	}

	ids := make([]string, 0, len(recipients))
	for accountID := range recipients {
		ids = append(ids, accountID)
	}
	sort.Strings(ids)

	return ids
}

func routeDecision(
	conv conversation.Conversation,
	message conversation.Message,
	accountID string,
) (NotificationKind, string, int, bool) {
	if accountID == "" || accountID == message.SenderAccountID {
		return NotificationKindUnspecified, "", 0, false
	}

	if isMentioned(accountID, message.MentionAccountIDs) {
		return NotificationKindMention, "mention", 100, true
	}
	if replyAccountID := strings.TrimSpace(message.ReplyTo.SenderAccountID); replyAccountID != "" && replyAccountID == accountID {
		return NotificationKindReply, "reply", 90, true
	}

	switch conv.Kind {
	case conversation.ConversationKindDirect:
		return NotificationKindDirect, "direct", 80, true
	case conversation.ConversationKindGroup:
		return NotificationKindGroup, "group", 60, true
	case conversation.ConversationKindChannel:
		return NotificationKindChannel, "channel", 50, true
	default:
		return NotificationKindUnspecified, "", 0, false
	}
}

func shouldDeliver(
	preference Preference,
	override ConversationOverride,
	member conversation.ConversationMember,
	kind NotificationKind,
	reason string,
	now time.Time,
) bool {
	if !preference.Enabled {
		return false
	}
	if !activeConversationMember(member) {
		return false
	}

	switch kind {
	case NotificationKindDirect:
		if !preference.DirectEnabled {
			return false
		}
	case NotificationKindGroup:
		if !preference.GroupEnabled {
			return false
		}
	case NotificationKindChannel:
		if !preference.ChannelEnabled {
			return false
		}
	case NotificationKindMention:
		if !preference.MentionEnabled {
			return false
		}
	case NotificationKindReply:
		if !preference.ReplyEnabled {
			return false
		}
	}

	if reason != "mention" && reason != "reply" {
		if !override.MutedUntil.IsZero() && now.Before(override.MutedUntil) {
			return false
		}
		if override.Muted {
			return false
		}
		if override.MentionsOnly {
			return false
		}
		if !preference.MutedUntil.IsZero() && now.Before(preference.MutedUntil) {
			return false
		}
		if quietHoursActive(now, preference.QuietHours) {
			return false
		}
	}

	return true
}

func activeConversationMember(member conversation.ConversationMember) bool {
	return member.LeftAt.IsZero() && !member.Banned
}

func isMentioned(accountID string, mentionAccountIDs []string) bool {
	for _, mentionAccountID := range mentionAccountIDs {
		if mentionAccountID == accountID {
			return true
		}
	}

	return false
}

func deliveryModeForPresence(snapshot presence.Snapshot, tokens []PushToken) DeliveryMode {
	if snapshot.State == presence.PresenceStateOnline || snapshot.State == presence.PresenceStateAway {
		return DeliveryModeInApp
	}
	if len(tokens) > 0 {
		return DeliveryModePush
	}

	return DeliveryModeInApp
}

func quietHoursActive(now time.Time, quietHours QuietHours) bool {
	if !quietHours.Enabled {
		return false
	}
	if quietHours.StartMinute == quietHours.EndMinute {
		return false
	}

	locationName := strings.TrimSpace(quietHours.Timezone)
	if locationName == "" {
		locationName = "UTC"
	}
	location, err := time.LoadLocation(locationName)
	if err != nil {
		location = time.UTC
	}

	local := now.In(location)
	currentMinute := local.Hour()*60 + local.Minute()
	start := quietHours.StartMinute
	end := quietHours.EndMinute
	if start < end {
		return currentMinute >= start && currentMinute < end
	}

	return currentMinute >= start || currentMinute < end
}

func queuedDelivery(
	now time.Time,
	eventID string,
	conv conversation.Conversation,
	message conversation.Message,
	accountID string,
	deviceID string,
	pushTokenID string,
	kind NotificationKind,
	reason string,
	mode DeliveryMode,
	priority int,
) Delivery {
	dedupKey := strings.Join([]string{
		eventID,
		conv.ID,
		message.ID,
		accountID,
		deviceID,
		string(kind),
		reason,
	}, ":")
	return Delivery{
		ID:             dedupKey,
		DedupKey:       dedupKey,
		EventID:        eventID,
		ConversationID: conv.ID,
		MessageID:      message.ID,
		AccountID:      accountID,
		DeviceID:       deviceID,
		PushTokenID:    pushTokenID,
		Kind:           kind,
		Reason:         reason,
		Mode:           mode,
		State:          DeliveryStateQueued,
		Priority:       priority,
		Attempts:       0,
		NextAttemptAt:  now.UTC(),
		CreatedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
	}
}

func suppressedDelivery(
	now time.Time,
	eventID string,
	conv conversation.Conversation,
	message conversation.Message,
	accountID string,
	kind NotificationKind,
	reason string,
	priority int,
) Delivery {
	dedupKey := strings.Join([]string{
		eventID,
		conv.ID,
		message.ID,
		accountID,
		"muted",
		string(kind),
		reason,
	}, ":")
	return Delivery{
		ID:             dedupKey,
		DedupKey:       dedupKey,
		EventID:        eventID,
		ConversationID: conv.ID,
		MessageID:      message.ID,
		AccountID:      accountID,
		Kind:           kind,
		Reason:         reason,
		Mode:           DeliveryModeInApp,
		State:          DeliveryStateSuppressed,
		Priority:       priority,
		CreatedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
	}
}

func membershipByAccountID(members []conversation.ConversationMember, accountID string) conversation.ConversationMember {
	for _, member := range members {
		if member.AccountID == accountID {
			return member
		}
	}

	return conversation.ConversationMember{}
}
