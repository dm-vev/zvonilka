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
	case "editMessageText":
		a.editMessageText(writer, request, token)
	case "deleteMessage":
		a.deleteMessage(writer, request, token)
	case "getChat":
		a.getChat(writer, request, token)
	case "getChatMember":
		a.getChatMember(writer, request, token)
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
