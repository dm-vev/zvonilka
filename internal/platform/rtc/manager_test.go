package rtc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

func TestManagerAllocatesUniqueCandidatePorts(t *testing.T) {
	t.Parallel()

	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(41000, 41002),
	)

	first, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-1",
		ConversationID: "conv-1",
	})
	require.NoError(t, err)
	second, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-2",
		ConversationID: "conv-2",
	})
	require.NoError(t, err)

	require.NotEqual(t, first.CandidatePort, second.CandidatePort)
	require.Equal(t, "127.0.0.1", first.CandidateHost)
	require.NotEmpty(t, first.IceUfrag)
	require.NotEmpty(t, first.IcePwd)
	require.NotEmpty(t, first.DTLSFingerprint)
}

func TestManagerReusesReleasedPort(t *testing.T) {
	t.Parallel()

	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithUDPPortRange(41000, 41000),
	)

	first, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-1",
		ConversationID: "conv-1",
	})
	require.NoError(t, err)
	require.Equal(t, 41000, first.CandidatePort)

	err = manager.CloseSession(context.Background(), first.SessionID)
	require.NoError(t, err)

	second, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-2",
		ConversationID: "conv-2",
	})
	require.NoError(t, err)
	require.Equal(t, 41000, second.CandidatePort)
}
