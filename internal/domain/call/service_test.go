package call_test

import (
	"context"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	calltest "github.com/dm-vev/zvonilka/internal/domain/call/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	platformrtc "github.com/dm-vev/zvonilka/internal/platform/rtc"
)

func TestCallLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 26, 19, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	conversations := conversationtest.NewMemoryStore()
	require.NoError(t, seedDirectConversation(conversations))

	service, err := domaincall.NewService(
		calltest.NewMemoryStore(),
		conversations,
		platformrtc.NewManager(
			"webrtc://test/calls",
			15*time.Minute,
			platformrtc.WithCandidateHost("127.0.0.1"),
			platformrtc.WithUDPPortRange(41000, 41010),
		),
		domaincall.WithNow(clock),
		domaincall.WithSettings(domaincall.Settings{
			InviteTimeout:  45 * time.Second,
			RingingTimeout: 45 * time.Second,
			ReconnectGrace: 5 * time.Second,
			MaxDuration:    2 * time.Hour,
		}),
		domaincall.WithRTC(domaincall.RTCConfig{
			PublicEndpoint: "webrtc://test/calls",
			CredentialTTL:  15 * time.Minute,
			CandidateHost:  "127.0.0.1",
			UDPPortMin:     41000,
			UDPPortMax:     41010,
		}),
	)
	require.NoError(t, err)

	started, events, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
		WithVideo:      true,
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateRinging, started.State)
	require.Len(t, events, 2)

	accepted, events, err := service.AcceptCall(context.Background(), domaincall.AcceptParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateActive, accepted.State)
	require.Len(t, events, 1)

	joined, transport, events, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)
	require.Equal(t, started.ID, joined.ID)
	require.NotEmpty(t, transport.SessionID)
	require.NotEmpty(t, transport.IceUfrag)
	require.NotEmpty(t, transport.IcePwd)
	require.NotEmpty(t, transport.DTLSFingerprint)
	require.Equal(t, "127.0.0.1", transport.CandidateHost)
	require.NotZero(t, transport.CandidatePort)
	require.Len(t, events, 1)

	updated, participant, events, err := service.UpdateCallMediaState(context.Background(), domaincall.UpdateParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		Media: domaincall.MediaState{
			AudioMuted:         true,
			VideoMuted:         false,
			CameraEnabled:      true,
			ScreenShareEnabled: true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, started.ID, updated.ID)
	require.True(t, participant.MediaState.AudioMuted)
	require.True(t, participant.MediaState.ScreenShareEnabled)
	require.Len(t, events, 1)

	offer := mustCreateCallOffer(t)

	descriptionEvent, err := service.PublishDescription(context.Background(), domaincall.PublishDescriptionParams{
		CallID:    started.ID,
		SessionID: transport.SessionID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		Description: domaincall.SessionDescription{
			Type: "offer",
			SDP:  offer,
		},
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.EventTypeSignalDescription, descriptionEvent.EventType)
	require.Equal(t, transport.SessionID, descriptionEvent.Metadata["session_id"])
	require.Equal(t, "offer", descriptionEvent.Metadata["description_type"])

	candidateEvent, err := service.PublishIceCandidate(context.Background(), domaincall.PublishCandidateParams{
		CallID:    started.ID,
		SessionID: transport.SessionID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		IceCandidate: domaincall.Candidate{
			Candidate:        "candidate:1 1 udp 2130706431 127.0.0.1 41000 typ host",
			SDPMid:           "0",
			SDPMLineIndex:    0,
			UsernameFragment: transport.IceUfrag,
		},
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.EventTypeSignalCandidate, candidateEvent.EventType)
	require.Equal(t, transport.SessionID, candidateEvent.Metadata["session_id"])
	require.Equal(t, "candidate:1 1 udp 2130706431 127.0.0.1 41000 typ host", candidateEvent.Metadata["candidate"])

	left, events, err := service.LeaveCall(context.Background(), domaincall.LeaveParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateActive, left.State)
	require.Len(t, events, 1)

	now = now.Add(6 * time.Second)
	expired, err := service.GetCall(context.Background(), domaincall.GetParams{
		CallID:    started.ID,
		AccountID: "acc-a",
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateEnded, expired.State)
	require.Equal(t, domaincall.EndReasonEnded, expired.EndReason)
}

func mustCreateCallOffer(t *testing.T) string {
	t.Helper()

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

	return client.LocalDescription().SDP
}

func TestStartCallRejectsNonDirectConversation(t *testing.T) {
	t.Parallel()

	conversations := conversationtest.NewMemoryStore()
	ctx := context.Background()
	_, err := conversations.SaveConversation(ctx, conversation.Conversation{
		ID:             "conv-channel",
		Kind:           conversation.ConversationKindChannel,
		OwnerAccountID: "acc-a",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	})
	require.NoError(t, err)
	_, err = conversations.SaveConversationMember(ctx, conversation.ConversationMember{
		ConversationID: "conv-channel",
		AccountID:      "acc-a",
		Role:           conversation.MemberRoleOwner,
		JoinedAt:       time.Now().UTC(),
	})
	require.NoError(t, err)

	service, err := domaincall.NewService(
		calltest.NewMemoryStore(),
		conversations,
		platformrtc.NewManager("webrtc://test/calls", 15*time.Minute),
	)
	require.NoError(t, err)

	_, _, err = service.StartCall(ctx, domaincall.StartParams{
		ConversationID: "conv-channel",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
	})
	require.ErrorIs(t, err, domaincall.ErrConflict)
}

func TestGroupCallLifecycleWithScreenShare(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 26, 20, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	conversations := conversationtest.NewMemoryStore()
	require.NoError(t, seedGroupConversation(conversations))

	service, err := domaincall.NewService(
		calltest.NewMemoryStore(),
		conversations,
		platformrtc.NewManager(
			"webrtc://test/calls",
			15*time.Minute,
			platformrtc.WithCandidateHost("127.0.0.1"),
			platformrtc.WithUDPPortRange(41040, 41060),
		),
		domaincall.WithNow(clock),
		domaincall.WithSettings(domaincall.Settings{
			InviteTimeout:  45 * time.Second,
			RingingTimeout: 45 * time.Second,
			ReconnectGrace: 5 * time.Second,
			MaxDuration:    2 * time.Hour,
		}),
		domaincall.WithRTC(domaincall.RTCConfig{
			PublicEndpoint: "webrtc://test/calls",
			CredentialTTL:  15 * time.Minute,
			CandidateHost:  "127.0.0.1",
			UDPPortMin:     41040,
			UDPPortMax:     41060,
		}),
	)
	require.NoError(t, err)

	started, events, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-group",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
		WithVideo:      true,
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateActive, started.State)
	require.NotEmpty(t, started.ActiveSessionID)
	require.Len(t, events, 4)

	joined, transport, events, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateActive, joined.State)
	require.NotEmpty(t, transport.SessionID)
	require.Len(t, events, 1)

	updated, participant, events, err := service.UpdateCallMediaState(context.Background(), domaincall.UpdateParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		Media: domaincall.MediaState{
			AudioMuted:         false,
			VideoMuted:         false,
			CameraEnabled:      true,
			ScreenShareEnabled: true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateActive, updated.State)
	require.True(t, participant.MediaState.ScreenShareEnabled)
	require.Len(t, events, 1)
}

func TestCallExpiresAfterMaxDuration(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 26, 21, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	conversations := conversationtest.NewMemoryStore()
	require.NoError(t, seedDirectConversation(conversations))

	service, err := domaincall.NewService(
		calltest.NewMemoryStore(),
		conversations,
		platformrtc.NewManager("webrtc://test/calls", 15*time.Minute),
		domaincall.WithNow(clock),
		domaincall.WithSettings(domaincall.Settings{
			InviteTimeout:  45 * time.Second,
			RingingTimeout: 45 * time.Second,
			ReconnectGrace: 20 * time.Second,
			MaxDuration:    30 * time.Second,
		}),
	)
	require.NoError(t, err)

	started, _, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
	})
	require.NoError(t, err)
	accepted, _, err := service.AcceptCall(context.Background(), domaincall.AcceptParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateActive, accepted.State)

	now = now.Add(31 * time.Second)
	expired, err := service.GetCall(context.Background(), domaincall.GetParams{
		CallID:    started.ID,
		AccountID: "acc-a",
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateEnded, expired.State)
	require.Equal(t, domaincall.EndReasonEnded, expired.EndReason)
}

func TestGetDiagnosticsAggregatesEndedCallReport(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 26, 23, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	conversations := conversationtest.NewMemoryStore()
	require.NoError(t, seedDirectConversation(conversations))

	service, err := domaincall.NewService(
		calltest.NewMemoryStore(),
		conversations,
		platformrtc.NewManager(
			"webrtc://test/calls",
			15*time.Minute,
			platformrtc.WithCandidateHost("127.0.0.1"),
			platformrtc.WithUDPPortRange(41020, 41030),
		),
		domaincall.WithNow(clock),
		domaincall.WithRTC(domaincall.RTCConfig{
			PublicEndpoint: "webrtc://test/calls",
			CredentialTTL:  15 * time.Minute,
			CandidateHost:  "127.0.0.1",
			UDPPortMin:     41020,
			UDPPortMax:     41030,
		}),
	)
	require.NoError(t, err)

	started, _, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
		WithVideo:      true,
	})
	require.NoError(t, err)

	_, _, err = service.AcceptCall(context.Background(), domaincall.AcceptParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)

	joined, transport, _, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, transport.SessionID)

	offer := mustCreateCallOffer(t)
	_, err = service.PublishDescription(context.Background(), domaincall.PublishDescriptionParams{
		CallID:    started.ID,
		SessionID: transport.SessionID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		Description: domaincall.SessionDescription{
			Type: "offer",
			SDP:  offer,
		},
	})
	require.NoError(t, err)

	now = now.Add(10 * time.Second)
	ended, _, err := service.EndCall(context.Background(), domaincall.EndParams{
		CallID:    started.ID,
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		Reason:    domaincall.EndReasonEnded,
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateEnded, ended.State)

	report, err := service.GetDiagnostics(context.Background(), domaincall.GetParams{
		CallID:    joined.ID,
		AccountID: "acc-a",
	})
	require.NoError(t, err)
	require.Equal(t, joined.ID, report.Call.ID)
	require.GreaterOrEqual(t, report.DurationSeconds, uint32(10))
	require.NotEmpty(t, report.PeakQoSEscalation)
}

func seedDirectConversation(store conversation.Store) error {
	ctx := context.Background()
	now := time.Date(2026, time.March, 26, 18, 0, 0, 0, time.UTC)

	if _, err := store.SaveConversation(ctx, conversation.Conversation{
		ID:             "conv-direct",
		Kind:           conversation.ConversationKindDirect,
		OwnerAccountID: "acc-a",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		return err
	}
	if _, err := store.SaveConversationMember(ctx, conversation.ConversationMember{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		Role:           conversation.MemberRoleOwner,
		JoinedAt:       now,
	}); err != nil {
		return err
	}
	if _, err := store.SaveConversationMember(ctx, conversation.ConversationMember{
		ConversationID: "conv-direct",
		AccountID:      "acc-b",
		Role:           conversation.MemberRoleMember,
		JoinedAt:       now,
	}); err != nil {
		return err
	}
	return nil
}

func seedGroupConversation(store conversation.Store) error {
	ctx := context.Background()
	now := time.Date(2026, time.March, 26, 18, 0, 0, 0, time.UTC)

	if _, err := store.SaveConversation(ctx, conversation.Conversation{
		ID:             "conv-group",
		Kind:           conversation.ConversationKindGroup,
		OwnerAccountID: "acc-a",
		Title:          "Group",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		return err
	}
	members := []conversation.ConversationMember{
		{
			ConversationID: "conv-group",
			AccountID:      "acc-a",
			Role:           conversation.MemberRoleOwner,
			JoinedAt:       now,
		},
		{
			ConversationID: "conv-group",
			AccountID:      "acc-b",
			Role:           conversation.MemberRoleMember,
			JoinedAt:       now,
		},
		{
			ConversationID: "conv-group",
			AccountID:      "acc-c",
			Role:           conversation.MemberRoleMember,
			JoinedAt:       now,
		},
	}
	for _, member := range members {
		if _, err := store.SaveConversationMember(ctx, member); err != nil {
			return err
		}
	}
	return nil
}
