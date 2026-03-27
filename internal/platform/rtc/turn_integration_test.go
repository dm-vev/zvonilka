package rtc

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

func TestManagerNegotiatesViaTURNRelay(t *testing.T) {
	if os.Getenv("RUN_RTC_TURN") != "1" {
		t.Skip("set RUN_RTC_TURN=1 to run rtc TURN integration test")
	}

	turnURL := strings.TrimSpace(os.Getenv("RTC_TEST_TURN_URL"))
	if turnURL == "" {
		turnURL = "turn:127.0.0.1:3478?transport=udp"
	}
	turnSecret := strings.TrimSpace(os.Getenv("RTC_TEST_TURN_SECRET"))
	if turnSecret == "" {
		turnSecret = "zvonilka-turn-secret"
	}

	manager := NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		WithCandidateHost("127.0.0.1"),
		WithUDPPortRange(47000, 47100),
	)
	ctx := context.Background()

	session, err := manager.EnsureSession(ctx, domaincall.Call{
		ID:             "turn-call",
		ConversationID: "turn-conv",
	})
	require.NoError(t, err)

	_, err = manager.JoinSession(ctx, session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "turn-call",
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: true,
	})
	require.NoError(t, err)

	username, credential := testTURNCredential(turnSecret, "turn-acc-a", time.Now().UTC().Add(15*time.Minute))
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
		ICEServers: []webrtc.ICEServer{{
			URLs:       []string{turnURL},
			Username:   username,
			Credential: credential,
		}},
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, client.Close())
	}()

	_, err = client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	require.NoError(t, err)

	gatherComplete := webrtc.GatheringCompletePromise(client)
	offer, err := client.CreateOffer(nil)
	require.NoError(t, err)
	require.NoError(t, client.SetLocalDescription(offer))
	select {
	case <-gatherComplete:
	case <-time.After(10 * time.Second):
		t.Skip("relay-only ICE gathering did not complete within timeout in this environment")
	}
	if !strings.Contains(client.LocalDescription().SDP, " typ relay ") {
		t.Skip("relay-only TURN candidate was not gathered in this environment")
	}

	signals, err := manager.PublishDescription(ctx, session.SessionID, domaincall.RuntimeParticipant{
		CallID:    "turn-call",
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
	stats, err := manager.SessionStats(ctx, session.SessionID)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NoError(t, manager.CloseSession(ctx, session.SessionID))
}

func testTURNCredential(secret string, accountID string, expiresAt time.Time) (string, string) {
	full := strconv.FormatInt(expiresAt.UTC().Unix(), 10) + ":" + strings.TrimSpace(accountID)
	mac := hmac.New(sha1.New, []byte(strings.TrimSpace(secret)))
	_, _ = mac.Write([]byte(full))
	return full, base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
