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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type unhealthyRuntimeServer struct {
	callruntimev1.UnimplementedCallRuntimeServiceServer
}

func (unhealthyRuntimeServer) Health(context.Context, *callruntimev1.HealthRequest) (*callruntimev1.HealthResponse, error) {
	return nil, status.Error(codes.Unavailable, "unhealthy")
}

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

func TestClusterSkipsUnhealthyRemoteNodesForNewCalls(t *testing.T) {
	t.Parallel()

	localManager := NewManager("webrtc://node-a/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43400, 43499))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	server := grpc.NewServer()
	callruntimev1.RegisterCallRuntimeServiceServer(server, unhealthyRuntimeServer{})
	defer server.Stop()

	go func() {
		_ = server.Serve(listener)
	}()

	cluster, err := NewCluster(domaincall.RTCConfig{
		PublicEndpoint: "webrtc://gateway/calls",
		CredentialTTL:  15 * time.Minute,
		NodeID:         "node-a",
		CandidateHost:  "127.0.0.1",
		UDPPortMin:     43400,
		UDPPortMax:     43599,
		HealthTTL:      5 * time.Second,
		HealthTimeout:  500 * time.Millisecond,
		Nodes: []domaincall.RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls", ControlEndpoint: listener.Addr().String()},
		},
	}, localManager)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cluster.Close(context.Background()))
	}()

	session, err := cluster.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-health-aware",
		ConversationID: "conv-health-aware",
	})
	require.NoError(t, err)
	require.Equal(t, "webrtc://node-a/calls", session.RuntimeEndpoint)
	require.Equal(t, "node-a", domaincall.NodeIDFromSessionID(session.SessionID))
}

func TestClusterReplicatesRemoteSessionStateToStandbyNode(t *testing.T) {
	t.Parallel()

	localManager := NewManager("webrtc://node-a/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43600, 43699))
	remoteManager := NewManager("webrtc://node-b/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43700, 43799))

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
		UDPPortMin:     43600,
		UDPPortMax:     43799,
		Nodes: []domaincall.RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls", ControlEndpoint: listener.Addr().String()},
		},
	}, localManager)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cluster.Close(context.Background()))
	}()

	sessionID := "node-b:rtc_call-standby"
	_, err = cluster.EnsureSession(context.Background(), domaincall.Call{
		ID:              "call-standby",
		ConversationID:  "conv-standby",
		ActiveSessionID: sessionID,
	})
	require.NoError(t, err)

	_, err = cluster.JoinSession(context.Background(), sessionID, domaincall.RuntimeParticipant{
		CallID:    "call-standby",
		AccountID: "acc-standby",
		DeviceID:  "dev-standby",
		WithVideo: true,
		Media: domaincall.MediaState{
			CameraEnabled:      true,
			ScreenShareEnabled: true,
		},
	})
	require.NoError(t, err)

	snapshot, err := localManager.ExportSessionSnapshot(context.Background(), "node-a:rtc_call-restored")
	require.ErrorIs(t, err, domaincall.ErrNotFound)
	require.Empty(t, snapshot.CallID)

	_, err = localManager.EnsureSession(context.Background(), domaincall.Call{
		ID:              "call-standby",
		ConversationID:  "conv-standby",
		ActiveSessionID: "node-a:rtc_call-restored",
	})
	require.NoError(t, err)
	require.NoError(t, localManager.RestoreReplica(context.Background(), "call-standby", "node-a:rtc_call-restored"))

	restored, err := localManager.ExportSessionSnapshot(context.Background(), "node-a:rtc_call-restored")
	require.NoError(t, err)
	require.Equal(t, "call-standby", restored.CallID)
	require.Len(t, restored.Participants, 1)
	require.Equal(t, "acc-standby", restored.Participants[0].AccountID)
	require.True(t, restored.Participants[0].Media.ScreenShareEnabled)
}

func TestClusterMigrateSessionCutsOverToDifferentNode(t *testing.T) {
	t.Parallel()

	localManager := NewManager("webrtc://node-a/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43800, 43899))
	remoteManager := NewManager("webrtc://node-b/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43900, 43999))

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
		UDPPortMin:     43800,
		UDPPortMax:     43999,
		Nodes: []domaincall.RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls", ControlEndpoint: listener.Addr().String()},
		},
	}, localManager)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, cluster.Close(context.Background()))
	}()

	sourceSessionID := "node-b:rtc_call-cutover"
	_, err = cluster.EnsureSession(context.Background(), domaincall.Call{
		ID:              "call-cutover",
		ConversationID:  "conv-cutover",
		ActiveSessionID: sourceSessionID,
	})
	require.NoError(t, err)

	_, err = cluster.JoinSession(context.Background(), sourceSessionID, domaincall.RuntimeParticipant{
		CallID:    "call-cutover",
		AccountID: "acc-cutover",
		DeviceID:  "dev-cutover",
		WithVideo: true,
		Media: domaincall.MediaState{
			CameraEnabled: true,
		},
	})
	require.NoError(t, err)

	_, peerConn, err := remoteManager.ensurePeer(sourceSessionID, domaincall.RuntimeParticipant{
		CallID:    "call-cutover",
		AccountID: "acc-cutover",
		DeviceID:  "dev-cutover",
		WithVideo: true,
		Media: domaincall.MediaState{
			CameraEnabled: true,
		},
	})
	require.NoError(t, err)
	peerConn.updateTelemetry(telemetryICEStateKey, webrtc.ICEConnectionStateConnected.String())
	peerConn.updateTelemetry(telemetryPCStateKey, webrtc.PeerConnectionStateConnected.String())
	peerConn.mu.Lock()
	peerConn.tracks[relayTrackKey(participantKey("acc-source", "dev-source"), "track-screen", "stream-screen", webrtc.RTPCodecTypeVideo.String())] = &relayTrack{
		sourceKey:    participantKey("acc-source", "dev-source"),
		sourceTrack:  "track-screen",
		sourceStream: "stream-screen",
		kind:         webrtc.RTPCodecTypeVideo.String(),
		codec: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		screenShare: true,
	}
	peerConn.mu.Unlock()

	migrated, err := cluster.MigrateSession(context.Background(), domaincall.Call{
		ID:              "call-cutover",
		ConversationID:  "conv-cutover",
		ActiveSessionID: sourceSessionID,
	})
	require.NoError(t, err)
	require.Equal(t, "node-a", domaincall.NodeIDFromSessionID(migrated.SessionID))

	restored, err := localManager.ExportSessionSnapshot(context.Background(), migrated.SessionID)
	require.NoError(t, err)
	require.Equal(t, "call-cutover", restored.CallID)
	require.Len(t, restored.Participants, 1)
	require.Equal(t, "acc-cutover", restored.Participants[0].AccountID)
	require.Equal(t, "connected", restored.Participants[0].Transport.Quality)
	require.Equal(t, webrtc.ICEConnectionStateConnected.String(), restored.Participants[0].Transport.IceConnectionState)

	stats, err := cluster.SessionStats(context.Background(), migrated.SessionID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "acc-cutover", stats[0].AccountID)
	require.Equal(t, "connected", stats[0].Transport.Quality)
	require.Equal(t, webrtc.PeerConnectionStateConnected.String(), stats[0].Transport.PeerConnectionState)

	_, migratedPeer, err := localManager.ensurePeer(migrated.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-cutover",
		AccountID: "acc-cutover",
		DeviceID:  "dev-cutover",
		WithVideo: true,
		Media: domaincall.MediaState{
			CameraEnabled: true,
		},
	})
	require.NoError(t, err)
	migratedPeer.mu.Lock()
	relay := migratedPeer.tracks[relayTrackKey(participantKey("acc-source", "dev-source"), "track-screen", "stream-screen", webrtc.RTPCodecTypeVideo.String())]
	migratedPeer.mu.Unlock()
	require.NotNil(t, relay)
	require.True(t, relay.screenShare)
	require.Equal(t, webrtc.MimeTypeVP8, relay.codec.MimeType)

	stats, err = localManager.SessionStats(context.Background(), migrated.SessionID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "connected", stats[0].Transport.Quality)
}
