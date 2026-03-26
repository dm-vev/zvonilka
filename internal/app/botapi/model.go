package botapi

import (
	"encoding/json"
	"strconv"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

type textID string

func (t *textID) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*t = ""
		return nil
	}

	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		*t = textID(stringValue)
		return nil
	}

	var intValue int64
	if err := json.Unmarshal(data, &intValue); err == nil {
		*t = textID(strconv.FormatInt(intValue, 10))
		return nil
	}

	return json.Unmarshal(data, (*string)(t))
}

type getUpdatesRequest struct {
	Offset         int64    `json:"offset"`
	Limit          int      `json:"limit"`
	Timeout        int      `json:"timeout"`
	AllowedUpdates []string `json:"allowed_updates"`
}

type setWebhookRequest struct {
	URL                string   `json:"url"`
	MaxConnections     int      `json:"max_connections"`
	AllowedUpdates     []string `json:"allowed_updates"`
	DropPendingUpdates bool     `json:"drop_pending_updates"`
	SecretToken        string   `json:"secret_token"`
}

type deleteWebhookRequest struct {
	DropPendingUpdates bool `json:"drop_pending_updates"`
}

type sendMessageRequest struct {
	ChatID                textID                          `json:"chat_id"`
	MessageThreadID       textID                          `json:"message_thread_id"`
	Text                  string                          `json:"text"`
	ReplyToMessageID      textID                          `json:"reply_to_message_id"`
	ReplyParameters       *replyData                      `json:"reply_parameters"`
	ReplyMarkup           *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification   bool                            `json:"disable_notification"`
	DisableWebPagePreview bool                            `json:"disable_web_page_preview"`
	LinkPreviewOptions    *previewData                    `json:"link_preview_options"`
}

type sendPhotoRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Photo               string                          `json:"photo"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyParameters     *replyData                      `json:"reply_parameters"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendDocumentRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Document            string                          `json:"document"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyParameters     *replyData                      `json:"reply_parameters"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendVideoRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Video               string                          `json:"video"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyParameters     *replyData                      `json:"reply_parameters"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendVoiceRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Voice               string                          `json:"voice"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyParameters     *replyData                      `json:"reply_parameters"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendStickerRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Sticker             string                          `json:"sticker"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyParameters     *replyData                      `json:"reply_parameters"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendAnimationRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Animation           string                          `json:"animation"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyParameters     *replyData                      `json:"reply_parameters"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendAudioRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Audio               string                          `json:"audio"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyParameters     *replyData                      `json:"reply_parameters"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendVideoNoteRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	VideoNote           string                          `json:"video_note"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyParameters     *replyData                      `json:"reply_parameters"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendLocationRequest struct {
	ChatID               textID                          `json:"chat_id"`
	MessageThreadID      textID                          `json:"message_thread_id"`
	Latitude             float64                         `json:"latitude"`
	Longitude            float64                         `json:"longitude"`
	HorizontalAccuracy   float64                         `json:"horizontal_accuracy"`
	LivePeriod           int                             `json:"live_period"`
	Heading              int                             `json:"heading"`
	ProximityAlertRadius int                             `json:"proximity_alert_radius"`
	ReplyToMessageID     textID                          `json:"reply_to_message_id"`
	ReplyParameters      *replyData                      `json:"reply_parameters"`
	ReplyMarkup          *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification  bool                            `json:"disable_notification"`
}

type sendContactRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	PhoneNumber         string                          `json:"phone_number"`
	FirstName           string                          `json:"first_name"`
	LastName            string                          `json:"last_name"`
	VCard               string                          `json:"vcard"`
	UserID              textID                          `json:"user_id"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyParameters     *replyData                      `json:"reply_parameters"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendPollRequest struct {
	ChatID                textID                          `json:"chat_id"`
	MessageThreadID       textID                          `json:"message_thread_id"`
	Question              string                          `json:"question"`
	Options               pollData                        `json:"options"`
	IsAnonymous           bool                            `json:"is_anonymous"`
	Type                  string                          `json:"type"`
	AllowsMultipleAnswers bool                            `json:"allows_multiple_answers"`
	ReplyToMessageID      textID                          `json:"reply_to_message_id"`
	ReplyParameters       *replyData                      `json:"reply_parameters"`
	ReplyMarkup           *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification   bool                            `json:"disable_notification"`
}

type sendGameRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	GameShortName       string                          `json:"game_short_name"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyParameters     *replyData                      `json:"reply_parameters"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type forwardMessagesRequest struct {
	ChatID              textID   `json:"chat_id"`
	MessageThreadID     textID   `json:"message_thread_id"`
	FromChatID          textID   `json:"from_chat_id"`
	MessageIDs          []textID `json:"message_ids"`
	DisableNotification bool     `json:"disable_notification"`
}

type copyMessagesRequest struct {
	ChatID              textID   `json:"chat_id"`
	MessageThreadID     textID   `json:"message_thread_id"`
	FromChatID          textID   `json:"from_chat_id"`
	MessageIDs          []textID `json:"message_ids"`
	DisableNotification bool     `json:"disable_notification"`
	RemoveCaption       bool     `json:"remove_caption"`
}

type editMessageTextRequest struct {
	ChatID                textID                          `json:"chat_id"`
	MessageID             textID                          `json:"message_id"`
	Text                  string                          `json:"text"`
	ReplyMarkup           *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableWebPagePreview bool                            `json:"disable_web_page_preview"`
	LinkPreviewOptions    *previewData                    `json:"link_preview_options"`
}

type editMediaData struct {
	Type    string  `json:"type"`
	Media   string  `json:"media"`
	Caption *string `json:"caption"`
}

type editMessageMediaRequest struct {
	ChatID      textID                          `json:"chat_id"`
	MessageID   textID                          `json:"message_id"`
	Media       editMediaData                   `json:"media"`
	ReplyMarkup *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
}

type editLiveLocationRequest struct {
	ChatID               textID                          `json:"chat_id"`
	MessageID            textID                          `json:"message_id"`
	Latitude             float64                         `json:"latitude"`
	Longitude            float64                         `json:"longitude"`
	LivePeriod           int                             `json:"live_period"`
	HorizontalAccuracy   float64                         `json:"horizontal_accuracy"`
	Heading              int                             `json:"heading"`
	ProximityAlertRadius int                             `json:"proximity_alert_radius"`
	ReplyMarkup          *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
}

type stopPollRequest struct {
	ChatID      textID                          `json:"chat_id"`
	MessageID   textID                          `json:"message_id"`
	ReplyMarkup *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
}

type answerCallbackQueryRequest struct {
	CallbackQueryID textID `json:"callback_query_id"`
	Text            string `json:"text"`
	ShowAlert       bool   `json:"show_alert"`
	URL             string `json:"url"`
	CacheTime       int    `json:"cache_time"`
}

type inlineQueryResultRequest struct {
	Type                string                           `json:"type"`
	ID                  string                           `json:"id"`
	Title               string                           `json:"title"`
	Description         string                           `json:"description"`
	Caption             string                           `json:"caption"`
	InputMessageContent *inlineTextMessageContentRequest `json:"input_message_content"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup  `json:"reply_markup"`
	PhotoURL            string                           `json:"photo_url"`
	AudioURL            string                           `json:"audio_url"`
	DocumentURL         string                           `json:"document_url"`
	GIFURL              string                           `json:"gif_url"`
	Mpeg4URL            string                           `json:"mpeg4_url"`
	VideoURL            string                           `json:"video_url"`
	MimeType            string                           `json:"mime_type"`
	ThumbURL            string                           `json:"thumb_url"`
	ThumbnailURL        string                           `json:"thumbnail_url"`
}

type inlineTextMessageContentRequest struct {
	MessageText string `json:"message_text"`
}

type answerInlineQueryRequest struct {
	InlineQueryID textID                     `json:"inline_query_id"`
	Results       []inlineQueryResultRequest `json:"results"`
	CacheTime     int                        `json:"cache_time"`
	IsPersonal    bool                       `json:"is_personal"`
	NextOffset    string                     `json:"next_offset"`
	SwitchPMText  string                     `json:"switch_pm_text"`
	SwitchPMParam string                     `json:"switch_pm_parameter"`
}

type deleteMessageRequest struct {
	ChatID    textID `json:"chat_id"`
	MessageID textID `json:"message_id"`
}

type getChatRequest struct {
	ChatID textID `json:"chat_id"`
}

type getChatMemberRequest struct {
	ChatID textID `json:"chat_id"`
	UserID textID `json:"user_id"`
}
