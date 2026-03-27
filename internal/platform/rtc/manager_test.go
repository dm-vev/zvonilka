package rtc

import (
	"context"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
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
