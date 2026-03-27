package rtc

import (
	"context"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

func BenchmarkPeerEnqueueSignalTransport(b *testing.B) {
	peerConn := &peer{}
	signal := domaincall.RuntimeSignal{
		Metadata: map[string]string{
			telemetryKindKey:     telemetryKindTransport,
			telemetryICEStateKey: "connected",
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		peerConn.enqueueSignal(signal)
	}
}

func BenchmarkPeerEnqueueSignalMixed(b *testing.B) {
	peerConn := &peer{}
	transport := domaincall.RuntimeSignal{
		Metadata: map[string]string{
			telemetryKindKey:     telemetryKindTransport,
			telemetryICEStateKey: "connected",
		},
	}
	description := domaincall.RuntimeSignal{
		Description: &domaincall.SessionDescription{
			Type: "offer",
			SDP:  "v=0",
		},
	}
	candidate := domaincall.RuntimeSignal{
		IceCandidate: &domaincall.Candidate{
			Candidate: "candidate:1 1 udp 1 127.0.0.1 40000 typ host",
		},
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		switch i % 3 {
		case 0:
			peerConn.enqueueSignal(transport)
		case 1:
			peerConn.enqueueSignal(description)
		default:
			peerConn.enqueueSignal(candidate)
		}
		if i%64 == 0 {
			_ = (&Manager{}).drainSignals(peerConn)
		}
	}
}

func BenchmarkDrainSignalsWithTransport(b *testing.B) {
	manager := &Manager{}
	makePeer := func() *peer {
		peerConn := &peer{}
		for i := 0; i < 32; i++ {
			peerConn.enqueueSignal(domaincall.RuntimeSignal{
				Description: &domaincall.SessionDescription{Type: "offer", SDP: "v=0"},
			})
		}
		peerConn.enqueueSignal(domaincall.RuntimeSignal{
			Metadata: map[string]string{
				telemetryKindKey:     telemetryKindTransport,
				telemetryICEStateKey: "connected",
			},
		})
		return peerConn
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		peerConn := makePeer()
		_ = manager.drainSignals(peerConn)
	}
}

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

func TestManagerNegotiatesOfferWithPionClient(t *testing.T) {
	t.Parallel()

	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(41010, 41020),
	)

	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-negotiation",
		ConversationID: "conv-negotiation",
	})
	require.NoError(t, err)

	joined, err := manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-negotiation",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
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
	require.NotEmpty(t, client.LocalDescription().SDP)

	signals, err := manager.PublishDescription(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-negotiation",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
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

	_, err = manager.PublishCandidate(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-negotiation",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	}, domaincall.Candidate{
		Candidate:     "candidate:1 1 udp 2130706431 127.0.0.1 41010 typ host",
		SDPMid:        "0",
		SDPMLineIndex: 0,
	})
	require.NoError(t, err)

	require.Equal(t, joined.SessionID, session.SessionID)
}

func TestManagerRenegotiatesWhenRelayTrackAdded(t *testing.T) {
	t.Parallel()

	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(41030, 41040),
	)

	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-relay",
		ConversationID: "conv-relay",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-relay",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	client := mustNewPeerConnection(t)
	defer func() {
		require.NoError(t, client.Close())
	}()
	_, err = client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	require.NoError(t, err)

	offer := mustCreateLocalOffer(t, client)
	_, err = manager.PublishDescription(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-relay",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	}, domaincall.SessionDescription{
		Type: "offer",
		SDP:  offer,
	})
	require.NoError(t, err)

	_, peerB, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-relay",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	localTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"audio",
		"relay",
	)
	require.NoError(t, err)
	_, err = peerB.pc.AddTrack(localTrack)
	require.NoError(t, err)

	require.NoError(t, manager.renegotiatePeer(session.SessionID, peerB))

	signals := manager.drainSignals(peerB)
	require.NotEmpty(t, signals)
	var offerSignal *domaincall.RuntimeSignal
	for i := range signals {
		if signals[i].Description != nil && signals[i].Description.Type == "offer" {
			offerSignal = &signals[i]
			break
		}
	}
	require.NotNil(t, offerSignal)
	require.Equal(t, "acc-b", offerSignal.TargetAccountID)
	require.Equal(t, "dev-b", offerSignal.TargetDeviceID)
}

func TestManagerLeaveSessionRemovesSourceRelayTracks(t *testing.T) {
	t.Parallel()

	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(41041, 41050),
	)

	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-cleanup",
		ConversationID: "conv-cleanup",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-cleanup",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)
	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-cleanup",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	client := mustNewPeerConnection(t)
	defer func() {
		require.NoError(t, client.Close())
	}()
	_, err = client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	require.NoError(t, err)

	offer := mustCreateLocalOffer(t, client)
	_, err = manager.PublishDescription(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-cleanup",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	}, domaincall.SessionDescription{
		Type: "offer",
		SDP:  offer,
	})
	require.NoError(t, err)

	_, peerB, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-cleanup",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	relayLocalTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"audio",
		"relay",
	)
	require.NoError(t, err)
	sender, err := peerB.pc.AddTrack(relayLocalTrack)
	require.NoError(t, err)
	peerB.tracks[relayTrackKey(participantKey("acc-a", "dev-a"), "audio", "relay", "audio")] = &relayTrack{
		track:  relayLocalTrack,
		sender: sender,
	}

	require.NoError(t, manager.LeaveSession(context.Background(), session.SessionID, "acc-a", "dev-a"))
	require.Empty(t, peerB.tracks)

	signals := manager.drainSignals(peerB)
	require.NotEmpty(t, signals)
	var offerSignal *domaincall.RuntimeSignal
	for i := range signals {
		if signals[i].Description != nil && signals[i].Description.Type == "offer" {
			offerSignal = &signals[i]
			break
		}
	}
	require.NotNil(t, offerSignal)
	require.Equal(t, "acc-b", offerSignal.TargetAccountID)
	require.Equal(t, "dev-b", offerSignal.TargetDeviceID)
}

func TestManagerCleanupRelayTrackRemovesOneSourceTrack(t *testing.T) {
	t.Parallel()

	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(41051, 41060),
	)

	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-track-cleanup",
		ConversationID: "conv-track-cleanup",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-track-cleanup",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	client := mustNewPeerConnection(t)
	defer func() {
		require.NoError(t, client.Close())
	}()
	_, err = client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	require.NoError(t, err)

	offer := mustCreateLocalOffer(t, client)
	_, err = manager.PublishDescription(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-track-cleanup",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	}, domaincall.SessionDescription{
		Type: "offer",
		SDP:  offer,
	})
	require.NoError(t, err)

	_, peerB, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-track-cleanup",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	firstTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"audio-1",
		"relay-1",
	)
	require.NoError(t, err)
	firstSender, err := peerB.pc.AddTrack(firstTrack)
	require.NoError(t, err)

	secondTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"audio-2",
		"relay-2",
	)
	require.NoError(t, err)
	secondSender, err := peerB.pc.AddTrack(secondTrack)
	require.NoError(t, err)

	firstKey := relayTrackKey(participantKey("acc-a", "dev-a"), "audio-1", "relay-1", "audio")
	secondKey := relayTrackKey(participantKey("acc-c", "dev-c"), "audio-2", "relay-2", "audio")
	peerB.tracks[firstKey] = &relayTrack{
		track:  firstTrack,
		sender: firstSender,
	}
	peerB.tracks[secondKey] = &relayTrack{
		track:  secondTrack,
		sender: secondSender,
	}

	manager.cleanupRelayTrack(session.SessionID, firstKey)

	require.NotContains(t, peerB.tracks, firstKey)
	require.Contains(t, peerB.tracks, secondKey)

	signals := manager.drainSignals(peerB)
	require.NotEmpty(t, signals)
	var offerSignal *domaincall.RuntimeSignal
	for i := range signals {
		if signals[i].Description != nil && signals[i].Description.Type == "offer" {
			offerSignal = &signals[i]
			break
		}
	}
	require.NotNil(t, offerSignal)
	require.Equal(t, "acc-b", offerSignal.TargetAccountID)
	require.Equal(t, "dev-b", offerSignal.TargetDeviceID)
}

func TestManagerCoalescesRenegotiationUntilAnswer(t *testing.T) {
	t.Parallel()

	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(41061, 41070),
	)

	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-coalesce",
		ConversationID: "conv-coalesce",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-coalesce",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	client := mustNewPeerConnection(t)
	defer func() {
		require.NoError(t, client.Close())
	}()
	_, err = client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	require.NoError(t, err)

	offer := mustCreateLocalOffer(t, client)
	signals, err := manager.PublishDescription(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-coalesce",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	}, domaincall.SessionDescription{
		Type: "offer",
		SDP:  offer,
	})
	require.NoError(t, err)

	var answer string
	for _, signal := range signals {
		if signal.Description != nil && signal.Description.Type == "answer" {
			answer = signal.Description.SDP
			break
		}
	}
	require.NotEmpty(t, answer)
	require.NoError(t, client.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answer,
	}))

	_, peerB, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-coalesce",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	require.NoError(t, manager.renegotiatePeer(session.SessionID, peerB))
	require.NoError(t, manager.renegotiatePeer(session.SessionID, peerB))

	firstSignals := manager.drainSignals(peerB)
	var firstOffer *domaincall.RuntimeSignal
	for i := range firstSignals {
		if firstSignals[i].Description != nil && firstSignals[i].Description.Type == "offer" {
			firstOffer = &firstSignals[i]
			break
		}
	}
	require.NotNil(t, firstOffer)

	require.NoError(t, client.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  firstOffer.Description.SDP,
	}))
	answerTwo, err := client.CreateAnswer(nil)
	require.NoError(t, err)
	require.NoError(t, client.SetLocalDescription(answerTwo))

	secondSignals, err := manager.PublishDescription(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-coalesce",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	}, domaincall.SessionDescription{
		Type: "answer",
		SDP:  client.LocalDescription().SDP,
	})
	require.NoError(t, err)
	var secondOffer *domaincall.RuntimeSignal
	for i := range secondSignals {
		if secondSignals[i].Description != nil && secondSignals[i].Description.Type == "offer" {
			secondOffer = &secondSignals[i]
			break
		}
	}
	require.NotNil(t, secondOffer)
}

func TestManagerEmitsDeduplicatedTelemetrySignals(t *testing.T) {
	t.Parallel()

	manager := NewManager("webrtc://gateway/calls", 15*time.Minute)
	target := &peer{
		accountID: "acc-a",
		deviceID:  "dev-a",
	}

	manager.emitPeerTelemetry("rtc_call-1", target, telemetryICEStateKey, webrtc.ICEConnectionStateChecking.String())
	manager.emitPeerTelemetry("rtc_call-1", target, telemetryICEStateKey, webrtc.ICEConnectionStateChecking.String())
	manager.emitPeerTelemetry("rtc_call-1", target, telemetryPCStateKey, webrtc.PeerConnectionStateConnecting.String())

	signals := manager.drainSignals(target)
	require.Len(t, signals, 1)
	require.Equal(t, "acc-a", signals[0].TargetAccountID)
	require.Equal(t, "dev-a", signals[0].TargetDeviceID)
	require.Equal(t, telemetryKindTransport, signals[0].Metadata[telemetryKindKey])
	require.Equal(t, "connecting", signals[0].Metadata[telemetryQualityKey])
	require.Equal(t, webrtc.ICEConnectionStateChecking.String(), signals[0].Metadata[telemetryICEStateKey])
	require.Equal(t, webrtc.PeerConnectionStateConnecting.String(), signals[0].Metadata[telemetryPCStateKey])
}

func TestPeerSignalQueueCoalescesTelemetryAndKeepsCriticalSignals(t *testing.T) {
	t.Parallel()

	target := &peer{
		accountID: "acc-a",
		deviceID:  "dev-a",
	}

	for i := 0; i < maxPendingSignals+10; i++ {
		target.enqueueSignal(domaincall.RuntimeSignal{
			TargetAccountID: "acc-a",
			TargetDeviceID:  "dev-a",
			SessionID:       "rtc-1",
			Metadata: map[string]string{
				telemetryKindKey:     telemetryKindTransport,
				telemetryICEStateKey: webrtc.ICEConnectionStateChecking.String(),
			},
		})
	}
	target.enqueueSignal(domaincall.RuntimeSignal{
		TargetAccountID: "acc-a",
		TargetDeviceID:  "dev-a",
		SessionID:       "rtc-1",
		Description: &domaincall.SessionDescription{
			Type: "offer",
			SDP:  "v=0",
		},
	})

	manager := &Manager{}
	signals := manager.drainSignals(target)
	require.Len(t, signals, 2)
	var foundOffer bool
	for _, signal := range signals {
		if signal.Description != nil && signal.Description.Type == "offer" {
			foundOffer = true
			break
		}
	}
	require.True(t, foundOffer)
}

func TestManagerSessionStatsExposeTransportCounters(t *testing.T) {
	t.Parallel()

	manager := NewManager("webrtc://gateway/calls", 15*time.Minute)
	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-stats",
		ConversationID: "conv-stats",
	})
	require.NoError(t, err)
	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-stats",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	_, peerA, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-stats",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	manager.emitPeerTelemetry(session.SessionID, peerA, telemetryICEStateKey, webrtc.ICEConnectionStateConnected.String())
	peerA.recordRelayWrite(321, false)
	peerA.recordRelayWrite(0, true)
	peerA.recordRelayWrite(0, true)
	peerA.recordRelayWrite(0, true)

	stats, err := manager.SessionStats(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "acc-a", stats[0].AccountID)
	require.Equal(t, "dev-a", stats[0].DeviceID)
	require.Equal(t, "connected", stats[0].Transport.Quality)
	require.Equal(t, "audio_only", stats[0].Transport.RecommendedProfile)
	require.Equal(t, "relay_write_errors", stats[0].Transport.RecommendationReason)
	require.True(t, stats[0].Transport.VideoFallbackRecommended)
	require.False(t, stats[0].Transport.ReconnectRecommended)
	require.True(t, stats[0].Transport.SuppressOutgoingVideo)
	require.True(t, stats[0].Transport.SuppressIncomingVideo)
	require.False(t, stats[0].Transport.SuppressOutgoingAudio)
	require.False(t, stats[0].Transport.SuppressIncomingAudio)
	require.Equal(t, "stable", stats[0].Transport.QualityTrend)
	require.EqualValues(t, 0, stats[0].Transport.DegradedTransitions)
	require.EqualValues(t, 0, stats[0].Transport.RecoveredTransitions)
	require.Len(t, stats[0].Transport.RecentSamples, 2)
	require.Equal(t, "connected", stats[0].Transport.RecentSamples[0].Quality)
	require.Equal(t, "full", stats[0].Transport.RecentSamples[0].RecommendedProfile)
	require.Equal(t, "connected", stats[0].Transport.RecentSamples[1].Quality)
	require.Equal(t, "audio_only", stats[0].Transport.RecentSamples[1].RecommendedProfile)
	require.EqualValues(t, 1, stats[0].Transport.RelayPackets)
	require.EqualValues(t, 321, stats[0].Transport.RelayBytes)
	require.EqualValues(t, 3, stats[0].Transport.RelayWriteErrors)
	require.False(t, stats[0].Transport.LastUpdatedAt.IsZero())
}

func TestManagerAdaptiveRecommendationRecoversAfterHealthyRelayWrite(t *testing.T) {
	t.Parallel()

	manager := NewManager("webrtc://gateway/calls", 15*time.Minute)
	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-recovery",
		ConversationID: "conv-recovery",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-recovery",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	_, peerA, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-recovery",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	manager.emitPeerTelemetry(session.SessionID, peerA, telemetryICEStateKey, webrtc.ICEConnectionStateConnected.String())
	peerA.recordRelayWrite(0, true)
	peerA.recordRelayWrite(0, true)
	metadata := peerA.recordRelayWrite(0, true)
	require.Equal(t, "audio_only", metadata[telemetryProfileKey])

	metadata = peerA.recordRelayWrite(512, false)
	require.Equal(t, "full", metadata[telemetryProfileKey])
	require.Equal(t, "", metadata[telemetryReasonKey])
	require.Equal(t, "false", metadata[telemetryVideoKey])
	require.Equal(t, "false", metadata[telemetryReconnectKey])

	stats, err := manager.SessionStats(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "stable", stats[0].Transport.QualityTrend)
	require.Len(t, stats[0].Transport.RecentSamples, 3)
}

func TestManagerPrioritizesScreenShareBeforeFullVideoFallback(t *testing.T) {
	t.Parallel()

	manager := NewManager("webrtc://gateway/calls", 15*time.Minute)
	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-screen-share",
		ConversationID: "conv-screen-share",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-screen-share",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
		Media: domaincall.MediaState{
			CameraEnabled:      true,
			ScreenShareEnabled: true,
		},
	})
	require.NoError(t, err)

	_, peerA, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-screen-share",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
		Media: domaincall.MediaState{
			CameraEnabled:      true,
			ScreenShareEnabled: true,
		},
	})
	require.NoError(t, err)

	manager.emitPeerTelemetry(session.SessionID, peerA, telemetryICEStateKey, webrtc.ICEConnectionStateConnected.String())
	peerA.recordRelayWrite(0, true)
	peerA.recordRelayWrite(0, true)
	metadata := peerA.recordRelayWrite(0, true)
	require.Equal(t, "screen_share_only", metadata[telemetryProfileKey])
	require.Equal(t, "preserve_screen_share", metadata[telemetryReasonKey])
	require.Equal(t, "true", metadata[telemetryVideoKey])
	require.Equal(t, "true", metadata["screen_share_priority"])
	require.Equal(t, "true", metadata["suppress_camera_video"])
	require.Equal(t, "false", metadata[telemetrySuppressOutgoingVideoKey])
	require.Equal(t, "false", metadata[telemetrySuppressIncomingVideoKey])

	stats, err := manager.SessionStats(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "screen_share_only", stats[0].Transport.RecommendedProfile)
	require.True(t, stats[0].Transport.ScreenSharePriority)
	require.True(t, stats[0].Transport.SuppressCameraVideo)
	require.False(t, stats[0].Transport.SuppressOutgoingVideo)
	require.False(t, stats[0].Transport.SuppressIncomingVideo)
}

func TestManagerFailedTransportRecommendsReconnect(t *testing.T) {
	t.Parallel()

	manager := NewManager("webrtc://gateway/calls", 15*time.Minute)
	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-failed",
		ConversationID: "conv-failed",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-failed",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	_, peerA, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-failed",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	peerA.updateTelemetry(telemetryICEStateKey, webrtc.ICEConnectionStateConnected.String())
	metadata := peerA.updateTelemetry(telemetryPCStateKey, webrtc.PeerConnectionStateFailed.String())
	require.Equal(t, "reconnect", metadata[telemetryProfileKey])
	require.Equal(t, "transport_failed", metadata[telemetryReasonKey])
	require.Equal(t, "true", metadata[telemetryReconnectKey])
	require.Equal(t, "false", metadata[telemetryVideoKey])
	require.Equal(t, "true", metadata[telemetrySuppressOutgoingAudioKey])
	require.Equal(t, "true", metadata[telemetrySuppressIncomingAudioKey])
	require.Equal(t, "false", metadata[telemetrySuppressOutgoingVideoKey])
	require.Equal(t, "false", metadata[telemetrySuppressIncomingVideoKey])
	require.Equal(t, "1", metadata[telemetryReconnectAttemptKey])
	require.NotEmpty(t, metadata[telemetryReconnectBackoffKey])

	stats, err := manager.SessionStats(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "degrading", stats[0].Transport.QualityTrend)
	require.EqualValues(t, 1, stats[0].Transport.DegradedTransitions)
	require.EqualValues(t, 0, stats[0].Transport.RecoveredTransitions)
	require.EqualValues(t, 1, stats[0].Transport.ReconnectAttempt)
	require.False(t, stats[0].Transport.ReconnectBackoffUntil.IsZero())
	require.True(t, stats[0].Transport.SuppressOutgoingAudio)
	require.True(t, stats[0].Transport.SuppressIncomingAudio)
	require.False(t, stats[0].Transport.SuppressOutgoingVideo)
	require.False(t, stats[0].Transport.SuppressIncomingVideo)
	require.False(t, stats[0].Transport.LastQualityChangeAt.IsZero())
	require.Len(t, stats[0].Transport.RecentSamples, 2)
	require.Equal(t, "connected", stats[0].Transport.RecentSamples[0].Quality)
	require.Equal(t, "failed", stats[0].Transport.RecentSamples[1].Quality)
	require.Equal(t, "reconnect", stats[0].Transport.RecentSamples[1].RecommendedProfile)
}

func TestManagerQualityTrendTracksRecovery(t *testing.T) {
	t.Parallel()

	manager := NewManager("webrtc://gateway/calls", 15*time.Minute)
	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-trend",
		ConversationID: "conv-trend",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-trend",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	_, peerA, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-trend",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	peerA.updateTelemetry(telemetryICEStateKey, webrtc.ICEConnectionStateConnected.String())
	peerA.updateTelemetry(telemetryICEStateKey, webrtc.ICEConnectionStateDisconnected.String())
	peerA.updateTelemetry(telemetryICEStateKey, webrtc.ICEConnectionStateConnected.String())

	stats, err := manager.SessionStats(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "improving", stats[0].Transport.QualityTrend)
	require.EqualValues(t, 1, stats[0].Transport.DegradedTransitions)
	require.EqualValues(t, 1, stats[0].Transport.RecoveredTransitions)
	require.Len(t, stats[0].Transport.RecentSamples, 3)
	require.Equal(t, "connected", stats[0].Transport.RecentSamples[0].Quality)
	require.Equal(t, "degraded", stats[0].Transport.RecentSamples[1].Quality)
	require.Equal(t, "connected", stats[0].Transport.RecentSamples[2].Quality)
}

func TestManagerAcknowledgeAdaptationClearsPendingState(t *testing.T) {
	t.Parallel()

	manager := NewManager("webrtc://gateway/calls", 15*time.Minute)
	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-ack",
		ConversationID: "conv-ack",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-ack",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	_, peerA, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-ack",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	peerA.updateTelemetry(telemetryICEStateKey, webrtc.ICEConnectionStateConnected.String())
	peerA.recordRelayWrite(0, true)
	peerA.recordRelayWrite(0, true)
	peerA.recordRelayWrite(0, true)

	stats, err := manager.SessionStats(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.True(t, stats[0].Transport.PendingAdaptation)
	require.NotZero(t, stats[0].Transport.AdaptationRevision)

	err = manager.AcknowledgeAdaptation(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-ack",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	}, stats[0].Transport.AdaptationRevision, "audio_only")
	require.NoError(t, err)

	stats, err = manager.SessionStats(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.False(t, stats[0].Transport.PendingAdaptation)
	require.Equal(t, stats[0].Transport.AdaptationRevision, stats[0].Transport.AckedAdaptationRevision)
	require.Equal(t, "audio_only", stats[0].Transport.AppliedProfile)
	require.False(t, stats[0].Transport.AppliedAt.IsZero())
}

func TestManagerQoSFeedbackEscalatesToCritical(t *testing.T) {
	t.Parallel()

	manager := NewManager("webrtc://gateway/calls", 15*time.Minute)
	session, err := manager.EnsureSession(context.Background(), domaincall.Call{
		ID:             "call-qos",
		ConversationID: "conv-qos",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(context.Background(), session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-qos",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	_, peerA, err := manager.ensurePeer(session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "call-qos",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	metadata := peerA.recordQoSFeedback([]rtcp.Packet{
		&rtcp.ReceiverReport{
			Reports: []rtcp.ReceptionReport{{
				FractionLost: 20,
				Jitter:       800,
			}},
		},
	})
	require.Equal(t, "elevated", metadata["qos_escalation"])

	metadata = peerA.recordQoSFeedback([]rtcp.Packet{
		&rtcp.ReceiverReport{
			Reports: []rtcp.ReceptionReport{{
				FractionLost: 128,
				Jitter:       4000,
			}},
		},
		&rtcp.TransportLayerNack{},
		&rtcp.TransportLayerNack{},
		&rtcp.TransportLayerNack{},
	})
	require.Equal(t, "critical", metadata["qos_escalation"])

	stats, err := manager.SessionStats(context.Background(), session.SessionID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.InDelta(t, 50.0, stats[0].Transport.PacketLossPct, 0.1)
	require.EqualValues(t, 4000, stats[0].Transport.JitterScore)
	require.Equal(t, "critical", stats[0].Transport.QoSEscalation)
	require.Equal(t, "worsening", stats[0].Transport.QoSTrend)
	require.True(t, stats[0].Transport.ReconnectRecommended)
	require.Len(t, stats[0].Transport.RecentQoSSamples, 2)
}

func mustNewPeerConnection(t *testing.T) *webrtc.PeerConnection {
	t.Helper()

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)

	return pc
}

func mustCreateLocalOffer(t *testing.T, client *webrtc.PeerConnection) string {
	t.Helper()

	offer, err := client.CreateOffer(nil)
	require.NoError(t, err)
	require.NoError(t, client.SetLocalDescription(offer))
	require.NotNil(t, client.LocalDescription())
	require.NotEmpty(t, client.LocalDescription().SDP)

	return client.LocalDescription().SDP
}
