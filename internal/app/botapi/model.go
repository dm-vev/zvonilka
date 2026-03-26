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
	ReplyMarkup           *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification   bool                            `json:"disable_notification"`
	DisableWebPagePreview bool                            `json:"disable_web_page_preview"`
}

type sendPhotoRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Photo               string                          `json:"photo"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendDocumentRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Document            string                          `json:"document"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendVideoRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Video               string                          `json:"video"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendVoiceRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Voice               string                          `json:"voice"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendStickerRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Sticker             string                          `json:"sticker"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendAnimationRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Animation           string                          `json:"animation"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendAudioRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	Audio               string                          `json:"audio"`
	Caption             string                          `json:"caption"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type sendVideoNoteRequest struct {
	ChatID              textID                          `json:"chat_id"`
	MessageThreadID     textID                          `json:"message_thread_id"`
	VideoNote           string                          `json:"video_note"`
	ReplyToMessageID    textID                          `json:"reply_to_message_id"`
	ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableNotification bool                            `json:"disable_notification"`
}

type editMessageTextRequest struct {
	ChatID                textID                          `json:"chat_id"`
	MessageID             textID                          `json:"message_id"`
	Text                  string                          `json:"text"`
	ReplyMarkup           *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	DisableWebPagePreview bool                            `json:"disable_web_page_preview"`
}

type answerCallbackQueryRequest struct {
	CallbackQueryID textID `json:"callback_query_id"`
	Text            string `json:"text"`
	ShowAlert       bool   `json:"show_alert"`
	URL             string `json:"url"`
	CacheTime       int    `json:"cache_time"`
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
