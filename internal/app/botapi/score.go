package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) setGameScore(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		UserID             textID `json:"user_id"`
		Score              int    `json:"score"`
		Force              bool   `json:"force"`
		DisableEditMessage bool   `json:"disable_edit_message"`
		ChatID             textID `json:"chat_id"`
		MessageID          textID `json:"message_id"`
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
	userID, err := a.internalUserID(request.Context(), payload.UserID)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	message, err := a.bot.SetGameScore(request.Context(), domainbot.SetGameScoreParams{
		BotToken:           token,
		ChatID:             chatID,
		MessageID:          messageID,
		UserID:             userID,
		Score:              payload.Score,
		Force:              payload.Force,
		DisableEditMessage: payload.DisableEditMessage,
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

func (a *api) getGameHighScores(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		UserID    textID `json:"user_id"`
		ChatID    textID `json:"chat_id"`
		MessageID textID `json:"message_id"`
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
	userID, err := a.internalUserID(request.Context(), payload.UserID)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	scores, err := a.bot.GetGameHighScores(request.Context(), domainbot.GetGameHighScoresParams{
		BotToken:  token,
		ChatID:    chatID,
		MessageID: messageID,
		UserID:    userID,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	result, err := a.telegramHighScores(request.Context(), scores)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}
