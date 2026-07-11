package user

import (
	"context"
	"fmt"
	"strings"
)

const maxSynchronizedMediaBytes int64 = 4 << 30

// GetAccountSettings returns synchronized account settings, materializing defaults when needed.
func (s *Service) GetAccountSettings(ctx context.Context, accountID string) (AccountSettings, error) {
	if err := s.validateContext(ctx, "get account settings"); err != nil {
		return AccountSettings{}, err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return AccountSettings{}, ErrInvalidInput
	}
	if _, err := s.directory.AccountByID(ctx, accountID); err != nil {
		return AccountSettings{}, fmt.Errorf("load account %s for settings: %w", accountID, err)
	}
	settings, err := s.store.AccountSettingsByAccountID(ctx, accountID)
	if err == nil {
		return settings, nil
	}
	if err != ErrNotFound {
		return AccountSettings{}, fmt.Errorf("load account settings for %s: %w", accountID, err)
	}
	return defaultAccountSettings(accountID, s.currentTime()), nil
}

// UpdateAccountSettings applies only the top-level fields named by FieldMask.
func (s *Service) UpdateAccountSettings(ctx context.Context, params UpdateAccountSettingsParams) (AccountSettings, error) {
	if err := s.validateContext(ctx, "update account settings"); err != nil {
		return AccountSettings{}, err
	}
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.AccountID == "" || len(params.FieldMask) == 0 {
		return AccountSettings{}, ErrInvalidInput
	}

	settings, err := s.GetAccountSettings(ctx, params.AccountID)
	if err != nil {
		return AccountSettings{}, err
	}
	for _, path := range params.FieldMask {
		switch path {
		case "account_ttl_days":
			settings.AccountTTLDays = params.Settings.AccountTTLDays
		case "inactive_session_ttl_days":
			settings.InactiveSessionTTLDays = params.Settings.InactiveSessionTTLDays
		case "default_reaction":
			settings.DefaultReaction = strings.TrimSpace(params.Settings.DefaultReaction)
		case "default_message_auto_delete_seconds":
			settings.DefaultMessageAutoDeleteSeconds = params.Settings.DefaultMessageAutoDeleteSeconds
		case "auto_download":
			settings.AutoDownload = params.Settings.AutoDownload
		case "autosave":
			settings.Autosave = params.Settings.Autosave
		case "browser":
			settings.Browser = params.Settings.Browser
		case "reaction_notifications":
			settings.ReactionNotifications = params.Settings.ReactionNotifications
		case "allow_new_chats_from_unknown_users":
			settings.AllowNewChatsFromUnknownUsers = params.Settings.AllowNewChatsFromUnknownUsers
		case "incoming_paid_message_star_count":
			settings.IncomingPaidMessageStarCount = params.Settings.IncomingPaidMessageStarCount
		default:
			return AccountSettings{}, ErrInvalidInput
		}
	}
	for i := range settings.Browser.Exceptions {
		settings.Browser.Exceptions[i].Domain = strings.ToLower(strings.TrimSpace(settings.Browser.Exceptions[i].Domain))
	}
	if err := validateAccountSettings(settings); err != nil {
		return AccountSettings{}, err
	}

	now := s.currentTime()
	settings.AccountID = params.AccountID
	if settings.CreatedAt.IsZero() {
		settings.CreatedAt = now
	}
	settings.UpdatedAt = now
	saved, err := s.store.SaveAccountSettings(ctx, settings)
	if err != nil {
		return AccountSettings{}, fmt.Errorf("save account settings for %s: %w", params.AccountID, err)
	}
	return saved, nil
}

func validateAccountSettings(settings AccountSettings) error {
	if settings.AccountTTLDays < 30 || settings.AccountTTLDays > 730 ||
		settings.InactiveSessionTTLDays < 1 || settings.InactiveSessionTTLDays > 730 ||
		settings.DefaultMessageAutoDeleteSeconds > 31_536_000 || settings.DefaultMessageAutoDeleteSeconds%86_400 != 0 ||
		settings.IncomingPaidMessageStarCount < 0 || settings.IncomingPaidMessageStarCount > 1_000_000 ||
		(settings.IncomingPaidMessageStarCount > 0 && !settings.AllowNewChatsFromUnknownUsers) ||
		settings.DefaultReaction == "" || len(settings.DefaultReaction) > 32 || len(settings.ReactionNotifications.SoundID) > 256 ||
		!validReactionSource(settings.ReactionNotifications.MessageReactions) ||
		!validReactionSource(settings.ReactionNotifications.StoryReactions) ||
		!validReactionSource(settings.ReactionNotifications.PollVotes) {
		return ErrInvalidInput
	}
	for _, policy := range []MediaDownloadSettings{settings.AutoDownload.Mobile, settings.AutoDownload.WiFi, settings.AutoDownload.Roaming} {
		if !validMediaSizes(policy.MaxPhotoBytes, policy.MaxVideoBytes, policy.MaxFileBytes) {
			return ErrInvalidInput
		}
	}
	for _, policy := range []MediaAutosaveSettings{settings.Autosave.PrivateChats, settings.Autosave.GroupChats, settings.Autosave.ChannelChats} {
		if !validMediaSizes(policy.MaxVideoBytes) {
			return ErrInvalidInput
		}
	}
	if len(settings.Browser.Exceptions) > 256 {
		return ErrInvalidInput
	}
	seenDomains := make(map[string]struct{}, len(settings.Browser.Exceptions))
	for i := range settings.Browser.Exceptions {
		domain := settings.Browser.Exceptions[i].Domain
		if domain == "" || len(domain) > 253 || strings.ContainsAny(domain, "/:@ ") {
			return ErrInvalidInput
		}
		if _, exists := seenDomains[domain]; exists {
			return ErrInvalidInput
		}
		seenDomains[domain] = struct{}{}
	}
	return nil
}

func validMediaSizes(sizes ...int64) bool {
	for _, size := range sizes {
		if size < 0 || size > maxSynchronizedMediaBytes {
			return false
		}
	}
	return true
}

func validReactionSource(source ReactionNotificationSource) bool {
	return source == ReactionNotificationSourceNone || source == ReactionNotificationSourceContacts || source == ReactionNotificationSourceAll
}
