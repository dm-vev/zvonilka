package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) answerCallbackQuery(writer http.ResponseWriter, request *http.Request, token string) {
	var payload answerCallbackQueryRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	err := a.bot.AnswerCallbackQuery(request.Context(), domainbot.AnswerCallbackQueryParams{
		BotToken:         token,
		CallbackQueryID:  string(payload.CallbackQueryID),
		Text:             payload.Text,
		ShowAlert:        payload.ShowAlert,
		URL:              payload.URL,
		CacheTimeSeconds: payload.CacheTime,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}
