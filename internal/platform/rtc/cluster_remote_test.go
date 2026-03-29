package rtc

import (
	"context"
	"net"
	"testing"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	callruntimev1 "github.com/dm-vev/zvonilka/internal/genproto/callruntime/v1"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestClusterRoutesSessionsThroughRemoteControlEndpoint(t *testing.T) {
	t.Parallel()

	localManager := NewManager("webrtc://node-a/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43000, 43099))
	remoteManager := NewManager("webrtc://node-b/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43100, 43199))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	server := grpc.NewServer()
	callruntimev1.RegisterCallRuntimeServiceServer(server, NewGRPCRuntimeServer(remoteManager))
	defer server.Stop()

	go func() {
		_ = server.Serve(listener)
	}()

	cluster, err := NewCluster(domaincall.RTCConfig{
		PublicEndpoint: "webrtc://gateway/calls",
		CredentialTTL:  15 * time.Minute,
		NodeID:         "node-a",
		CandidateHost:  "127.0.0.1",
		UDPPortMin:     43000,
		UDPPortMax:     43199,
		Nodes: []domaincall.RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls", ControlEndpoint: listener.Addr().String()},
		},
	}, localManager)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cluster.Close(context.Background()))
	}()

	sessionID := "node-b:rtc_call-remote"
	session, err := cluster.EnsureSession(context.Background(), domaincall.Call{
		ID:              "call-remote",
		ConversationID:  "conv-remote",
		ActiveSessionID: sessionID,
	})
	require.NoError(t, err)
	require.Equal(t, sessionID, session.SessionID)
	require.Equal(t, "webrtc://node-b/calls", session.RuntimeEndpoint)

	join, err := cluster.JoinSession(context.Background(), sessionID, domaincall.RuntimeParticipant{
		CallID:    "call-remote",
		AccountID: "acc-remote",
		DeviceID:  "dev-remote",
		WithVideo: true,
		Media: domaincall.MediaState{
			CameraEnabled: true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "webrtc://node-b/calls", join.RuntimeEndpoint)

	stats, err := cluster.SessionStats(context.Background(), sessionID)
	require.NoError(t, err)
	require.Empty(t, stats)

	require.NoError(t, cluster.CloseSession(context.Background(), sessionID))
}

func TestClusterPublishesRemoteDescriptionThroughControlEndpoint(t *testing.T) {
	t.Parallel()

	localManager := NewManager("webrtc://node-a/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43200, 43299))
	remoteManager := NewManager("webrtc://node-b/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43300, 43399))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	server := grpc.NewServer()
	callruntimev1.RegisterCallRuntimeServiceServer(server, NewGRPCRuntimeServer(remoteManager))
	defer server.Stop()

	go func() {
		_ = server.Serve(listener)
	}()

	cluster, err := NewCluster(domaincall.RTCConfig{
		PublicEndpoint: "webrtc://gateway/calls",
		CredentialTTL:  15 * time.Minute,
		NodeID:         "node-a",
		CandidateHost:  "127.0.0.1",
		UDPPortMin:     43200,
		UDPPortMax:     43399,
		Nodes: []domaincall.RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls", ControlEndpoint: listener.Addr().String()},
		},
	}, localManager)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cluster.Close(context.Background()))
	}()

	sessionID := "node-b:rtc_call-remote-signaling"
	_, err = cluster.EnsureSession(context.Background(), domaincall.Call{
		ID:              "call-remote-signaling",
		ConversationID:  "conv-remote-signaling",
		ActiveSessionID: sessionID,
	})
	require.NoError(t, err)

	_, err = cluster.JoinSession(context.Background(), sessionID, domaincall.RuntimeParticipant{
		CallID:    "call-remote-signaling",
		AccountID: "acc-remote",
		DeviceID:  "dev-remote",
		WithVideo: true,
	})
	require.NoError(t, err)

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, client.Close())
	}()

	_, err = client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	require.NoError(t, err)

	offer, err := client.CreateOffer(nil)
	require.NoError(t, err)
	require.NoError(t, client.SetLocalDescription(offer))
	require.NotNil(t, client.LocalDescription())

	signals, err := cluster.PublishDescription(context.Background(), sessionID, domaincall.RuntimeParticipant{
		CallID:    "call-remote-signaling",
		AccountID: "acc-remote",
		DeviceID:  "dev-remote",
		WithVideo: true,
	}, domaincall.SessionDescription{
		Type: "offer",
		SDP:  client.LocalDescription().SDP,
	})
	require.NoError(t, err)

	var answer *domaincall.SessionDescription
	for _, signal := range signals {
		if signal.Description != nil && signal.Description.Type == "answer" {
			answer = signal.Description
			break
		}
	}
	require.NotNil(t, answer)
	require.NoError(t, client.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answer.SDP,
	}))
}
