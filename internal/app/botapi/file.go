package botapi

import (
	"net/http"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) getFile(writer http.ResponseWriter, request *http.Request, token string) {
	var payload struct {
		FileID string `json:"file_id"`
	}
	if err := decodeRequest(request, &payload); err != nil {
		writeError(writer, http.StatusBadRequest, "Bad Request")
		return
	}

	file, err := a.bot.GetFile(request.Context(), domainbot.GetFileParams{
		BotToken: token,
		FileID:   payload.FileID,
	})
	if err != nil {
		code, description := botError(err)
		writeError(writer, code, description)
		return
	}

	writeResult(writer, telegramFile(file))
}
