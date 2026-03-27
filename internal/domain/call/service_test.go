package call_test

import (
	"context"
	"testing"
	"time"

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
			AudioMuted:    true,
			VideoMuted:    false,
			CameraEnabled: true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, started.ID, updated.ID)
	require.True(t, participant.MediaState.AudioMuted)
	require.Len(t, events, 1)

	descriptionEvent, err := service.PublishDescription(context.Background(), domaincall.PublishDescriptionParams{
		CallID:    started.ID,
		SessionID: transport.SessionID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		Description: domaincall.SessionDescription{
			Type: "offer",
			SDP:  "v=0\r\ns=test\r\n",
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
	require.Equal(t, domaincall.StateEnded, left.State)
	require.Len(t, events, 2)
}

func TestStartCallRejectsNonDirectConversation(t *testing.T) {
	t.Parallel()

	conversations := conversationtest.NewMemoryStore()
	ctx := context.Background()
	_, err := conversations.SaveConversation(ctx, conversation.Conversation{
		ID:             "conv-group",
		Kind:           conversation.ConversationKindGroup,
		OwnerAccountID: "acc-a",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	})
	require.NoError(t, err)
	_, err = conversations.SaveConversationMember(ctx, conversation.ConversationMember{
		ConversationID: "conv-group",
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
		ConversationID: "conv-group",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
	})
	require.ErrorIs(t, err, domaincall.ErrConflict)
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
