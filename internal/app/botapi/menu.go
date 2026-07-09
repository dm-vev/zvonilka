package botapi

import (
	"encoding/json"
	"net/http"
	"strings"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

type webappData struct {
	URL string `json:"url"`
}

type menuData struct {
	Type   string      `json:"type"`
	Text   string      `json:"text"`
	WebApp *webappData `json:"web_app"`
}

func (m *menuData) UnmarshalJSON(data []byte) error {
	type rawMenu menuData
	var value rawMenu
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*m = menuData(value)
	return nil
}

func (m menuData) domain() domainbot.MenuButton {
	button := domainbot.MenuButton{
		Type: domainbot.MenuButtonType(strings.TrimSpace(m.Type)),
		Text: m.Text,
	}
	if m.WebApp != nil {
		button.WebAppURL = m.WebApp.URL
	}
	return button
}

func (a *api) setChatMenuButton(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		ChatID     textID    `json:"chat_id"`
		MenuButton *menuData `json:"menu_button"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}
	if payload.MenuButton == nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	var chatID string
	if payload.ChatID != "" {
		var err error
		chatID, err = a.internalChatID(request.Context(), payload.ChatID)
		if err != nil {
			code, description := botError(err)
			writeError(writer, code, description)
			return
		}
	}

	err := a.bot.SetChatMenuButton(request.Context(), domainbot.SetMenuParams{
		BotToken: token,
		ChatID:   chatID,
		Button:   payload.MenuButton.domain(),
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}

func (a *api) getChatMenuButton(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		ChatID textID `json:"chat_id"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	var chatID string
	if payload.ChatID != "" {
		var err error
		chatID, err = a.internalChatID(request.Context(), payload.ChatID)
		if err != nil {
			code, description := botError(err)
			writeError(writer, code, description)
			return
		}
	}

	button, err := a.bot.GetChatMenuButton(request.Context(), domainbot.GetMenuParams{
		BotToken: token,
		ChatID:   chatID,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, telegramMenu(button))
}
