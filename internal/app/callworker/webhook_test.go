package callworker

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/callhook"
)

func TestWebhookHandlerPostsRecordingAndTranscription(t *testing.T) {
	t.Parallel()

	var requests int
	var lastSignature string
	var lastBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		lastSignature = request.Header.Get(callhook.SignatureHeader)
		lastBody, _ = io.ReadAll(request.Body)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	handler := &webhookHandler{
		client:           &http.Client{Timeout: time.Second},
		recordingURL:     server.URL + "/recording",
		transcriptionURL: server.URL + "/transcription",
		secret:           "shared-secret",
	}

	payload := domaincall.HookPayload{
		Event: domaincall.Event{
			EventID:   "evt-1",
			CallID:    "call-1",
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{
			ID:             "call-1",
			RecordingState: domaincall.RecordingStateActive,
		},
	}
	require.NoError(t, handler.HandleRecording(context.Background(), payload))

	payload.Event.EventType = domaincall.EventTypeTranscriptionUpdated
	payload.Call.TranscriptionState = domaincall.TranscriptionStateActive
	require.NoError(t, handler.HandleTranscription(context.Background(), payload))

	require.Equal(t, 2, requests)
	require.Equal(t, callhook.SignPayload("shared-secret", lastBody), lastSignature)
}
