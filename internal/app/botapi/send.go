package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

type normalizedMediaRequest struct {
	ChatID              textID
	MessageThreadID     textID
	MediaID             string
	Caption             string
	ReplyToMessageID    textID
	ReplyMarkup         *domainbot.InlineKeyboardMarkup
	DisableNotification bool
}

func (a *api) sendMessage(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendMessageRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	result, err := a.bot.SendMessage(request.Context(), domainbot.SendMessageParams{
		BotToken:              token,
		ChatID:                string(payload.ChatID),
		MessageThreadID:       string(payload.MessageThreadID),
		Text:                  payload.Text,
		ReplyToMessageID:      string(payload.ReplyToMessageID),
		ReplyMarkup:           payload.ReplyMarkup,
		DisableNotification:   payload.DisableNotification,
		DisableWebPagePreview: payload.DisableWebPagePreview,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) sendPhoto(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendPhotoRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Photo,
			Caption:             payload.Caption,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendPhoto(request.Context(), domainbot.SendPhotoParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			Caption:             payload.Caption,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	})
}

func (a *api) sendDocument(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendDocumentRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Document,
			Caption:             payload.Caption,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendDocument(request.Context(), domainbot.SendDocumentParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			Caption:             payload.Caption,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	})
}

func (a *api) sendVideo(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendVideoRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Video,
			Caption:             payload.Caption,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendVideo(request.Context(), domainbot.SendVideoParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			Caption:             payload.Caption,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	})
}

func (a *api) sendVoice(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendVoiceRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Voice,
			Caption:             payload.Caption,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendVoice(request.Context(), domainbot.SendVoiceParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			Caption:             payload.Caption,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	})
}

func (a *api) sendSticker(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendStickerRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Sticker,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendSticker(request.Context(), domainbot.SendStickerParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	})
}

func (a *api) sendMedia(
	writer http.ResponseWriter,
	request *http.Request,
	target any,
	normalize func() normalizedMediaRequest,
	call func(normalizedMediaRequest) (domainbot.Message, error),
) {
	if err := decodeRequest(request, target); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	result, err := call(normalize())
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) editMessageText(writer http.ResponseWriter, request *http.Request, token string) {
	var payload editMessageTextRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	result, err := a.bot.EditMessageText(request.Context(), domainbot.EditMessageTextParams{
		BotToken:              token,
		ChatID:                string(payload.ChatID),
		MessageID:             string(payload.MessageID),
		Text:                  payload.Text,
		ReplyMarkup:           payload.ReplyMarkup,
		DisableWebPagePreview: payload.DisableWebPagePreview,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) deleteMessage(writer http.ResponseWriter, request *http.Request, token string) {
	var payload deleteMessageRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	if err := a.bot.DeleteMessage(request.Context(), domainbot.DeleteMessageParams{
		BotToken:  token,
		ChatID:    string(payload.ChatID),
		MessageID: string(payload.MessageID),
	}); err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}
