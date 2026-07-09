package translation

import "context"

// Store persists translation cache entries.
type Store interface {
	SaveTranslation(ctx context.Context, translation Translation) (Translation, error)
	TranslationByMessageAndLanguage(ctx context.Context, messageID string, targetLanguage string) (Translation, error)
}

// Provider translates text into the requested target language.
type Provider interface {
	Translate(ctx context.Context, request ProviderRequest) (ProviderResult, error)
}
