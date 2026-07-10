package botapi

import (
	"net/http"
	"strings"

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
			inputEntities := result.InputMessageContent.Entities
			if len(inputEntities) == 0 {
				inputEntities = result.Entities
			}
			messageText, entities, err := a.formatText(
				request.Context(),
				result.InputMessageContent.MessageText,
				result.InputMessageContent.ParseMode,
				inputEntities,
			)
			if err != nil {
				writeError(writer, http.StatusBadRequest, "Bad Request")
				return
			}
			input = &domainbot.InputTextMessageContent{
				MessageText: messageText,
				Entities:    entities,
			}
		}
		caption, captionEntities, err := a.formatText(
			request.Context(),
			result.Caption,
			result.ParseMode,
			result.CaptionEntities,
		)
		if err != nil {
			writeError(writer, http.StatusBadRequest, "Bad Request")
			return
		}
		results = append(results, domainbot.InlineQueryResult{
			Type:                result.Type,
			ID:                  result.ID,
			Title:               result.Title,
			Description:         result.Description,
			Caption:             caption,
			CaptionEntities:     captionEntities,
			InputMessageContent: input,
			ReplyMarkup:         result.ReplyMarkup,
			PhotoURL:            result.PhotoURL,
			AudioURL:            result.AudioURL,
			DocumentURL:         result.DocumentURL,
			GIFURL:              result.GIFURL,
			Mpeg4URL:            result.Mpeg4URL,
			VideoURL:            result.VideoURL,
			MimeType:            result.MimeType,
			ThumbURL:            result.thumbnailURL(),
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

func (r inlineQueryResultRequest) thumbnailURL() string {
	if value := strings.TrimSpace(r.ThumbnailURL); value != "" {
		return value
	}

	return strings.TrimSpace(r.ThumbURL)
}
