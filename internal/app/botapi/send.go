package botapi

import (
	"net/http"
	"strings"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
)

type normalizedMediaRequest struct {
	ChatID              textID
	MessageThreadID     textID
	MediaID             string
	Caption             string
	ReplyToMessageID    textID
	ReplyMarkup         *domainbot.InlineKeyboardMarkup
	DisableNotification bool
}

func (a *api) sendMessage(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendMessageRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	result, err := a.bot.SendMessage(request.Context(), domainbot.SendMessageParams{
		BotToken:              token,
		ChatID:                string(payload.ChatID),
		MessageThreadID:       string(payload.MessageThreadID),
		Text:                  payload.Text,
		ReplyToMessageID:      string(payload.ReplyToMessageID),
		ReplyMarkup:           payload.ReplyMarkup,
		DisableNotification:   payload.DisableNotification,
		DisableWebPagePreview: payload.DisableWebPagePreview,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) sendPhoto(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendPhotoRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Photo,
			Caption:             payload.Caption,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendPhoto(request.Context(), domainbot.SendPhotoParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			Caption:             payload.Caption,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	}, token, "photo", domainmedia.MediaKindImage)
}

func (a *api) sendDocument(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendDocumentRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Document,
			Caption:             payload.Caption,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendDocument(request.Context(), domainbot.SendDocumentParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			Caption:             payload.Caption,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	}, token, "document", domainmedia.MediaKindDocument)
}

func (a *api) sendVideo(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendVideoRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Video,
			Caption:             payload.Caption,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendVideo(request.Context(), domainbot.SendVideoParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			Caption:             payload.Caption,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	}, token, "video", domainmedia.MediaKindVideo)
}

func (a *api) sendAnimation(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendAnimationRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Animation,
			Caption:             payload.Caption,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendAnimation(request.Context(), domainbot.SendAnimationParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			Caption:             payload.Caption,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	}, token, "animation", domainmedia.MediaKindGIF)
}

func (a *api) sendAudio(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendAudioRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Audio,
			Caption:             payload.Caption,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendAudio(request.Context(), domainbot.SendAudioParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			Caption:             payload.Caption,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	}, token, "audio", domainmedia.MediaKindFile)
}

func (a *api) sendVideoNote(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendVideoNoteRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.VideoNote,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendVideoNote(request.Context(), domainbot.SendVideoNoteParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	}, token, "video_note", domainmedia.MediaKindVideo)
}

func (a *api) sendLocation(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendLocationRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	result, err := a.bot.SendLocation(request.Context(), domainbot.SendLocationParams{
		BotToken:             token,
		ChatID:               string(payload.ChatID),
		MessageThreadID:      string(payload.MessageThreadID),
		Latitude:             payload.Latitude,
		Longitude:            payload.Longitude,
		HorizontalAccuracy:   payload.HorizontalAccuracy,
		LivePeriod:           payload.LivePeriod,
		Heading:              payload.Heading,
		ProximityAlertRadius: payload.ProximityAlertRadius,
		ReplyToMessageID:     string(payload.ReplyToMessageID),
		ReplyMarkup:          payload.ReplyMarkup,
		DisableNotification:  payload.DisableNotification,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) sendContact(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendContactRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	result, err := a.bot.SendContact(request.Context(), domainbot.SendContactParams{
		BotToken:            token,
		ChatID:              string(payload.ChatID),
		MessageThreadID:     string(payload.MessageThreadID),
		PhoneNumber:         payload.PhoneNumber,
		FirstName:           payload.FirstName,
		LastName:            payload.LastName,
		VCard:               payload.VCard,
		UserID:              string(payload.UserID),
		ReplyToMessageID:    string(payload.ReplyToMessageID),
		ReplyMarkup:         payload.ReplyMarkup,
		DisableNotification: payload.DisableNotification,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) sendPoll(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendPollRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	result, err := a.bot.SendPoll(request.Context(), domainbot.SendPollParams{
		BotToken:              token,
		ChatID:                string(payload.ChatID),
		MessageThreadID:       string(payload.MessageThreadID),
		Question:              payload.Question,
		Options:               payload.Options,
		IsAnonymous:           payload.IsAnonymous,
		Type:                  payload.Type,
		AllowsMultipleAnswers: payload.AllowsMultipleAnswers,
		ReplyToMessageID:      string(payload.ReplyToMessageID),
		ReplyMarkup:           payload.ReplyMarkup,
		DisableNotification:   payload.DisableNotification,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) sendVoice(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendVoiceRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Voice,
			Caption:             payload.Caption,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendVoice(request.Context(), domainbot.SendVoiceParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			Caption:             payload.Caption,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	}, token, "voice", domainmedia.MediaKindVoice)
}

func (a *api) sendSticker(writer http.ResponseWriter, request *http.Request, token string) {
	var payload sendStickerRequest
	a.sendMedia(writer, request, &payload, func() normalizedMediaRequest {
		return normalizedMediaRequest{
			ChatID:              payload.ChatID,
			MessageThreadID:     payload.MessageThreadID,
			MediaID:             payload.Sticker,
			ReplyToMessageID:    payload.ReplyToMessageID,
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		}
	}, func(payload normalizedMediaRequest) (domainbot.Message, error) {
		return a.bot.SendSticker(request.Context(), domainbot.SendStickerParams{
			BotToken:            token,
			ChatID:              string(payload.ChatID),
			MessageThreadID:     string(payload.MessageThreadID),
			MediaID:             payload.MediaID,
			ReplyToMessageID:    string(payload.ReplyToMessageID),
			ReplyMarkup:         payload.ReplyMarkup,
			DisableNotification: payload.DisableNotification,
		})
	}, token, "sticker", domainmedia.MediaKindSticker)
}

func (a *api) sendMedia(
	writer http.ResponseWriter,
	request *http.Request,
	target any,
	normalize func() normalizedMediaRequest,
	call func(normalizedMediaRequest) (domainbot.Message, error),
	token string,
	field string,
	kind domainmedia.MediaKind,
) {
	if strings.HasPrefix(request.Header.Get("Content-Type"), "multipart/form-data") {
		memoryLimit := int64(defaultMultipartMemory)
		if a.uploadLimit > 0 && a.uploadLimit < memoryLimit {
			memoryLimit = a.uploadLimit
		}
		if err := request.ParseMultipartForm(memoryLimit); err != nil {
			writeError(writer, http.StatusBadRequest, "Bad Request")
			return
		}
	}
	if err := decodeRequest(request, target); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	payload := normalize()
	mediaID, err := a.resolveMediaID(request.Context(), request, token, field, kind, payload.MediaID)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}
	payload.MediaID = mediaID

	result, err := call(payload)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) editMessageText(writer http.ResponseWriter, request *http.Request, token string) {
	var payload editMessageTextRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	result, err := a.bot.EditMessageText(request.Context(), domainbot.EditMessageTextParams{
		BotToken:              token,
		ChatID:                string(payload.ChatID),
		MessageID:             string(payload.MessageID),
		Text:                  payload.Text,
		ReplyMarkup:           payload.ReplyMarkup,
		DisableWebPagePreview: payload.DisableWebPagePreview,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, result)
}

func (a *api) deleteMessage(writer http.ResponseWriter, request *http.Request, token string) {
	var payload deleteMessageRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	if err := a.bot.DeleteMessage(request.Context(), domainbot.DeleteMessageParams{
		BotToken:  token,
		ChatID:    string(payload.ChatID),
		MessageID: string(payload.MessageID),
	}); err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}
