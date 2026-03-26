package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"

	tgmodels "github.com/go-telegram/bot/models"
)

func (a *api) forwardMessage(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		ChatID              textID `json:"chat_id"`
		MessageThreadID     textID `json:"message_thread_id"`
		FromChatID          textID `json:"from_chat_id"`
		MessageID           textID `json:"message_id"`
		DisableNotification bool   `json:"disable_notification"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	chatID, fromChatID, messageID, threadID, err := a.resolveRelayIDs(request, payload.ChatID, payload.FromChatID, payload.MessageID, payload.MessageThreadID)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	message, err := a.bot.ForwardMessage(request.Context(), domainbot.ForwardMessageParams{
		BotToken:            token,
		ChatID:              chatID,
		MessageThreadID:     threadID,
		FromChatID:          fromChatID,
		MessageID:           messageID,
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

func (a *api) copyMessage(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		ChatID              textID                          `json:"chat_id"`
		MessageThreadID     textID                          `json:"message_thread_id"`
		FromChatID          textID                          `json:"from_chat_id"`
		MessageID           textID                          `json:"message_id"`
		Caption             string                          `json:"caption"`
		ReplyToMessageID    textID                          `json:"reply_to_message_id"`
		ReplyParameters     *replyData                      `json:"reply_parameters"`
		ReplyMarkup         *domainbot.InlineKeyboardMarkup `json:"reply_markup"`
		DisableNotification bool                            `json:"disable_notification"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	chatID, fromChatID, messageID, threadID, err := a.resolveRelayIDs(request, payload.ChatID, payload.FromChatID, payload.MessageID, payload.MessageThreadID)
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

	messageIDValue, err := a.bot.CopyMessage(request.Context(), domainbot.CopyMessageParams{
		BotToken:            token,
		ChatID:              chatID,
		MessageThreadID:     threadID,
		FromChatID:          fromChatID,
		MessageID:           messageID,
		Caption:             payload.Caption,
		ReplyToMessageID:    replyToMessageID,
		ReplyMarkup:         payload.ReplyMarkup,
		DisableNotification: payload.DisableNotification,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	publicID, err := a.bot.PublicMessageID(request.Context(), messageIDValue)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, tgmodels.MessageID{ID: int(publicID)})
}

func (a *api) resolveRelayIDs(
	request *http.Request,
	chatID textID,
	fromChatID textID,
	messageID textID,
	threadID textID,
) (string, string, string, string, error) {
	resolvedChatID, err := a.internalChatID(request.Context(), chatID)
	if err != nil {
		return "", "", "", "", err
	}
	resolvedFromChatID, err := a.internalChatID(request.Context(), fromChatID)
	if err != nil {
		return "", "", "", "", err
	}
	resolvedMessageID, err := a.internalMessageID(request.Context(), messageID)
	if err != nil {
		return "", "", "", "", err
	}
	resolvedThreadID, err := a.internalTopicID(request.Context(), threadID)
	if err != nil {
		return "", "", "", "", err
	}

	return resolvedChatID, resolvedFromChatID, resolvedMessageID, resolvedThreadID, nil
}
