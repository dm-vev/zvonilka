package teststore

import (
	"context"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (s *memoryStore) SaveProfile(_ context.Context, value bot.ProfileValue) (bot.ProfileValue, error) {
	now := time.Now().UTC()
	kind, languageCode, normalizedValue, err := normalizeProfile(value.Kind, value.LanguageCode, value.Value)
	if err != nil {
		return bot.ProfileValue{}, err
	}

	value.BotAccountID = strings.TrimSpace(value.BotAccountID)
	value.Kind = kind
	value.LanguageCode = languageCode
	value.Value = normalizedValue
	if value.BotAccountID == "" || value.Value == "" {
		return bot.ProfileValue{}, bot.ErrInvalidInput
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	if value.UpdatedAt.IsZero() {
		value.UpdatedAt = value.CreatedAt
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.profilesByKey[profileKey(value.BotAccountID, value.Kind, value.LanguageCode)] = cloneProfileValue(value)
	s.version++

	return cloneProfileValue(value), nil
}

func (s *memoryStore) ProfileByLanguage(
	_ context.Context,
	botAccountID string,
	kind bot.ProfileKind,
	languageCode string,
) (bot.ProfileValue, error) {
	botAccountID = strings.TrimSpace(botAccountID)
	normalizedKind, normalizedLanguageCode, _, err := normalizeProfile(kind, languageCode, "value")
	if err != nil {
		return bot.ProfileValue{}, err
	}
	if botAccountID == "" {
		return bot.ProfileValue{}, bot.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.profilesByKey[profileKey(botAccountID, normalizedKind, normalizedLanguageCode)]
	if !ok {
		return bot.ProfileValue{}, bot.ErrNotFound
	}

	return cloneProfileValue(value), nil
}

func (s *memoryStore) DeleteProfile(
	_ context.Context,
	botAccountID string,
	kind bot.ProfileKind,
	languageCode string,
) error {
	botAccountID = strings.TrimSpace(botAccountID)
	normalizedKind, normalizedLanguageCode, _, err := normalizeProfile(kind, languageCode, "value")
	if err != nil {
		return err
	}
	if botAccountID == "" {
		return bot.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := profileKey(botAccountID, normalizedKind, normalizedLanguageCode)
	if _, ok := s.profilesByKey[key]; !ok {
		return bot.ErrNotFound
	}
	delete(s.profilesByKey, key)
	s.version++

	return nil
}

func normalizeProfile(kind bot.ProfileKind, languageCode string, value string) (bot.ProfileKind, string, string, error) {
	return bot.NormalizeProfileInput(kind, languageCode, value)
}
