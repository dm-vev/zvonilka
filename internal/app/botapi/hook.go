package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) setWebhook(writer http.ResponseWriter, request *http.Request, token string) {
	var payload setWebhookRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	allowed := make([]domainbot.UpdateType, 0, len(payload.AllowedUpdates))
	for _, value := range payload.AllowedUpdates {
		allowed = append(allowed, domainbot.UpdateType(value))
	}

	result, err := a.bot.SetWebhook(request.Context(), domainbot.SetWebhookParams{
		BotToken:           token,
		URL:                payload.URL,
		MaxConnections:     payload.MaxConnections,
		AllowedUpdates:     allowed,
		DropPendingUpdates: payload.DropPendingUpdates,
		SecretToken:        payload.SecretToken,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) deleteWebhook(writer http.ResponseWriter, request *http.Request, token string) {
	var payload deleteWebhookRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	if err := a.bot.DeleteWebhook(request.Context(), domainbot.DeleteWebhookParams{
		BotToken:           token,
		DropPendingUpdates: payload.DropPendingUpdates,
	}); err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}
