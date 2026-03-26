package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) editMessageCaption(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		ChatID      textID                          `json:"chat_id"`
		MessageID   textID                          `json:"message_id"`
		Caption     string                          `json:"caption"`
		ReplyMarkup *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	chatID, err := a.internalChatID(request.Context(), payload.ChatID)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}
	messageID, err := a.internalMessageID(request.Context(), payload.MessageID)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	message, err := a.bot.EditMessageCaption(request.Context(), domainbot.EditCaptionParams{
		BotToken:    token,
		ChatID:      chatID,
		MessageID:   messageID,
		Caption:     payload.Caption,
		ReplyMarkup: payload.ReplyMarkup,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	result, err := a.telegramMessage(request.Context(), message)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) editMessageReplyMarkup(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		ChatID      textID                          `json:"chat_id"`
		MessageID   textID                          `json:"message_id"`
		ReplyMarkup *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	chatID, err := a.internalChatID(request.Context(), payload.ChatID)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}
	messageID, err := a.internalMessageID(request.Context(), payload.MessageID)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	message, err := a.bot.EditMessageReplyMarkup(request.Context(), domainbot.EditMarkupParams{
		BotToken:    token,
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: payload.ReplyMarkup,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	result, err := a.telegramMessage(request.Context(), message)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}
