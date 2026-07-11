package user

import (
	"context"
	"fmt"
	"time"
)

// Service coordinates privacy, contact, and block operations.
type Service struct {
	store     Store
	directory Directory
	now       func() time.Time
}

// NewService constructs a user-domain service.
func NewService(store Store, directory Directory, opts ...Option) (*Service, error) {
	if store == nil || directory == nil {
		return nil, ErrInvalidInput
	}

	service := &Service{
		store:     store,
		directory: directory,
		now:       func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}

	return service, nil
}

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}

	return s.now().UTC()
}

func (s *Service) validateContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

func defaultPrivacy(accountID string, now time.Time) Privacy {
	privacy := Privacy{
		AccountID:           accountID,
		PhoneVisibility:     VisibilityContacts,
		LastSeenVisibility:  VisibilityContacts,
		MessagePrivacy:      VisibilityEveryone,
		BirthdayVisibility:  VisibilityNobody,
		AllowContactSync:    true,
		AllowUnknownSenders: true,
		AllowUsernameSearch: true,
		ShowReadDate:        true,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	privacy.ShowStatus = PrivacyRule{Base: privacy.LastSeenVisibility}
	privacy.ShowProfilePhoto = PrivacyRule{Base: VisibilityEveryone}
	privacy.ShowProfileAudio = PrivacyRule{Base: VisibilityEveryone}
	privacy.ShowBirthdate = PrivacyRule{Base: privacy.BirthdayVisibility}
	privacy.ShowBio = PrivacyRule{Base: VisibilityEveryone}
	privacy.ShowPhoneNumber = PrivacyRule{Base: privacy.PhoneVisibility}
	privacy.AllowFindingByPhoneNumber = PrivacyRule{Base: VisibilityEveryone}
	privacy.ShowLinkInForwardedMessages = PrivacyRule{Base: VisibilityEveryone}
	privacy.AllowChatInvites = PrivacyRule{Base: VisibilityEveryone}
	privacy.AllowPrivateVoiceAndVideoNoteMessages = PrivacyRule{Base: VisibilityEveryone}
	privacy.AllowCalls = PrivacyRule{Base: VisibilityEveryone}
	privacy.AllowPeerToPeerCalls = PrivacyRule{Base: VisibilityContacts}
	privacy.AutosaveGifts = PrivacyRule{Base: VisibilityEveryone}
	return privacy
}

func defaultAccountSettings(accountID string, now time.Time) AccountSettings {
	return AccountSettings{
		AccountID:              accountID,
		AccountTTLDays:         180,
		InactiveSessionTTLDays: 180,
		DefaultReaction:        "👍",
		AutoDownload: AutoDownloadSettings{
			Mobile:  MediaDownloadSettings{Enabled: true, MaxPhotoBytes: 10 << 20, MaxVideoBytes: 50 << 20, MaxFileBytes: 20 << 20, UseLessDataForCalls: true},
			WiFi:    MediaDownloadSettings{Enabled: true, MaxPhotoBytes: 10 << 20, MaxVideoBytes: 100 << 20, MaxFileBytes: 100 << 20, PreloadLargeVideos: true, PreloadNextAudio: true, PreloadStories: true},
			Roaming: MediaDownloadSettings{UseLessDataForCalls: true},
		},
		Browser: BrowserSettings{DisplayCloseButton: true},
		ReactionNotifications: ReactionNotificationSettings{
			MessageReactions: ReactionNotificationSourceContacts,
			StoryReactions:   ReactionNotificationSourceContacts,
			PollVotes:        ReactionNotificationSourceContacts,
			ShowPreview:      true,
		},
		AllowNewChatsFromUnknownUsers: true,
		CreatedAt:                     now,
		UpdatedAt:                     now,
	}
}
