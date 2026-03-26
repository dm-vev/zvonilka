package bot

import "time"

// GetUpdatesParams describes one getUpdates request.
type GetUpdatesParams struct {
	BotToken       string
	Offset         int64
	Limit          int
	Timeout        time.Duration
	AllowedUpdates []UpdateType
}

// SetWebhookParams describes one setWebhook request.
type SetWebhookParams struct {
	BotToken           string
	URL                string
	MaxConnections     int
	AllowedUpdates     []UpdateType
	DropPendingUpdates bool
	SecretToken        string
}

// DeleteWebhookParams describes one deleteWebhook request.
type DeleteWebhookParams struct {
	BotToken           string
	DropPendingUpdates bool
}

// WebhookInfoParams describes one getWebhookInfo request.
type WebhookInfoParams struct {
	BotToken string
}

// SendMessageParams describes one sendMessage request.
type SendMessageParams struct {
	BotToken              string
	ChatID                string
	MessageThreadID       string
	Text                  string
	ReplyToMessageID      string
	ReplyMarkup           *InlineKeyboardMarkup
	DisableNotification   bool
	DisableWebPagePreview bool
}

// SendPhotoParams describes one sendPhoto request.
type SendPhotoParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	MediaID             string
	Caption             string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendDocumentParams describes one sendDocument request.
type SendDocumentParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	MediaID             string
	Caption             string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendVideoParams describes one sendVideo request.
type SendVideoParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	MediaID             string
	Caption             string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendVoiceParams describes one sendVoice request.
type SendVoiceParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	MediaID             string
	Caption             string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendStickerParams describes one sendSticker request.
type SendStickerParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	MediaID             string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendAnimationParams describes one sendAnimation request.
type SendAnimationParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	MediaID             string
	Caption             string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendAudioParams describes one sendAudio request.
type SendAudioParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	MediaID             string
	Caption             string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendVideoNoteParams describes one sendVideoNote request.
type SendVideoNoteParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	MediaID             string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendLocationParams describes one sendLocation request.
type SendLocationParams struct {
	BotToken             string
	ChatID               string
	MessageThreadID      string
	Latitude             float64
	Longitude            float64
	HorizontalAccuracy   float64
	LivePeriod           int
	Heading              int
	ProximityAlertRadius int
	ReplyToMessageID     string
	ReplyMarkup          *InlineKeyboardMarkup
	DisableNotification  bool
}

// SendContactParams describes one sendContact request.
type SendContactParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	PhoneNumber         string
	FirstName           string
	LastName            string
	VCard               string
	UserID              string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendPollParams describes one sendPoll request.
type SendPollParams struct {
	BotToken              string
	ChatID                string
	MessageThreadID       string
	Question              string
	Options               []string
	IsAnonymous           bool
	Type                  string
	AllowsMultipleAnswers bool
	ReplyToMessageID      string
	ReplyMarkup           *InlineKeyboardMarkup
	DisableNotification   bool
}

// EditMessageTextParams describes one editMessageText request.
type EditMessageTextParams struct {
	BotToken              string
	ChatID                string
	MessageID             string
	Text                  string
	ReplyMarkup           *InlineKeyboardMarkup
	DisableWebPagePreview bool
}

// DeleteMessageParams describes one deleteMessage request.
type DeleteMessageParams struct {
	BotToken  string
	ChatID    string
	MessageID string
}

// GetChatParams describes one getChat request.
type GetChatParams struct {
	BotToken string
	ChatID   string
}

// GetChatMemberParams describes one getChatMember request.
type GetChatMemberParams struct {
	BotToken string
	ChatID   string
	UserID   string
}

// GetMessageParams describes one internal message lookup request.
type GetMessageParams struct {
	BotToken  string
	ChatID    string
	MessageID string
}

// TriggerCallbackParams describes one internally generated callback query.
type TriggerCallbackParams struct {
	ConversationID string
	MessageID      string
	FromAccountID  string
	Data           string
}

// AnswerCallbackQueryParams describes one answerCallbackQuery request.
type AnswerCallbackQueryParams struct {
	BotToken         string
	CallbackQueryID  string
	Text             string
	ShowAlert        bool
	URL              string
	CacheTimeSeconds int
}

// RetryParams describes one webhook retry update.
type RetryParams struct {
	BotAccountID  string
	UpdateID      int64
	AttemptedAt   time.Time
	NextAttemptAt time.Time
	LastError     string
}
