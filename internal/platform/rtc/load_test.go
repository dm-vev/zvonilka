package rtc

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

func BenchmarkManagerEnsureJoinCloseParallel(b *testing.B) {
	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(43000, 43999),
	)
	ctx := context.Background()
	var counter atomic.Uint64

	b.ReportAllocs()
	b.SetParallelism(8)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			callID := fmt.Sprintf("bench-call-%d", counter.Add(1))

			session, err := manager.EnsureSession(ctx, domaincall.Call{
				ID:             callID,
				ConversationID: "bench-conv-" + callID,
			})
			if err != nil {
				b.Fatalf("ensure session: %v", err)
			}
			if _, err := manager.JoinSession(ctx, session.SessionID, domaincall.RuntimeParticipant{
				CallID:    callID,
				AccountID: "acc-a",
				DeviceID:  "dev-a",
				WithVideo: true,
			}); err != nil {
				b.Fatalf("join a: %v", err)
			}
			if _, err := manager.JoinSession(ctx, session.SessionID, domaincall.RuntimeParticipant{
				CallID:    callID,
				AccountID: "acc-b",
				DeviceID:  "dev-b",
				WithVideo: true,
			}); err != nil {
				b.Fatalf("join b: %v", err)
			}
			if err := manager.CloseSession(ctx, session.SessionID); err != nil {
				b.Fatalf("close session: %v", err)
			}
		}
	})
}

func BenchmarkManagerOfferAnswerParallel(b *testing.B) {
	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(44000, 44999),
	)
	ctx := context.Background()
	var counter atomic.Uint64

	b.ReportAllocs()
	b.SetParallelism(4)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			callID := fmt.Sprintf("bench-negotiate-%d", counter.Add(1))

			session, err := manager.EnsureSession(ctx, domaincall.Call{
				ID:             callID,
				ConversationID: "bench-conv-" + callID,
			})
			if err != nil {
				b.Fatalf("ensure session: %v", err)
			}
			if _, err := manager.JoinSession(ctx, session.SessionID, domaincall.RuntimeParticipant{
				CallID:    callID,
				AccountID: "acc-a",
				DeviceID:  "dev-a",
				WithVideo: true,
			}); err != nil {
				b.Fatalf("join session: %v", err)
			}

			client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
			if err != nil {
				b.Fatalf("new peer connection: %v", err)
			}
			if _, err := client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
				b.Fatalf("add transceiver: %v", err)
			}
			offer, err := client.CreateOffer(nil)
			if err != nil {
				b.Fatalf("create offer: %v", err)
			}
			if err := client.SetLocalDescription(offer); err != nil {
				b.Fatalf("set local description: %v", err)
			}

			signals, err := manager.PublishDescription(ctx, session.SessionID, domaincall.RuntimeParticipant{
				CallID:    callID,
				AccountID: "acc-a",
				DeviceID:  "dev-a",
				WithVideo: true,
			}, domaincall.SessionDescription{
				Type: "offer",
				SDP:  client.LocalDescription().SDP,
			})
			if err != nil {
				_ = client.Close()
				b.Fatalf("publish description: %v", err)
			}

			var answer *domaincall.SessionDescription
			for _, signal := range signals {
				if signal.Description != nil && signal.Description.Type == "answer" {
					answer = signal.Description
					break
				}
			}
			if answer == nil {
				_ = client.Close()
				b.Fatal("expected answer")
			}
			if err := client.SetRemoteDescription(webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  answer.SDP,
			}); err != nil {
				_ = client.Close()
				b.Fatalf("set remote description: %v", err)
			}
			if err := client.Close(); err != nil {
				b.Fatalf("close peer connection: %v", err)
			}
			if err := manager.CloseSession(ctx, session.SessionID); err != nil {
				b.Fatalf("close session: %v", err)
			}
		}
	})
}

func BenchmarkManagerSessionStatsParallel(b *testing.B) {
	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(45000, 45099),
	)
	ctx := context.Background()

	session, err := manager.EnsureSession(ctx, domaincall.Call{
		ID:             "bench-stats",
		ConversationID: "bench-stats-conv",
	})
	if err != nil {
		b.Fatalf("ensure session: %v", err)
	}
	for i := range 32 {
		if _, err := manager.JoinSession(ctx, session.SessionID, domaincall.RuntimeParticipant{
			CallID:    "bench-stats",
			AccountID: fmt.Sprintf("acc-%d", i),
			DeviceID:  fmt.Sprintf("dev-%d", i),
			WithVideo: true,
		}); err != nil {
			b.Fatalf("join participant %d: %v", i, err)
		}
	}
	defer func() {
		if err := manager.CloseSession(ctx, session.SessionID); err != nil {
			b.Fatalf("close session: %v", err)
		}
	}()

	b.ReportAllocs()
	b.SetParallelism(8)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stats, err := manager.SessionStats(ctx, session.SessionID)
			if err != nil {
				b.Fatalf("session stats: %v", err)
			}
			if stats == nil {
				b.Fatal("expected stats slice")
			}
		}
	})
}
