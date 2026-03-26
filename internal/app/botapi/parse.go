package botapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func decodeRequest(request *http.Request, target any) error {
	if request == nil || target == nil {
		return errors.New("request is required")
	}

	contentType := request.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		defer request.Body.Close()
		decoder := json.NewDecoder(request.Body)
		if err := decoder.Decode(target); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		return nil
	}

	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := request.ParseMultipartForm(defaultMultipartMemory); err != nil {
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") {
				return nil
			}
			return err
		}
		payload := formPayload(request.Form)
		if len(payload) == 0 {
			return nil
		}

		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		return json.Unmarshal(raw, target)
	}

	if err := request.ParseForm(); err != nil {
		return err
	}
	payload := formPayload(request.Form)
	if len(payload) == 0 {
		payload = formPayload(request.URL.Query())
	}
	if len(payload) == 0 {
		return nil
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return json.Unmarshal(raw, target)
}

func formPayload(values url.Values) map[string]any {
	result := make(map[string]any, len(values))
	for key, items := range values {
		if len(items) == 0 {
			continue
		}
		if len(items) == 1 {
			result[key] = scalar(items[0])
			continue
		}

		values := make([]any, 0, len(items))
		for _, item := range items {
			values = append(values, scalar(item))
		}
		result[key] = values
	}

	return result
}

func scalar(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") {
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err == nil {
			return decoded
		}
	}
	if value == "true" || value == "false" {
		return value == "true"
	}
	if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intValue
	}

	return value
}
