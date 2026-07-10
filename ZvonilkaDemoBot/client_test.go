package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBotAPIClientSendsJSONAndDecodesResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/botsecret/sendMessage" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", request.Header.Get("Content-Type"))
		}

		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["text"] != "hello" {
			t.Fatalf("payload = %#v", payload)
		}
		_, _ = writer.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
	}))
	defer server.Close()

	client := newBotAPIClient(server.URL, "secret")
	var result struct {
		MessageID int `json:"message_id"`
	}
	if err := client.call(context.Background(), "sendMessage", map[string]any{"text": "hello"}, &result); err != nil {
		t.Fatalf("call: %v", err)
	}
	if result.MessageID != 7 {
		t.Fatalf("message id = %d", result.MessageID)
	}
}

func TestBotAPIClientReturnsAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request"}`))
	}))
	defer server.Close()

	client := newBotAPIClient(server.URL, "secret")
	err := client.call(context.Background(), "getMe", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "api 400: Bad Request") {
		t.Fatalf("error = %v", err)
	}
}

func TestDemoBotSendsHTMLParseMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["parse_mode"] != "HTML" {
			t.Fatalf("parse_mode = %#v", payload["parse_mode"])
		}
		if payload["text"] != "<b>hello</b>" {
			t.Fatalf("text = %#v", payload["text"])
		}
		_, _ = writer.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	bot := &demoBot{client: newBotAPIClient(server.URL, "secret")}
	if err := bot.sendText(context.Background(), "7", "<b>hello</b>", nil); err != nil {
		t.Fatalf("sendText: %v", err)
	}
}
