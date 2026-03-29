package rtc

import (
	"context"
	"net"
	"testing"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	callruntimev1 "github.com/dm-vev/zvonilka/internal/genproto/callruntime/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type testCallRuntimeServer struct {
	callruntimev1.UnimplementedCallRuntimeServiceServer

	manager *Manager
}

func (s *testCallRuntimeServer) EnsureSession(
	ctx context.Context,
	req *callruntimev1.EnsureSessionRequest,
) (*callruntimev1.EnsureSessionResponse, error) {
	session, err := s.manager.EnsureSession(ctx, domaincall.Call{
		ID:              req.GetCall().GetCallId(),
		ConversationID:  req.GetCall().GetConversationId(),
		ActiveSessionID: req.GetCall().GetActiveSessionId(),
	})
	if err != nil {
		return nil, err
	}

	return &callruntimev1.EnsureSessionResponse{
		SessionId:       session.SessionID,
		RuntimeEndpoint: session.RuntimeEndpoint,
		IceUfrag:        session.IceUfrag,
		IcePwd:          session.IcePwd,
		DtlsFingerprint: session.DTLSFingerprint,
		CandidateHost:   session.CandidateHost,
		CandidatePort:   int32(session.CandidatePort),
	}, nil
}

func (s *testCallRuntimeServer) JoinSession(
	ctx context.Context,
	req *callruntimev1.JoinSessionRequest,
) (*callruntimev1.JoinSessionResponse, error) {
	join, err := s.manager.JoinSession(ctx, req.GetSessionId(), domaincall.RuntimeParticipant{
		CallID:    req.GetParticipant().GetCallId(),
		AccountID: req.GetParticipant().GetAccountId(),
		DeviceID:  req.GetParticipant().GetDeviceId(),
		WithVideo: req.GetParticipant().GetWithVideo(),
		Media: domaincall.MediaState{
			AudioMuted:         req.GetParticipant().GetMediaState().GetAudioMuted(),
			VideoMuted:         req.GetParticipant().GetMediaState().GetVideoMuted(),
			CameraEnabled:      req.GetParticipant().GetMediaState().GetCameraEnabled(),
			ScreenShareEnabled: req.GetParticipant().GetMediaState().GetScreenShareEnabled(),
		},
	})
	if err != nil {
		return nil, err
	}

	return &callruntimev1.JoinSessionResponse{
		SessionId:       join.SessionID,
		SessionToken:    join.SessionToken,
		RuntimeEndpoint: join.RuntimeEndpoint,
		ExpiresAtUnix:   join.ExpiresAt.Unix(),
		IceUfrag:        join.IceUfrag,
		IcePwd:          join.IcePwd,
		DtlsFingerprint: join.DTLSFingerprint,
		CandidateHost:   join.CandidateHost,
		CandidatePort:   int32(join.CandidatePort),
	}, nil
}

func (s *testCallRuntimeServer) SessionStats(
	ctx context.Context,
	req *callruntimev1.SessionStatsRequest,
) (*callruntimev1.SessionStatsResponse, error) {
	stats, err := s.manager.SessionStats(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}

	result := make([]*callruntimev1.RuntimeStat, 0, len(stats))
	for _, item := range stats {
		result = append(result, &callruntimev1.RuntimeStat{
			AccountId:      item.AccountID,
			DeviceId:       item.DeviceID,
			TransportStats: nil,
		})
	}

	return &callruntimev1.SessionStatsResponse{Stats: result}, nil
}

func (s *testCallRuntimeServer) CloseSession(
	ctx context.Context,
	req *callruntimev1.CloseSessionRequest,
) (*callruntimev1.CloseSessionResponse, error) {
	if err := s.manager.CloseSession(ctx, req.GetSessionId()); err != nil {
		return nil, err
	}

	return &callruntimev1.CloseSessionResponse{}, nil
}

func TestClusterRoutesSessionsThroughRemoteControlEndpoint(t *testing.T) {
	t.Parallel()

	localManager := NewManager("webrtc://node-a/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43000, 43099))
	remoteManager := NewManager("webrtc://node-b/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(43100, 43199))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	server := grpc.NewServer()
	callruntimev1.RegisterCallRuntimeServiceServer(server, &testCallRuntimeServer{manager: remoteManager})
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
