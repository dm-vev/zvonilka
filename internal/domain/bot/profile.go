package bot

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// ProfileKind identifies one bot profile field.
type ProfileKind string

const (
	ProfileKindName             ProfileKind = "name"
	ProfileKindDescription      ProfileKind = "description"
	ProfileKindShortDescription ProfileKind = "short_description"
)

// ProfileValue stores one localized bot profile field.
type ProfileValue struct {
	BotAccountID string
	Kind         ProfileKind
	LanguageCode string
	Value        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SetNameParams describes one setMyName request.
type SetNameParams struct {
	BotToken     string
	LanguageCode string
	Name         string
}

// GetNameParams describes one getMyName request.
type GetNameParams struct {
	BotToken     string
	LanguageCode string
}

// SetDescriptionParams describes one setMyDescription request.
type SetDescriptionParams struct {
	BotToken     string
	LanguageCode string
	Description  string
}

// GetDescriptionParams describes one getMyDescription request.
type GetDescriptionParams struct {
	BotToken     string
	LanguageCode string
}

// SetShortDescriptionParams describes one setMyShortDescription request.
type SetShortDescriptionParams struct {
	BotToken         string
	LanguageCode     string
	ShortDescription string
}

// GetShortDescriptionParams describes one getMyShortDescription request.
type GetShortDescriptionParams struct {
	BotToken     string
	LanguageCode string
}

// SetMyName stores one localized bot name override.
func (s *Service) SetMyName(ctx context.Context, params SetNameParams) error {
	return s.setProfileValue(ctx, params.BotToken, ProfileKindName, params.LanguageCode, params.Name)
}

// GetMyName resolves one localized bot name.
func (s *Service) GetMyName(ctx context.Context, params GetNameParams) (string, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return "", err
	}

	value, err := s.getProfileValue(ctx, account.ID, ProfileKindName, params.LanguageCode)
	if err != nil {
		return "", err
	}
	if value != "" {
		return value, nil
	}
	if strings.TrimSpace(account.DisplayName) != "" {
		return strings.TrimSpace(account.DisplayName), nil
	}

	return strings.TrimSpace(account.Username), nil
}

// SetMyDescription stores one localized bot description override.
func (s *Service) SetMyDescription(ctx context.Context, params SetDescriptionParams) error {
	return s.setProfileValue(ctx, params.BotToken, ProfileKindDescription, params.LanguageCode, params.Description)
}

// GetMyDescription resolves one localized bot description.
func (s *Service) GetMyDescription(ctx context.Context, params GetDescriptionParams) (string, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return "", err
	}

	value, err := s.getProfileValue(ctx, account.ID, ProfileKindDescription, params.LanguageCode)
	if err != nil {
		return "", err
	}
	if value != "" {
		return value, nil
	}

	return strings.TrimSpace(account.Bio), nil
}

// SetMyShortDescription stores one localized bot short description override.
func (s *Service) SetMyShortDescription(ctx context.Context, params SetShortDescriptionParams) error {
	return s.setProfileValue(ctx, params.BotToken, ProfileKindShortDescription, params.LanguageCode, params.ShortDescription)
}

// GetMyShortDescription resolves one localized bot short description.
func (s *Service) GetMyShortDescription(ctx context.Context, params GetShortDescriptionParams) (string, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return "", err
	}

	value, err := s.getProfileValue(ctx, account.ID, ProfileKindShortDescription, params.LanguageCode)
	if err != nil {
		return "", err
	}
	if value != "" {
		return value, nil
	}

	return truncateRunes(strings.TrimSpace(account.Bio), 120), nil
}

func (s *Service) setProfileValue(
	ctx context.Context,
	botToken string,
	kind ProfileKind,
	languageCode string,
	value string,
) error {
	account, err := s.botAccount(ctx, botToken)
	if err != nil {
		return err
	}

	kind, languageCode, value, err = normalizeProfileInput(kind, languageCode, value)
	if err != nil {
		return err
	}
	if value == "" {
		err = s.store.DeleteProfile(ctx, account.ID, kind, languageCode)
		if err == ErrNotFound {
			return nil
		}

		if err != nil {
			return fmt.Errorf("delete bot profile value: %w", err)
		}

		return nil
	}

	now := s.currentTime()
	_, err = s.store.SaveProfile(ctx, ProfileValue{
		BotAccountID: account.ID,
		Kind:         kind,
		LanguageCode: languageCode,
		Value:        value,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return fmt.Errorf("save bot profile value: %w", err)
	}

	return nil
}

func (s *Service) getProfileValue(
	ctx context.Context,
	botAccountID string,
	kind ProfileKind,
	languageCode string,
) (string, error) {
	kind, languageCode, _, err := normalizeProfileInput(kind, languageCode, "")
	if err != nil {
		return "", err
	}

	value, err := s.store.ProfileByLanguage(ctx, botAccountID, kind, languageCode)
	if err == nil {
		return value.Value, nil
	}
	if err != ErrNotFound {
		return "", fmt.Errorf("load bot profile value: %w", err)
	}
	if languageCode == "" {
		return "", nil
	}

	value, err = s.store.ProfileByLanguage(ctx, botAccountID, kind, "")
	if err == nil {
		return value.Value, nil
	}
	if err == ErrNotFound {
		return "", nil
	}

	return "", fmt.Errorf("load bot profile fallback: %w", err)
}

func normalizeProfileInput(kind ProfileKind, languageCode string, value string) (ProfileKind, string, string, error) {
	kind = ProfileKind(strings.ToLower(strings.TrimSpace(string(kind))))
	languageCode = strings.ToLower(strings.TrimSpace(languageCode))
	value = strings.TrimSpace(value)

	switch kind {
	case ProfileKindName:
		if value != "" && utf8.RuneCountInString(value) > 64 {
			return "", "", "", ErrInvalidInput
		}
	case ProfileKindDescription:
		if value != "" && utf8.RuneCountInString(value) > 512 {
			return "", "", "", ErrInvalidInput
		}
	case ProfileKindShortDescription:
		if value != "" && utf8.RuneCountInString(value) > 120 {
			return "", "", "", ErrInvalidInput
		}
	default:
		return "", "", "", ErrInvalidInput
	}

	return kind, languageCode, value, nil
}

// NormalizeProfileInput validates and normalizes one bot profile tuple.
func NormalizeProfileInput(kind ProfileKind, languageCode string, value string) (ProfileKind, string, string, error) {
	return normalizeProfileInput(kind, languageCode, value)
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || value == "" {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}

	runes := []rune(value)
	return string(runes[:limit])
}
