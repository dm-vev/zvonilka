package callhook_test

import (
	"context"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/callhook"
	callhooktest "github.com/dm-vev/zvonilka/internal/domain/callhook/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/media"
)

func TestExecutorProcessesPendingRecordingAndTranscriptionJobs(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	now := time.Date(2026, time.March, 29, 20, 0, 0, 0, time.UTC)

	_, err := store.SaveRecordingJob(context.Background(), callhook.RecordingJob{
		OwnerAccountID:    "acc-1",
		ConversationID:    "conv-1",
		CallID:            "call-1",
		LastEventID:       "evt-recording",
		LastEventSequence: 1,
		State:             domaincall.RecordingStateInactive,
		StartedAt:         now.Add(-5 * time.Minute),
		StoppedAt:         now.Add(-time.Minute),
		NextAttemptAt:     now,
		UpdatedAt:         now,
	})
	require.NoError(t, err)

	_, err = store.SaveTranscriptionJob(context.Background(), callhook.TranscriptionJob{
		OwnerAccountID:    "acc-1",
		ConversationID:    "conv-1",
		CallID:            "call-1",
		LastEventID:       "evt-transcription",
		LastEventSequence: 2,
		State:             domaincall.TranscriptionStateInactive,
		StartedAt:         now.Add(-5 * time.Minute),
		StoppedAt:         now.Add(-time.Minute),
		NextAttemptAt:     now,
		UpdatedAt:         now,
	})
	require.NoError(t, err)

	uploader := &stubUploader{}
	executor, err := callhook.NewExecutor(store, uploader, callhook.ExecutorSettings{PollInterval: time.Second, BatchSize: 10})
	require.NoError(t, err)

	err = executor.ProcessOnceForTests(context.Background())
	require.NoError(t, err)

	recordingJob, err := store.RecordingJobByCallID(context.Background(), "call-1")
	require.NoError(t, err)
	require.Equal(t, "media-1", recordingJob.OutputMediaID)

	transcriptionJob, err := store.TranscriptionJobByCallID(context.Background(), "call-1")
	require.NoError(t, err)
	require.Equal(t, "media-2", transcriptionJob.TranscriptMediaID)

	require.Len(t, uploader.uploads, 2)
	require.Equal(t, media.MediaKindDocument, uploader.uploads[0].Kind)
	require.Equal(t, "call_recording", uploader.uploads[0].Metadata[media.MetadataPurposeKey])
	require.Equal(t, "call_transcription", uploader.uploads[1].Metadata[media.MetadataPurposeKey])
	require.True(t, strings.Contains(uploader.payloads[0], "\"call_id\":\"call-1\""))
	require.True(t, strings.Contains(uploader.payloads[1], "Call call-1 transcription"))
}

func TestExecutorSkipsJobsWithoutTerminalArtifacts(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	now := time.Date(2026, time.March, 29, 20, 0, 0, 0, time.UTC)

	_, err := store.SaveRecordingJob(context.Background(), callhook.RecordingJob{
		OwnerAccountID:    "acc-1",
		ConversationID:    "conv-1",
		CallID:            "call-2",
		LastEventID:       "evt-recording",
		LastEventSequence: 1,
		State:             domaincall.RecordingStateActive,
		StartedAt:         now.Add(-5 * time.Minute),
		NextAttemptAt:     now,
		UpdatedAt:         now,
	})
	require.NoError(t, err)

	uploader := &stubUploader{}
	executor, err := callhook.NewExecutor(store, uploader, callhook.ExecutorSettings{PollInterval: time.Second, BatchSize: 10})
	require.NoError(t, err)

	err = executor.ProcessOnceForTests(context.Background())
	require.NoError(t, err)
	require.Empty(t, uploader.uploads)
}

func TestExecutorRetriesFailedRecordingUpload(t *testing.T) {
	t.Parallel()

	store := callhooktest.NewMemoryStore()
	now := time.Date(2026, time.March, 29, 20, 0, 0, 0, time.UTC)

	_, err := store.SaveRecordingJob(context.Background(), callhook.RecordingJob{
		OwnerAccountID:    "acc-1",
		ConversationID:    "conv-1",
		CallID:            "call-3",
		LastEventID:       "evt-recording",
		LastEventSequence: 1,
		State:             domaincall.RecordingStateInactive,
		StartedAt:         now.Add(-5 * time.Minute),
		StoppedAt:         now.Add(-time.Minute),
		NextAttemptAt:     now,
		UpdatedAt:         now,
	})
	require.NoError(t, err)

	uploader := &stubUploader{failuresRemaining: 1}
	executor, err := callhook.NewExecutor(store, uploader, callhook.ExecutorSettings{
		PollInterval:        time.Second,
		BatchSize:           10,
		RetryInitialBackoff: time.Second,
		RetryMaxBackoff:     5 * time.Second,
		LeaseTTL:            30 * time.Second,
	})
	require.NoError(t, err)

	err = executor.ProcessOnceForTests(context.Background())
	require.Error(t, err)

	recordingJob, err := store.RecordingJobByCallID(context.Background(), "call-3")
	require.NoError(t, err)
	require.Equal(t, 1, recordingJob.Attempts)
	require.Empty(t, recordingJob.OutputMediaID)
	require.Empty(t, recordingJob.LeaseToken)
	require.False(t, recordingJob.LastAttemptAt.IsZero())
	require.True(t, recordingJob.NextAttemptAt.After(recordingJob.LastAttemptAt) || recordingJob.NextAttemptAt.Equal(recordingJob.LastAttemptAt))
	require.Contains(t, recordingJob.LastError, "upload recording artifact")
}

type stubUploader struct {
	uploads           []media.UploadParams
	payloads          []string
	failuresRemaining int
}

func (s *stubUploader) Upload(_ context.Context, params media.UploadParams) (media.MediaAsset, error) {
	if s.failuresRemaining > 0 {
		s.failuresRemaining--
		return media.MediaAsset{}, io.ErrUnexpectedEOF
	}

	s.uploads = append(s.uploads, params)
	body, _ := io.ReadAll(params.Body)
	s.payloads = append(s.payloads, string(body))
	return media.MediaAsset{ID: "media-" + strconv.Itoa(len(s.uploads))}, nil
}
