package rtc

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

func TestManagerNegotiationSoak(t *testing.T) {
	if os.Getenv("RUN_RTC_SOAK") != "1" {
		t.Skip("set RUN_RTC_SOAK=1 to run rtc soak test")
	}

	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(46000, 46999),
	)
	ctx := context.Background()

	const sessions = 100
	for i := range sessions {
		callID := fmt.Sprintf("soak-call-%d", i)
		session, err := manager.EnsureSession(ctx, domaincall.Call{
			ID:             callID,
			ConversationID: "soak-conv-" + callID,
		})
		require.NoError(t, err)

		_, err = manager.JoinSession(ctx, session.SessionID, domaincall.RuntimeParticipant{
			CallID:    callID,
			AccountID: "acc-a",
			DeviceID:  "dev-a",
			WithVideo: true,
		})
		require.NoError(t, err)

		client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		require.NoError(t, err)
		_, err = client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
		require.NoError(t, err)

		offer, err := client.CreateOffer(nil)
		require.NoError(t, err)
		require.NoError(t, client.SetLocalDescription(offer))

		signals, err := manager.PublishDescription(ctx, session.SessionID, domaincall.RuntimeParticipant{
			CallID:    callID,
			AccountID: "acc-a",
			DeviceID:  "dev-a",
			WithVideo: true,
		}, domaincall.SessionDescription{
			Type: "offer",
			SDP:  client.LocalDescription().SDP,
		})
		require.NoError(t, err)
		require.NotEmpty(t, signals)

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

		stats, err := manager.SessionStats(ctx, session.SessionID)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		require.NoError(t, client.Close())
		require.NoError(t, manager.CloseSession(ctx, session.SessionID))
	}
}
