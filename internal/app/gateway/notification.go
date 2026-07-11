package gateway

import (
	"context"
	"strings"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	notificationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/notification/v1"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
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
		ConversationID:                    req.GetOverride().GetConversationId(),
		AccountID:                         authContext.Account.ID,
		Muted:                             req.GetOverride().GetMuted(),
		MentionsOnly:                      req.GetOverride().GetMentionsOnly(),
		MutedUntil:                        zeroTime(req.GetOverride().GetMutedUntil()),
		UpdatedAt:                         zeroTime(req.GetOverride().GetUpdatedAt()),
		ShowPreview:                       req.GetOverride().GetShowPreview(),
		SoundID:                           req.GetOverride().GetSoundId(),
		MuteStories:                       req.GetOverride().GetMuteStories(),
		StorySoundID:                      req.GetOverride().GetStorySoundId(),
		ShowStorySender:                   req.GetOverride().GetShowStorySender(),
		DisablePinnedMessageNotifications: req.GetOverride().GetDisablePinnedMessageNotifications(),
		DisableMentionNotifications:       req.GetOverride().GetDisableMentionNotifications(),
		UseDefaultMuteFor:                 req.GetOverride().GetUseDefaultMuteFor(),
		UseDefaultSound:                   req.GetOverride().GetUseDefaultSound(),
		UseDefaultShowPreview:             req.GetOverride().GetUseDefaultShowPreview(),
		UseDefaultMuteStories:             req.GetOverride().GetUseDefaultMuteStories(),
		UseDefaultStorySound:              req.GetOverride().GetUseDefaultStorySound(),
		UseDefaultShowStorySender:         req.GetOverride().GetUseDefaultShowStorySender(),
		UseDefaultDisablePinnedMessageNotifications: req.GetOverride().GetUseDefaultDisablePinnedMessageNotifications(),
		UseDefaultDisableMentionNotifications:       req.GetOverride().GetUseDefaultDisableMentionNotifications(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &notificationv1.SetConversationNotificationOverrideResponse{
		Override: notificationOverrideProto(override),
	}, nil
}

func (a *api) GetScopeNotificationSettings(ctx context.Context, req *notificationv1.GetScopeNotificationSettingsRequest) (*notificationv1.GetScopeNotificationSettingsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := a.notification.ScopeSettingsByAccountAndScope(ctx, authContext.Account.ID, notificationScopeFromProto(req.GetScope()))
	if err != nil {
		return nil, grpcError(err)
	}
	return &notificationv1.GetScopeNotificationSettingsResponse{Settings: scopeSettingsProto(settings)}, nil
}

func (a *api) SetScopeNotificationSettings(ctx context.Context, req *notificationv1.SetScopeNotificationSettingsRequest) (*notificationv1.SetScopeNotificationSettingsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSettings() == nil {
		return nil, grpcError(domainnotification.ErrInvalidInput)
	}
	value := req.GetSettings()
	settings, err := a.notification.SetScopeSettings(ctx, domainnotification.SetScopeSettingsParams{
		AccountID: authContext.Account.ID, Scope: notificationScopeFromProto(value.GetScope()), MutedUntil: zeroTime(value.GetMutedUntil()),
		ShowPreview: value.GetShowPreview(), SoundID: value.GetSoundId(), MuteStories: value.GetMuteStories(), StorySoundID: value.GetStorySoundId(),
		ShowStorySender: value.GetShowStorySender(), DisablePinnedMessageNotifications: value.GetDisablePinnedMessageNotifications(),
		DisableMentionNotifications: value.GetDisableMentionNotifications(), UpdatedAt: zeroTime(value.GetUpdatedAt()),
		UseDefaultMuteStories: value.GetUseDefaultMuteStories(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &notificationv1.SetScopeNotificationSettingsResponse{Settings: scopeSettingsProto(settings)}, nil
}

func (a *api) GetReactionNotificationSettings(ctx context.Context, _ *notificationv1.GetReactionNotificationSettingsRequest) (*notificationv1.GetReactionNotificationSettingsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := a.notification.ReactionSettingsByAccountID(ctx, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}
	return &notificationv1.GetReactionNotificationSettingsResponse{Settings: reactionSettingsProto(settings)}, nil
}

func (a *api) SetReactionNotificationSettings(ctx context.Context, req *notificationv1.SetReactionNotificationSettingsRequest) (*notificationv1.SetReactionNotificationSettingsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSettings() == nil {
		return nil, grpcError(domainnotification.ErrInvalidInput)
	}
	value := req.GetSettings()
	settings, err := a.notification.SetReactionSettings(ctx, domainnotification.SetReactionSettingsParams{AccountID: authContext.Account.ID, MessageReactionSource: reactionSourceFromProto(value.GetMessageReactionSource()), StoryReactionSource: reactionSourceFromProto(value.GetStoryReactionSource()), PollVoteSource: reactionSourceFromProto(value.GetPollVoteSource()), SoundID: value.GetSoundId(), ShowPreview: value.GetShowPreview(), UpdatedAt: zeroTime(value.GetUpdatedAt())})
	if err != nil {
		return nil, grpcError(err)
	}
	return &notificationv1.SetReactionNotificationSettingsResponse{Settings: reactionSettingsProto(settings)}, nil
}

func (a *api) ListSavedNotificationSounds(ctx context.Context, _ *notificationv1.ListSavedNotificationSoundsRequest) (*notificationv1.ListSavedNotificationSoundsResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	sounds, err := a.notification.SavedSoundsByAccountID(ctx, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}
	response := make([]*notificationv1.SavedNotificationSound, 0, len(sounds))
	for _, sound := range sounds {
		response = append(response, savedSoundProto(sound))
	}
	return &notificationv1.ListSavedNotificationSoundsResponse{Sounds: response}, nil
}

func (a *api) AddSavedNotificationSound(ctx context.Context, req *notificationv1.AddSavedNotificationSoundRequest) (*notificationv1.AddSavedNotificationSoundResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	asset, err := a.media.GetMedia(ctx, authContext.Account.ID, req.GetMediaId())
	if err != nil {
		return nil, grpcError(err)
	}
	if asset.Status != domainmedia.MediaStatusReady || !strings.HasPrefix(strings.ToLower(asset.ContentType), "audio/") {
		return nil, grpcError(domainnotification.ErrInvalidInput)
	}
	title := strings.TrimSpace(req.GetTitle())
	if title == "" {
		title = asset.FileName
	}
	sound, err := a.notification.AddSavedSound(ctx, domainnotification.AddSavedSoundParams{AccountID: authContext.Account.ID, MediaID: asset.ID, Title: title})
	if err != nil {
		return nil, grpcError(err)
	}
	return &notificationv1.AddSavedNotificationSoundResponse{Sound: savedSoundProto(sound)}, nil
}

func (a *api) RemoveSavedNotificationSound(ctx context.Context, req *notificationv1.RemoveSavedNotificationSoundRequest) (*notificationv1.RemoveSavedNotificationSoundResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := a.notification.RemoveSavedSound(ctx, authContext.Account.ID, req.GetSoundId()); err != nil {
		return nil, grpcError(err)
	}
	return &notificationv1.RemoveSavedNotificationSoundResponse{}, nil
}

// DeleteConversationNotificationOverride restores inherited notification settings.
func (a *api) DeleteConversationNotificationOverride(
	ctx context.Context,
	req *notificationv1.DeleteConversationNotificationOverrideRequest,
) (*notificationv1.DeleteConversationNotificationOverrideResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if err := a.requireConversationAccess(ctx, authContext.Account.ID, req.GetConversationId()); err != nil {
		return nil, grpcError(err)
	}
	if err := a.notification.DeleteConversationOverride(ctx, req.GetConversationId(), authContext.Account.ID); err != nil {
		return nil, grpcError(err)
	}
	return &notificationv1.DeleteConversationNotificationOverrideResponse{}, nil
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
		ConversationId:                    override.ConversationID,
		Muted:                             override.Muted,
		MentionsOnly:                      override.MentionsOnly,
		MutedUntil:                        protoTime(override.MutedUntil),
		UpdatedAt:                         protoTime(override.UpdatedAt),
		ShowPreview:                       override.ShowPreview,
		SoundId:                           override.SoundID,
		MuteStories:                       override.MuteStories,
		StorySoundId:                      override.StorySoundID,
		ShowStorySender:                   override.ShowStorySender,
		DisablePinnedMessageNotifications: override.DisablePinnedMessageNotifications,
		DisableMentionNotifications:       override.DisableMentionNotifications,
		UseDefaultMuteFor:                 override.UseDefaultMuteFor,
		UseDefaultSound:                   override.UseDefaultSound,
		UseDefaultShowPreview:             override.UseDefaultShowPreview,
		UseDefaultMuteStories:             override.UseDefaultMuteStories,
		UseDefaultStorySound:              override.UseDefaultStorySound,
		UseDefaultShowStorySender:         override.UseDefaultShowStorySender,
		UseDefaultDisablePinnedMessageNotifications: override.UseDefaultDisablePinnedMessageNotifications,
		UseDefaultDisableMentionNotifications:       override.UseDefaultDisableMentionNotifications,
	}
}

func scopeSettingsProto(settings domainnotification.ScopeSettings) *notificationv1.ScopeNotificationSettings {
	return &notificationv1.ScopeNotificationSettings{Scope: notificationScopeProto(settings.Scope), MutedUntil: protoTime(settings.MutedUntil), ShowPreview: settings.ShowPreview, SoundId: settings.SoundID, MuteStories: settings.MuteStories, StorySoundId: settings.StorySoundID, ShowStorySender: settings.ShowStorySender, DisablePinnedMessageNotifications: settings.DisablePinnedMessageNotifications, DisableMentionNotifications: settings.DisableMentionNotifications, UpdatedAt: protoTime(settings.UpdatedAt), UseDefaultMuteStories: settings.UseDefaultMuteStories}
}

func reactionSettingsProto(settings domainnotification.ReactionSettings) *notificationv1.ReactionNotificationSettings {
	return &notificationv1.ReactionNotificationSettings{MessageReactionSource: reactionSourceProto(settings.MessageReactionSource), StoryReactionSource: reactionSourceProto(settings.StoryReactionSource), PollVoteSource: reactionSourceProto(settings.PollVoteSource), SoundId: settings.SoundID, ShowPreview: settings.ShowPreview, UpdatedAt: protoTime(settings.UpdatedAt)}
}

func savedSoundProto(sound domainnotification.SavedSound) *notificationv1.SavedNotificationSound {
	return &notificationv1.SavedNotificationSound{SoundId: sound.SoundID, MediaId: sound.MediaID, Title: sound.Title, CreatedAt: protoTime(sound.CreatedAt)}
}

func notificationScopeFromProto(scope notificationv1.NotificationSettingsScope) domainnotification.SettingsScope {
	switch scope {
	case notificationv1.NotificationSettingsScope_NOTIFICATION_SETTINGS_SCOPE_PRIVATE_CHATS:
		return domainnotification.SettingsScopePrivateChats
	case notificationv1.NotificationSettingsScope_NOTIFICATION_SETTINGS_SCOPE_GROUP_CHATS:
		return domainnotification.SettingsScopeGroupChats
	case notificationv1.NotificationSettingsScope_NOTIFICATION_SETTINGS_SCOPE_CHANNEL_CHATS:
		return domainnotification.SettingsScopeChannelChats
	default:
		return domainnotification.SettingsScopeUnspecified
	}
}

func notificationScopeProto(scope domainnotification.SettingsScope) notificationv1.NotificationSettingsScope {
	switch scope {
	case domainnotification.SettingsScopePrivateChats:
		return notificationv1.NotificationSettingsScope_NOTIFICATION_SETTINGS_SCOPE_PRIVATE_CHATS
	case domainnotification.SettingsScopeGroupChats:
		return notificationv1.NotificationSettingsScope_NOTIFICATION_SETTINGS_SCOPE_GROUP_CHATS
	case domainnotification.SettingsScopeChannelChats:
		return notificationv1.NotificationSettingsScope_NOTIFICATION_SETTINGS_SCOPE_CHANNEL_CHATS
	default:
		return notificationv1.NotificationSettingsScope_NOTIFICATION_SETTINGS_SCOPE_UNSPECIFIED
	}
}

func reactionSourceFromProto(source notificationv1.ReactionNotificationSource) domainnotification.ReactionSource {
	switch source {
	case notificationv1.ReactionNotificationSource_REACTION_NOTIFICATION_SOURCE_NONE:
		return domainnotification.ReactionSourceNone
	case notificationv1.ReactionNotificationSource_REACTION_NOTIFICATION_SOURCE_CONTACTS:
		return domainnotification.ReactionSourceContacts
	case notificationv1.ReactionNotificationSource_REACTION_NOTIFICATION_SOURCE_ALL:
		return domainnotification.ReactionSourceAll
	default:
		return domainnotification.ReactionSourceUnspecified
	}
}

func reactionSourceProto(source domainnotification.ReactionSource) notificationv1.ReactionNotificationSource {
	switch source {
	case domainnotification.ReactionSourceNone:
		return notificationv1.ReactionNotificationSource_REACTION_NOTIFICATION_SOURCE_NONE
	case domainnotification.ReactionSourceContacts:
		return notificationv1.ReactionNotificationSource_REACTION_NOTIFICATION_SOURCE_CONTACTS
	case domainnotification.ReactionSourceAll:
		return notificationv1.ReactionNotificationSource_REACTION_NOTIFICATION_SOURCE_ALL
	default:
		return notificationv1.ReactionNotificationSource_REACTION_NOTIFICATION_SOURCE_UNSPECIFIED
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
