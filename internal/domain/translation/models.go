package translation

import "time"

// Translation captures one cached message translation.
type Translation struct {
	MessageID      string
	TargetLanguage string
	SourceLanguage string
	Provider       string
	TranslatedText string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ProviderRequest describes one outbound translation request.
type ProviderRequest struct {
	Text           string
	SourceLanguage string
	TargetLanguage string
}

// ProviderResult captures the normalized provider response.
type ProviderResult struct {
	TranslatedText string
	SourceLanguage string
	Provider       string
}
