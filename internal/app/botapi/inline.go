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

	results := make([]domainbot.InlineQueryResult, 0, len(payload.Results))
	for _, result := range payload.Results {
		var input *domainbot.InputTextMessageContent
		if result.InputMessageContent != nil {
			input = &domainbot.InputTextMessageContent{
				MessageText: result.InputMessageContent.MessageText,
			}
		}
		results = append(results, domainbot.InlineQueryResult{
			Type:                result.Type,
			ID:                  result.ID,
			Title:               result.Title,
			Description:         result.Description,
			Caption:             result.Caption,
			InputMessageContent: input,
			ReplyMarkup:         result.ReplyMarkup,
			PhotoURL:            result.PhotoURL,
			DocumentURL:         result.DocumentURL,
			VideoURL:            result.VideoURL,
			MimeType:            result.MimeType,
			ThumbURL:            result.ThumbURL,
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
