package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) sendVenue(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		ChatID              textID                          `json:"chat_id"`
		MessageThreadID     textID                          `json:"message_thread_id"`
		Latitude            float64                         `json:"latitude"`
		Longitude           float64                         `json:"longitude"`
		Title               string                          `json:"title"`
		Address             string                          `json:"address"`
		FoursquareID        string                          `json:"foursquare_id"`
		FoursquareType      string                          `json:"foursquare_type"`
		GooglePlaceID       string                          `json:"google_place_id"`
		GooglePlaceType     string                          `json:"google_place_type"`
		ReplyToMessageID    textID                          `json:"reply_to_message_id"`
		ReplyParameters     *replyData                      `json:"reply_parameters"`
		ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
		DisableNotification bool                            `json:"disable_notification"`
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

	message, err := a.bot.SendVenue(request.Context(), domainbot.SendVenueParams{
		BotToken:            token,
		ChatID:              chatID,
		MessageThreadID:     threadID,
		Latitude:            payload.Latitude,
		Longitude:           payload.Longitude,
		Title:               payload.Title,
		Address:             payload.Address,
		FoursquareID:        payload.FoursquareID,
		FoursquareType:      payload.FoursquareType,
		GooglePlaceID:       payload.GooglePlaceID,
		GooglePlaceType:     payload.GooglePlaceType,
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

func (a *api) sendDice(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		ChatID              textID                          `json:"chat_id"`
		MessageThreadID     textID                          `json:"message_thread_id"`
		Emoji               string                          `json:"emoji"`
		ReplyToMessageID    textID                          `json:"reply_to_message_id"`
		ReplyParameters     *replyData                      `json:"reply_parameters"`
		ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
		DisableNotification bool                            `json:"disable_notification"`
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

	message, err := a.bot.SendDice(request.Context(), domainbot.SendDiceParams{
		BotToken:            token,
		ChatID:              chatID,
		MessageThreadID:     threadID,
		Emoji:               payload.Emoji,
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

func (a *api) stopPoll(writer http.ResponseWriter, request *http.Request, token string) {
	var payload stopPollRequest
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

	poll, err := a.bot.StopPoll(request.Context(), domainbot.StopPollParams{
		BotToken:    token,
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: payload.ReplyMarkup,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, a.telegramPoll(&poll))
}
