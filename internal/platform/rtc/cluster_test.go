package rtc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

func TestClusterPlacesCallsOnDeterministicNodes(t *testing.T) {
	t.Parallel()

	cluster, err := NewCluster(domaincall.RTCConfig{
		PublicEndpoint: "webrtc://gateway/calls",
		CredentialTTL:  15 * time.Minute,
		NodeID:         "node-a",
		CandidateHost:  "127.0.0.1",
		UDPPortMin:     41000,
		UDPPortMax:     41099,
		Nodes: []domaincall.RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls"},
		},
	}, NewManager("webrtc://node-a/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(41000, 41099)))
	require.NoError(t, err)

	first, err := cluster.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-1",
		ConversationID: "conv-1",
	})
	require.NoError(t, err)

	second, err := cluster.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-1",
		ConversationID: "conv-1",
	})
	require.NoError(t, err)

	require.Equal(t, first.SessionID, second.SessionID)
	require.Equal(t, first.RuntimeEndpoint, second.RuntimeEndpoint)
	require.NotEmpty(t, domaincall.NodeIDFromSessionID(first.SessionID))
}

func TestClusterRoutesSessionOperationsByNodePrefix(t *testing.T) {
	t.Parallel()

	cluster, err := NewCluster(domaincall.RTCConfig{
		PublicEndpoint: "webrtc://gateway/calls",
		CredentialTTL:  15 * time.Minute,
		NodeID:         "node-a",
		CandidateHost:  "127.0.0.1",
		UDPPortMin:     42000,
		UDPPortMax:     42099,
		Nodes: []domaincall.RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls"},
		},
	}, NewManager("webrtc://node-a/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(42000, 42099)))
	require.NoError(t, err)

	sessionID := "node-b:rtc_call-2"
	session, err := cluster.EnsureSession(context.Background(), domaincall.Call{
		ID:              "call-2",
		ConversationID:  "conv-2",
		ActiveSessionID: sessionID,
	})
	require.NoError(t, err)
	require.Equal(t, sessionID, session.SessionID)
	require.Equal(t, "webrtc://node-b/calls", session.RuntimeEndpoint)

	join, err := cluster.JoinSession(context.Background(), sessionID, domaincall.RuntimeParticipant{
		CallID:    "call-2",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)
	require.Equal(t, sessionID, join.SessionID)
	require.Equal(t, "webrtc://node-b/calls", join.RuntimeEndpoint)

	stats, err := cluster.SessionStats(context.Background(), sessionID)
	require.NoError(t, err)
	require.Empty(t, stats)

	require.NoError(t, cluster.CloseSession(context.Background(), sessionID))
}
