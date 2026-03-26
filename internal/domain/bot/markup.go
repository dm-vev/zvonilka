package bot

import (
	"encoding/json"
	"strings"
)

const metadataReplyMarkupKey = "bot.reply_markup"

func normalizeReplyMarkup(markup *InlineKeyboardMarkup) (*InlineKeyboardMarkup, error) {
	if markup == nil {
		return nil, nil
	}

	value, err := markup.normalize()
	if err != nil {
		return nil, err
	}

	return &value, nil
}

func markupMetadata(metadata map[string]string, markup *InlineKeyboardMarkup) (map[string]string, error) {
	markup, err := normalizeReplyMarkup(markup)
	if err != nil {
		return nil, err
	}
	if metadata == nil && markup == nil {
		return nil, nil
	}

	result := make(map[string]string, len(metadata)+1)
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}
	if markup == nil {
		return result, nil
	}

	raw, err := json.Marshal(markup)
	if err != nil {
		return nil, ErrInvalidInput
	}
	result[metadataReplyMarkupKey] = string(raw)

	return result, nil
}

func replyMarkup(message Message) *InlineKeyboardMarkup {
	return message.ReplyMarkup
}

func messageReplyMarkup(metadata map[string]string) *InlineKeyboardMarkup {
	raw := strings.TrimSpace(metadata[metadataReplyMarkupKey])
	if raw == "" {
		return nil
	}

	var markup InlineKeyboardMarkup
	if err := json.Unmarshal([]byte(raw), &markup); err != nil {
		return nil
	}
	value, err := markup.normalize()
	if err != nil {
		return nil
	}

	return &value
}

func callbackAllowed(markup *InlineKeyboardMarkup, data string) bool {
	data = strings.TrimSpace(data)
	if markup == nil || data == "" {
		return false
	}
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if button.CallbackData == data {
				return true
			}
		}
	}

	return false
}
