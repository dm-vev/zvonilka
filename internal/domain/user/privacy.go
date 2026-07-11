package user

import (
	"context"
	"fmt"
	"strings"
)

// GetPrivacy returns one account's privacy settings, materializing defaults when needed.
func (s *Service) GetPrivacy(ctx context.Context, accountID string) (Privacy, error) {
	return s.privacyByAccountID(ctx, s.store, accountID)
}

func (s *Service) privacyByAccountID(ctx context.Context, store Store, accountID string) (Privacy, error) {
	if err := s.validateContext(ctx, "get privacy"); err != nil {
		return Privacy{}, err
	}

	accountID = strings.TrimSpace(accountID)
	if accountID == "" || store == nil {
		return Privacy{}, ErrInvalidInput
	}

	if _, err := s.directory.AccountByID(ctx, accountID); err != nil {
		return Privacy{}, fmt.Errorf("load account %s for privacy: %w", accountID, err)
	}

	privacy, err := store.PrivacyByAccountID(ctx, accountID)
	if err == nil {
		return privacy, nil
	}
	if err != ErrNotFound {
		return Privacy{}, fmt.Errorf("load privacy for %s: %w", accountID, err)
	}

	return defaultPrivacy(accountID, s.currentTime()), nil
}

// UpdatePrivacy replaces the persisted privacy settings for one account.
func (s *Service) UpdatePrivacy(ctx context.Context, params UpdatePrivacyParams) (Privacy, error) {
	if err := s.validateContext(ctx, "update privacy"); err != nil {
		return Privacy{}, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.AccountID == "" {
		return Privacy{}, ErrInvalidInput
	}
	if _, err := s.directory.AccountByID(ctx, params.AccountID); err != nil {
		return Privacy{}, fmt.Errorf("load account %s for privacy update: %w", params.AccountID, err)
	}

	now := s.currentTime()
	privacy, err := s.GetPrivacy(ctx, params.AccountID)
	if err != nil {
		return Privacy{}, err
	}

	privacy.AccountID = params.AccountID
	privacy.PhoneVisibility = normalizeVisibility(params.Privacy.PhoneVisibility)
	privacy.LastSeenVisibility = normalizeVisibility(params.Privacy.LastSeenVisibility)
	privacy.MessagePrivacy = normalizeVisibility(params.Privacy.MessagePrivacy)
	privacy.BirthdayVisibility = normalizeVisibility(params.Privacy.BirthdayVisibility)
	privacy.AllowContactSync = params.Privacy.AllowContactSync
	privacy.AllowUnknownSenders = params.Privacy.AllowUnknownSenders
	privacy.AllowUsernameSearch = params.Privacy.AllowUsernameSearch
	if params.Privacy.ShowStatus.Base == VisibilityUnspecified {
		privacy.ShowStatus.Base = privacy.LastSeenVisibility
	}
	if params.Privacy.ShowBirthdate.Base == VisibilityUnspecified {
		privacy.ShowBirthdate.Base = privacy.BirthdayVisibility
	}
	if params.Privacy.ShowPhoneNumber.Base == VisibilityUnspecified {
		privacy.ShowPhoneNumber.Base = privacy.PhoneVisibility
	}
	updates := []struct {
		dst *PrivacyRule
		src PrivacyRule
	}{
		{&privacy.ShowStatus, params.Privacy.ShowStatus},
		{&privacy.ShowProfilePhoto, params.Privacy.ShowProfilePhoto},
		{&privacy.ShowProfileAudio, params.Privacy.ShowProfileAudio},
		{&privacy.ShowBirthdate, params.Privacy.ShowBirthdate},
		{&privacy.ShowBio, params.Privacy.ShowBio},
		{&privacy.ShowPhoneNumber, params.Privacy.ShowPhoneNumber},
		{&privacy.AllowFindingByPhoneNumber, params.Privacy.AllowFindingByPhoneNumber},
		{&privacy.ShowLinkInForwardedMessages, params.Privacy.ShowLinkInForwardedMessages},
		{&privacy.AllowChatInvites, params.Privacy.AllowChatInvites},
		{&privacy.AllowPrivateVoiceAndVideoNoteMessages, params.Privacy.AllowPrivateVoiceAndVideoNoteMessages},
		{&privacy.AllowCalls, params.Privacy.AllowCalls},
		{&privacy.AllowPeerToPeerCalls, params.Privacy.AllowPeerToPeerCalls},
		{&privacy.AutosaveGifts, params.Privacy.AutosaveGifts},
	}
	for _, update := range updates {
		if update.src.Base == VisibilityUnspecified {
			continue
		}
		rule, ok := normalizePrivacyRule(update.src)
		if !ok {
			return Privacy{}, ErrInvalidInput
		}
		*update.dst = rule
	}
	if params.ShowReadDate != nil {
		privacy.ShowReadDate = *params.ShowReadDate
	}
	if privacy.CreatedAt.IsZero() {
		privacy.CreatedAt = now
	}
	privacy.UpdatedAt = now

	if privacy.PhoneVisibility == VisibilityUnspecified ||
		privacy.LastSeenVisibility == VisibilityUnspecified ||
		privacy.MessagePrivacy == VisibilityUnspecified ||
		privacy.BirthdayVisibility == VisibilityUnspecified {
		return Privacy{}, ErrInvalidInput
	}

	saved, err := s.store.SavePrivacy(ctx, privacy)
	if err != nil {
		return Privacy{}, fmt.Errorf("save privacy for %s: %w", params.AccountID, err)
	}

	return saved, nil
}
