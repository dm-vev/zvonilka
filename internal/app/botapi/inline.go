package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) answerInlineQuery(writer http.ResponseWriter, request *http.Request, token string) {
	var payload answerInlineQueryRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	results := make([]domainbot.InlineQueryResultArticle, 0, len(payload.Results))
	for _, result := range payload.Results {
		results = append(results, domainbot.InlineQueryResultArticle{
			Type:        result.Type,
			ID:          result.ID,
			Title:       result.Title,
			Description: result.Description,
			InputMessageContent: domainbot.InputTextMessageContent{
				MessageText: result.InputMessageContent.MessageText,
			},
			ReplyMarkup: result.ReplyMarkup,
		})
	}

	err := a.bot.AnswerInlineQuery(request.Context(), domainbot.AnswerInlineQueryParams{
		BotToken:      token,
		InlineQueryID: string(payload.InlineQueryID),
		Results:       results,
		CacheTime:     payload.CacheTime,
		IsPersonal:    payload.IsPersonal,
		NextOffset:    payload.NextOffset,
		SwitchPMText:  payload.SwitchPMText,
		SwitchPMParam: payload.SwitchPMParam,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}
