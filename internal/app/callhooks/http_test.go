package callhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/callhook"
	callhooktest "github.com/dm-vev/zvonilka/internal/domain/callhook/teststore"
)

func TestRecordingAndTranscriptionHooksPersistJobs(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	service, err := callhook.NewService(store)
	require.NoError(t, err)

	handler := (&api{service: service, hookSecret: "shared-secret", maxBodyBytes: 64 << 10}).routes()
	now := time.Date(2026, time.March, 29, 19, 0, 0, 0, time.UTC)

	recordingBody, err := json.Marshal(webhookEnvelope{
		Event: domaincall.Event{
			EventID:   "evt-recording",
			CallID:    "call-1",
			Sequence:  1,
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{
			ID:                 "call-1",
			RecordingState:     domaincall.RecordingStateActive,
			RecordingStartedAt: now,
		},
	})
	require.NoError(t, err)

	request := signedHookRequest(http.MethodPost, "/recording", recordingBody, "shared-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	recordingJob, err := store.RecordingJobByCallID(context.Background(), "call-1")
	require.NoError(t, err)
	require.Equal(t, domaincall.RecordingStateActive, recordingJob.State)

	transcriptionBody, err := json.Marshal(webhookEnvelope{
		Event: domaincall.Event{
			EventID:   "evt-transcription",
			CallID:    "call-1",
			Sequence:  2,
			EventType: domaincall.EventTypeTranscriptionUpdated,
		},
		Call: domaincall.Call{
			ID:                     "call-1",
			TranscriptionState:     domaincall.TranscriptionStateActive,
			TranscriptionStartedAt: now,
		},
	})
	require.NoError(t, err)

	request = signedHookRequest(http.MethodPost, "/transcription", transcriptionBody, "shared-secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	transcriptionJob, err := store.TranscriptionJobByCallID(context.Background(), "call-1")
	require.NoError(t, err)
	require.Equal(t, domaincall.TranscriptionStateActive, transcriptionJob.State)
}

func TestHookRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	service, err := callhook.NewService(store)
	require.NoError(t, err)

	handler := (&api{service: service, hookSecret: "shared-secret", maxBodyBytes: 64 << 10}).routes()
	body, err := json.Marshal(webhookEnvelope{
		Event: domaincall.Event{
			EventID:   "evt-recording",
			CallID:    "call-1",
			Sequence:  1,
			EventType: domaincall.EventTypeRecordingUpdated,
		},
		Call: domaincall.Call{ID: "call-1"},
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodPost, "/recording", bytes.NewReader(body))
	request.Header.Set(callhook.SignatureHeader, "sha256=bad")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestHookRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	service, err := callhook.NewService(store)
	require.NoError(t, err)

	handler := (&api{service: service, hookSecret: "shared-secret", maxBodyBytes: 8}).routes()
	body := []byte(`{"event":{"event_id":"evt"}}`)
	request := signedHookRequest(http.MethodPost, "/recording", body, "shared-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
}

func TestHookRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	service, err := callhook.NewService(store)
	require.NoError(t, err)

	handler := (&api{service: service, hookSecret: "shared-secret", maxBodyBytes: 64 << 10}).routes()
	body := []byte(`{"event":{"EventID":"evt-recording","CallID":"call-1","Sequence":1,"EventType":"call.recording_updated"},"call":{"ID":"call-1"},"Extra":true}`)
	request := signedHookRequest(http.MethodPost, "/recording", body, "shared-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
}

func signedHookRequest(method string, target string, body []byte, secret string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(callhook.SignatureHeader, callhook.SignPayload(secret, body))

	return request
}
