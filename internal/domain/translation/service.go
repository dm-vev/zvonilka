package translation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"golang.org/x/text/language"
)

// Service owns message translation caching and provider execution.
type Service struct {
	store         Store
	conversations *conversation.Service
	provider      Provider
	now           func() time.Time
}

// NewService constructs a translation service.
func NewService(
	store Store,
	conversations *conversation.Service,
	provider Provider,
	opts ...Option,
) (*Service, error) {
	if store == nil || conversations == nil || provider == nil {
		return nil, ErrInvalidInput
	}

	service := &Service{
		store:         store,
		conversations: conversations,
		provider:      provider,
		now:           func() time.Time { return time.Now().UTC() },
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

// TranslateMessage translates one visible text message and caches the result.
func (s *Service) TranslateMessage(
	ctx context.Context,
	params TranslateMessageParams,
) (Translation, bool, error) {
	if ctx == nil {
		return Translation{}, false, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return Translation{}, false, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	params.RequesterID = strings.TrimSpace(params.RequesterID)
	targetLanguage, err := normalizeLanguage(params.TargetLanguage, true)
	if err != nil {
		return Translation{}, false, err
	}
	sourceLanguage, err := normalizeLanguage(params.SourceLanguage, false)
	if err != nil {
		return Translation{}, false, err
	}
	if params.ConversationID == "" || params.MessageID == "" || params.RequesterID == "" {
		return Translation{}, false, ErrInvalidInput
	}

	var cached Translation
	cacheFound := false
	cached, err = s.store.TranslationByMessageAndLanguage(ctx, params.MessageID, targetLanguage)
	switch {
	case err == nil:
		cacheFound = true
		if !params.ForceRefresh {
			return cached, true, nil
		}
	case err != ErrNotFound:
		return Translation{}, false, fmt.Errorf("load cached translation for message %s: %w", params.MessageID, err)
	}

	conversationRow, _, err := s.conversations.GetConversation(ctx, conversation.GetConversationParams{
		ConversationID: params.ConversationID,
		AccountID:      params.RequesterID,
	})
	if err != nil {
		switch {
		case errors.Is(err, conversation.ErrForbidden):
			return Translation{}, false, ErrForbidden
		case errors.Is(err, conversation.ErrInvalidInput):
			return Translation{}, false, ErrInvalidInput
		default:
			return Translation{}, false, err
		}
	}
	if conversationRow.Settings.RequireEncryptedMessages {
		return Translation{}, false, ErrUnsupported
	}

	message, err := s.conversations.GetMessage(ctx, conversation.GetMessageParams{
		ConversationID: params.ConversationID,
		MessageID:      params.MessageID,
		AccountID:      params.RequesterID,
	})
	if err != nil {
		switch {
		case errors.Is(err, conversation.ErrForbidden):
			return Translation{}, false, ErrForbidden
		case errors.Is(err, conversation.ErrInvalidInput):
			return Translation{}, false, ErrInvalidInput
		case errors.Is(err, conversation.ErrNotFound):
			return Translation{}, false, ErrNotFound
		default:
			return Translation{}, false, err
		}
	}
	if message.Kind != conversation.MessageKindText {
		return Translation{}, false, ErrUnsupported
	}

	text := strings.TrimSpace(string(message.Payload.Ciphertext))
	if text == "" || !utf8.ValidString(text) {
		return Translation{}, false, ErrUnsupported
	}

	result, err := s.provider.Translate(ctx, ProviderRequest{
		Text:           text,
		SourceLanguage: sourceLanguage,
		TargetLanguage: targetLanguage,
	})
	if err != nil {
		return Translation{}, false, fmt.Errorf("translate message %s: %w", params.MessageID, err)
	}

	detectedSource, err := normalizeLanguage(result.SourceLanguage, false)
	if err != nil {
		return Translation{}, false, ErrInvalidInput
	}
	if detectedSource == "" {
		detectedSource = sourceLanguage
	}

	now := s.currentTime()
	translation := Translation{
		MessageID:      message.ID,
		TargetLanguage: targetLanguage,
		SourceLanguage: detectedSource,
		Provider:       strings.TrimSpace(result.Provider),
		TranslatedText: strings.TrimSpace(result.TranslatedText),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if translation.Provider == "" {
		translation.Provider = "translation-http"
	}
	if translation.TranslatedText == "" {
		return Translation{}, false, ErrInvalidInput
	}
	if cacheFound {
		translation.CreatedAt = cached.CreatedAt
	}

	saved, err := s.store.SaveTranslation(ctx, translation)
	if err != nil {
		return Translation{}, false, fmt.Errorf("save translation for message %s: %w", params.MessageID, err)
	}

	return saved, false, nil
}

func normalizeLanguage(value string, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return "", ErrInvalidInput
		}
		return "", nil
	}

	tag, err := language.Parse(value)
	if err != nil {
		return "", ErrInvalidInput
	}

	return tag.String(), nil
}
