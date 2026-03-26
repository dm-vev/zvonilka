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

	writeResult(writer, result)
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

	writeResult(writer, result)
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

	result, err := a.bot.GetChat(request.Context(), domainbot.GetChatParams{
		BotToken: token,
		ChatID:   string(payload.ChatID),
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) getChatMember(writer http.ResponseWriter, request *http.Request, token string) {
	var payload getChatMemberRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	result, err := a.bot.GetChatMember(request.Context(), domainbot.GetChatMemberParams{
		BotToken: token,
		ChatID:   string(payload.ChatID),
		UserID:   string(payload.UserID),
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}
