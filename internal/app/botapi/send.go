package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

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
