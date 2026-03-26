package botapi

import (
	"encoding/json"
	"strconv"
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
	ChatID                textID `json:"chat_id"`
	MessageThreadID       textID `json:"message_thread_id"`
	Text                  string `json:"text"`
	ReplyToMessageID      textID `json:"reply_to_message_id"`
	DisableNotification   bool   `json:"disable_notification"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

type editMessageTextRequest struct {
	ChatID                textID `json:"chat_id"`
	MessageID             textID `json:"message_id"`
	Text                  string `json:"text"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
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
