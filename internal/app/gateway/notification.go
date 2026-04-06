package gateway

import (
	"context"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	notificationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/notification/v1"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	domainnotification "github.com/dm-vev/zvonilka/internal/domain/notification"
)

// GetNotificationPreference returns the effective notification preference for the authenticated account.
func (a *api) GetNotificationPreference(
	ctx context.Context,
	_ *notificationv1.GetNotificationPreferenceRequest,
) (*notificationv1.GetNotificationPreferenceResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	preference, err := a.notification.PreferenceByAccountID(ctx, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	return &notificationv1.GetNotificationPreferenceResponse{
		Preference: notificationPreferenceProto(preference),
	}, nil
}

// SetNotificationPreference replaces the authenticated account's notification preference.
func (a *api) SetNotificationPreference(
	ctx context.Context,
	req *notificationv1.SetNotificationPreferenceRequest,
) (*notificationv1.SetNotificationPreferenceResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetPreference() == nil {
		return nil, grpcError(domainnotification.ErrInvalidInput)
	}

	preference, err := a.notification.SetPreference(ctx, domainnotification.SetPreferenceParams{
		AccountID:      authContext.Account.ID,
		Enabled:        req.GetPreference().GetEnabled(),
		DirectEnabled:  req.GetPreference().GetDirectEnabled(),
		GroupEnabled:   req.GetPreference().GetGroupEnabled(),
		ChannelEnabled: req.GetPreference().GetChannelEnabled(),
		MentionEnabled: req.GetPreference().GetMentionEnabled(),
		ReplyEnabled:   req.GetPreference().GetReplyEnabled(),
		QuietHours:     quietHoursFromProto(req.GetPreference().GetQuietHours()),
		MutedUntil:     zeroTime(req.GetPreference().GetMutedUntil()),
		UpdatedAt:      zeroTime(req.GetPreference().GetUpdatedAt()),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &notificationv1.SetNotificationPreferenceResponse{
		Preference: notificationPreferenceProto(preference),
	}, nil
}

// GetConversationNotificationOverride returns the authenticated account's per-conversation override.
func (a *api) GetConversationNotificationOverride(
	ctx context.Context,
	req *notificationv1.GetConversationNotificationOverrideRequest,
) (*notificationv1.GetConversationNotificationOverrideResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := a.requireConversationAccess(ctx, authContext.Account.ID, req.GetConversationId()); err != nil {
		return nil, grpcError(err)
	}

	override, err := a.notification.ConversationOverrideByConversationAndAccount(
		ctx,
		req.GetConversationId(),
		authContext.Account.ID,
	)
	if err != nil {
		return nil, grpcError(err)
	}

	return &notificationv1.GetConversationNotificationOverrideResponse{
		Override: notificationOverrideProto(override),
	}, nil
}

// SetConversationNotificationOverride replaces the authenticated account's per-conversation override.
func (a *api) SetConversationNotificationOverride(
	ctx context.Context,
	req *notificationv1.SetConversationNotificationOverrideRequest,
) (*notificationv1.SetConversationNotificationOverrideResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetOverride() == nil {
		return nil, grpcError(domainnotification.ErrInvalidInput)
	}
	if err := a.requireConversationAccess(ctx, authContext.Account.ID, req.GetOverride().GetConversationId()); err != nil {
		return nil, grpcError(err)
	}

	override, err := a.notification.SetConversationOverride(ctx, domainnotification.SetOverrideParams{
		ConversationID: req.GetOverride().GetConversationId(),
		AccountID:      authContext.Account.ID,
		Muted:          req.GetOverride().GetMuted(),
		MentionsOnly:   req.GetOverride().GetMentionsOnly(),
		MutedUntil:     zeroTime(req.GetOverride().GetMutedUntil()),
		UpdatedAt:      zeroTime(req.GetOverride().GetUpdatedAt()),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &notificationv1.SetConversationNotificationOverrideResponse{
		Override: notificationOverrideProto(override),
	}, nil
}

// ListPushTokens returns the active push tokens registered for the authenticated account.
func (a *api) ListPushTokens(
	ctx context.Context,
	_ *notificationv1.ListPushTokensRequest,
) (*notificationv1.ListPushTokensResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	tokens, err := a.notification.PushTokensByAccountID(ctx, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}

	response := make([]*notificationv1.PushToken, 0, len(tokens))
	for _, token := range tokens {
		response = append(response, notificationPushTokenProto(token))
	}

	return &notificationv1.ListPushTokensResponse{PushTokens: response}, nil
}

// RegisterPushToken registers or refreshes the authenticated device's push token.
func (a *api) RegisterPushToken(
	ctx context.Context,
	req *notificationv1.RegisterPushTokenRequest,
) (*notificationv1.RegisterPushTokenResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	token, err := a.notification.RegisterPushToken(ctx, domainnotification.RegisterPushTokenParams{
		AccountID: authContext.Account.ID,
		DeviceID:  authContext.Device.ID,
		Provider:  req.GetProvider(),
		Token:     req.GetToken(),
		Platform:  authContext.Device.Platform,
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &notificationv1.RegisterPushTokenResponse{
		PushToken: notificationPushTokenProto(token),
	}, nil
}

// RevokePushToken revokes one push token owned by the authenticated account.
func (a *api) RevokePushToken(
	ctx context.Context,
	req *notificationv1.RevokePushTokenRequest,
) (*notificationv1.RevokePushTokenResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	token, err := a.notification.PushTokenByID(ctx, req.GetTokenId())
	if err != nil {
		return nil, grpcError(err)
	}
	if token.AccountID != authContext.Account.ID {
		return nil, grpcError(domainconversation.ErrForbidden)
	}

	revoked, err := a.notification.RevokePushToken(ctx, domainnotification.RevokePushTokenParams{
		TokenID: req.GetTokenId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &notificationv1.RevokePushTokenResponse{
		PushToken: notificationPushTokenProto(revoked),
	}, nil
}

func (a *api) requireConversationAccess(ctx context.Context, accountID string, conversationID string) error {
	_, _, err := a.conversation.GetConversation(ctx, domainconversation.GetConversationParams{
		ConversationID: conversationID,
		AccountID:      accountID,
	})
	return err
}

func notificationPreferenceProto(preference domainnotification.Preference) *notificationv1.NotificationPreference {
	return &notificationv1.NotificationPreference{
		Enabled:        preference.Enabled,
		DirectEnabled:  preference.DirectEnabled,
		GroupEnabled:   preference.GroupEnabled,
		ChannelEnabled: preference.ChannelEnabled,
		MentionEnabled: preference.MentionEnabled,
		ReplyEnabled:   preference.ReplyEnabled,
		QuietHours:     quietHoursProto(preference.QuietHours),
		MutedUntil:     protoTime(preference.MutedUntil),
		UpdatedAt:      protoTime(preference.UpdatedAt),
	}
}

func quietHoursProto(quietHours domainnotification.QuietHours) *notificationv1.QuietHours {
	return &notificationv1.QuietHours{
		Enabled:     quietHours.Enabled,
		StartMinute: uint32(quietHours.StartMinute),
		EndMinute:   uint32(quietHours.EndMinute),
		Timezone:    quietHours.Timezone,
	}
}

func quietHoursFromProto(quietHours *notificationv1.QuietHours) domainnotification.QuietHours {
	if quietHours == nil {
		return domainnotification.QuietHours{}
	}

	return domainnotification.QuietHours{
		Enabled:     quietHours.GetEnabled(),
		StartMinute: int(quietHours.GetStartMinute()),
		EndMinute:   int(quietHours.GetEndMinute()),
		Timezone:    quietHours.GetTimezone(),
	}
}

func notificationOverrideProto(override domainnotification.ConversationOverride) *notificationv1.ConversationNotificationOverride {
	return &notificationv1.ConversationNotificationOverride{
		ConversationId: override.ConversationID,
		Muted:          override.Muted,
		MentionsOnly:   override.MentionsOnly,
		MutedUntil:     protoTime(override.MutedUntil),
		UpdatedAt:      protoTime(override.UpdatedAt),
	}
}

func notificationPushTokenProto(token domainnotification.PushToken) *notificationv1.PushToken {
	return &notificationv1.PushToken{
		TokenId:   token.ID,
		DeviceId:  token.DeviceID,
		Provider:  token.Provider,
		Platform:  commonv1.DevicePlatform(identityPlatformToProto(token.Platform)),
		Enabled:   token.Enabled,
		CreatedAt: protoTime(token.CreatedAt),
		UpdatedAt: protoTime(token.UpdatedAt),
		RevokedAt: protoTime(token.RevokedAt),
	}
}
