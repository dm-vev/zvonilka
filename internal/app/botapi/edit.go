package botapi

import (
	"net/http"
	"strings"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
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

func (a *api) editMessageMedia(writer http.ResponseWriter, request *http.Request, token string) {
	var payload editMessageMediaRequest
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

	mediaKind, ok := editMediaKind(payload.Media.Type)
	if !ok {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}
	mediaID, err := a.resolveMediaID(
		request.Context(),
		request,
		token,
		"media",
		mediaKind,
		payload.Media.Media,
	)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	message, err := a.bot.EditMessageMedia(request.Context(), domainbot.EditMediaParams{
		BotToken:    token,
		ChatID:      chatID,
		MessageID:   messageID,
		MediaID:     mediaID,
		Shape:       strings.TrimSpace(payload.Media.Type),
		Caption:     payload.Media.Caption,
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

func editMediaKind(value string) (domainmedia.MediaKind, bool) {
	switch strings.TrimSpace(value) {
	case "photo":
		return domainmedia.MediaKindImage, true
	case "document":
		return domainmedia.MediaKindDocument, true
	case "video":
		return domainmedia.MediaKindVideo, true
	case "animation":
		return domainmedia.MediaKindGIF, true
	case "audio":
		return domainmedia.MediaKindFile, true
	default:
		return domainmedia.MediaKindUnspecified, false
	}
}
