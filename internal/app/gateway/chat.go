package gateway

import (
	"context"
	"strings"
	"time"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
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
	a.notifySyncSubscribers()

	return &conversationv1.CreateConversationResponse{
		Conversation: conversationProto(conversationRow),
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

	return &conversationv1.GetConversationResponse{
		Conversation: conversationProto(conversationRow),
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
		result = append(result, conversationProto(conversationRow))
	}

	return &conversationv1.ListConversationsResponse{
		Conversations: result,
		Page: &commonv1.PageResponse{
			NextPageToken: offsetToken("conversations", end),
			TotalSize:     uint64(len(conversations)),
		},
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
	a.notifySyncSubscribers()

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

	message, event, err := a.conversation.SendMessage(ctx, domainconversation.SendMessageParams{
		ConversationID:  req.GetConversationId(),
		SenderAccountID: authContext.Account.ID,
		SenderDeviceID:  authContext.Device.ID,
		Draft:           draftFromProto(req.GetDraft()),
		CausationID:     req.GetIdempotencyKey(),
		CorrelationID:   req.GetDraft().GetClientMessageId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

	profiles, err := a.profilesByID(ctx, []string{message.SenderAccountID}, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.SendMessageResponse{
		Message: messageProto(message, profiles[message.SenderAccountID]),
		Event:   eventProto(event),
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

	message, _, err := a.conversation.EditMessage(ctx, domainconversation.EditMessageParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		ActorAccountID: authContext.Account.ID,
		ActorDeviceID:  authContext.Device.ID,
		Draft:          draftFromProto(req.GetDraft()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

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

	message, _, err := a.conversation.DeleteMessage(ctx, domainconversation.DeleteMessageParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		ActorAccountID: authContext.Account.ID,
		ActorDeviceID:  authContext.Device.ID,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

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

	message, _, err := a.conversation.AddMessageReaction(ctx, domainconversation.AddMessageReactionParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		ActorAccountID: authContext.Account.ID,
		ActorDeviceID:  authContext.Device.ID,
		Reaction:       req.GetEmoji(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

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

	message, _, err := a.conversation.RemoveMessageReaction(ctx, domainconversation.RemoveMessageReactionParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		ActorAccountID: authContext.Account.ID,
		ActorDeviceID:  authContext.Device.ID,
		Reaction:       req.GetEmoji(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

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

	message, _, err := a.conversation.PinMessage(ctx, domainconversation.PinMessageParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		ActorAccountID: authContext.Account.ID,
		ActorDeviceID:  authContext.Device.ID,
		Pinned:         true,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

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

	state, _, err := a.conversation.MarkRead(ctx, domainconversation.MarkReadParams{
		ConversationID:      req.GetConversationId(),
		AccountID:           authContext.Account.ID,
		DeviceID:            authContext.Device.ID,
		ReadThroughSequence: readThrough,
		CausationID:         req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

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

	topic, _, err := a.conversation.CreateTopic(ctx, domainconversation.CreateTopicParams{
		ConversationID:   req.GetConversationId(),
		RootMessageID:    req.GetRootMessageId(),
		CreatorAccountID: authContext.Account.ID,
		Title:            req.GetTitle(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	a.notifySyncSubscribers()

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
		ConversationID: req.GetConversationId(),
		AccountID:      authContext.Account.ID,
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

func conversationSettingsFromProto(settings *conversationv1.ConversationSettings) domainconversation.ConversationSettings {
	if settings == nil {
		return domainconversation.ConversationSettings{}
	}

	slowMode := timeDuration(settings.GetSlowModeInterval())
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
	}
}

func conversationProto(conversationRow domainconversation.Conversation) *conversationv1.Conversation {
	return &conversationv1.Conversation{
		ConversationId:     conversationRow.ID,
		Kind:               conversationKindToProto(conversationRow.Kind),
		Title:              conversationRow.Title,
		Description:        conversationRow.Description,
		AvatarMediaId:      conversationRow.AvatarMediaID,
		OwnerUserId:        conversationRow.OwnerAccountID,
		Settings:           conversationSettingsProto(conversationRow.Settings),
		Archived:           conversationRow.Archived,
		Muted:              conversationRow.Muted,
		Pinned:             conversationRow.Pinned,
		Hidden:             conversationRow.Hidden,
		LastSequence:       conversationRow.LastSequence,
		CreatedAt:          protoTime(conversationRow.CreatedAt),
		UpdatedAt:          protoTime(conversationRow.UpdatedAt),
		LastMessageAt:      protoTime(conversationRow.LastMessageAt),
		UnreadCount:        conversationRow.UnreadCount,
		UnreadMentionCount: conversationRow.UnreadMentionCount,
	}
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
	}
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

	return value.AsTime()
}

func timeDuration(value interface{ AsDuration() time.Duration }) time.Duration {
	if value == nil {
		return 0
	}

	return value.AsDuration()
}
