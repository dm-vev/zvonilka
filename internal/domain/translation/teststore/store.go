package teststore

import (
	"context"
	"strings"
	"sync"

	"github.com/dm-vev/zvonilka/internal/domain/translation"
)

// NewMemoryStore builds an in-memory translation store for tests.
func NewMemoryStore() translation.Store {
	return &memoryStore{
		translationsByKey: make(map[string]translation.Translation),
	}
}

type memoryStore struct {
	mu                sync.RWMutex
	translationsByKey map[string]translation.Translation
}

func translationKey(messageID string, targetLanguage string) string {
	return strings.TrimSpace(messageID) + "|" + strings.ToLower(strings.TrimSpace(targetLanguage))
}

func (s *memoryStore) SaveTranslation(
	ctx context.Context,
	row translation.Translation,
) (translation.Translation, error) {
	if ctx == nil {
		return translation.Translation{}, translation.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return translation.Translation{}, err
	}

	row.MessageID = strings.TrimSpace(row.MessageID)
	row.TargetLanguage = strings.ToLower(strings.TrimSpace(row.TargetLanguage))
	row.SourceLanguage = strings.TrimSpace(row.SourceLanguage)
	row.Provider = strings.TrimSpace(row.Provider)
	row.TranslatedText = strings.TrimSpace(row.TranslatedText)
	if row.MessageID == "" || row.TargetLanguage == "" || row.TranslatedText == "" {
		return translation.Translation{}, translation.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.translationsByKey[translationKey(row.MessageID, row.TargetLanguage)] = row
	return row, nil
}

func (s *memoryStore) TranslationByMessageAndLanguage(
	ctx context.Context,
	messageID string,
	targetLanguage string,
) (translation.Translation, error) {
	if ctx == nil {
		return translation.Translation{}, translation.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return translation.Translation{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	row, ok := s.translationsByKey[translationKey(messageID, targetLanguage)]
	if !ok {
		return translation.Translation{}, translation.ErrNotFound
	}

	return row, nil
}
