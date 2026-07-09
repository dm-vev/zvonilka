package translation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domaintranslation "github.com/dm-vev/zvonilka/internal/domain/translation"
	"github.com/dm-vev/zvonilka/internal/platform/config"
)

func TestHTTPProviderTranslate(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["text"] != "hello world" || payload["target_language"] != "ru" || payload["source_language"] != "en" {
			t.Fatalf("unexpected request payload: %+v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"translated_text":          "privet mir",
			"detected_source_language": "en",
			"provider":                 "remote-test",
		})
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(config.TranslationConfig{
		EndpointURL:  server.URL,
		APIKey:       "secret-token",
		Timeout:      5 * time.Second,
		MaxTextBytes: 1024,
		ProviderName: "fallback-provider",
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	result, err := provider.Translate(context.Background(), domaintranslation.ProviderRequest{
		Text:           "hello world",
		SourceLanguage: "en",
		TargetLanguage: "ru",
	})
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.TranslatedText != "privet mir" || result.SourceLanguage != "en" || result.Provider != "remote-test" {
		t.Fatalf("unexpected translation result: %+v", result)
	}
}
