package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) sendGame(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendGameRequest
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
	threadID, err := a.internalTopicID(request.Context(), payload.MessageThreadID)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}
	replyToMessageID, err := a.internalMessageID(request.Context(), replyMessageID(payload.ReplyToMessageID, payload.ReplyParameters))
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	message, err := a.bot.SendGame(request.Context(), domainbot.SendGameParams{
		BotToken:            token,
		ChatID:              chatID,
		MessageThreadID:     threadID,
		GameShortName:       payload.GameShortName,
		ReplyToMessageID:    replyToMessageID,
		ReplyMarkup:         payload.ReplyMarkup,
		DisableNotification: payload.DisableNotification,
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
