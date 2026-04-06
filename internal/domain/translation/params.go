package translation

// TranslateMessageParams identifies one message translation request.
type TranslateMessageParams struct {
	ConversationID string
	MessageID      string
	RequesterID    string
	TargetLanguage string
	SourceLanguage string
	ForceRefresh   bool
}
