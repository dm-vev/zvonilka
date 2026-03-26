package botapi

import (
	"net/http"

	tgmodels "github.com/go-telegram/bot/models"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) setMyName(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		Name         string `json:"name"`
		LanguageCode string `json:"language_code"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	err := a.bot.SetMyName(request.Context(), domainbot.SetNameParams{
		BotToken:     token,
		LanguageCode: payload.LanguageCode,
		Name:         payload.Name,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}

func (a *api) getMyName(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		LanguageCode string `json:"language_code"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	name, err := a.bot.GetMyName(request.Context(), domainbot.GetNameParams{
		BotToken:     token,
		LanguageCode: payload.LanguageCode,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, tgmodels.BotName{Name: name})
}

func (a *api) setMyDescription(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		Description  string `json:"description"`
		LanguageCode string `json:"language_code"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	err := a.bot.SetMyDescription(request.Context(), domainbot.SetDescriptionParams{
		BotToken:     token,
		LanguageCode: payload.LanguageCode,
		Description:  payload.Description,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}

func (a *api) getMyDescription(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		LanguageCode string `json:"language_code"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	description, err := a.bot.GetMyDescription(request.Context(), domainbot.GetDescriptionParams{
		BotToken:     token,
		LanguageCode: payload.LanguageCode,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, tgmodels.BotDescription{Description: description})
}

func (a *api) setMyShortDescription(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		ShortDescription string `json:"short_description"`
		LanguageCode     string `json:"language_code"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	err := a.bot.SetMyShortDescription(request.Context(), domainbot.SetShortDescriptionParams{
		BotToken:         token,
		LanguageCode:     payload.LanguageCode,
		ShortDescription: payload.ShortDescription,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}

func (a *api) getMyShortDescription(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		LanguageCode string `json:"language_code"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	description, err := a.bot.GetMyShortDescription(request.Context(), domainbot.GetShortDescriptionParams{
		BotToken:     token,
		LanguageCode: payload.LanguageCode,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, tgmodels.BotShortDescription{ShortDescription: description})
}

func (a *api) setMyDefaultAdministratorRights(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		Rights      *tgmodels.ChatAdministratorRights `json:"rights"`
		ForChannels bool                              `json:"for_channels"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	if payload.Rights == nil {
		rights, err := a.bot.GetMyDefaultAdministratorRights(request.Context(), domainbot.GetRightsParams{
			BotToken:    token,
			ForChannels: payload.ForChannels,
		})
		if err != nil {
			code, description := botError(err)
			writeError(writer, code, description)
			return
		}

		writeResult(writer, telegramAdminRights(rights))
		return
	}

	err := a.bot.SetMyDefaultAdministratorRights(request.Context(), domainbot.SetRightsParams{
		BotToken:    token,
		ForChannels: payload.ForChannels,
		Rights:      botAdminRights(*payload.Rights),
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}

func (a *api) getMyDefaultAdministratorRights(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		ForChannels bool `json:"for_channels"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	rights, err := a.bot.GetMyDefaultAdministratorRights(request.Context(), domainbot.GetRightsParams{
		BotToken:    token,
		ForChannels: payload.ForChannels,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, telegramAdminRights(rights))
}
