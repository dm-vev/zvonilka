package call_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	calltest "github.com/dm-vev/zvonilka/internal/domain/call/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
)

type fakeRuntime struct {
	session   domaincall.RuntimeSession
	join      domaincall.RuntimeJoin
	stats     map[string]domaincall.RuntimeStats
	acked     []ackCall
	closed    []string
	left      []string
	signals   []domaincall.RuntimeSignal
	joinError error
	ackError  error
}

type ackCall struct {
	sessionID string
	accountID string
	deviceID  string
	revision  uint64
	profile   string
}

func newFakeRuntime(now time.Time) *fakeRuntime {
	return &fakeRuntime{
		session: domaincall.RuntimeSession{
			SessionID:       "sess-1",
			RuntimeEndpoint: "webrtc://runtime",
			IceUfrag:        "ufrag",
			IcePwd:          "pwd",
			DTLSFingerprint: "sha-256 test",
			CandidateHost:   "127.0.0.1",
			CandidatePort:   41000,
		},
		join: domaincall.RuntimeJoin{
			SessionID:       "sess-1",
			SessionToken:    "token-1",
			RuntimeEndpoint: "",
			ExpiresAt:       now.Add(15 * time.Minute),
			IceUfrag:        "ufrag",
			IcePwd:          "pwd",
			DTLSFingerprint: "sha-256 test",
			CandidateHost:   "127.0.0.1",
			CandidatePort:   41000,
		},
		stats: make(map[string]domaincall.RuntimeStats),
	}
}

func (f *fakeRuntime) EnsureSession(context.Context, domaincall.Call) (domaincall.RuntimeSession, error) {
	return f.session, nil
}

func (f *fakeRuntime) JoinSession(
	_ context.Context,
	sessionID string,
	participant domaincall.RuntimeParticipant,
) (domaincall.RuntimeJoin, error) {
	if f.joinError != nil {
		return domaincall.RuntimeJoin{}, f.joinError
	}

	join := f.join
	join.SessionID = sessionID
	f.stats[runtimeParticipantKey(participant.AccountID, participant.DeviceID)] = domaincall.RuntimeStats{
		AccountID: participant.AccountID,
		DeviceID:  participant.DeviceID,
	}

	return join, nil
}

func (f *fakeRuntime) PublishDescription(
	context.Context,
	string,
	domaincall.RuntimeParticipant,
	domaincall.SessionDescription,
) ([]domaincall.RuntimeSignal, error) {
	return cloneRuntimeSignals(f.signals), nil
}

func (f *fakeRuntime) PublishCandidate(
	context.Context,
	string,
	domaincall.RuntimeParticipant,
	domaincall.Candidate,
) ([]domaincall.RuntimeSignal, error) {
	return cloneRuntimeSignals(f.signals), nil
}

func (f *fakeRuntime) AcknowledgeAdaptation(
	_ context.Context,
	sessionID string,
	participant domaincall.RuntimeParticipant,
	adaptationRevision uint64,
	appliedProfile string,
) error {
	if f.ackError != nil {
		return f.ackError
	}

	key := runtimeParticipantKey(participant.AccountID, participant.DeviceID)
	item := f.stats[key]
	item.AccountID = participant.AccountID
	item.DeviceID = participant.DeviceID
	item.Transport.AdaptationRevision = adaptationRevision
	item.Transport.PendingAdaptation = false
	item.Transport.AckedAdaptationRevision = adaptationRevision
	item.Transport.AppliedProfile = appliedProfile
	item.Transport.AppliedAt = time.Date(2026, time.March, 27, 12, 0, 0, 0, time.UTC)
	f.stats[key] = item
	f.acked = append(f.acked, ackCall{
		sessionID: sessionID,
		accountID: participant.AccountID,
		deviceID:  participant.DeviceID,
		revision:  adaptationRevision,
		profile:   appliedProfile,
	})

	return nil
}

func (f *fakeRuntime) SessionStats(context.Context, string) ([]domaincall.RuntimeStats, error) {
	result := make([]domaincall.RuntimeStats, 0, len(f.stats))
	for _, item := range f.stats {
		result = append(result, item)
	}

	return result, nil
}

func (f *fakeRuntime) LeaveSession(_ context.Context, sessionID string, accountID string, deviceID string) error {
	f.left = append(f.left, sessionID+"|"+accountID+"|"+deviceID)
	return nil
}

func (f *fakeRuntime) CloseSession(_ context.Context, sessionID string) error {
	f.closed = append(f.closed, sessionID)
	return nil
}

func runtimeParticipantKey(accountID string, deviceID string) string {
	return accountID + "|" + deviceID
}

func cloneRuntimeSignals(src []domaincall.RuntimeSignal) []domaincall.RuntimeSignal {
	if len(src) == 0 {
		return nil
	}

	dst := make([]domaincall.RuntimeSignal, 0, len(src))
	for _, item := range src {
		cloned := domaincall.RuntimeSignal{
			TargetAccountID: item.TargetAccountID,
			TargetDeviceID:  item.TargetDeviceID,
			SessionID:       item.SessionID,
		}
		if len(item.Metadata) > 0 {
			cloned.Metadata = make(map[string]string, len(item.Metadata))
			for key, value := range item.Metadata {
				cloned.Metadata[key] = value
			}
		}
		if item.Description != nil {
			value := *item.Description
			cloned.Description = &value
		}
		if item.IceCandidate != nil {
			value := *item.IceCandidate
			cloned.IceCandidate = &value
		}
		dst = append(dst, cloned)
	}

	return dst
}

func newTestService(t *testing.T, now time.Time, runtime domaincall.Runtime, opts ...domaincall.Option) *domaincall.Service {
	t.Helper()

	conversations := conversationtest.NewMemoryStore()
	require.NoError(t, seedCallDirectConversation(conversations))

	options := []domaincall.Option{
		domaincall.WithNow(func() time.Time { return now }),
		domaincall.WithSettings(domaincall.Settings{
			InviteTimeout:  45 * time.Second,
			RingingTimeout: 45 * time.Second,
			ReconnectGrace: 5 * time.Second,
			MaxDuration:    2 * time.Hour,
		}),
		domaincall.WithRTC(domaincall.RTCConfig{
			PublicEndpoint: "webrtc://public",
			CredentialTTL:  15 * time.Minute,
			CandidateHost:  "127.0.0.1",
			UDPPortMin:     41000,
			UDPPortMax:     41010,
			STUNURLs:       []string{"stun:stun.example.org:3478"},
			TURNURLs:       []string{"turn:turn.example.org:3478?transport=udp"},
			TURNSecret:     "secret",
		}),
	}
	options = append(options, opts...)

	service, err := domaincall.NewService(calltest.NewMemoryStore(), conversations, runtime, options...)
	require.NoError(t, err)

	return service
}

func seedCallDirectConversation(store conversation.Store) error {
	ctx := context.Background()
	now := time.Date(2026, time.March, 27, 10, 0, 0, 0, time.UTC)

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
	if _, err := store.SaveConversationMember(ctx, conversation.ConversationMember{
		ConversationID: "conv-direct",
		AccountID:      "acc-c",
		Role:           conversation.MemberRoleMember,
		JoinedAt:       now,
	}); err != nil {
		return err
	}

	return nil
}

func TestDeclineCancelListCallsAndMissedLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 11, 0, 0, 0, time.UTC)
	current := now
	service := newTestService(
		t,
		now,
		newFakeRuntime(now),
		domaincall.WithNow(func() time.Time { return current }),
	)

	cancelled, events, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
	})
	require.NoError(t, err)
	require.Len(t, events, 3)

	cancelled, events, err = service.CancelCall(context.Background(), domaincall.CancelParams{
		CallID:    cancelled.ID,
		AccountID: "acc-a",
		DeviceID:  "dev-a",
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateEnded, cancelled.State)
	require.Equal(t, domaincall.EndReasonCancelled, cancelled.EndReason)
	require.Len(t, events, 1)

	cancelledAgain, events, err := service.CancelCall(context.Background(), domaincall.CancelParams{
		CallID:    cancelled.ID,
		AccountID: "acc-a",
		DeviceID:  "dev-a",
	})
	require.NoError(t, err)
	require.Equal(t, cancelled.ID, cancelledAgain.ID)
	require.Empty(t, events)

	declined, _, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
	})
	require.NoError(t, err)

	declined, events, err = service.DeclineCall(context.Background(), domaincall.DeclineParams{
		CallID:    declined.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateRinging, declined.State)
	require.Len(t, events, 1)

	declinedAgain, events, err := service.DeclineCall(context.Background(), domaincall.DeclineParams{
		CallID:    declined.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	require.Equal(t, declined.ID, declinedAgain.ID)
	require.Empty(t, events)

	declined, events, err = service.DeclineCall(context.Background(), domaincall.DeclineParams{
		CallID:    declined.ID,
		AccountID: "acc-c",
		DeviceID:  "dev-c",
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateEnded, declined.State)
	require.Equal(t, domaincall.EndReasonDeclined, declined.EndReason)
	require.Len(t, events, 2)

	missed, _, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
	})
	require.NoError(t, err)
	current = current.Add(50 * time.Second)

	missed, err = service.GetCall(context.Background(), domaincall.GetParams{
		CallID:    missed.ID,
		AccountID: "acc-a",
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateEnded, missed.State)
	require.Equal(t, domaincall.EndReasonMissed, missed.EndReason)
}

func TestEventsFiltersTargetedSignalsByDevice(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 12, 0, 0, 0, time.UTC)
	runtime := newFakeRuntime(now)
	runtime.signals = []domaincall.RuntimeSignal{
		{
			TargetAccountID: "acc-b",
			TargetDeviceID:  "dev-b",
			SessionID:       "sess-1",
			Description: &domaincall.SessionDescription{
				Type: "answer",
				SDP:  "v=0",
			},
		},
	}
	service := newTestService(t, now, runtime)

	started, _, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
	})
	require.NoError(t, err)
	_, _, err = service.AcceptCall(context.Background(), domaincall.AcceptParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	_, join, _, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)

	_, err = service.PublishDescription(context.Background(), domaincall.PublishDescriptionParams{
		CallID:    started.ID,
		SessionID: join.SessionID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		Description: domaincall.SessionDescription{
			Type: "offer",
			SDP:  "v=0",
		},
	})
	require.NoError(t, err)

	targeted, err := service.Events(context.Background(), domaincall.EventParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	require.NotEmpty(t, targeted)

	foundTargeted := false
	for _, event := range targeted {
		if event.EventType == domaincall.EventTypeSignalDescription && event.Metadata["description_type"] == "answer" {
			foundTargeted = true
			break
		}
	}
	require.True(t, foundTargeted)

	otherDevice, err := service.Events(context.Background(), domaincall.EventParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-x",
	})
	require.NoError(t, err)
	for _, event := range otherDevice {
		require.False(t, event.EventType == domaincall.EventTypeSignalDescription && event.Metadata["description_type"] == "answer")
	}
}

func TestGetIceConfigAndAcknowledgeAdaptation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 13, 0, 0, 0, time.UTC)
	runtime := newFakeRuntime(now)
	service := newTestService(t, now, runtime)

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
	joined, join, _, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	key := runtimeParticipantKey("acc-b", "dev-b")
	runtime.stats[key] = domaincall.RuntimeStats{
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		Transport: domaincall.TransportStats{
			AdaptationRevision: 2,
			PendingAdaptation:  true,
			RecommendedProfile: "audio_only",
		},
	}

	iceServers, expiresAt, endpoint, err := service.GetIceConfig(context.Background(), domaincall.IceParams{
		CallID:    started.ID,
		AccountID: "acc-b",
	})
	require.NoError(t, err)
	require.Len(t, iceServers, 2)
	require.False(t, expiresAt.IsZero())
	require.Equal(t, "webrtc://public", endpoint)
	require.NotEmpty(t, iceServers[1].Username)
	require.NotEmpty(t, iceServers[1].Credential)

	updated, participant, events, err := service.AcknowledgeCallAdaptation(context.Background(), domaincall.AcknowledgeAdaptationParams{
		CallID:             joined.ID,
		SessionID:          join.SessionID,
		AccountID:          "acc-b",
		DeviceID:           "dev-b",
		AdaptationRevision: 2,
		AppliedProfile:     "audio_only",
	})
	require.NoError(t, err)
	require.Equal(t, joined.ID, updated.ID)
	require.Equal(t, uint64(2), participant.Transport.AckedAdaptationRevision)
	require.False(t, participant.Transport.PendingAdaptation)
	require.Equal(t, "audio_only", participant.Transport.AppliedProfile)
	require.Len(t, events, 1)
	require.Len(t, runtime.acked, 1)

	_, _, _, err = service.GetIceConfig(context.Background(), domaincall.IceParams{
		AccountID: "acc-b",
	})
	require.ErrorIs(t, err, domaincall.ErrInvalidInput)

	ended, _, err := service.EndCall(context.Background(), domaincall.EndParams{
		CallID:    started.ID,
		AccountID: "acc-a",
		DeviceID:  "dev-a",
	})
	require.NoError(t, err)
	require.Equal(t, domaincall.StateEnded, ended.State)

	_, _, _, err = service.GetIceConfig(context.Background(), domaincall.IceParams{
		CallID:    started.ID,
		AccountID: "acc-b",
	})
	require.ErrorIs(t, err, domaincall.ErrConflict)
}

func TestAcknowledgeAdaptationPropagatesRuntimeError(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 18, 0, 0, 0, time.UTC)
	runtime := newFakeRuntime(now)
	service := newTestService(t, now, runtime)

	started, _, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
	})
	require.NoError(t, err)
	_, _, err = service.AcceptCall(context.Background(), domaincall.AcceptParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	_, join, _, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)

	runtime.ackError = errors.New("ack failed")
	_, _, _, err = service.AcknowledgeCallAdaptation(context.Background(), domaincall.AcknowledgeAdaptationParams{
		CallID:             started.ID,
		SessionID:          join.SessionID,
		AccountID:          "acc-b",
		DeviceID:           "dev-b",
		AdaptationRevision: 1,
		AppliedProfile:     "audio_only",
	})
	require.ErrorContains(t, err, "ack failed")
}

func TestCallGuardrailsAndInvalidBranches(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 22, 0, 0, 0, time.UTC)
	runtime := newFakeRuntime(now)
	service := newTestService(t, now, runtime)

	started, _, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
	})
	require.NoError(t, err)

	_, _, err = service.AcceptCall(context.Background(), domaincall.AcceptParams{
		CallID:    started.ID,
		AccountID: "acc-a",
		DeviceID:  "dev-a",
	})
	require.ErrorIs(t, err, domaincall.ErrForbidden)

	_, _, _, err = service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.ErrorIs(t, err, domaincall.ErrConflict)

	_, _, err = service.AcceptCall(context.Background(), domaincall.AcceptParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)

	acceptedAgain, events, err := service.AcceptCall(context.Background(), domaincall.AcceptParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	require.Equal(t, started.ID, acceptedAgain.ID)
	require.Empty(t, events)

	_, _, err = service.EndCall(context.Background(), domaincall.EndParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.ErrorIs(t, err, domaincall.ErrForbidden)

	left, events, err := service.LeaveCall(context.Background(), domaincall.LeaveParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	require.Equal(t, started.ID, left.ID)
	require.Empty(t, events)

	_, _, _, err = service.UpdateCallMediaState(context.Background(), domaincall.UpdateParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.ErrorIs(t, err, domaincall.ErrForbidden)

	joined, join, _, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	_, err = service.PublishDescription(context.Background(), domaincall.PublishDescriptionParams{
		CallID:    started.ID,
		SessionID: join.SessionID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		Description: domaincall.SessionDescription{
			Type: "answer",
			SDP:  "   ",
		},
	})
	require.ErrorIs(t, err, domaincall.ErrInvalidInput)

	_, err = service.PublishDescription(context.Background(), domaincall.PublishDescriptionParams{
		CallID:    started.ID,
		SessionID: join.SessionID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		Description: domaincall.SessionDescription{
			Type: "bogus",
			SDP:  "v=0",
		},
	})
	require.ErrorIs(t, err, domaincall.ErrInvalidInput)

	_, err = service.PublishIceCandidate(context.Background(), domaincall.PublishCandidateParams{
		CallID:    started.ID,
		SessionID: join.SessionID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.ErrorIs(t, err, domaincall.ErrInvalidInput)

	_, err = service.PublishIceCandidate(context.Background(), domaincall.PublishCandidateParams{
		CallID:    started.ID,
		SessionID: "wrong-session",
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		IceCandidate: domaincall.Candidate{
			Candidate: "candidate:1 1 udp 2130706431 127.0.0.1 41000 typ host",
		},
	})
	require.ErrorIs(t, err, domaincall.ErrConflict)

	_, _, _, err = service.AcknowledgeCallAdaptation(context.Background(), domaincall.AcknowledgeAdaptationParams{
		CallID:             joined.ID,
		SessionID:          "wrong-session",
		AccountID:          "acc-b",
		DeviceID:           "dev-b",
		AdaptationRevision: 1,
		AppliedProfile:     "audio_only",
	})
	require.ErrorIs(t, err, domaincall.ErrConflict)

	_, _, err = service.LeaveCall(context.Background(), domaincall.LeaveParams{
		CallID:    started.ID,
		AccountID: "acc-c",
		DeviceID:  "dev-b",
	})
	require.ErrorIs(t, err, domaincall.ErrForbidden)

	_, err = service.GetCall(context.Background(), domaincall.GetParams{
		CallID:    started.ID,
		AccountID: "acc-z",
	})
	require.ErrorIs(t, err, domaincall.ErrForbidden)

	_, _, err = service.StartCall(context.Background(), domaincall.StartParams{})
	require.ErrorIs(t, err, domaincall.ErrInvalidInput)

	_, _, _, err = service.JoinCall(context.Background(), domaincall.JoinParams{})
	require.ErrorIs(t, err, domaincall.ErrInvalidInput)

	_, _, err = service.EndCall(context.Background(), domaincall.EndParams{})
	require.ErrorIs(t, err, domaincall.ErrInvalidInput)
}

func TestStartCallRejectsSecondActiveCallAndPublishAfterEnd(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 23, 0, 0, 0, time.UTC)
	service := newTestService(t, now, newFakeRuntime(now))

	started, _, err := service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-a",
	})
	require.NoError(t, err)

	_, _, err = service.StartCall(context.Background(), domaincall.StartParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		DeviceID:       "dev-x",
	})
	require.ErrorIs(t, err, domaincall.ErrConflict)

	_, _, err = service.AcceptCall(context.Background(), domaincall.AcceptParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	_, join, _, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	_, _, err = service.EndCall(context.Background(), domaincall.EndParams{
		CallID:    started.ID,
		AccountID: "acc-a",
		DeviceID:  "dev-a",
	})
	require.NoError(t, err)

	_, err = service.PublishDescription(context.Background(), domaincall.PublishDescriptionParams{
		CallID:    started.ID,
		SessionID: join.SessionID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		Description: domaincall.SessionDescription{
			Type: "offer",
			SDP:  "v=0",
		},
	})
	require.ErrorIs(t, err, domaincall.ErrConflict)
}
