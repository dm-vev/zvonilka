package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

type commandScopeData struct {
	Type   string `json:"type"`
	ChatID textID `json:"chat_id"`
	UserID textID `json:"user_id"`
}

func (c commandScopeData) domain() domainbot.CommandScope {
	return domainbot.CommandScope{
		Type:   domainbot.CommandScopeType(c.Type),
		ChatID: string(c.ChatID),
		UserID: string(c.UserID),
	}
}

type commandData struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

type commandRequest struct {
	Commands     []commandData    `json:"commands"`
	Scope        commandScopeData `json:"scope"`
	LanguageCode string           `json:"language_code"`
}

func (a *api) setMyCommands(writer http.ResponseWriter, request *http.Request, token string) {
	var payload commandRequest
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	scope, err := a.commandScope(request, payload.Scope)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	commands := make([]domainbot.Command, 0, len(payload.Commands))
	for _, command := range payload.Commands {
		commands = append(commands, domainbot.Command{
			Command:     command.Command,
			Description: command.Description,
		})
	}

	err = a.bot.SetMyCommands(request.Context(), domainbot.SetCommandsParams{
		BotToken:     token,
		Scope:        scope,
		LanguageCode: payload.LanguageCode,
		Commands:     commands,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}

func (a *api) getMyCommands(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		Scope        commandScopeData `json:"scope"`
		LanguageCode string           `json:"language_code"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	scope, err := a.commandScope(request, payload.Scope)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	commands, err := a.bot.GetMyCommands(request.Context(), domainbot.GetCommandsParams{
		BotToken:     token,
		Scope:        scope,
		LanguageCode: payload.LanguageCode,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, telegramCommands(commands))
}

func (a *api) deleteMyCommands(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		Scope        commandScopeData `json:"scope"`
		LanguageCode string           `json:"language_code"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	scope, err := a.commandScope(request, payload.Scope)
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	err = a.bot.DeleteMyCommands(request.Context(), domainbot.DeleteCommandsParams{
		BotToken:     token,
		Scope:        scope,
		LanguageCode: payload.LanguageCode,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, true)
}

func (a *api) commandScope(request *http.Request, value commandScopeData) (domainbot.CommandScope, error) {
	scope := value.domain()
	var err error
	if scope.ChatID != "" {
		scope.ChatID, err = a.internalChatID(request.Context(), textID(scope.ChatID))
		if err != nil {
			return domainbot.CommandScope{}, err
		}
	}
	if scope.UserID != "" {
		scope.UserID, err = a.internalUserID(request.Context(), textID(scope.UserID))
		if err != nil {
			return domainbot.CommandScope{}, err
		}
	}

	return scope, nil
}
