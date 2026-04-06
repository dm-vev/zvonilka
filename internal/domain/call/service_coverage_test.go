package call_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	calltest "github.com/dm-vev/zvonilka/internal/domain/call/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
)

type fakeRuntime struct {
	session   domaincall.RuntimeSession
	join      domaincall.RuntimeJoin
	stats     map[string]domaincall.RuntimeStats
	statsErr  map[string]error
	updated   []domaincall.RuntimeParticipant
	acked     []ackCall
	closed    []string
	left      []string
	signals   []domaincall.RuntimeSignal
	snapshot  domaincall.RuntimeSnapshot
	joinError error
	ackError  error
	ensureN   int
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
		stats:    make(map[string]domaincall.RuntimeStats),
		statsErr: make(map[string]error),
	}
}

func (f *fakeRuntime) EnsureSession(context.Context, domaincall.Call) (domaincall.RuntimeSession, error) {
	f.ensureN++
	if f.ensureN > 1 {
		next := f.session
		next.SessionID = "sess-2"
		next.RuntimeEndpoint = "webrtc://runtime-2"
		next.CandidatePort = 42000
		f.session = next

		join := f.join
		join.SessionID = next.SessionID
		join.RuntimeEndpoint = next.RuntimeEndpoint
		join.CandidatePort = next.CandidatePort
		f.join = join
	}
	return f.session, nil
}

func (f *fakeRuntime) JoinSession(
	_ context.Context,
	sessionID string,
	participant domaincall.RuntimeParticipant,
) (domaincall.RuntimeJoin, error) {
	if f.joinError != nil && sessionID == "sess-1" {
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

func (f *fakeRuntime) UpdateParticipant(
	_ context.Context,
	_ string,
	participant domaincall.RuntimeParticipant,
) error {
	key := runtimeParticipantKey(participant.AccountID, participant.DeviceID)
	item := f.stats[key]
	item.AccountID = participant.AccountID
	item.DeviceID = participant.DeviceID
	f.stats[key] = item
	f.updated = append(f.updated, participant)
	return nil
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

func (f *fakeRuntime) SessionStats(_ context.Context, sessionID string) ([]domaincall.RuntimeStats, error) {
	if err := f.statsErr[sessionID]; err != nil {
		return nil, err
	}

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

func (f *fakeRuntime) SessionState(_ context.Context, callRow domaincall.Call) (domaincall.RuntimeState, error) {
	return domaincall.RuntimeState{
		CallID:                        callRow.ID,
		ConversationID:                callRow.ConversationID,
		SessionID:                     callRow.ActiveSessionID,
		NodeID:                        domaincall.NodeIDFromSessionID(callRow.ActiveSessionID),
		RuntimeEndpoint:               f.session.RuntimeEndpoint,
		Active:                        callRow.State == domaincall.StateActive && callRow.ActiveSessionID != "",
		Healthy:                       callRow.ActiveSessionID != "",
		ConfiguredReplicaNodeIDs:      []string{"node-b"},
		HealthyMigrationTargetNodeIDs: []string{"node-b"},
		ObservedAt:                    time.Date(2026, time.March, 27, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakeRuntime) SessionSnapshot(_ context.Context, callRow domaincall.Call) (domaincall.RuntimeSnapshot, error) {
	snapshot := f.snapshot
	if snapshot.CallID == "" {
		snapshot = domaincall.RuntimeSnapshot{
			CallID:         callRow.ID,
			ConversationID: callRow.ConversationID,
			SessionID:      callRow.ActiveSessionID,
			NodeID:         domaincall.NodeIDFromSessionID(callRow.ActiveSessionID),
			ObservedAt:     time.Date(2026, time.March, 27, 12, 0, 0, 0, time.UTC),
		}
		for _, item := range f.stats {
			snapshot.Participants = append(snapshot.Participants, domaincall.RuntimeSnapshotParticipant{
				AccountID: item.AccountID,
				DeviceID:  item.DeviceID,
				Transport: item.Transport,
			})
		}
	}

	return domaincall.CloneRuntimeSnapshot(snapshot), nil
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

func TestUpdateCallMediaStatePropagatesScreenShareToRuntime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 16, 0, 0, 0, time.UTC)
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

	_, _, _, err = service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	_, participant, _, err := service.UpdateCallMediaState(context.Background(), domaincall.UpdateParams{
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
	require.True(t, participant.MediaState.ScreenShareEnabled)
	require.Len(t, runtime.updated, 1)
	require.True(t, runtime.updated[0].Media.ScreenShareEnabled)
	require.True(t, runtime.updated[0].WithVideo)
}

func TestHandoffCallMovesJoinedDeviceWithinSameAccount(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 17, 0, 0, 0, time.UTC)
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
	_, _, _, err = service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)
	_, _, _, err = service.UpdateCallMediaState(context.Background(), domaincall.UpdateParams{
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

	callRow, participant, details, events, err := service.HandoffCall(context.Background(), domaincall.HandoffParams{
		CallID:       started.ID,
		AccountID:    "acc-b",
		FromDeviceID: "dev-b",
		ToDeviceID:   "dev-b-2",
	})
	require.NoError(t, err)
	require.Equal(t, started.ID, callRow.ID)
	require.Equal(t, "dev-b-2", participant.DeviceID)
	require.True(t, participant.MediaState.AudioMuted)
	require.True(t, participant.MediaState.ScreenShareEnabled)
	require.Equal(t, details.SessionID, callRow.ActiveSessionID)
	require.Len(t, events, 2)
	require.Equal(t, domaincall.EventTypeJoined, events[0].EventType)
	require.Equal(t, "true", events[0].Metadata["handoff"])
	require.Equal(t, "dev-b", events[0].Metadata["from_device_id"])
	require.Equal(t, domaincall.EventTypeLeft, events[1].EventType)
	require.Equal(t, "dev-b-2", events[1].Metadata["to_device_id"])
	require.Contains(t, runtime.left, details.SessionID+"|acc-b|dev-b")
}

func TestHandoffCallRejectsSameDeviceTarget(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 17, 30, 0, 0, time.UTC)
	service := newTestService(t, now, newFakeRuntime(now))

	_, _, _, _, err := service.HandoffCall(context.Background(), domaincall.HandoffParams{
		CallID:       "call-1",
		AccountID:    "acc-a",
		FromDeviceID: "dev-a",
		ToDeviceID:   "dev-a",
	})
	require.ErrorIs(t, err, domaincall.ErrInvalidInput)
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

func TestJoinCallMigratesSessionOnRuntimeUnavailable(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 28, 0, 0, 0, 0, time.UTC)
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

	runtime.joinError = status.Error(codes.Unavailable, "rtc unavailable")
	callRow, details, events, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)
	require.Equal(t, "sess-2", callRow.ActiveSessionID)
	require.Equal(t, "sess-2", details.SessionID)
	require.Len(t, events, 2)
	require.Equal(t, domaincall.EventTypeSessionMigrated, events[0].EventType)
	require.Equal(t, "sess-1", events[0].Metadata["previous_session_id"])
	require.Equal(t, "sess-2", events[0].Metadata["session_id"])
	require.Equal(t, "join", events[0].Metadata["failover_reason"])
	require.Equal(t, domaincall.EventTypeJoined, events[1].EventType)
}

func TestGetCallMigratesSessionOnRuntimeStatsUnavailable(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 28, 1, 0, 0, 0, time.UTC)
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

	joined, _, _, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)
	require.Equal(t, "sess-1", joined.ActiveSessionID)

	runtime.statsErr["sess-1"] = status.Error(codes.Unavailable, "rtc unavailable")

	callRow, err := service.GetCall(context.Background(), domaincall.GetParams{
		CallID:    started.ID,
		AccountID: "acc-a",
	})
	require.NoError(t, err)
	require.Equal(t, "sess-2", callRow.ActiveSessionID)

	events, err := service.Events(context.Background(), domaincall.EventParams{
		CallID:    started.ID,
		AccountID: "acc-a",
	})
	require.NoError(t, err)

	var migrated *domaincall.Event
	for i := range events {
		if events[i].EventType == domaincall.EventTypeSessionMigrated {
			migrated = &events[i]
			break
		}
	}
	require.NotNil(t, migrated)
	require.Equal(t, "sess-1", migrated.Metadata["previous_session_id"])
	require.Equal(t, "sess-2", migrated.Metadata["session_id"])
	require.Equal(t, "stats", migrated.Metadata["failover_reason"])

	targeted, err := service.Events(context.Background(), domaincall.EventParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)

	var targetedMigration *domaincall.Event
	for i := range targeted {
		if targeted[i].EventType != domaincall.EventTypeSessionMigrated {
			continue
		}
		if targeted[i].Metadata["target_account_id"] != "acc-b" || targeted[i].Metadata["target_device_id"] != "dev-b" {
			continue
		}
		targetedMigration = &targeted[i]
		break
	}
	require.NotNil(t, targetedMigration)
	require.Equal(t, "true", targetedMigration.Metadata["reconnect_required"])
	require.Equal(t, "token-1", targetedMigration.Metadata["session_token"])
	require.Equal(t, "webrtc://runtime-2", targetedMigration.Metadata["runtime_endpoint"])
	require.Equal(t, "42000", targetedMigration.Metadata["candidate_port"])
}

func TestReconnectCallMigratesSessionOnRuntimeUnavailable(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 28, 2, 0, 0, 0, time.UTC)
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

	_, _, _, err = service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	runtime.joinError = status.Error(codes.Unavailable, "rtc unavailable")

	callRow, details, events, err := service.ReconnectCall(context.Background(), domaincall.ReconnectParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.NoError(t, err)
	require.Equal(t, "sess-2", callRow.ActiveSessionID)
	require.Equal(t, "sess-2", details.SessionID)
	require.Len(t, events, 2)
	require.Equal(t, domaincall.EventTypeSessionMigrated, events[0].EventType)
	require.Equal(t, domaincall.EventTypeSessionMigrated, events[1].EventType)
	require.Equal(t, "reconnect", events[0].Metadata["failover_reason"])
	require.Equal(t, "acc-b", events[1].Metadata["target_account_id"])
	require.Equal(t, "dev-b", events[1].Metadata["target_device_id"])
}

func TestGetRuntimeStateAndSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 28, 3, 0, 0, 0, time.UTC)
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

	joined, _, _, err := service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	runtime.snapshot = domaincall.RuntimeSnapshot{
		CallID:         started.ID,
		ConversationID: "conv-direct",
		SessionID:      joined.ActiveSessionID,
		NodeID:         domaincall.NodeIDFromSessionID(joined.ActiveSessionID),
		ObservedAt:     now,
		Participants: []domaincall.RuntimeSnapshotParticipant{
			{
				AccountID: "acc-b",
				DeviceID:  "dev-b",
				WithVideo: true,
				Media: domaincall.MediaState{
					CameraEnabled: true,
				},
				Transport: domaincall.TransportStats{
					Quality: "connected",
				},
				Relay: []domaincall.RuntimeRelayTrack{
					{
						SourceAccountID: "acc-a",
						SourceDeviceID:  "dev-a",
						TrackID:         "track-1",
					},
				},
			},
		},
	}

	state, err := service.GetRuntimeState(context.Background(), domaincall.GetParams{
		CallID:    started.ID,
		AccountID: "acc-a",
	})
	require.NoError(t, err)
	require.Equal(t, started.ID, state.CallID)
	require.Equal(t, joined.ActiveSessionID, state.SessionID)
	require.True(t, state.Active)
	require.True(t, state.Healthy)

	snapshot, err := service.GetSessionSnapshot(context.Background(), domaincall.GetParams{
		CallID:    started.ID,
		AccountID: "acc-a",
	})
	require.NoError(t, err)
	require.Equal(t, joined.ActiveSessionID, snapshot.SessionID)
	require.Len(t, snapshot.Participants, 1)
	require.Equal(t, "acc-b", snapshot.Participants[0].AccountID)
	require.Len(t, snapshot.Participants[0].Relay, 1)
	require.Equal(t, "track-1", snapshot.Participants[0].Relay[0].TrackID)
}

func TestMigrateCallSessionRequiresHostAndUpdatesSession(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 28, 4, 0, 0, 0, time.UTC)
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

	_, _, _, err = service.JoinCall(context.Background(), domaincall.JoinParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: true,
	})
	require.NoError(t, err)

	_, _, err = service.MigrateCallSession(context.Background(), domaincall.MigrateParams{
		CallID:    started.ID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
	})
	require.ErrorIs(t, err, domaincall.ErrForbidden)

	callRow, events, err := service.MigrateCallSession(context.Background(), domaincall.MigrateParams{
		CallID:    started.ID,
		AccountID: "acc-a",
		DeviceID:  "dev-a",
	})
	require.NoError(t, err)
	require.Equal(t, "sess-2", callRow.ActiveSessionID)
	require.Len(t, events, 2)
	require.Equal(t, domaincall.EventTypeSessionMigrated, events[0].EventType)
	require.Equal(t, "manual", events[0].Metadata["failover_reason"])
	require.Equal(t, "sess-1", events[0].Metadata["previous_session_id"])
	require.Equal(t, "sess-2", events[0].Metadata["session_id"])
}
