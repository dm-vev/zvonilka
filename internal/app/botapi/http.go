package botapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

type response struct {
	OK          bool   `json:"ok"`
	Result      any    `json:"result,omitempty"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
}

func (a *api) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.serve)
	return mux
}

func (a *api) serve(writer http.ResponseWriter, request *http.Request) {
	token, method, ok := route(request.URL.Path)
	if !ok {
		writeError(writer, http.StatusNotFound, "Not Found")
		return
	}

	switch method {
	case "getMe":
		a.getMe(writer, request, token)
	case "getUpdates":
		a.getUpdates(writer, request, token)
	case "setWebhook":
		a.setWebhook(writer, request, token)
	case "deleteWebhook":
		a.deleteWebhook(writer, request, token)
	case "getWebhookInfo":
		a.getWebhookInfo(writer, request, token)
	case "sendMessage":
		a.sendMessage(writer, request, token)
	case "sendPhoto":
		a.sendPhoto(writer, request, token)
	case "sendDocument":
		a.sendDocument(writer, request, token)
	case "sendVideo":
		a.sendVideo(writer, request, token)
	case "sendAnimation":
		a.sendAnimation(writer, request, token)
	case "sendAudio":
		a.sendAudio(writer, request, token)
	case "sendVideoNote":
		a.sendVideoNote(writer, request, token)
	case "sendLocation":
		a.sendLocation(writer, request, token)
	case "sendVenue":
		a.sendVenue(writer, request, token)
	case "sendContact":
		a.sendContact(writer, request, token)
	case "sendPoll":
		a.sendPoll(writer, request, token)
	case "sendGame":
		a.sendGame(writer, request, token)
	case "sendDice":
		a.sendDice(writer, request, token)
	case "sendVoice":
		a.sendVoice(writer, request, token)
	case "sendSticker":
		a.sendSticker(writer, request, token)
	case "forwardMessage":
		a.forwardMessage(writer, request, token)
	case "forwardMessages":
		a.forwardMessages(writer, request, token)
	case "copyMessage":
		a.copyMessage(writer, request, token)
	case "copyMessages":
		a.copyMessages(writer, request, token)
	case "editMessageText":
		a.editMessageText(writer, request, token)
	case "editMessageCaption":
		a.editMessageCaption(writer, request, token)
	case "editMessageMedia":
		a.editMessageMedia(writer, request, token)
	case "editMessageLiveLocation":
		a.editMessageLiveLocation(writer, request, token)
	case "editMessageReplyMarkup":
		a.editMessageReplyMarkup(writer, request, token)
	case "stopPoll":
		a.stopPoll(writer, request, token)
	case "deleteMessage":
		a.deleteMessage(writer, request, token)
	case "getChat":
		a.getChat(writer, request, token)
	case "getChatMember":
		a.getChatMember(writer, request, token)
	case "getFile":
		a.getFile(writer, request, token)
	case "setMyCommands":
		a.setMyCommands(writer, request, token)
	case "getMyCommands":
		a.getMyCommands(writer, request, token)
	case "deleteMyCommands":
		a.deleteMyCommands(writer, request, token)
	case "setMyName":
		a.setMyName(writer, request, token)
	case "getMyName":
		a.getMyName(writer, request, token)
	case "setMyDescription":
		a.setMyDescription(writer, request, token)
	case "getMyDescription":
		a.getMyDescription(writer, request, token)
	case "setMyShortDescription":
		a.setMyShortDescription(writer, request, token)
	case "getMyShortDescription":
		a.getMyShortDescription(writer, request, token)
	case "setChatMenuButton":
		a.setChatMenuButton(writer, request, token)
	case "getChatMenuButton":
		a.getChatMenuButton(writer, request, token)
	case "setMyDefaultAdministratorRights":
		a.setMyDefaultAdministratorRights(writer, request, token)
	case "getMyDefaultAdministratorRights":
		a.getMyDefaultAdministratorRights(writer, request, token)
	case "answerCallbackQuery":
		a.answerCallbackQuery(writer, request, token)
	case "answerInlineQuery":
		a.answerInlineQuery(writer, request, token)
	default:
		writeError(writer, http.StatusNotFound, "method not found")
	}
}

func route(path string) (string, string, bool) {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	if path == "" || !strings.HasPrefix(path, "bot") {
		return "", "", false
	}

	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return "", "", false
	}

	token := strings.TrimPrefix(parts[0], "bot")
	method := strings.TrimSpace(parts[1])
	if token == "" || method == "" {
		return "", "", false
	}

	return token, method, true
}

func writeResult(writer http.ResponseWriter, result any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(response{
		OK:     true,
		Result: result,
	})
}

func writeError(writer http.ResponseWriter, code int, description string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(response{
		OK:          false,
		ErrorCode:   code,
		Description: description,
	})
}

func botError(err error) (int, string) {
	switch {
	case errors.Is(err, domainbot.ErrInvalidInput):
		return http.StatusBadRequest, "Bad Request"
	case errors.Is(err, domainbot.ErrForbidden):
		return http.StatusForbidden, "Forbidden"
	case errors.Is(err, domainbot.ErrConflict):
		return http.StatusConflict, "Conflict"
	case errors.Is(err, domainbot.ErrNotFound):
		return http.StatusBadRequest, "Bad Request: not found"
	default:
		return http.StatusInternalServerError, "Internal Server Error"
	}
}
