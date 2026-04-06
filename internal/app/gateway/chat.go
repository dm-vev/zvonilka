package gateway

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	e2eev1 "github.com/dm-vev/zvonilka/gen/proto/contracts/e2ee/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	domaine2ee "github.com/dm-vev/zvonilka/internal/domain/e2ee"
	"google.golang.org/grpc/status"
)

// CreateConversation creates a direct chat, group, channel, or saved-messages conversation.
func (a *api) CreateConversation(
	ctx context.Context,
	req *conversationv1.CreateConversationRequest,
) (*conversationv1.CreateConversationResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	conversationRow, _, err := a.conversation.CreateConversation(ctx, domainconversation.CreateConversationParams{
		OwnerAccountID:   authContext.Account.ID,
		Kind:             conversationKindFromProto(req.GetKind()),
		Title:            req.GetTitle(),
		Description:      req.GetDescription(),
		AvatarMediaID:    req.GetAvatarMediaId(),
		MemberAccountIDs: req.GetMemberUserIds(),
		Settings:         conversationSettingsFromProto(req.GetSettings()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	overlays, err := a.conversationE2EEOverlays(ctx, authContext.Account.ID, authContext.Session.DeviceID)
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	return &conversationv1.CreateConversationResponse{
		Conversation: conversationProto(conversationRow, overlays[conversationRow.ID]),
	}, nil
}

// GetConversation returns one conversation and its current members.
func (a *api) GetConversation(
	ctx context.Context,
	req *conversationv1.GetConversationRequest,
) (*conversationv1.GetConversationResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	conversationRow, members, err := a.conversation.GetConversation(ctx, domainconversation.GetConversationParams{
		ConversationID: req.GetConversationId(),
		AccountID:      authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	memberProfiles, err := a.memberProfiles(ctx, members, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}
	overlays, err := a.conversationE2EEOverlays(ctx, authContext.Account.ID, authContext.Session.DeviceID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.GetConversationResponse{
		Conversation: conversationProto(conversationRow, overlays[conversationRow.ID]),
		Members:      membersProto(members, memberProfiles),
	}, nil
}

// ListConversations returns the authenticated account's conversations.
func (a *api) ListConversations(
	ctx context.Context,
	req *conversationv1.ListConversationsRequest,
) (*conversationv1.ListConversationsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	conversations, err := a.conversation.ListConversations(ctx, domainconversation.ListConversationsParams{
		AccountID:       authContext.Account.ID,
		IncludeArchived: req.GetIncludeArchived(),
		IncludeMuted:    true,
		IncludeHidden:   req.GetIncludeHidden(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	overlays, err := a.conversationE2EEOverlays(ctx, authContext.Account.ID, authContext.Session.DeviceID)
	if err != nil {
		return nil, grpcError(err)
	}

	if len(req.GetKinds()) > 0 {
		allowed := make(map[domainconversation.ConversationKind]struct{}, len(req.GetKinds()))
		for _, kind := range req.GetKinds() {
			allowed[conversationKindFromProto(kind)] = struct{}{}
		}
		filtered := conversations[:0]
		for _, conversationRow := range conversations {
			if _, ok := allowed[conversationRow.Kind]; !ok {
				continue
			}
			filtered = append(filtered, conversationRow)
		}
		conversations = filtered
	}

	offset, err := decodeOffset(req.GetPage(), "conversations")
	if err != nil {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(conversations) {
		end = len(conversations)
	}

	page := conversations
	if offset < len(conversations) {
		page = conversations[offset:end]
	} else {
		page = nil
	}

	result := make([]*conversationv1.Conversation, 0, len(page))
	for _, conversationRow := range page {
		result = append(result, conversationProto(conversationRow, overlays[conversationRow.ID]))
	}

	return &conversationv1.ListConversationsResponse{
		Conversations: result,
		Page: &commonv1.PageResponse{
			NextPageToken: offsetToken("conversations", end),
			TotalSize:     uint64(len(conversations)),
		},
	}, nil
}

// UpdateConversation updates mutable conversation metadata and settings.
func (a *api) UpdateConversation(
	ctx context.Context,
	req *conversationv1.UpdateConversationRequest,
) (*conversationv1.UpdateConversationResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetConversation() == nil {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}

	currentConversation, _, err := a.conversation.GetConversation(ctx, domainconversation.GetConversationParams{
		ConversationID: req.GetConversation().GetConversationId(),
		AccountID:      authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	params, err := conversationUpdateParamsFromRequest(req, authContext.Account.ID, currentConversation.Settings)
	if err != nil {
		return nil, grpcError(err)
	}
	conversationRow, err := a.conversation.UpdateConversation(ctx, params)
	if err != nil {
		return nil, grpcError(err)
	}
	overlays, err := a.conversationE2EEOverlays(ctx, authContext.Account.ID, authContext.Session.DeviceID)
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	return &conversationv1.UpdateConversationResponse{
		Conversation: conversationProto(conversationRow, overlays[conversationRow.ID]),
	}, nil
}

// ListMembers returns the conversation member roster visible to the caller.
func (a *api) ListMembers(
	ctx context.Context,
	req *conversationv1.ListMembersRequest,
) (*conversationv1.ListMembersResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	members, err := a.conversation.ListMembers(ctx, domainconversation.GetConversationParams{
		ConversationID: req.GetConversationId(),
		AccountID:      authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), "members")
	if err != nil {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(members) {
		end = len(members)
	}

	page := members
	if offset < len(members) {
		page = members[offset:end]
	} else {
		page = nil
	}

	memberProfiles, err := a.memberProfiles(ctx, page, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.ListMembersResponse{
		Members: membersProto(page, memberProfiles),
		Page: &commonv1.PageResponse{
			NextPageToken: offsetToken("members", end),
			TotalSize:     uint64(len(members)),
		},
	}, nil
}

// AddMembers adds one or more members to a conversation.
func (a *api) AddMembers(
	ctx context.Context,
	req *conversationv1.AddMembersRequest,
) (*conversationv1.AddMembersResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if inviter := strings.TrimSpace(req.GetInviterUserId()); inviter != "" && inviter != authContext.Account.ID {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}

	members, err := a.conversation.AddMembers(ctx, domainconversation.AddMembersParams{
		ConversationID:     req.GetConversationId(),
		ActorAccountID:     authContext.Account.ID,
		InvitedByAccountID: authContext.Account.ID,
		AccountIDs:         req.GetUserIds(),
		Role:               memberRoleFromProto(req.GetRole()),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	memberProfiles, err := a.memberProfiles(ctx, members, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	return &conversationv1.AddMembersResponse{
		Members: membersProto(members, memberProfiles),
	}, nil
}

// RemoveMembers removes one or more members from a conversation.
func (a *api) RemoveMembers(
	ctx context.Context,
	req *conversationv1.RemoveMembersRequest,
) (*conversationv1.RemoveMembersResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	removed, err := a.conversation.RemoveMembers(ctx, domainconversation.RemoveMembersParams{
		ConversationID: req.GetConversationId(),
		ActorAccountID: authContext.Account.ID,
		AccountIDs:     req.GetUserIds(),
		Reason:         req.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	return &conversationv1.RemoveMembersResponse{RemovedMembers: removed}, nil
}

// UpdateMemberRole updates one conversation member role.
func (a *api) UpdateMemberRole(
	ctx context.Context,
	req *conversationv1.UpdateMemberRoleRequest,
) (*conversationv1.UpdateMemberRoleResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	member, err := a.conversation.UpdateMemberRole(ctx, domainconversation.UpdateMemberRoleParams{
		ConversationID:  req.GetConversationId(),
		ActorAccountID:  authContext.Account.ID,
		TargetAccountID: req.GetUserId(),
		Role:            memberRoleFromProto(req.GetRole()),
		Reason:          req.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	memberProfiles, err := a.memberProfiles(ctx, []domainconversation.ConversationMember{member}, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	return &conversationv1.UpdateMemberRoleResponse{
		Member: membersProto([]domainconversation.ConversationMember{member}, memberProfiles)[0],
	}, nil
}

// CreateInvite creates one reusable invite for a conversation.
func (a *api) CreateInvite(
	ctx context.Context,
	req *conversationv1.CreateInviteRequest,
) (*conversationv1.CreateInviteResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	allowedRoles := make([]domainconversation.MemberRole, 0, len(req.GetAllowedRoles()))
	for _, role := range req.GetAllowedRoles() {
		allowedRoles = append(allowedRoles, memberRoleFromProto(role))
	}

	invite, err := a.conversation.CreateInvite(ctx, domainconversation.CreateInviteParams{
		ConversationID: req.GetConversationId(),
		ActorAccountID: authContext.Account.ID,
		AllowedRoles:   allowedRoles,
		ExpiresAt:      zeroTime(req.GetExpiresAt()),
		MaxUses:        req.GetMaxUses(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	return &conversationv1.CreateInviteResponse{Invite: inviteProto(invite)}, nil
}

// ListInvites lists conversation invites visible to the caller.
func (a *api) ListInvites(
	ctx context.Context,
	req *conversationv1.ListInvitesRequest,
) (*conversationv1.ListInvitesResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	invites, err := a.conversation.ListInvites(ctx, domainconversation.ListInvitesParams{
		ConversationID: req.GetConversationId(),
		AccountID:      authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), "invites")
	if err != nil {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(invites) {
		end = len(invites)
	}

	page := invites
	if offset < len(invites) {
		page = invites[offset:end]
	} else {
		page = nil
	}

	result := make([]*conversationv1.Invite, 0, len(page))
	for _, invite := range page {
		result = append(result, inviteProto(invite))
	}

	nextToken := ""
	if end < len(invites) {
		nextToken = offsetToken("invites", end)
	}

	return &conversationv1.ListInvitesResponse{
		Invites: result,
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
			TotalSize:     uint64(len(invites)),
		},
	}, nil
}

// RevokeInvite revokes one reusable invite.
func (a *api) RevokeInvite(
	ctx context.Context,
	req *conversationv1.RevokeInviteRequest,
) (*conversationv1.RevokeInviteResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	invite, err := a.conversation.RevokeInvite(ctx, domainconversation.RevokeInviteParams{
		ConversationID: req.GetConversationId(),
		InviteID:       req.GetInviteId(),
		ActorAccountID: authContext.Account.ID,
		Reason:         req.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	return &conversationv1.RevokeInviteResponse{Invite: inviteProto(invite)}, nil
}

// ListMessages returns one page of messages ordered by sequence.
func (a *api) ListMessages(
	ctx context.Context,
	req *conversationv1.ListMessagesRequest,
) (*conversationv1.ListMessagesResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetIncludeForwarded() {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}

	fromSequence, err := decodeSequence(req.GetPage(), "messages")
	if err != nil {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}
	limit := pageSize(req.GetPage())

	messages, err := a.conversation.ListMessages(ctx, domainconversation.ListMessagesParams{
		AccountID:      authContext.Account.ID,
		ConversationID: req.GetConversationId(),
		ThreadID:       req.GetThreadId(),
		FromSequence:   fromSequence,
		Limit:          limit + 1,
		IncludeDeleted: req.GetIncludeDeleted(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	hasMore := false
	if len(messages) > limit {
		hasMore = true
		messages = messages[:limit]
	}

	profiles, err := a.messageProfiles(ctx, messages, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	nextToken := ""
	if hasMore && len(messages) > 0 {
		nextToken = sequenceToken("messages", messages[len(messages)-1].Sequence)
	}

	return &conversationv1.ListMessagesResponse{
		Messages: messagesProto(messages, profiles),
		Page: &commonv1.PageResponse{
			NextPageToken: nextToken,
		},
	}, nil
}

// ListScheduledMessages returns the authenticated sender's scheduled messages.
func (a *api) ListScheduledMessages(
	ctx context.Context,
	req *conversationv1.ListScheduledMessagesRequest,
) (*conversationv1.ListScheduledMessagesResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if !a.features.ScheduledMessagesEnabled {
		return nil, featureDisabledError("scheduled messages")
	}

	offset, err := decodeOffset(req.GetPage(), "scheduled_messages")
	if err != nil {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size

	messages, err := a.conversation.ListScheduledMessages(ctx, domainconversation.ListScheduledMessagesParams{
		AccountID:      authContext.Account.ID,
		ConversationID: req.GetConversationId(),
		ThreadID:       req.GetThreadId(),
		Limit:          end,
		IncludeFailed:  req.GetIncludeFailed(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	if end > len(messages) {
		end = len(messages)
	}

	page := messages
	if offset < len(messages) {
		page = messages[offset:end]
	} else {
		page = nil
	}

	profiles, err := a.messageProfiles(ctx, page, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.ListScheduledMessagesResponse{
		Messages: messagesProto(page, profiles),
		Page: &commonv1.PageResponse{
			NextPageToken: offsetToken("scheduled_messages", end),
			TotalSize:     uint64(len(messages)),
		},
	}, nil
}

// GetMessage returns one message visible to the caller.
func (a *api) GetMessage(
	ctx context.Context,
	req *conversationv1.GetMessageRequest,
) (*conversationv1.GetMessageResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	message, err := a.conversation.GetMessage(ctx, domainconversation.GetMessageParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		AccountID:      authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	profiles, err := a.profilesByID(ctx, []string{message.SenderAccountID}, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.GetMessageResponse{
		Message: messageProto(message, profiles[message.SenderAccountID]),
	}, nil
}

// SendMessage persists a new message in the conversation.
func (a *api) SendMessage(
	ctx context.Context,
	req *conversationv1.SendMessageRequest,
) (*conversationv1.SendMessageResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	draft := draftFromProto(req.GetDraft())
	if !draft.DeliverAt.IsZero() && !a.features.ScheduledMessagesEnabled {
		return nil, featureDisabledError("scheduled messages")
	}
	conversationRow, _, err := a.conversation.GetConversation(ctx, domainconversation.GetConversationParams{
		ConversationID: req.GetConversationId(),
		AccountID:      authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	if conversationRow.Settings.RequireEncryptedMessages {
		if err := a.e2ee.ValidateConversationPayload(ctx, domaine2ee.ValidateConversationPayloadParams{
			ConversationID:  req.GetConversationId(),
			SenderAccountID: authContext.Account.ID,
			SenderDeviceID:  authContext.Device.ID,
			PayloadKeyID:    draft.Payload.KeyID,
			PayloadMetadata: draft.Payload.Metadata,
		}); err != nil {
			return nil, a.encryptedSendError(ctx, authContext.Account.ID, authContext.Device.ID, conversationRow, err)
		}
	}

	message, event, err := a.conversation.SendMessage(ctx, domainconversation.SendMessageParams{
		ConversationID:  req.GetConversationId(),
		SenderAccountID: authContext.Account.ID,
		SenderDeviceID:  authContext.Device.ID,
		Draft:           draft,
		CausationID:     req.GetIdempotencyKey(),
		CorrelationID:   req.GetDraft().GetClientMessageId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	if event.EventID != "" {
		a.publishSyncEvent(event)
	}

	profiles, err := a.profilesByID(ctx, []string{message.SenderAccountID}, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	var responseEvent *commonv1.EventEnvelope
	if event.EventID != "" {
		responseEvent = eventProto(event)
	}

	return &conversationv1.SendMessageResponse{
		Message: messageProto(message, profiles[message.SenderAccountID]),
		Event:   responseEvent,
	}, nil
}

// EditMessage updates a message payload in place.
func (a *api) EditMessage(
	ctx context.Context,
	req *conversationv1.EditMessageRequest,
) (*conversationv1.EditMessageResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	message, event, err := a.conversation.EditMessage(ctx, domainconversation.EditMessageParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		ActorAccountID: authContext.Account.ID,
		ActorDeviceID:  authContext.Device.ID,
		Draft:          draftFromProto(req.GetDraft()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	profiles, err := a.profilesByID(ctx, []string{message.SenderAccountID}, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.EditMessageResponse{
		Message: messageProto(message, profiles[message.SenderAccountID]),
	}, nil
}

// DeleteMessage marks a message deleted.
func (a *api) DeleteMessage(
	ctx context.Context,
	req *conversationv1.DeleteMessageRequest,
) (*conversationv1.DeleteMessageResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	message, event, err := a.conversation.DeleteMessage(ctx, domainconversation.DeleteMessageParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		ActorAccountID: authContext.Account.ID,
		ActorDeviceID:  authContext.Device.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	profiles, err := a.profilesByID(ctx, []string{message.SenderAccountID}, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.DeleteMessageResponse{
		Message: messageProto(message, profiles[message.SenderAccountID]),
	}, nil
}

// AddReaction stores one reaction for the authenticated account.
func (a *api) AddReaction(
	ctx context.Context,
	req *conversationv1.AddReactionRequest,
) (*conversationv1.AddReactionResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	message, event, err := a.conversation.AddMessageReaction(ctx, domainconversation.AddMessageReactionParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		ActorAccountID: authContext.Account.ID,
		ActorDeviceID:  authContext.Device.ID,
		Reaction:       req.GetEmoji(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	profiles, err := a.profilesByID(ctx, []string{message.SenderAccountID}, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.AddReactionResponse{
		Message: messageProto(message, profiles[message.SenderAccountID]),
	}, nil
}

// RemoveReaction removes one reaction for the authenticated account.
func (a *api) RemoveReaction(
	ctx context.Context,
	req *conversationv1.RemoveReactionRequest,
) (*conversationv1.RemoveReactionResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	message, event, err := a.conversation.RemoveMessageReaction(ctx, domainconversation.RemoveMessageReactionParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		ActorAccountID: authContext.Account.ID,
		ActorDeviceID:  authContext.Device.ID,
		Reaction:       req.GetEmoji(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	profiles, err := a.profilesByID(ctx, []string{message.SenderAccountID}, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.RemoveReactionResponse{
		Message: messageProto(message, profiles[message.SenderAccountID]),
	}, nil
}

// PinMessage updates the pin state of one message.
func (a *api) PinMessage(
	ctx context.Context,
	req *conversationv1.PinMessageRequest,
) (*conversationv1.PinMessageResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	message, event, err := a.conversation.PinMessage(ctx, domainconversation.PinMessageParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		ActorAccountID: authContext.Account.ID,
		ActorDeviceID:  authContext.Device.ID,
		Pinned:         true,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	profiles, err := a.profilesByID(ctx, []string{message.SenderAccountID}, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.PinMessageResponse{
		Message: messageProto(message, profiles[message.SenderAccountID]),
	}, nil
}

// MarkRead advances the read watermark for the authenticated device.
func (a *api) MarkRead(
	ctx context.Context,
	req *conversationv1.MarkReadRequest,
) (*conversationv1.MarkReadResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	readThrough := req.GetReadThroughSequence()
	if readThrough == 0 && strings.TrimSpace(req.GetMessageId()) != "" {
		message, loadErr := a.conversation.GetMessage(ctx, domainconversation.GetMessageParams{
			ConversationID: req.GetConversationId(),
			MessageID:      req.GetMessageId(),
			AccountID:      authContext.Account.ID,
		})
		if loadErr != nil {
			return nil, grpcError(loadErr)
		}
		readThrough = message.Sequence
	}
	if readThrough == 0 {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}

	state, event, err := a.conversation.MarkRead(ctx, domainconversation.MarkReadParams{
		ConversationID:      req.GetConversationId(),
		AccountID:           authContext.Account.ID,
		DeviceID:            authContext.Device.ID,
		ReadThroughSequence: readThrough,
		CausationID:         req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	return &conversationv1.MarkReadResponse{ReadThroughSequence: state.LastReadSequence}, nil
}

// CreateThread creates a group topic.
func (a *api) CreateThread(
	ctx context.Context,
	req *conversationv1.CreateThreadRequest,
) (*conversationv1.CreateThreadResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	topic, event, err := a.conversation.CreateTopic(ctx, domainconversation.CreateTopicParams{
		ConversationID:   req.GetConversationId(),
		RootMessageID:    req.GetRootMessageId(),
		CreatorAccountID: authContext.Account.ID,
		Title:            req.GetTitle(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	return &conversationv1.CreateThreadResponse{Thread: threadProto(topic)}, nil
}

// GetThread returns one topic together with its first message page.
func (a *api) GetThread(
	ctx context.Context,
	req *conversationv1.GetThreadRequest,
) (*conversationv1.GetThreadResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	topic, err := a.conversation.GetTopic(ctx, domainconversation.GetTopicParams{
		ConversationID: req.GetConversationId(),
		TopicID:        req.GetThreadId(),
		AccountID:      authContext.Account.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	messages, err := a.conversation.ListMessages(ctx, domainconversation.ListMessagesParams{
		AccountID:      authContext.Account.ID,
		ConversationID: req.GetConversationId(),
		ThreadID:       req.GetThreadId(),
		Limit:          defaultPageSize,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	profiles, err := a.messageProfiles(ctx, messages, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.GetThreadResponse{
		Thread:   threadProto(topic),
		Messages: messagesProto(messages, profiles),
	}, nil
}

// ListThreads lists the topics in one conversation.
func (a *api) ListThreads(
	ctx context.Context,
	req *conversationv1.ListThreadsRequest,
) (*conversationv1.ListThreadsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	topics, err := a.conversation.ListTopics(ctx, domainconversation.ListTopicsParams{
		ConversationID:  req.GetConversationId(),
		AccountID:       authContext.Account.ID,
		IncludeArchived: req.GetIncludeArchived(),
		IncludeClosed:   req.GetIncludeClosed(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), "threads")
	if err != nil {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(topics) {
		end = len(topics)
	}

	page := topics
	if offset < len(topics) {
		page = topics[offset:end]
	} else {
		page = nil
	}

	result := make([]*conversationv1.Thread, 0, len(page))
	for _, topic := range page {
		result = append(result, threadProto(topic))
	}

	return &conversationv1.ListThreadsResponse{
		Threads: result,
		Page: &commonv1.PageResponse{
			NextPageToken: offsetToken("threads", end),
			TotalSize:     uint64(len(topics)),
		},
	}, nil
}

// RenameThread renames one existing thread.
func (a *api) RenameThread(
	ctx context.Context,
	req *conversationv1.RenameThreadRequest,
) (*conversationv1.RenameThreadResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	topic, event, err := a.conversation.RenameTopic(ctx, domainconversation.RenameTopicParams{
		ConversationID: req.GetConversationId(),
		TopicID:        req.GetThreadId(),
		ActorAccountID: authContext.Account.ID,
		Title:          req.GetTitle(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	return &conversationv1.RenameThreadResponse{Thread: threadProto(topic)}, nil
}

// ArchiveThread updates the archived state of one thread.
func (a *api) ArchiveThread(
	ctx context.Context,
	req *conversationv1.ArchiveThreadRequest,
) (*conversationv1.ArchiveThreadResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	topic, event, err := a.conversation.ArchiveTopic(ctx, domainconversation.ArchiveTopicParams{
		ConversationID: req.GetConversationId(),
		TopicID:        req.GetThreadId(),
		ActorAccountID: authContext.Account.ID,
		Archived:       req.GetArchived(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	return &conversationv1.ArchiveThreadResponse{Thread: threadProto(topic)}, nil
}

// CloseThread updates the closed state of one thread.
func (a *api) CloseThread(
	ctx context.Context,
	req *conversationv1.CloseThreadRequest,
) (*conversationv1.CloseThreadResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	topic, event, err := a.conversation.CloseTopic(ctx, domainconversation.CloseTopicParams{
		ConversationID: req.GetConversationId(),
		TopicID:        req.GetThreadId(),
		ActorAccountID: authContext.Account.ID,
		Closed:         req.GetClosed(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	return &conversationv1.CloseThreadResponse{Thread: threadProto(topic)}, nil
}

// PinThread updates the pinned state of one thread.
func (a *api) PinThread(
	ctx context.Context,
	req *conversationv1.PinThreadRequest,
) (*conversationv1.PinThreadResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	topic, event, err := a.conversation.PinTopic(ctx, domainconversation.PinTopicParams{
		ConversationID: req.GetConversationId(),
		TopicID:        req.GetThreadId(),
		ActorAccountID: authContext.Account.ID,
		Pinned:         req.GetPinned(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.publishSyncEvent(event)

	return &conversationv1.PinThreadResponse{Thread: threadProto(topic)}, nil
}

// GetModerationPolicy returns the effective moderation policy for one target.
func (a *api) GetModerationPolicy(
	ctx context.Context,
	req *conversationv1.GetModerationPolicyRequest,
) (*conversationv1.GetModerationPolicyResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	conversationRow, targetKind, targetID, err := a.requireModerationTarget(ctx, authContext.Account.ID, req.GetTarget())
	if err != nil {
		return nil, grpcError(err)
	}

	policy, err := a.effectiveModerationPolicy(ctx, conversationRow, targetKind, targetID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.GetModerationPolicyResponse{
		Policy: moderationPolicyProto(policy),
	}, nil
}

// GetModerationRateState returns the current slow-mode and anti-spam counters.
func (a *api) GetModerationRateState(
	ctx context.Context,
	req *conversationv1.GetModerationRateStateRequest,
) (*conversationv1.GetModerationRateStateResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	_, members, targetKind, targetID, err := a.moderationContext(ctx, authContext.Account.ID, req.GetTarget())
	if err != nil {
		return nil, grpcError(err)
	}

	subjectAccountID := moderationSubjectAccountID(authContext.Account.ID, req.GetUserId())
	if !canInspectModerationAccount(members, authContext.Account.ID, subjectAccountID) {
		return nil, grpcError(domainconversation.ErrForbidden)
	}

	state, err := a.conversation.ModerationRateStateByTargetAndAccount(ctx, targetKind, targetID, subjectAccountID)
	if err != nil && !errors.Is(err, domainconversation.ErrNotFound) {
		return nil, grpcError(err)
	}
	if errors.Is(err, domainconversation.ErrNotFound) {
		state = domainconversation.ModerationRateState{
			TargetKind: targetKind,
			TargetID:   targetID,
			AccountID:  subjectAccountID,
		}
	}

	return &conversationv1.GetModerationRateStateResponse{
		State: moderationRateStateProto(state),
	}, nil
}

// CheckModerationWrite evaluates whether a target account may write right now.
func (a *api) CheckModerationWrite(
	ctx context.Context,
	req *conversationv1.CheckModerationWriteRequest,
) (*conversationv1.CheckModerationWriteResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	conversationRow, members, targetKind, targetID, err := a.moderationContext(ctx, authContext.Account.ID, req.GetTarget())
	if err != nil {
		return nil, grpcError(err)
	}

	subjectAccountID := moderationSubjectAccountID(authContext.Account.ID, req.GetUserId())
	if !canInspectModerationAccount(members, authContext.Account.ID, subjectAccountID) {
		return nil, grpcError(domainconversation.ErrForbidden)
	}

	subjectMember, ok := conversationMemberByAccount(members, subjectAccountID)
	if !ok || !isConversationMemberActive(subjectMember) {
		return &conversationv1.CheckModerationWriteResponse{
			Decision: moderationWriteDecisionProto(domainconversation.ModerationDecision{}),
		}, nil
	}

	policy, err := a.effectiveModerationPolicy(ctx, conversationRow, targetKind, targetID)
	if err != nil {
		return nil, grpcError(err)
	}

	decision, err := a.conversation.CheckModerationWrite(ctx, domainconversation.CheckModerationWriteParams{
		TargetKind:     targetKind,
		TargetID:       targetID,
		ActorAccountID: subjectAccountID,
		ActorRole:      subjectMember.Role,
		BasePolicy:     policy,
	})
	if err != nil {
		if errors.Is(err, domainconversation.ErrForbidden) || errors.Is(err, domainconversation.ErrRateLimited) {
			return &conversationv1.CheckModerationWriteResponse{
				Decision: moderationWriteDecisionProto(decision),
			}, nil
		}

		return nil, grpcError(err)
	}

	return &conversationv1.CheckModerationWriteResponse{
		Decision: moderationWriteDecisionProto(decision),
	}, nil
}

// SetModerationPolicy persists a moderation policy override.
func (a *api) SetModerationPolicy(
	ctx context.Context,
	req *conversationv1.SetModerationPolicyRequest,
) (*conversationv1.SetModerationPolicyResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	params, err := moderationPolicyParamsFromProto(req.GetPolicy(), authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	policy, err := a.conversation.SetModerationPolicy(ctx, params)
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	return &conversationv1.SetModerationPolicyResponse{
		Policy: moderationPolicyProto(policy),
	}, nil
}

// ApplyModerationRestriction applies one active moderation restriction.
func (a *api) ApplyModerationRestriction(
	ctx context.Context,
	req *conversationv1.ApplyModerationRestrictionRequest,
) (*conversationv1.ApplyModerationRestrictionResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	targetKind, targetID, _, err := moderationTargetFromProto(req.GetTarget())
	if err != nil {
		return nil, grpcError(err)
	}

	restriction, err := a.conversation.ApplyModerationRestriction(ctx, domainconversation.ApplyModerationRestrictionParams{
		TargetKind:      targetKind,
		TargetID:        targetID,
		ActorAccountID:  authContext.Account.ID,
		TargetAccountID: req.GetUserId(),
		State:           moderationRestrictionStateFromProto(req.GetState()),
		Reason:          req.GetReason(),
		Duration:        timeDuration(req.GetDuration()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	return &conversationv1.ApplyModerationRestrictionResponse{
		Restriction: moderationRestrictionProto(restriction),
	}, nil
}

// LiftModerationRestriction removes one active moderation restriction.
func (a *api) LiftModerationRestriction(
	ctx context.Context,
	req *conversationv1.LiftModerationRestrictionRequest,
) (*conversationv1.LiftModerationRestrictionResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	targetKind, targetID, _, err := moderationTargetFromProto(req.GetTarget())
	if err != nil {
		return nil, grpcError(err)
	}

	if err := a.conversation.LiftModerationRestriction(ctx, domainconversation.LiftModerationRestrictionParams{
		TargetKind:      targetKind,
		TargetID:        targetID,
		ActorAccountID:  authContext.Account.ID,
		TargetAccountID: req.GetUserId(),
		Reason:          req.GetReason(),
	}); err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	return &conversationv1.LiftModerationRestrictionResponse{}, nil
}

// ListModerationRestrictions lists active moderation restrictions for one target.
func (a *api) ListModerationRestrictions(
	ctx context.Context,
	req *conversationv1.ListModerationRestrictionsRequest,
) (*conversationv1.ListModerationRestrictionsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	_, targetKind, targetID, err := a.requireModerationTarget(ctx, authContext.Account.ID, req.GetTarget())
	if err != nil {
		return nil, grpcError(err)
	}

	restrictions, err := a.conversation.ModerationRestrictionsByTarget(ctx, targetKind, targetID)
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), "moderation_restrictions")
	if err != nil {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(restrictions) {
		end = len(restrictions)
	}

	page := restrictions
	if offset < len(restrictions) {
		page = restrictions[offset:end]
	} else {
		page = nil
	}

	result := make([]*conversationv1.ModerationRestriction, 0, len(page))
	for _, restriction := range page {
		result = append(result, moderationRestrictionProto(restriction))
	}

	return &conversationv1.ListModerationRestrictionsResponse{
		Restrictions: result,
		Page: &commonv1.PageResponse{
			NextPageToken: offsetToken("moderation_restrictions", end),
			TotalSize:     uint64(len(restrictions)),
		},
	}, nil
}

// SubmitModerationReport creates one moderation report.
func (a *api) SubmitModerationReport(
	ctx context.Context,
	req *conversationv1.SubmitModerationReportRequest,
) (*conversationv1.SubmitModerationReportResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	targetKind, targetID, _, err := moderationTargetFromProto(req.GetTarget())
	if err != nil {
		return nil, grpcError(err)
	}

	report, err := a.conversation.SubmitModerationReport(ctx, domainconversation.SubmitModerationReportParams{
		TargetKind:        targetKind,
		TargetID:          targetID,
		ReporterAccountID: authContext.Account.ID,
		TargetAccountID:   req.GetTargetUserId(),
		Reason:            req.GetReason(),
		Details:           req.GetDetails(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.SubmitModerationReportResponse{
		Report: moderationReportProto(report),
	}, nil
}

// GetModerationReport returns one moderation report visible to moderators.
func (a *api) GetModerationReport(
	ctx context.Context,
	req *conversationv1.GetModerationReportRequest,
) (*conversationv1.GetModerationReportResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	report, err := a.conversation.ModerationReportByID(ctx, req.GetReportId())
	if err != nil {
		return nil, grpcError(err)
	}
	if _, _, _, err := a.requireModerationTarget(ctx, authContext.Account.ID, moderationTargetProto(report.TargetKind, report.TargetID)); err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.GetModerationReportResponse{
		Report: moderationReportProto(report),
	}, nil
}

// ListModerationReports lists moderation reports for one target.
func (a *api) ListModerationReports(
	ctx context.Context,
	req *conversationv1.ListModerationReportsRequest,
) (*conversationv1.ListModerationReportsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	_, targetKind, targetID, err := a.requireModerationTarget(ctx, authContext.Account.ID, req.GetTarget())
	if err != nil {
		return nil, grpcError(err)
	}

	reports, err := a.conversation.ModerationReportsByTarget(ctx, targetKind, targetID)
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), "moderation_reports")
	if err != nil {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(reports) {
		end = len(reports)
	}

	page := reports
	if offset < len(reports) {
		page = reports[offset:end]
	} else {
		page = nil
	}

	result := make([]*conversationv1.ModerationReport, 0, len(page))
	for _, report := range page {
		result = append(result, moderationReportProto(report))
	}

	return &conversationv1.ListModerationReportsResponse{
		Reports: result,
		Page: &commonv1.PageResponse{
			NextPageToken: offsetToken("moderation_reports", end),
			TotalSize:     uint64(len(reports)),
		},
	}, nil
}

// ResolveModerationReport resolves or rejects one moderation report.
func (a *api) ResolveModerationReport(
	ctx context.Context,
	req *conversationv1.ResolveModerationReportRequest,
) (*conversationv1.ResolveModerationReportResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	report, err := a.conversation.ResolveModerationReport(ctx, domainconversation.ResolveModerationReportParams{
		ReportID:          req.GetReportId(),
		ResolverAccountID: authContext.Account.ID,
		Resolved:          req.GetResolved(),
		Resolution:        req.GetResolution(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.ResolveModerationReportResponse{
		Report: moderationReportProto(report),
	}, nil
}

// ListModerationActions lists moderation audit entries for one target.
func (a *api) ListModerationActions(
	ctx context.Context,
	req *conversationv1.ListModerationActionsRequest,
) (*conversationv1.ListModerationActionsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	_, targetKind, targetID, err := a.requireModerationTarget(ctx, authContext.Account.ID, req.GetTarget())
	if err != nil {
		return nil, grpcError(err)
	}

	actions, err := a.conversation.ModerationActionsByTarget(ctx, targetKind, targetID)
	if err != nil {
		return nil, grpcError(err)
	}

	offset, err := decodeOffset(req.GetPage(), "moderation_actions")
	if err != nil {
		return nil, grpcError(domainconversation.ErrInvalidInput)
	}
	size := pageSize(req.GetPage())
	end := offset + size
	if end > len(actions) {
		end = len(actions)
	}

	page := actions
	if offset < len(actions) {
		page = actions[offset:end]
	} else {
		page = nil
	}

	result := make([]*conversationv1.ModerationAction, 0, len(page))
	for _, action := range page {
		result = append(result, moderationActionProto(action))
	}

	return &conversationv1.ListModerationActionsResponse{
		Actions: result,
		Page: &commonv1.PageResponse{
			NextPageToken: offsetToken("moderation_actions", end),
			TotalSize:     uint64(len(actions)),
		},
	}, nil
}

func conversationSettingsFromProto(settings *conversationv1.ConversationSettings) domainconversation.ConversationSettings {
	if settings == nil {
		return domainconversation.ConversationSettings{}
	}

	slowMode := timeDuration(settings.GetSlowModeInterval())
	requireTrustedDevices := settings.GetRequireTrustedDevices()
	switch settings.GetE2EeTrustPolicy() {
	case conversationv1.ConversationE2EETrustPolicy_CONVERSATION_E2EE_TRUST_POLICY_ALLOW_UNTRUSTED:
		requireTrustedDevices = false
	case conversationv1.ConversationE2EETrustPolicy_CONVERSATION_E2EE_TRUST_POLICY_TRUSTED_ONLY:
		requireTrustedDevices = true
	}
	return domainconversation.ConversationSettings{
		OnlyAdminsCanWrite:       settings.GetOnlyAdminsCanWrite(),
		OnlyAdminsCanAddMembers:  settings.GetOnlyAdminsCanAddMembers(),
		AllowReactions:           settings.GetAllowReactions(),
		AllowForwards:            settings.GetAllowForwards(),
		AllowThreads:             settings.GetAllowThreads(),
		RequireJoinApproval:      settings.GetRequireJoinApproval(),
		PinnedMessagesOnlyAdmins: settings.GetPinnedMessagesOnlyAdmins(),
		SlowModeInterval:         slowMode,
		RequireEncryptedMessages: settings.GetRequireEncryptedMessages(),
		RequireTrustedDevices:    requireTrustedDevices,
	}
}

type conversationE2EEOverlay struct {
	VerificationRequiredDevices uint32
	UntrustedDevices            uint32
	CompromisedDevices          uint32
	RequiredAction              conversationv1.ConversationE2EERequiredAction
	PrimaryRemediationHint      conversationv1.ConversationE2EERemediationHint
	CanSendEncryptedNow         bool
	EncryptedSendBlockReason    conversationv1.ConversationEncryptedSendBlockReason
	BlockedDevices              []*conversationv1.ConversationBlockedDevice
}

func conversationProto(
	conversationRow domainconversation.Conversation,
	overlay conversationE2EEOverlay,
) *conversationv1.Conversation {
	requiredAction := overlay.RequiredAction
	if requiredAction == conversationv1.ConversationE2EERequiredAction_CONVERSATION_E2EE_REQUIRED_ACTION_UNSPECIFIED {
		requiredAction = conversationv1.ConversationE2EERequiredAction_CONVERSATION_E2EE_REQUIRED_ACTION_NONE
	}
	primaryRemediationHint := overlay.PrimaryRemediationHint
	if primaryRemediationHint == conversationv1.ConversationE2EERemediationHint_CONVERSATION_E2EE_REMEDIATION_HINT_UNSPECIFIED {
		primaryRemediationHint = conversationv1.ConversationE2EERemediationHint_CONVERSATION_E2EE_REMEDIATION_HINT_NONE
	}
	blockReason := overlay.EncryptedSendBlockReason
	if blockReason == conversationv1.ConversationEncryptedSendBlockReason_CONVERSATION_ENCRYPTED_SEND_BLOCK_REASON_UNSPECIFIED {
		if overlay.CanSendEncryptedNow {
			blockReason = conversationv1.ConversationEncryptedSendBlockReason_CONVERSATION_ENCRYPTED_SEND_BLOCK_REASON_NONE
		} else {
			blockReason = conversationv1.ConversationEncryptedSendBlockReason_CONVERSATION_ENCRYPTED_SEND_BLOCK_REASON_UNSUPPORTED_CONVERSATION_KIND
		}
	}

	return &conversationv1.Conversation{
		ConversationId:              conversationRow.ID,
		Kind:                        conversationKindToProto(conversationRow.Kind),
		Title:                       conversationRow.Title,
		Description:                 conversationRow.Description,
		AvatarMediaId:               conversationRow.AvatarMediaID,
		OwnerUserId:                 conversationRow.OwnerAccountID,
		Settings:                    conversationSettingsProto(conversationRow.Settings),
		Archived:                    conversationRow.Archived,
		Muted:                       conversationRow.Muted,
		Pinned:                      conversationRow.Pinned,
		Hidden:                      conversationRow.Hidden,
		LastSequence:                conversationRow.LastSequence,
		CreatedAt:                   protoTime(conversationRow.CreatedAt),
		UpdatedAt:                   protoTime(conversationRow.UpdatedAt),
		LastMessageAt:               protoTime(conversationRow.LastMessageAt),
		UnreadCount:                 conversationRow.UnreadCount,
		UnreadMentionCount:          conversationRow.UnreadMentionCount,
		VerificationRequiredDevices: overlay.VerificationRequiredDevices,
		UntrustedDevices:            overlay.UntrustedDevices,
		CompromisedDevices:          overlay.CompromisedDevices,
		BlockedDevices:              cloneBlockedDevices(overlay.BlockedDevices),
		PrimaryE2EeRemediationHint:  primaryRemediationHint,
		CanSendEncryptedNow:         overlay.CanSendEncryptedNow,
		EncryptedSendBlockReason:    blockReason,
		E2EeRequiredAction:          requiredAction,
	}
}

func (a *api) conversationE2EEOverlays(
	ctx context.Context,
	accountID string,
	deviceID string,
) (map[string]conversationE2EEOverlay, error) {
	if a == nil || a.e2ee == nil {
		return nil, nil
	}

	devices, err := a.e2ee.ListVerificationRequiredDevices(ctx, domaine2ee.ListVerificationRequiredDevicesParams{
		ObserverAccountID: accountID,
		ObserverDeviceID:  deviceID,
	})
	if err != nil {
		return nil, err
	}

	byConversation := make(map[string]map[string]domaine2ee.VerificationRequiredDevice)
	for _, item := range devices {
		deviceKey := item.AccountID + ":" + item.DeviceID
		for _, conversationID := range item.ConversationIDs {
			deviceSet := byConversation[conversationID]
			if deviceSet == nil {
				deviceSet = make(map[string]domaine2ee.VerificationRequiredDevice)
				byConversation[conversationID] = deviceSet
			}
			deviceSet[deviceKey] = item
		}
	}

	overlays := make(map[string]conversationE2EEOverlay, len(byConversation))
	for conversationID, deviceSet := range byConversation {
		overlay := conversationE2EEOverlay{
			VerificationRequiredDevices: uint32(len(deviceSet)),
			RequiredAction:              conversationv1.ConversationE2EERequiredAction_CONVERSATION_E2EE_REQUIRED_ACTION_VERIFY_DEVICES,
		}
		blockedDevices := make([]domaine2ee.VerificationRequiredDevice, 0, len(deviceSet))
		for _, device := range deviceSet {
			blockedDevices = append(blockedDevices, device)
			switch device.TrustState {
			case domaine2ee.DeviceTrustStateCompromised:
				overlay.CompromisedDevices++
			default:
				overlay.UntrustedDevices++
			}
		}
		slices.SortFunc(blockedDevices, func(left domaine2ee.VerificationRequiredDevice, right domaine2ee.VerificationRequiredDevice) int {
			if left.TrustState != right.TrustState {
				if left.TrustState == domaine2ee.DeviceTrustStateCompromised {
					return -1
				}
				if right.TrustState == domaine2ee.DeviceTrustStateCompromised {
					return 1
				}
			}
			if left.AccountID != right.AccountID {
				if left.AccountID < right.AccountID {
					return -1
				}
				return 1
			}
			if left.DeviceID < right.DeviceID {
				return -1
			}
			if left.DeviceID > right.DeviceID {
				return 1
			}
			return 0
		})
		overlay.BlockedDevices = make([]*conversationv1.ConversationBlockedDevice, 0, len(blockedDevices))
		for _, device := range blockedDevices {
			trustState := deviceTrustStateProto(device.TrustState)
			if trustState == 0 {
				trustState = e2eeDeviceTrustStateForConversationPreview(device.TrustState)
			}
			remediationHint := conversationE2EERemediationHintForTrustState(device.TrustState)
			overlay.BlockedDevices = append(overlay.BlockedDevices, &conversationv1.ConversationBlockedDevice{
				UserId:          device.AccountID,
				DeviceId:        device.DeviceID,
				TrustState:      trustState,
				KeyFingerprint:  device.KeyFingerprint,
				RemediationHint: remediationHint,
			})
		}
		overlay.PrimaryRemediationHint = conversationPrimaryE2EERemediationHint(overlay)
		overlays[conversationID] = overlay
	}

	conversations, err := a.conversation.ListConversations(ctx, domainconversation.ListConversationsParams{
		AccountID:       accountID,
		IncludeArchived: true,
		IncludeMuted:    true,
		IncludeHidden:   true,
	})
	if err != nil {
		return nil, err
	}
	for _, conversationRow := range conversations {
		overlay := overlays[conversationRow.ID]
		overlay.CanSendEncryptedNow, overlay.EncryptedSendBlockReason = a.conversationEncryptedSendState(
			ctx,
			accountID,
			deviceID,
			conversationRow,
			overlay,
		)
		overlays[conversationRow.ID] = overlay
	}
	return overlays, nil
}

func cloneBlockedDevices(values []*conversationv1.ConversationBlockedDevice) []*conversationv1.ConversationBlockedDevice {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]*conversationv1.ConversationBlockedDevice, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		cloned = append(cloned, &conversationv1.ConversationBlockedDevice{
			UserId:          value.UserId,
			DeviceId:        value.DeviceId,
			TrustState:      value.TrustState,
			KeyFingerprint:  value.KeyFingerprint,
			RemediationHint: value.RemediationHint,
		})
	}
	return cloned
}

func (a *api) encryptedSendError(
	ctx context.Context,
	accountID string,
	deviceID string,
	conversationRow domainconversation.Conversation,
	err error,
) error {
	base := grpcError(err)
	if a == nil || a.e2ee == nil || (!errors.Is(err, domaine2ee.ErrConflict) && !errors.Is(err, domaine2ee.ErrForbidden)) {
		return base
	}

	overlays, overlayErr := a.conversationE2EEOverlays(ctx, accountID, deviceID)
	if overlayErr != nil {
		return base
	}
	overlay := overlays[conversationRow.ID]
	detail := &conversationv1.EncryptedSendBlockedDetail{
		ConversationId:              conversationRow.ID,
		CanSendEncryptedNow:         overlay.CanSendEncryptedNow,
		BlockReason:                 overlay.EncryptedSendBlockReason,
		PrimaryRemediationHint:      overlay.PrimaryRemediationHint,
		VerificationRequiredDevices: overlay.VerificationRequiredDevices,
		UntrustedDevices:            overlay.UntrustedDevices,
		CompromisedDevices:          overlay.CompromisedDevices,
		BlockedDevices:              cloneBlockedDevices(overlay.BlockedDevices),
	}

	st, statusErr := status.FromError(base)
	if !statusErr {
		return base
	}
	withDetails, detailErr := st.WithDetails(detail)
	if detailErr != nil {
		return base
	}
	return withDetails.Err()
}

func e2eeDeviceTrustStateForConversationPreview(value domaine2ee.DeviceTrustState) e2eev1.DeviceTrustState {
	switch value {
	case domaine2ee.DeviceTrustStateCompromised:
		return e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_COMPROMISED
	default:
		return e2eev1.DeviceTrustState_DEVICE_TRUST_STATE_UNTRUSTED
	}
}

func conversationE2EERemediationHintForTrustState(
	value domaine2ee.DeviceTrustState,
) conversationv1.ConversationE2EERemediationHint {
	switch value {
	case domaine2ee.DeviceTrustStateCompromised:
		return conversationv1.ConversationE2EERemediationHint_CONVERSATION_E2EE_REMEDIATION_HINT_REMOVE_COMPROMISED_DEVICE
	default:
		return conversationv1.ConversationE2EERemediationHint_CONVERSATION_E2EE_REMEDIATION_HINT_VERIFY_DEVICE
	}
}

func conversationPrimaryE2EERemediationHint(
	overlay conversationE2EEOverlay,
) conversationv1.ConversationE2EERemediationHint {
	if overlay.CompromisedDevices > 0 {
		return conversationv1.ConversationE2EERemediationHint_CONVERSATION_E2EE_REMEDIATION_HINT_REMOVE_COMPROMISED_DEVICE
	}
	if overlay.VerificationRequiredDevices > 0 {
		return conversationv1.ConversationE2EERemediationHint_CONVERSATION_E2EE_REMEDIATION_HINT_VERIFY_DEVICE
	}
	return conversationv1.ConversationE2EERemediationHint_CONVERSATION_E2EE_REMEDIATION_HINT_NONE
}

func (a *api) conversationEncryptedSendState(
	ctx context.Context,
	accountID string,
	deviceID string,
	conversationRow domainconversation.Conversation,
	overlay conversationE2EEOverlay,
) (bool, conversationv1.ConversationEncryptedSendBlockReason) {
	switch conversationRow.Kind {
	case domainconversation.ConversationKindDirect, domainconversation.ConversationKindGroup:
	default:
		return false, conversationv1.ConversationEncryptedSendBlockReason_CONVERSATION_ENCRYPTED_SEND_BLOCK_REASON_UNSUPPORTED_CONVERSATION_KIND
	}

	coverage, err := a.e2ee.GetConversationKeyCoverage(ctx, domaine2ee.GetConversationKeyCoverageParams{
		ConversationID:  conversationRow.ID,
		SenderAccountID: accountID,
		SenderDeviceID:  deviceID,
	})
	if err != nil {
		return false, conversationv1.ConversationEncryptedSendBlockReason_CONVERSATION_ENCRYPTED_SEND_BLOCK_REASON_MISSING_KEY_COVERAGE
	}
	for _, entry := range coverage {
		if entry.TrustState == domaine2ee.DeviceTrustStateCompromised {
			return false, conversationv1.ConversationEncryptedSendBlockReason_CONVERSATION_ENCRYPTED_SEND_BLOCK_REASON_COMPROMISED_DEVICE_PRESENT
		}
		if entry.State != domaine2ee.ConversationKeyCoverageStateReady {
			return false, conversationv1.ConversationEncryptedSendBlockReason_CONVERSATION_ENCRYPTED_SEND_BLOCK_REASON_MISSING_KEY_COVERAGE
		}
	}
	if conversationRow.Settings.RequireTrustedDevices && overlay.UntrustedDevices > 0 {
		return false, conversationv1.ConversationEncryptedSendBlockReason_CONVERSATION_ENCRYPTED_SEND_BLOCK_REASON_VERIFY_DEVICES_REQUIRED
	}
	return true, conversationv1.ConversationEncryptedSendBlockReason_CONVERSATION_ENCRYPTED_SEND_BLOCK_REASON_NONE
}

func conversationSettingsProto(settings domainconversation.ConversationSettings) *conversationv1.ConversationSettings {
	return &conversationv1.ConversationSettings{
		OnlyAdminsCanWrite:       settings.OnlyAdminsCanWrite,
		OnlyAdminsCanAddMembers:  settings.OnlyAdminsCanAddMembers,
		AllowReactions:           settings.AllowReactions,
		AllowForwards:            settings.AllowForwards,
		AllowThreads:             settings.AllowThreads,
		RequireJoinApproval:      settings.RequireJoinApproval,
		PinnedMessagesOnlyAdmins: settings.PinnedMessagesOnlyAdmins,
		SlowModeInterval:         protoDuration(settings.SlowModeInterval),
		RequireEncryptedMessages: settings.RequireEncryptedMessages,
		RequireTrustedDevices:    settings.RequireTrustedDevices,
		E2EeTrustPolicy:          conversationE2EETrustPolicyProto(settings.RequireTrustedDevices),
	}
}

func conversationE2EETrustPolicyProto(requireTrustedDevices bool) conversationv1.ConversationE2EETrustPolicy {
	if requireTrustedDevices {
		return conversationv1.ConversationE2EETrustPolicy_CONVERSATION_E2EE_TRUST_POLICY_TRUSTED_ONLY
	}
	return conversationv1.ConversationE2EETrustPolicy_CONVERSATION_E2EE_TRUST_POLICY_ALLOW_UNTRUSTED
}

func membersProto(
	members []domainconversation.ConversationMember,
	profiles map[string]*usersv1.UserProfile,
) []*conversationv1.ConversationMember {
	result := make([]*conversationv1.ConversationMember, 0, len(members))
	for _, member := range members {
		result = append(result, &conversationv1.ConversationMember{
			ConversationId:  member.ConversationID,
			UserId:          member.AccountID,
			Profile:         profiles[member.AccountID],
			Role:            memberRoleToProto(member.Role),
			InvitedByUserId: member.InvitedByAccountID,
			Muted:           member.Muted,
			Banned:          member.Banned,
			JoinedAt:        protoTime(member.JoinedAt),
			LeftAt:          protoTime(member.LeftAt),
		})
	}

	return result
}

func draftFromProto(draft *commonv1.MessageDraft) domainconversation.MessageDraft {
	if draft == nil {
		return domainconversation.MessageDraft{}
	}

	attachments := make([]domainconversation.AttachmentRef, 0, len(draft.GetAttachments()))
	for _, attachment := range draft.GetAttachments() {
		attachments = append(attachments, attachmentFromProto(attachment))
	}

	deliverAt := zeroTime(draft.GetDeliverAt())
	return domainconversation.MessageDraft{
		ClientMessageID:     draft.GetClientMessageId(),
		Kind:                messageKindFromProto(draft.GetKind()),
		Payload:             payloadFromProto(draft.GetPayload()),
		Attachments:         attachments,
		MentionAccountIDs:   draft.GetMentionUserIds(),
		ReplyTo:             referenceFromProto(draft.GetReplyTo()),
		ThreadID:            draft.GetThreadId(),
		DeliverAt:           deliverAt,
		Silent:              draft.GetSilent(),
		DisableLinkPreviews: draft.GetDisableLinkPreviews(),
		Metadata:            draft.GetMetadata(),
	}
}

func messageProto(message domainconversation.Message, senderProfile *usersv1.UserProfile) *conversationv1.Message {
	reactions := make([]*conversationv1.Reaction, 0, len(message.Reactions))
	for _, reaction := range message.Reactions {
		reactions = append(reactions, &conversationv1.Reaction{
			Emoji:     reaction.Reaction,
			UserId:    reaction.AccountID,
			CreatedAt: protoTime(reaction.CreatedAt),
		})
	}

	attachments := make([]*commonv1.AttachmentRef, 0, len(message.Attachments))
	for _, attachment := range message.Attachments {
		attachments = append(attachments, attachmentToProto(attachment))
	}

	return &conversationv1.Message{
		MessageId:      message.ID,
		ConversationId: message.ConversationID,
		SenderUserId:   message.SenderAccountID,
		SenderProfile:  senderProfile,
		SenderDeviceId: message.SenderDeviceID,
		Kind:           messageKindToProto(message.Kind),
		Status:         messageStatusToProto(message.Status),
		Payload:        protoPayload(message.Payload),
		Attachments:    attachments,
		ReplyTo:        referenceToProto(message.ReplyTo),
		ThreadId:       message.ThreadID,
		Silent:         message.Silent,
		Pinned:         message.Pinned,
		Reactions:      reactions,
		ViewCount:      message.ViewCount,
		MentionUserIds: message.MentionAccountIDs,
		DeliverAt:      protoTime(message.DeliverAt),
		CreatedAt:      protoTime(message.CreatedAt),
		EditedAt:       protoTime(message.EditedAt),
		DeletedAt:      protoTime(message.DeletedAt),
	}
}

func messagesProto(
	messages []domainconversation.Message,
	profiles map[string]*usersv1.UserProfile,
) []*conversationv1.Message {
	result := make([]*conversationv1.Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, messageProto(message, profiles[message.SenderAccountID]))
	}

	return result
}

func threadProto(topic domainconversation.ConversationTopic) *conversationv1.Thread {
	return &conversationv1.Thread{
		ThreadId:       topic.ID,
		ConversationId: topic.ConversationID,
		RootMessageId:  topic.RootMessageID,
		Title:          topic.Title,
		ReplyCount:     uint32(topic.MessageCount),
		CreatedAt:      protoTime(topic.CreatedAt),
		UpdatedAt:      protoTime(topic.UpdatedAt),
		IsGeneral:      topic.IsGeneral,
		Archived:       topic.Archived,
		Pinned:         topic.Pinned,
		Closed:         topic.Closed,
		LastSequence:   topic.LastSequence,
		LastMessageAt:  protoTime(topic.LastMessageAt),
		ArchivedAt:     protoTime(topic.ArchivedAt),
		ClosedAt:       protoTime(topic.ClosedAt),
	}
}

func moderationPolicyProto(policy domainconversation.ModerationPolicy) *conversationv1.ModerationPolicy {
	return &conversationv1.ModerationPolicy{
		Target:                   moderationTargetProto(policy.TargetKind, policy.TargetID),
		OnlyAdminsCanWrite:       policy.OnlyAdminsCanWrite,
		OnlyAdminsCanAddMembers:  policy.OnlyAdminsCanAddMembers,
		AllowReactions:           policy.AllowReactions,
		AllowForwards:            policy.AllowForwards,
		AllowThreads:             policy.AllowThreads,
		RequireEncryptedMessages: policy.RequireEncryptedMessages,
		RequireTrustedDevices:    policy.RequireTrustedDevices,
		RequireJoinApproval:      policy.RequireJoinApproval,
		PinnedMessagesOnlyAdmins: policy.PinnedMessagesOnlyAdmins,
		SlowModeInterval:         protoDuration(policy.SlowModeInterval),
		AntiSpamWindow:           protoDuration(policy.AntiSpamWindow),
		AntiSpamBurstLimit:       uint32(policy.AntiSpamBurstLimit),
		ShadowMode:               policy.ShadowMode,
		CreatedAt:                protoTime(policy.CreatedAt),
		UpdatedAt:                protoTime(policy.UpdatedAt),
	}
}

func moderationRestrictionProto(
	restriction domainconversation.ModerationRestriction,
) *conversationv1.ModerationRestriction {
	return &conversationv1.ModerationRestriction{
		Target:          moderationTargetProto(restriction.TargetKind, restriction.TargetID),
		UserId:          restriction.AccountID,
		State:           moderationRestrictionStateToProto(restriction.State),
		AppliedByUserId: restriction.AppliedByAccountID,
		Reason:          restriction.Reason,
		ExpiresAt:       protoTime(restriction.ExpiresAt),
		CreatedAt:       protoTime(restriction.CreatedAt),
		UpdatedAt:       protoTime(restriction.UpdatedAt),
	}
}

func moderationReportProto(report domainconversation.ModerationReport) *conversationv1.ModerationReport {
	return &conversationv1.ModerationReport{
		ReportId:         report.ID,
		Target:           moderationTargetProto(report.TargetKind, report.TargetID),
		ReporterUserId:   report.ReporterAccountID,
		TargetUserId:     report.TargetAccountID,
		Reason:           report.Reason,
		Details:          report.Details,
		Status:           moderationReportStatusToProto(report.Status),
		ReviewedByUserId: report.ReviewedByAccountID,
		ReviewedAt:       protoTime(report.ReviewedAt),
		Resolution:       report.Resolution,
		CreatedAt:        protoTime(report.CreatedAt),
		UpdatedAt:        protoTime(report.UpdatedAt),
	}
}

func moderationActionProto(action domainconversation.ModerationAction) *conversationv1.ModerationAction {
	return &conversationv1.ModerationAction{
		ActionId:     action.ID,
		Target:       moderationTargetProto(action.TargetKind, action.TargetID),
		ActorUserId:  action.ActorAccountID,
		TargetUserId: action.TargetAccountID,
		ActionType:   moderationActionTypeToProto(action.Type),
		Duration:     protoDuration(action.Duration),
		Reason:       action.Reason,
		Metadata:     action.Metadata,
		CreatedAt:    protoTime(action.CreatedAt),
	}
}

func moderationRateStateProto(
	state domainconversation.ModerationRateState,
) *conversationv1.ModerationRateState {
	return &conversationv1.ModerationRateState{
		Target:          moderationTargetProto(state.TargetKind, state.TargetID),
		UserId:          state.AccountID,
		LastWriteAt:     protoTime(state.LastWriteAt),
		WindowStartedAt: protoTime(state.WindowStartedAt),
		WindowCount:     state.WindowCount,
		UpdatedAt:       protoTime(state.UpdatedAt),
	}
}

func moderationWriteDecisionProto(
	decision domainconversation.ModerationDecision,
) *conversationv1.ModerationWriteDecision {
	return &conversationv1.ModerationWriteDecision{
		Allowed:      decision.Allowed,
		ShadowHidden: decision.ShadowHidden,
		RetryAfter:   protoDuration(decision.RetryAfter),
	}
}

func eventProto(event domainconversation.EventEnvelope) *commonv1.EventEnvelope {
	return &commonv1.EventEnvelope{
		EventId:        event.EventID,
		EventType:      eventTypeToProto(event.EventType),
		ConversationId: event.ConversationID,
		ActorUserId:    event.ActorAccountID,
		ActorDeviceId:  event.ActorDeviceID,
		CausationId:    event.CausationID,
		CorrelationId:  event.CorrelationID,
		Sequence:       event.Sequence,
		PayloadType:    event.PayloadType,
		Payload:        protoPayload(event.Payload),
		Metadata:       event.Metadata,
		CreatedAt:      protoTime(event.CreatedAt),
	}
}

func (a *api) memberProfiles(
	ctx context.Context,
	members []domainconversation.ConversationMember,
	viewerID string,
) (map[string]*usersv1.UserProfile, error) {
	accountIDs := make([]string, 0, len(members))
	for _, member := range members {
		accountIDs = append(accountIDs, member.AccountID)
	}

	return a.profilesByID(ctx, accountIDs, viewerID)
}

func (a *api) messageProfiles(
	ctx context.Context,
	messages []domainconversation.Message,
	viewerID string,
) (map[string]*usersv1.UserProfile, error) {
	accountIDs := make([]string, 0, len(messages))
	seen := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		if _, ok := seen[message.SenderAccountID]; ok {
			continue
		}
		seen[message.SenderAccountID] = struct{}{}
		accountIDs = append(accountIDs, message.SenderAccountID)
	}

	return a.profilesByID(ctx, accountIDs, viewerID)
}

func zeroTime(value interface{ AsTime() time.Time }) time.Time {
	if value == nil {
		return time.Time{}
	}
	raw := reflect.ValueOf(value)
	if raw.Kind() == reflect.Pointer && raw.IsNil() {
		return time.Time{}
	}

	return value.AsTime()
}

func timeDuration(value interface{ AsDuration() time.Duration }) time.Duration {
	if value == nil {
		return 0
	}
	raw := reflect.ValueOf(value)
	if raw.Kind() == reflect.Pointer && raw.IsNil() {
		return 0
	}

	return value.AsDuration()
}

func moderationTargetFromProto(
	target *conversationv1.ModerationTarget,
) (domainconversation.ModerationTargetKind, string, string, error) {
	if target == nil {
		return domainconversation.ModerationTargetKindUnspecified, "", "", domainconversation.ErrInvalidInput
	}

	targetKind := moderationTargetKindFromProto(target.GetKind())
	conversationID := strings.TrimSpace(target.GetConversationId())
	threadID := strings.TrimSpace(target.GetThreadId())

	switch targetKind {
	case domainconversation.ModerationTargetKindConversation, domainconversation.ModerationTargetKindChannel:
		if conversationID == "" || threadID != "" {
			return domainconversation.ModerationTargetKindUnspecified, "", "", domainconversation.ErrInvalidInput
		}
		return targetKind, conversationID, conversationID, nil
	case domainconversation.ModerationTargetKindTopic:
		if conversationID == "" || threadID == "" {
			return domainconversation.ModerationTargetKindUnspecified, "", "", domainconversation.ErrInvalidInput
		}
		return targetKind, conversationID + gatewayModerationTargetSeparator + threadID, conversationID, nil
	default:
		return domainconversation.ModerationTargetKindUnspecified, "", "", domainconversation.ErrInvalidInput
	}
}

func moderationPolicyParamsFromProto(
	policy *conversationv1.ModerationPolicy,
	actorAccountID string,
) (domainconversation.SetModerationPolicyParams, error) {
	if policy == nil {
		return domainconversation.SetModerationPolicyParams{}, domainconversation.ErrInvalidInput
	}

	targetKind, targetID, _, err := moderationTargetFromProto(policy.GetTarget())
	if err != nil {
		return domainconversation.SetModerationPolicyParams{}, err
	}

	return domainconversation.SetModerationPolicyParams{
		TargetKind:               targetKind,
		TargetID:                 targetID,
		ActorAccountID:           actorAccountID,
		OnlyAdminsCanWrite:       policy.GetOnlyAdminsCanWrite(),
		OnlyAdminsCanAddMembers:  policy.GetOnlyAdminsCanAddMembers(),
		AllowReactions:           policy.GetAllowReactions(),
		AllowForwards:            policy.GetAllowForwards(),
		AllowThreads:             policy.GetAllowThreads(),
		RequireEncryptedMessages: policy.GetRequireEncryptedMessages(),
		RequireTrustedDevices:    policy.GetRequireTrustedDevices(),
		RequireJoinApproval:      policy.GetRequireJoinApproval(),
		PinnedMessagesOnlyAdmins: policy.GetPinnedMessagesOnlyAdmins(),
		SlowModeInterval:         timeDuration(policy.GetSlowModeInterval()),
		AntiSpamWindow:           timeDuration(policy.GetAntiSpamWindow()),
		AntiSpamBurstLimit:       int(policy.GetAntiSpamBurstLimit()),
		ShadowMode:               policy.GetShadowMode(),
	}, nil
}

func (a *api) moderationContext(
	ctx context.Context,
	accountID string,
	target *conversationv1.ModerationTarget,
) (
	domainconversation.Conversation,
	[]domainconversation.ConversationMember,
	domainconversation.ModerationTargetKind,
	string,
	error,
) {
	targetKind, targetID, conversationID, err := moderationTargetFromProto(target)
	if err != nil {
		return domainconversation.Conversation{}, nil, domainconversation.ModerationTargetKindUnspecified, "", err
	}

	conversationRow, members, err := a.conversation.GetConversation(ctx, domainconversation.GetConversationParams{
		ConversationID: conversationID,
		AccountID:      accountID,
	})
	if err != nil {
		return domainconversation.Conversation{}, nil, domainconversation.ModerationTargetKindUnspecified, "", err
	}

	switch targetKind {
	case domainconversation.ModerationTargetKindChannel:
		if conversationRow.Kind != domainconversation.ConversationKindChannel {
			return domainconversation.Conversation{}, nil, domainconversation.ModerationTargetKindUnspecified, "", domainconversation.ErrInvalidInput
		}
	case domainconversation.ModerationTargetKindTopic:
		_, topicID := splitGatewayModerationTarget(targetID)
		if _, err := a.conversation.GetTopic(ctx, domainconversation.GetTopicParams{
			ConversationID: conversationID,
			TopicID:        topicID,
			AccountID:      accountID,
		}); err != nil {
			return domainconversation.Conversation{}, nil, domainconversation.ModerationTargetKindUnspecified, "", err
		}
	}

	return conversationRow, members, targetKind, targetID, nil
}

func (a *api) requireModerationTarget(
	ctx context.Context,
	accountID string,
	target *conversationv1.ModerationTarget,
) (domainconversation.Conversation, domainconversation.ModerationTargetKind, string, error) {
	conversationRow, members, targetKind, targetID, err := a.moderationContext(ctx, accountID, target)
	if err != nil {
		return domainconversation.Conversation{}, domainconversation.ModerationTargetKindUnspecified, "", err
	}
	if !isConversationModerator(members, accountID) {
		return domainconversation.Conversation{}, domainconversation.ModerationTargetKindUnspecified, "", domainconversation.ErrForbidden
	}

	return conversationRow, targetKind, targetID, nil
}

func (a *api) effectiveModerationPolicy(
	ctx context.Context,
	conversationRow domainconversation.Conversation,
	targetKind domainconversation.ModerationTargetKind,
	targetID string,
) (domainconversation.ModerationPolicy, error) {
	policy, err := a.conversation.ModerationPolicyByTarget(ctx, targetKind, targetID)
	if err == nil {
		return policy, nil
	}
	if !errors.Is(err, domainconversation.ErrNotFound) {
		return domainconversation.ModerationPolicy{}, err
	}

	if targetKind == domainconversation.ModerationTargetKindTopic {
		inherited, inheritedErr := a.conversation.ModerationPolicyByTarget(
			ctx,
			domainconversation.ModerationTargetKindConversation,
			conversationRow.ID,
		)
		if inheritedErr == nil {
			inherited.TargetKind = targetKind
			inherited.TargetID = targetID
			return inherited, nil
		}
		if !errors.Is(inheritedErr, domainconversation.ErrNotFound) {
			return domainconversation.ModerationPolicy{}, inheritedErr
		}
	}

	return defaultModerationPolicy(conversationRow, targetKind, targetID), nil
}

func defaultModerationPolicy(
	conversationRow domainconversation.Conversation,
	targetKind domainconversation.ModerationTargetKind,
	targetID string,
) domainconversation.ModerationPolicy {
	return domainconversation.ModerationPolicy{
		TargetKind:               targetKind,
		TargetID:                 targetID,
		OnlyAdminsCanWrite:       conversationRow.Settings.OnlyAdminsCanWrite,
		OnlyAdminsCanAddMembers:  conversationRow.Settings.OnlyAdminsCanAddMembers,
		AllowReactions:           conversationRow.Settings.AllowReactions,
		AllowForwards:            conversationRow.Settings.AllowForwards,
		AllowThreads:             conversationRow.Settings.AllowThreads,
		RequireEncryptedMessages: conversationRow.Settings.RequireEncryptedMessages,
		RequireTrustedDevices:    conversationRow.Settings.RequireTrustedDevices,
		RequireJoinApproval:      conversationRow.Settings.RequireJoinApproval,
		PinnedMessagesOnlyAdmins: conversationRow.Settings.PinnedMessagesOnlyAdmins,
		SlowModeInterval:         conversationRow.Settings.SlowModeInterval,
		CreatedAt:                conversationRow.CreatedAt,
		UpdatedAt:                conversationRow.UpdatedAt,
	}
}

func isConversationModerator(members []domainconversation.ConversationMember, accountID string) bool {
	for _, member := range members {
		if member.AccountID != accountID || !isConversationMemberActive(member) {
			continue
		}
		switch member.Role {
		case domainconversation.MemberRoleOwner, domainconversation.MemberRoleAdmin:
			return true
		}
	}

	return false
}

func canInspectModerationAccount(
	members []domainconversation.ConversationMember,
	viewerAccountID string,
	subjectAccountID string,
) bool {
	if viewerAccountID == subjectAccountID {
		return true
	}

	return isConversationModerator(members, viewerAccountID)
}

func conversationMemberByAccount(
	members []domainconversation.ConversationMember,
	accountID string,
) (domainconversation.ConversationMember, bool) {
	for _, member := range members {
		if member.AccountID == accountID {
			return member, true
		}
	}

	return domainconversation.ConversationMember{}, false
}

func isConversationMemberActive(member domainconversation.ConversationMember) bool {
	return member.LeftAt.IsZero() && !member.Banned
}

func moderationSubjectAccountID(viewerAccountID string, requestedAccountID string) string {
	requestedAccountID = strings.TrimSpace(requestedAccountID)
	if requestedAccountID != "" {
		return requestedAccountID
	}

	return viewerAccountID
}

func conversationUpdateParamsFromRequest(
	req *conversationv1.UpdateConversationRequest,
	actorAccountID string,
	baseSettings domainconversation.ConversationSettings,
) (domainconversation.UpdateConversationParams, error) {
	if req.GetConversation() == nil {
		return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
	}

	row := req.GetConversation()
	maskPaths := req.GetUpdateMask().GetPaths()
	if len(maskPaths) == 0 {
		maskPaths = []string{
			"title",
			"description",
			"avatar_media_id",
			"settings",
		}
	}

	currentSettings := baseSettings
	settingsChanged := false
	var (
		title       *string
		description *string
		avatarID    *string
	)

	for _, path := range maskPaths {
		switch strings.TrimSpace(path) {
		case "title":
			value := row.GetTitle()
			title = &value
		case "description":
			value := row.GetDescription()
			description = &value
		case "avatar_media_id":
			value := row.GetAvatarMediaId()
			avatarID = &value
		case "settings":
			currentSettings = conversationSettingsFromProto(row.GetSettings())
			settingsChanged = true
		case "settings.only_admins_can_write":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.OnlyAdminsCanWrite = row.GetSettings().GetOnlyAdminsCanWrite()
			settingsChanged = true
		case "settings.only_admins_can_add_members":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.OnlyAdminsCanAddMembers = row.GetSettings().GetOnlyAdminsCanAddMembers()
			settingsChanged = true
		case "settings.allow_reactions":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.AllowReactions = row.GetSettings().GetAllowReactions()
			settingsChanged = true
		case "settings.allow_forwards":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.AllowForwards = row.GetSettings().GetAllowForwards()
			settingsChanged = true
		case "settings.allow_threads":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.AllowThreads = row.GetSettings().GetAllowThreads()
			settingsChanged = true
		case "settings.require_join_approval":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.RequireJoinApproval = row.GetSettings().GetRequireJoinApproval()
			settingsChanged = true
		case "settings.pinned_messages_only_admins":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.PinnedMessagesOnlyAdmins = row.GetSettings().GetPinnedMessagesOnlyAdmins()
			settingsChanged = true
		case "settings.slow_mode_interval":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.SlowModeInterval = timeDuration(row.GetSettings().GetSlowModeInterval())
			settingsChanged = true
		case "settings.require_encrypted_messages":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.RequireEncryptedMessages = row.GetSettings().GetRequireEncryptedMessages()
			settingsChanged = true
		case "settings.require_trusted_devices":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.RequireTrustedDevices = row.GetSettings().GetRequireTrustedDevices()
			settingsChanged = true
		case "settings.e2ee_trust_policy":
			if row.GetSettings() == nil {
				return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
			}
			currentSettings.RequireTrustedDevices = conversationSettingsFromProto(row.GetSettings()).RequireTrustedDevices
			settingsChanged = true
		default:
			return domainconversation.UpdateConversationParams{}, domainconversation.ErrInvalidInput
		}
	}

	params := domainconversation.UpdateConversationParams{
		ConversationID: row.GetConversationId(),
		ActorAccountID: actorAccountID,
		Title:          title,
		Description:    description,
		AvatarMediaID:  avatarID,
	}
	if settingsChanged {
		params.Settings = &currentSettings
	}

	return params, nil
}
