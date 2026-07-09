package rtc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	callruntimev1 "github.com/dm-vev/zvonilka/internal/genproto/callruntime/v1"
	"google.golang.org/grpc"
)

type benchmarkClusterHarness struct {
	cluster  *Cluster
	server   *grpc.Server
	listener net.Listener
}

func newBenchmarkClusterHarness(t testing.TB, portMin int, portMax int) *benchmarkClusterHarness {
	t.Helper()

	localManager := NewManager("webrtc://node-a/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(portMin, portMax-500))
	remoteManager := NewManager("webrtc://node-b/calls", 15*time.Minute, WithCandidateHost("127.0.0.1"), WithUDPPortRange(portMax-499, portMax))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := grpc.NewServer()
	callruntimev1.RegisterCallRuntimeServiceServer(server, NewGRPCRuntimeServer(remoteManager))
	go func() {
		_ = server.Serve(listener)
	}()

	cluster, err := NewCluster(domaincall.RTCConfig{
		PublicEndpoint: "webrtc://gateway/calls",
		CredentialTTL:  15 * time.Minute,
		NodeID:         "node-a",
		CandidateHost:  "127.0.0.1",
		UDPPortMin:     portMin,
		UDPPortMax:     portMax,
		Nodes: []domaincall.RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls", ControlEndpoint: listener.Addr().String()},
		},
	}, localManager)
	if err != nil {
		server.Stop()
		_ = listener.Close()
		t.Fatalf("new cluster: %v", err)
	}

	return &benchmarkClusterHarness{
		cluster:  cluster,
		server:   server,
		listener: listener,
	}
}

func (h *benchmarkClusterHarness) Close(t testing.TB) {
	t.Helper()

	if h == nil {
		return
	}
	if h.cluster != nil {
		if err := h.cluster.Close(context.Background()); err != nil {
			t.Fatalf("close cluster: %v", err)
		}
	}
	if h.server != nil {
		h.server.Stop()
	}
	if h.listener != nil {
		if err := h.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("close listener: %v", err)
		}
	}
}

func BenchmarkClusterEnsureJoinStatsParallel(b *testing.B) {
	harness := newBenchmarkClusterHarness(b, 50000, 50999)
	defer harness.Close(b)

	ctx := context.Background()
	var counter atomic.Uint64

	b.ReportAllocs()
	b.SetParallelism(8)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			callID := fmt.Sprintf("bench-cluster-call-%d", counter.Add(1))
			sessionID := "node-b:rtc_" + callID

			session, err := harness.cluster.EnsureSession(ctx, domaincall.Call{
				ID:              callID,
				ConversationID:  "bench-cluster-conv-" + callID,
				ActiveSessionID: sessionID,
			})
			if err != nil {
				b.Fatalf("ensure session: %v", err)
			}
			for _, participant := range []domaincall.RuntimeParticipant{
				{CallID: callID, AccountID: "acc-a", DeviceID: "dev-a", WithVideo: true},
				{CallID: callID, AccountID: "acc-b", DeviceID: "dev-b", WithVideo: true},
			} {
				if _, err := harness.cluster.JoinSession(ctx, session.SessionID, participant); err != nil {
					b.Fatalf("join session: %v", err)
				}
			}
			if _, err := harness.cluster.SessionStats(ctx, session.SessionID); err != nil {
				b.Fatalf("session stats: %v", err)
			}
			if err := harness.cluster.CloseSession(ctx, session.SessionID); err != nil {
				b.Fatalf("close session: %v", err)
			}
		}
	})
}

func BenchmarkClusterMigrateSessionParallel(b *testing.B) {
	harness := newBenchmarkClusterHarness(b, 51000, 51999)
	defer harness.Close(b)

	ctx := context.Background()
	var counter atomic.Uint64

	b.ReportAllocs()
	b.SetParallelism(4)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			callID := fmt.Sprintf("bench-cluster-migrate-%d", counter.Add(1))
			sourceSessionID := "node-b:rtc_" + callID

			session, err := harness.cluster.EnsureSession(ctx, domaincall.Call{
				ID:              callID,
				ConversationID:  "bench-cluster-conv-" + callID,
				ActiveSessionID: sourceSessionID,
			})
			if err != nil {
				b.Fatalf("ensure session: %v", err)
			}
			if _, err := harness.cluster.JoinSession(ctx, session.SessionID, domaincall.RuntimeParticipant{
				CallID:    callID,
				AccountID: "acc-a",
				DeviceID:  "dev-a",
				WithVideo: true,
			}); err != nil {
				b.Fatalf("join session: %v", err)
			}
			migrated, err := harness.cluster.MigrateSession(ctx, domaincall.Call{
				ID:              callID,
				ConversationID:  "bench-cluster-conv-" + callID,
				ActiveSessionID: session.SessionID,
			})
			if err != nil {
				b.Fatalf("migrate session: %v", err)
			}
			if _, err := harness.cluster.SessionStats(ctx, migrated.SessionID); err != nil {
				b.Fatalf("session stats after migrate: %v", err)
			}
			if err := harness.cluster.CloseSession(ctx, migrated.SessionID); err != nil {
				b.Fatalf("close migrated session: %v", err)
			}
			if session.SessionID != migrated.SessionID {
				if err := harness.cluster.CloseSession(ctx, session.SessionID); err != nil {
					b.Fatalf("close source session: %v", err)
				}
			}
		}
	})
}
