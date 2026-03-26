package botapi

import (
	"net/http"
	"time"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) getMe(writer http.ResponseWriter, request *http.Request, token string) {
	result, err := a.bot.GetMe(request.Context(), token)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	user, err := a.telegramUser(request.Context(), result)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, user)
}

func (a *api) getUpdates(writer http.ResponseWriter, request *http.Request, token string) {
	var payload getUpdatesRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	allowed := make([]domainbot.UpdateType, 0, len(payload.AllowedUpdates))
	for _, value := range payload.AllowedUpdates {
		allowed = append(allowed, domainbot.UpdateType(value))
	}

	result, err := a.bot.GetUpdates(request.Context(), domainbot.GetUpdatesParams{
		BotToken:       token,
		Offset:         payload.Offset,
		Limit:          payload.Limit,
		Timeout:        time.Duration(payload.Timeout) * time.Second,
		AllowedUpdates: allowed,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	updates, err := a.telegramUpdates(request.Context(), result)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, updates)
}

func (a *api) getWebhookInfo(writer http.ResponseWriter, request *http.Request, token string) {
	result, err := a.bot.WebhookInfo(request.Context(), domainbot.WebhookInfoParams{BotToken: token})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) getChat(writer http.ResponseWriter, request *http.Request, token string) {
	var payload getChatRequest
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

	result, err := a.bot.GetChat(request.Context(), domainbot.GetChatParams{
		BotToken: token,
		ChatID:   chatID,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	chat, err := a.telegramChat(request.Context(), result)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, chat)
}

func (a *api) getChatMember(writer http.ResponseWriter, request *http.Request, token string) {
	var payload getChatMemberRequest
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
	userID, err := a.internalUserID(request.Context(), payload.UserID)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	result, err := a.bot.GetChatMember(request.Context(), domainbot.GetChatMemberParams{
		BotToken: token,
		ChatID:   chatID,
		UserID:   userID,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	member, err := a.telegramMember(request.Context(), result)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, member)
}
