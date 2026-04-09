package federation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

const (
	federationOriginServerMetadataKey = "federation_origin_server"

	federatedEventIDPrefix        = "fedevt:"
	federatedConversationIDPrefix = "fedconv:"
	federatedAccountIDPrefix      = "fedacct:"
	federatedDeviceIDPrefix       = "feddev:"
	federatedMessageIDPrefix      = "fedmsg:"
	federatedTopicIDPrefix        = "fedtopic:"
)

// IdentityStore provisions shadow actors required by federated conversation state.
type IdentityStore interface {
	AccountByID(ctx context.Context, accountID string) (identity.Account, error)
	SaveAccount(ctx context.Context, account identity.Account) (identity.Account, error)
	DeviceByID(ctx context.Context, deviceID string) (identity.Device, error)
	SaveDevice(ctx context.Context, device identity.Device) (identity.Device, error)
}

func normalizeInboundEvent(
	serverName string,
	event conversation.EventEnvelope,
	now time.Time,
) conversation.EventEnvelope {
	serverName = strings.TrimSpace(strings.ToLower(serverName))
	normalized := event
	normalized.EventID = mapFederatedID(federatedEventIDPrefix, serverName, normalized.EventID)
	normalized.ConversationID = mapFederatedID(federatedConversationIDPrefix, serverName, normalized.ConversationID)
	normalized.ActorAccountID = mapFederatedID(
		federatedAccountIDPrefix,
		serverName,
		defaultFederatedActorAccountID(normalized.ActorAccountID),
	)
	normalized.ActorDeviceID = mapFederatedID(
		federatedDeviceIDPrefix,
		serverName,
		defaultFederatedActorDeviceID(normalized.ActorDeviceID),
	)
	normalized.MessageID = mapOptionalFederatedID(federatedMessageIDPrefix, serverName, normalized.MessageID)
	normalized.Metadata = normalizeInboundMetadata(serverName, normalized.Metadata)
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = now.UTC()
	}

	return normalized
}

func normalizeInboundMetadata(serverName string, metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return map[string]string{
			federationOriginServerMetadataKey: strings.TrimSpace(strings.ToLower(serverName)),
		}
	}

	normalized := make(map[string]string, len(metadata)+1)
	for key, value := range metadata {
		trimmedValue := strings.TrimSpace(value)
		switch key {
		case "message_id", "reply_message_id":
			normalized[key] = mapOptionalFederatedID(federatedMessageIDPrefix, serverName, trimmedValue)
		case "conversation_id", "reply_conversation_id":
			normalized[key] = mapOptionalFederatedID(federatedConversationIDPrefix, serverName, trimmedValue)
		case "thread_id", "topic_id":
			normalized[key] = mapOptionalFederatedTopicID(serverName, trimmedValue)
		case "reply_sender_account_id", "target_account_id":
			normalized[key] = mapOptionalFederatedID(federatedAccountIDPrefix, serverName, trimmedValue)
		case "target_account_ids":
			normalized[key] = strings.Join(mapFederatedIDs(serverName, trimmedValue, federatedAccountIDPrefix), ",")
		default:
			normalized[key] = trimmedValue
		}
	}
	if strings.TrimSpace(normalized[federationOriginServerMetadataKey]) == "" {
		normalized[federationOriginServerMetadataKey] = strings.TrimSpace(strings.ToLower(serverName))
	}

	return normalized
}

func ensureFederatedShadowState(
	ctx context.Context,
	identities IdentityStore,
	event conversation.EventEnvelope,
) error {
	if identities == nil {
		return ErrInvalidInput
	}
	if err := ensureFederatedAccount(ctx, identities, event.ActorAccountID, event.CreatedAt); err != nil {
		return err
	}
	if err := ensureFederatedDevice(ctx, identities, event.ActorAccountID, event.ActorDeviceID, event.CreatedAt); err != nil {
		return err
	}

	targetAccounts := memberTargetAccounts(event.Metadata)
	for _, accountID := range targetAccounts {
		if err := ensureFederatedAccount(ctx, identities, accountID, event.CreatedAt); err != nil {
			return err
		}
	}

	return nil
}

func ensureFederatedAccount(
	ctx context.Context,
	identities IdentityStore,
	accountID string,
	createdAt time.Time,
) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}

	if _, err := identities.AccountByID(ctx, accountID); err == nil {
		return nil
	} else if !errors.Is(err, identity.ErrNotFound) {
		return fmt.Errorf("load federated account %s: %w", accountID, err)
	}

	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	account := identity.Account{
		ID:          accountID,
		Kind:        identity.AccountKindUser,
		Username:    accountID,
		DisplayName: accountID,
		Status:      identity.AccountStatusActive,
		CreatedBy:   "federation",
		CreatedAt:   createdAt.UTC(),
		UpdatedAt:   createdAt.UTC(),
	}
	if _, err := identities.SaveAccount(ctx, account); err != nil {
		return fmt.Errorf("save federated account %s: %w", accountID, err)
	}

	return nil
}

func ensureFederatedDevice(
	ctx context.Context,
	identities IdentityStore,
	accountID string,
	deviceID string,
	createdAt time.Time,
) error {
	accountID = strings.TrimSpace(accountID)
	deviceID = strings.TrimSpace(deviceID)
	if accountID == "" || deviceID == "" {
		return nil
	}

	if _, err := identities.DeviceByID(ctx, deviceID); err == nil {
		return nil
	} else if !errors.Is(err, identity.ErrNotFound) {
		return fmt.Errorf("load federated device %s: %w", deviceID, err)
	}

	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	device := identity.Device{
		ID:         deviceID,
		AccountID:  accountID,
		Name:       deviceID,
		Platform:   identity.DevicePlatformServer,
		Status:     identity.DeviceStatusActive,
		CreatedAt:  createdAt.UTC(),
		LastSeenAt: createdAt.UTC(),
	}
	if _, err := identities.SaveDevice(ctx, device); err != nil {
		return fmt.Errorf("save federated device %s: %w", deviceID, err)
	}

	return nil
}

func materializeFederatedEvent(
	ctx context.Context,
	store conversation.Store,
	event conversation.EventEnvelope,
	savedEvent conversation.EventEnvelope,
	conversationRow conversation.Conversation,
) error {
	if err := ensureFederatedConversationMember(
		ctx,
		store,
		conversationRow.ID,
		conversationRow.OwnerAccountID,
		conversation.MemberRoleOwner,
		conversationRow.CreatedAt,
	); err != nil {
		return err
	}

	switch event.EventType {
	case conversation.EventTypeConversationCreated:
		return ensureFederatedConversationMember(
			ctx,
			store,
			conversationRow.ID,
			event.ActorAccountID,
			conversation.MemberRoleOwner,
			event.CreatedAt,
		)
	case conversation.EventTypeConversationMembers:
		return materializeFederatedMemberChange(ctx, store, event)
	case conversation.EventTypeMessageCreated,
		conversation.EventTypeMessageEdited,
		conversation.EventTypeMessageDeleted,
		conversation.EventTypeMessagePinned:
		return materializeFederatedMessage(ctx, store, event, savedEvent)
	case conversation.EventTypeMessageReactionAdded,
		conversation.EventTypeMessageReactionUpdated,
		conversation.EventTypeMessageReactionRemoved:
		return materializeFederatedReaction(ctx, store, event)
	default:
		return nil
	}
}

func ensureFederatedConversationMember(
	ctx context.Context,
	store conversation.Store,
	conversationID string,
	accountID string,
	role conversation.MemberRole,
	joinedAt time.Time,
) error {
	conversationID = strings.TrimSpace(conversationID)
	accountID = strings.TrimSpace(accountID)
	if conversationID == "" || accountID == "" || role == conversation.MemberRoleUnspecified {
		return nil
	}

	member, err := store.ConversationMemberByConversationAndAccount(ctx, conversationID, accountID)
	if err == nil {
		nextRole := strongerMemberRole(member.Role, role)
		if !member.LeftAt.IsZero() || member.Banned || member.Role != nextRole {
			member.Role = nextRole
			member.Banned = false
			member.LeftAt = time.Time{}
			member.JoinedAt = minNonZeroTime(member.JoinedAt, joinedAt)
			if _, err := store.SaveConversationMember(ctx, member); err != nil {
				return fmt.Errorf("save federated member %s/%s: %w", conversationID, accountID, err)
			}
		}
		return nil
	}
	if !errors.Is(err, conversation.ErrNotFound) {
		return fmt.Errorf("load federated member %s/%s: %w", conversationID, accountID, err)
	}

	if joinedAt.IsZero() {
		joinedAt = time.Now().UTC()
	}
	member = conversation.ConversationMember{
		ConversationID: conversationID,
		AccountID:      accountID,
		Role:           role,
		JoinedAt:       joinedAt.UTC(),
	}
	if _, err := store.SaveConversationMember(ctx, member); err != nil {
		return fmt.Errorf("create federated member %s/%s: %w", conversationID, accountID, err)
	}

	return nil
}

func ensureFederatedGeneralTopic(
	ctx context.Context,
	store conversation.Store,
	conversationID string,
	createdByAccountID string,
	createdAt time.Time,
) error {
	if _, err := store.TopicByConversationAndID(ctx, conversationID, ""); err == nil {
		return nil
	} else if !errors.Is(err, conversation.ErrNotFound) {
		return fmt.Errorf("load federated general topic for %s: %w", conversationID, err)
	}

	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	topic := conversation.ConversationTopic{
		ConversationID:     conversationID,
		ID:                 "",
		Title:              "General",
		CreatedByAccountID: createdByAccountID,
		IsGeneral:          true,
		CreatedAt:          createdAt.UTC(),
		UpdatedAt:          createdAt.UTC(),
	}
	if _, err := store.SaveTopic(ctx, topic); err != nil {
		return fmt.Errorf("create federated general topic for %s: %w", conversationID, err)
	}

	return nil
}

func materializeFederatedMemberChange(
	ctx context.Context,
	store conversation.Store,
	event conversation.EventEnvelope,
) error {
	targets := memberTargetAccounts(event.Metadata)
	if len(targets) == 0 {
		return nil
	}

	change := strings.TrimSpace(strings.ToLower(event.Metadata["change"]))
	switch change {
	case "added":
		role := federatedMemberRole(event.Metadata["role"])
		if role == conversation.MemberRoleUnspecified {
			role = conversation.MemberRoleMember
		}
		for _, accountID := range targets {
			if err := ensureFederatedConversationMember(
				ctx,
				store,
				event.ConversationID,
				accountID,
				role,
				event.CreatedAt,
			); err != nil {
				return err
			}
		}
	case "removed":
		for _, accountID := range targets {
			member, err := store.ConversationMemberByConversationAndAccount(ctx, event.ConversationID, accountID)
			if err != nil {
				if !errors.Is(err, conversation.ErrNotFound) {
					return fmt.Errorf("load federated member %s/%s: %w", event.ConversationID, accountID, err)
				}
				member = conversation.ConversationMember{
					ConversationID: event.ConversationID,
					AccountID:      accountID,
					Role:           conversation.MemberRoleMember,
					JoinedAt:       event.CreatedAt.UTC(),
				}
			}
			member.LeftAt = event.CreatedAt.UTC()
			member.Banned = false
			member.Muted = false
			if _, err := store.SaveConversationMember(ctx, member); err != nil {
				return fmt.Errorf("remove federated member %s/%s: %w", event.ConversationID, accountID, err)
			}
		}
	case "role_updated":
		role := federatedMemberRole(event.Metadata["role"])
		if role == conversation.MemberRoleUnspecified {
			role = conversation.MemberRoleMember
		}
		for _, accountID := range targets {
			if err := ensureFederatedConversationMember(
				ctx,
				store,
				event.ConversationID,
				accountID,
				role,
				event.CreatedAt,
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func materializeFederatedMessage(
	ctx context.Context,
	store conversation.Store,
	event conversation.EventEnvelope,
	savedEvent conversation.EventEnvelope,
) error {
	if strings.TrimSpace(event.MessageID) == "" {
		return nil
	}

	if err := ensureFederatedConversationMember(
		ctx,
		store,
		event.ConversationID,
		event.ActorAccountID,
		conversation.MemberRoleMember,
		event.CreatedAt,
	); err != nil {
		return err
	}

	current, err := store.MessageByID(ctx, event.ConversationID, event.MessageID)
	if err != nil && !errors.Is(err, conversation.ErrNotFound) {
		return fmt.Errorf("load federated message %s in %s: %w", event.MessageID, event.ConversationID, err)
	}
	missing := errors.Is(err, conversation.ErrNotFound)

	switch event.EventType {
	case conversation.EventTypeMessageCreated, conversation.EventTypeMessageEdited:
		message := current
		if missing {
			message = conversation.Message{
				ID:             event.MessageID,
				ConversationID: event.ConversationID,
				CreatedAt:      event.CreatedAt.UTC(),
			}
		}
		if message.CreatedAt.IsZero() {
			message.CreatedAt = event.CreatedAt.UTC()
		}
		message.SenderAccountID = event.ActorAccountID
		message.SenderDeviceID = event.ActorDeviceID
		message.Sequence = savedEvent.Sequence
		message.Kind = federatedMessageKind(event.Metadata["message_kind"])
		if message.Kind == conversation.MessageKindUnspecified {
			message.Kind = conversation.MessageKindText
		}
		message.Status = conversation.MessageStatusSent
		message.Payload = event.Payload
		message.ReplyTo = federatedReplyReference(event.Metadata)
		message.ThreadID = federatedTopicID(event)
		message.DisableLinkPreviews = metadataBool(event.Metadata, "disable_link_previews")
		message.Metadata = federatedMessageMetadata(event.Metadata)
		message.UpdatedAt = savedEvent.CreatedAt.UTC()
		if event.EventType == conversation.EventTypeMessageEdited {
			message.EditedAt = savedEvent.CreatedAt.UTC()
		}
		if _, err := store.SaveMessage(ctx, message); err != nil {
			return fmt.Errorf("save federated message %s in %s: %w", message.ID, message.ConversationID, err)
		}
	case conversation.EventTypeMessageDeleted:
		if missing {
			return nil
		}
		current.Status = conversation.MessageStatusDeleted
		current.Pinned = false
		current.Sequence = savedEvent.Sequence
		current.UpdatedAt = savedEvent.CreatedAt.UTC()
		current.DeletedAt = savedEvent.CreatedAt.UTC()
		if _, err := store.SaveMessage(ctx, current); err != nil {
			return fmt.Errorf("delete federated message %s in %s: %w", current.ID, current.ConversationID, err)
		}
	case conversation.EventTypeMessagePinned:
		if missing {
			return nil
		}
		current.Pinned = metadataBool(event.Metadata, "pinned")
		current.UpdatedAt = savedEvent.CreatedAt.UTC()
		if _, err := store.SaveMessage(ctx, current); err != nil {
			return fmt.Errorf("pin federated message %s in %s: %w", current.ID, current.ConversationID, err)
		}
	}

	return nil
}

func materializeFederatedReaction(
	ctx context.Context,
	store conversation.Store,
	event conversation.EventEnvelope,
) error {
	if strings.TrimSpace(event.MessageID) == "" {
		return nil
	}
	if _, err := store.MessageByID(ctx, event.ConversationID, event.MessageID); err != nil {
		if errors.Is(err, conversation.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("load federated message %s in %s for reaction: %w", event.MessageID, event.ConversationID, err)
	}

	switch event.EventType {
	case conversation.EventTypeMessageReactionAdded, conversation.EventTypeMessageReactionUpdated:
		reactionValue := strings.TrimSpace(event.Metadata["reaction"])
		if reactionValue == "" {
			return nil
		}
		createdAt := event.CreatedAt.UTC()
		reaction := conversation.MessageReaction{
			MessageID: event.MessageID,
			AccountID: event.ActorAccountID,
			Reaction:  reactionValue,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}
		if _, err := store.SaveMessageReaction(ctx, reaction); err != nil {
			return fmt.Errorf("save federated reaction %s/%s: %w", reaction.MessageID, reaction.AccountID, err)
		}
	case conversation.EventTypeMessageReactionRemoved:
		if err := store.DeleteMessageReaction(ctx, event.MessageID, event.ActorAccountID); err != nil {
			return fmt.Errorf("delete federated reaction %s/%s: %w", event.MessageID, event.ActorAccountID, err)
		}
	}

	return nil
}

func memberTargetAccounts(metadata map[string]string) []string {
	targets := make([]string, 0)
	seen := make(map[string]struct{})
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		targets = append(targets, value)
	}

	add(metadata["target_account_id"])
	for _, value := range strings.Split(strings.TrimSpace(metadata["target_account_ids"]), ",") {
		add(value)
	}

	return targets
}

func federatedReplyReference(metadata map[string]string) conversation.MessageReference {
	reference := conversation.MessageReference{
		ConversationID:  strings.TrimSpace(metadata["reply_conversation_id"]),
		MessageID:       strings.TrimSpace(metadata["reply_message_id"]),
		SenderAccountID: strings.TrimSpace(metadata["reply_sender_account_id"]),
		MessageKind:     federatedMessageKind(metadata["reply_kind"]),
	}

	return reference
}

func federatedMessageKind(value string) conversation.MessageKind {
	switch conversation.MessageKind(strings.TrimSpace(strings.ToLower(value))) {
	case conversation.MessageKindText,
		conversation.MessageKindImage,
		conversation.MessageKindVideo,
		conversation.MessageKindDocument,
		conversation.MessageKindVoice,
		conversation.MessageKindSticker,
		conversation.MessageKindGIF,
		conversation.MessageKindSystem:
		return conversation.MessageKind(strings.TrimSpace(strings.ToLower(value)))
	default:
		return conversation.MessageKindUnspecified
	}
}

func federatedMemberRole(value string) conversation.MemberRole {
	switch conversation.MemberRole(strings.TrimSpace(strings.ToLower(value))) {
	case conversation.MemberRoleOwner,
		conversation.MemberRoleAdmin,
		conversation.MemberRoleMember,
		conversation.MemberRoleGuest:
		return conversation.MemberRole(strings.TrimSpace(strings.ToLower(value)))
	default:
		return conversation.MemberRoleUnspecified
	}
}

func federatedMessageMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	filtered := make(map[string]string)
	for key, value := range metadata {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case federationOriginServerMetadataKey,
			"message_id",
			"thread_id",
			"topic_id",
			"reply_message_id",
			"reply_conversation_id",
			"reply_sender_account_id",
			"reply_kind",
			"message_kind",
			"disable_link_previews",
			"action",
			"reaction",
			"pinned":
			continue
		default:
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}

	return filtered
}

func mapFederatedIDs(serverName string, raw string, prefix string) []string {
	values := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		mapped := mapOptionalFederatedID(prefix, serverName, part)
		if mapped == "" {
			continue
		}
		values = append(values, mapped)
	}

	return values
}

func defaultFederatedActorAccountID(value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}

	return "system"
}

func defaultFederatedActorDeviceID(value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}

	return "system"
}

func mapOptionalFederatedID(prefix string, serverName string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	return mapFederatedID(prefix, serverName, value)
}

func mapOptionalFederatedTopicID(serverName string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	return mapFederatedID(federatedTopicIDPrefix, serverName, value)
}

func mapFederatedID(prefix string, serverName string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, prefix) {
		return value
	}

	return prefix + strings.TrimSpace(strings.ToLower(serverName)) + ":" + value
}

func minNonZeroTime(left time.Time, right time.Time) time.Time {
	if left.IsZero() {
		return right.UTC()
	}
	if right.IsZero() || left.Before(right) {
		return left.UTC()
	}

	return right.UTC()
}

func strongerMemberRole(left conversation.MemberRole, right conversation.MemberRole) conversation.MemberRole {
	if memberRoleRank(left) >= memberRoleRank(right) {
		return left
	}

	return right
}

func memberRoleRank(role conversation.MemberRole) int {
	switch role {
	case conversation.MemberRoleOwner:
		return 4
	case conversation.MemberRoleAdmin:
		return 3
	case conversation.MemberRoleMember:
		return 2
	case conversation.MemberRoleGuest:
		return 1
	default:
		return 0
	}
}
