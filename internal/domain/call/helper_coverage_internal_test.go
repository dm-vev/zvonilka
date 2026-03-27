package call

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func TestCallHelperCoverage(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 14, 0, 0, 0, time.UTC)

	credentialUser, credentialPass, err := turnCredential("secret", "acc-a", now.Add(time.Minute))
	require.NoError(t, err)
	require.NotEmpty(t, credentialUser)
	require.NotEmpty(t, credentialPass)
	_, _, err = turnCredential("", "acc-a", now)
	require.ErrorIs(t, err, ErrInvalidInput)

	cfg := RTCConfig{
		PublicEndpoint: "  webrtc://public  ",
		CredentialTTL:  0,
		UDPPortMin:     0,
		UDPPortMax:     -1,
		STUNURLs:       []string{" stun:one ", "", "stun:two"},
		TURNURLs:       []string{" turn:one ", ""},
		TURNSecret:     " secret ",
	}.normalize()
	require.Equal(t, 15*time.Minute, cfg.CredentialTTL)
	require.Equal(t, 40000, cfg.UDPPortMin)
	require.Equal(t, 40100, cfg.UDPPortMax)
	require.Equal(t, []string{"stun:one", "stun:two"}, cfg.STUNURLs)
	require.Equal(t, []string{"turn:one"}, cfg.TURNURLs)
	require.Equal(t, "secret", cfg.TURNSecret)

	settings := Settings{}.normalize()
	require.Equal(t, DefaultSettings(), settings)

	require.True(t, visibleSignalEvent(Event{}, "acc-a", "dev-a"))
	require.True(t, visibleSignalEvent(Event{Metadata: map[string]string{
		callMetadataTargetAccountID: "acc-a",
		callMetadataTargetDeviceID:  "dev-a",
	}}, "acc-a", "dev-a"))
	require.False(t, visibleSignalEvent(Event{Metadata: map[string]string{
		callMetadataTargetAccountID: "acc-b",
	}}, "acc-a", "dev-a"))
	require.False(t, visibleSignalEvent(Event{Metadata: map[string]string{
		callMetadataTargetDeviceID: "dev-b",
	}}, "acc-a", "dev-a"))

	require.True(t, liveInvite([]Invite{{State: InviteStatePending}}))
	require.True(t, liveInvite([]Invite{{State: InviteStateAccepted}}))
	require.False(t, liveInvite([]Invite{{State: InviteStateDeclined}}))

	require.Equal(t, 0, qualityRankForSummary("failed"))
	require.Equal(t, 4, qualityRankForSummary(""))
	require.Equal(t, 2, qosEscalationRankForDiagnostics("critical"))
	require.Equal(t, -1, qosEscalationRankForDiagnostics("weird"))

	samples := cloneTransportQoSSamples([]TransportQoSSample{{PacketLossPct: 1.2, JitterScore: 7, RecordedAt: now}})
	require.Len(t, samples, 1)
	samples[0].JitterScore = 99
	require.Equal(t, uint32(7), cloneTransportQoSSample(TransportQoSSample{JitterScore: 7}).JitterScore)

	servers := cloneIceServers([]IceServer{{URLs: []string{"stun:one"}, Username: "u"}})
	require.Len(t, servers, 1)
	servers[0].URLs[0] = "changed"
	require.Equal(t, "stun:one", cloneIceServers([]IceServer{{URLs: []string{"stun:one"}}})[0].URLs[0])
}

func TestCloneCallsDoesNotAliasNestedSlices(t *testing.T) {
	t.Parallel()

	original := []Call{{
		ID: "call-1",
		Participants: []Participant{{
			AccountID: "acc-a",
			Transport: TransportStats{
				RecentQoSSamples: []TransportQoSSample{{JitterScore: 1}},
			},
		}},
	}}
	cloned := cloneCalls(original)
	require.Len(t, cloned, 1)
	cloned[0].Participants[0].Transport.RecentQoSSamples[0].JitterScore = 99
	require.Equal(t, uint32(1), original[0].Participants[0].Transport.RecentQoSSamples[0].JitterScore)
}

func TestValidateContextAndRunTxErrors(t *testing.T) {
	t.Parallel()

	service := &Service{}
	require.ErrorIs(t, service.validateContext(nil, "op"), ErrInvalidInput)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorContains(t, service.validateContext(ctx, "op"), context.Canceled.Error())
	require.ErrorIs(t, service.runTx(context.Background(), nil), ErrInvalidInput)
}

func TestResolveRuntimeEndpointPrefersExplicitValue(t *testing.T) {
	t.Parallel()

	service := &Service{rtc: RTCConfig{PublicEndpoint: "webrtc://public"}}
	require.Equal(t, "webrtc://override", service.resolveRuntimeEndpoint(" webrtc://override "))
	require.Equal(t, "webrtc://public", service.resolveRuntimeEndpoint(""))
}

func TestTrimListDropsBlanks(t *testing.T) {
	t.Parallel()

	require.Nil(t, trimList(nil))
	require.Equal(t, []string{"a", "b"}, trimList([]string{" a ", "", "b"}))
}

func TestNewIDAndRandomTokenValidation(t *testing.T) {
	t.Parallel()

	id, err := newID("call")
	require.NoError(t, err)
	require.Contains(t, id, "call_")

	token, err := randomToken(8)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	_, err = randomToken(0)
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestIceServersForAccountRejectsBlankAccount(t *testing.T) {
	t.Parallel()

	service := &Service{
		rtc: RTCConfig{
			CredentialTTL: 15 * time.Minute,
			TURNURLs:      []string{"turn:turn.example.org:3478"},
			TURNSecret:    "secret",
		},
		now: func() time.Time { return time.Date(2026, time.March, 27, 15, 0, 0, 0, time.UTC) },
	}

	_, _, err := service.iceServersForAccount("", time.Time{})
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestBuildDiagnosticsTracksEventAggregates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 16, 0, 0, 0, time.UTC)
	callRow := Call{
		ID:         "call-1",
		StartedAt:  now.Add(-20 * time.Second),
		AnsweredAt: now.Add(-15 * time.Second),
		Participants: []Participant{
			{
				AccountID: "acc-a",
				DeviceID:  "dev-a",
				Transport: TransportStats{
					ReconnectAttempt: 1,
					QoSEscalation:    "elevated",
					AppliedProfile:   "audio_only",
					AppliedAt:        now.Add(-5 * time.Second),
				},
			},
		},
	}
	events := []Event{
		{
			EventType: EventTypeMediaUpdated,
			Metadata: map[string]string{
				"qos_escalation":            "critical",
				"adaptation_revision":       "2",
				"acked_adaptation_revision": "2",
				"applied_profile":           "reconnect",
				callMetadataTargetAccountID: "acc-a",
				callMetadataTargetDeviceID:  "dev-a",
			},
			CreatedAt: now.Add(-2 * time.Second),
		},
	}

	diagnostics := buildDiagnostics(now, callRow, events)
	require.Equal(t, uint32(20), diagnostics.DurationSeconds)
	require.Equal(t, uint32(15), diagnostics.ActiveDurationSeconds)
	require.Equal(t, "critical", diagnostics.PeakQoSEscalation)
	require.Equal(t, uint32(1), diagnostics.MaxReconnectAttempt)
	require.Equal(t, uint32(1), diagnostics.TotalAdaptationRevisions)
	require.Equal(t, uint32(1), diagnostics.TotalAdaptationAcks)
	require.Equal(t, "reconnect", diagnostics.LastAppliedProfile)
}

func TestServiceConstructorRejectsNilDeps(t *testing.T) {
	t.Parallel()

	_, err := NewService(nil, nil, nil)
	require.ErrorIs(t, err, ErrInvalidInput)
}

type stubRuntime struct{}

func (stubRuntime) EnsureSession(context.Context, Call) (RuntimeSession, error) {
	return RuntimeSession{}, nil
}
func (stubRuntime) JoinSession(context.Context, string, RuntimeParticipant) (RuntimeJoin, error) {
	return RuntimeJoin{}, nil
}
func (stubRuntime) PublishDescription(context.Context, string, RuntimeParticipant, SessionDescription) ([]RuntimeSignal, error) {
	return nil, nil
}
func (stubRuntime) PublishCandidate(context.Context, string, RuntimeParticipant, Candidate) ([]RuntimeSignal, error) {
	return nil, nil
}
func (stubRuntime) UpdateParticipant(context.Context, string, RuntimeParticipant) error { return nil }
func (stubRuntime) AcknowledgeAdaptation(context.Context, string, RuntimeParticipant, uint64, string) error {
	return nil
}
func (stubRuntime) SessionStats(context.Context, string) ([]RuntimeStats, error) { return nil, nil }
func (stubRuntime) LeaveSession(context.Context, string, string, string) error   { return nil }
func (stubRuntime) CloseSession(context.Context, string) error                   { return nil }

type stubStore struct {
	calls        map[string]Call
	invites      map[string][]Invite
	participants map[string][]Participant
	events       []Event
	nextSeq      uint64
}

func newStubStore() *stubStore {
	return &stubStore{
		calls:        make(map[string]Call),
		invites:      make(map[string][]Invite),
		participants: make(map[string][]Participant),
	}
}

func (s *stubStore) WithinTx(_ context.Context, fn func(Store) error) error { return fn(s) }
func (s *stubStore) SaveCall(_ context.Context, call Call) (Call, error) {
	s.calls[call.ID] = cloneCall(call)
	return cloneCall(call), nil
}
func (s *stubStore) CallByID(_ context.Context, callID string) (Call, error) {
	callRow, ok := s.calls[callID]
	if !ok {
		return Call{}, ErrNotFound
	}
	return cloneCall(callRow), nil
}
func (s *stubStore) ActiveCallByConversation(_ context.Context, conversationID string) (Call, error) {
	for _, callRow := range s.calls {
		if callRow.ConversationID == conversationID && callRow.State != StateEnded {
			return cloneCall(callRow), nil
		}
	}
	return Call{}, ErrNotFound
}
func (s *stubStore) CallsByConversation(_ context.Context, conversationID string, includeEnded bool) ([]Call, error) {
	var result []Call
	for _, callRow := range s.calls {
		if callRow.ConversationID != conversationID {
			continue
		}
		if !includeEnded && callRow.State == StateEnded {
			continue
		}
		result = append(result, cloneCall(callRow))
	}
	return result, nil
}
func (s *stubStore) SaveInvite(_ context.Context, invite Invite) (Invite, error) {
	rows := s.invites[invite.CallID]
	replaced := false
	for i := range rows {
		if rows[i].AccountID == invite.AccountID {
			rows[i] = cloneInvite(invite)
			replaced = true
			break
		}
	}
	if !replaced {
		rows = append(rows, cloneInvite(invite))
	}
	s.invites[invite.CallID] = rows
	return cloneInvite(invite), nil
}
func (s *stubStore) InviteByCallAndAccount(_ context.Context, callID string, accountID string) (Invite, error) {
	for _, invite := range s.invites[callID] {
		if invite.AccountID == accountID {
			return cloneInvite(invite), nil
		}
	}
	return Invite{}, ErrNotFound
}
func (s *stubStore) InvitesByCall(_ context.Context, callID string) ([]Invite, error) {
	return cloneInvites(s.invites[callID]), nil
}
func (s *stubStore) SaveParticipant(_ context.Context, participant Participant) (Participant, error) {
	rows := s.participants[participant.CallID]
	replaced := false
	for i := range rows {
		if rows[i].DeviceID == participant.DeviceID {
			rows[i] = cloneParticipant(participant)
			replaced = true
			break
		}
	}
	if !replaced {
		rows = append(rows, cloneParticipant(participant))
	}
	s.participants[participant.CallID] = rows
	return cloneParticipant(participant), nil
}
func (s *stubStore) ParticipantByCallAndDevice(_ context.Context, callID string, deviceID string) (Participant, error) {
	for _, participant := range s.participants[callID] {
		if participant.DeviceID == deviceID {
			return cloneParticipant(participant), nil
		}
	}
	return Participant{}, ErrNotFound
}
func (s *stubStore) ParticipantsByCall(_ context.Context, callID string) ([]Participant, error) {
	return cloneParticipants(s.participants[callID]), nil
}
func (s *stubStore) SaveEvent(_ context.Context, event Event) (Event, error) {
	s.nextSeq++
	event.Sequence = s.nextSeq
	s.events = append(s.events, cloneEvent(event))
	return cloneEvent(event), nil
}
func (s *stubStore) EventsAfterSequence(_ context.Context, fromSequence uint64, callID string, conversationID string, limit int) ([]Event, error) {
	result := make([]Event, 0)
	for _, event := range s.events {
		if event.Sequence <= fromSequence {
			continue
		}
		if callID != "" && event.CallID != callID {
			continue
		}
		if conversationID != "" && event.ConversationID != conversationID {
			continue
		}
		result = append(result, cloneEvent(event))
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

type stubConversations struct {
	conversation conversation.Conversation
	members      map[string]conversation.ConversationMember
}

func newStubConversations() *stubConversations {
	now := time.Date(2026, time.March, 27, 19, 0, 0, 0, time.UTC)
	return &stubConversations{
		conversation: conversation.Conversation{
			ID:             "conv-direct",
			Kind:           conversation.ConversationKindDirect,
			OwnerAccountID: "acc-a",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		members: map[string]conversation.ConversationMember{
			"acc-a": {
				ConversationID: "conv-direct",
				AccountID:      "acc-a",
				Role:           conversation.MemberRoleOwner,
				JoinedAt:       now,
			},
			"acc-b": {
				ConversationID: "conv-direct",
				AccountID:      "acc-b",
				Role:           conversation.MemberRoleMember,
				JoinedAt:       now,
			},
		},
	}
}

func (s *stubConversations) ConversationByID(context.Context, string) (conversation.Conversation, error) {
	return s.conversation, nil
}
func (s *stubConversations) ConversationMemberByConversationAndAccount(
	_ context.Context,
	_ string,
	accountID string,
) (conversation.ConversationMember, error) {
	member, ok := s.members[accountID]
	if !ok {
		return conversation.ConversationMember{}, conversation.ErrNotFound
	}
	return member, nil
}
func (s *stubConversations) ConversationMembersByConversationID(context.Context, string) ([]conversation.ConversationMember, error) {
	result := make([]conversation.ConversationMember, 0, len(s.members))
	for _, member := range s.members {
		result = append(result, member)
	}
	return result, nil
}

func TestListCallsReadPathAndAppendEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 20, 0, 0, 0, time.UTC)
	store := newStubStore()
	conversations := newStubConversations()
	store.calls["call-active"] = Call{
		ID:             "call-active",
		ConversationID: "conv-direct",
		State:          StateActive,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	store.calls["call-ended"] = Call{
		ID:             "call-ended",
		ConversationID: "conv-direct",
		State:          StateEnded,
		StartedAt:      now.Add(-time.Minute),
		EndedAt:        now,
		UpdatedAt:      now,
	}

	service, err := NewService(store, conversations, stubRuntime{}, WithNow(func() time.Time { return now }))
	require.NoError(t, err)

	rows, err := service.ListCalls(context.Background(), ListParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "call-active", rows[0].ID)

	rows, err = service.ListCalls(context.Background(), ListParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
		IncludeEnded:   true,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	event, err := service.appendEvent(context.Background(), store, store.calls["call-active"], EventTypeStarted, " acc-a ", " dev-a ", map[string]string{
		"  ":         "drop",
		"with_video": "true",
	}, time.Time{})
	require.NoError(t, err)
	require.NotEmpty(t, event.EventID)
	require.Equal(t, "acc-a", event.ActorAccountID)
	require.Equal(t, "dev-a", event.ActorDeviceID)
	require.Equal(t, "true", event.Metadata["with_video"])
}

func TestVisibleConversationAndCurrentTimeFallback(t *testing.T) {
	t.Parallel()

	conversations := newStubConversations()
	conversations.members["acc-b"] = conversation.ConversationMember{
		ConversationID: "conv-direct",
		AccountID:      "acc-b",
		Role:           conversation.MemberRoleMember,
		LeftAt:         time.Now().UTC(),
	}
	service, err := NewService(newStubStore(), conversations, stubRuntime{})
	require.NoError(t, err)

	_, _, err = service.visibleConversation(context.Background(), "conv-direct", "acc-b")
	require.ErrorIs(t, err, ErrForbidden)

	instant := (&Service{}).currentTime()
	require.False(t, instant.IsZero())
}

func TestLoadCallAndVisibleCallValidation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 21, 0, 0, 0, time.UTC)
	store := newStubStore()
	conversations := newStubConversations()
	service, err := NewService(store, conversations, stubRuntime{}, WithNow(func() time.Time { return now }))
	require.NoError(t, err)

	_, err = service.loadCall(context.Background(), store, "")
	require.ErrorIs(t, err, ErrInvalidInput)

	_, err = service.loadCall(context.Background(), store, "missing")
	require.ErrorIs(t, err, ErrNotFound)

	store.calls["call-1"] = Call{
		ID:             "call-1",
		ConversationID: "conv-direct",
		State:          StateActive,
		StartedAt:      now,
		UpdatedAt:      now,
	}

	_, err = service.visibleCall(context.Background(), store, "call-1", "acc-x")
	require.ErrorIs(t, err, ErrForbidden)

	callRow, err := service.visibleCall(context.Background(), store, "call-1", "acc-a")
	require.NoError(t, err)
	require.Equal(t, "call-1", callRow.ID)
}

type errorStore struct {
	*stubStore
	callsErr        error
	eventsErr       error
	invitesErr      error
	participantsErr error
}

func (s *errorStore) WithinTx(_ context.Context, fn func(Store) error) error { return fn(s) }

func (s *errorStore) CallsByConversation(ctx context.Context, conversationID string, includeEnded bool) ([]Call, error) {
	if s.callsErr != nil {
		return nil, s.callsErr
	}
	return s.stubStore.CallsByConversation(ctx, conversationID, includeEnded)
}

func (s *errorStore) EventsAfterSequence(
	ctx context.Context,
	fromSequence uint64,
	callID string,
	conversationID string,
	limit int,
) ([]Event, error) {
	if s.eventsErr != nil {
		return nil, s.eventsErr
	}
	return s.stubStore.EventsAfterSequence(ctx, fromSequence, callID, conversationID, limit)
}

func (s *errorStore) InvitesByCall(ctx context.Context, callID string) ([]Invite, error) {
	if s.invitesErr != nil {
		return nil, s.invitesErr
	}
	return s.stubStore.InvitesByCall(ctx, callID)
}

func (s *errorStore) ParticipantsByCall(ctx context.Context, callID string) ([]Participant, error) {
	if s.participantsErr != nil {
		return nil, s.participantsErr
	}
	return s.stubStore.ParticipantsByCall(ctx, callID)
}

type errorConversations struct {
	*stubConversations
	convErr   error
	memberErr error
}

func (s *errorConversations) ConversationByID(ctx context.Context, conversationID string) (conversation.Conversation, error) {
	if s.convErr != nil {
		return conversation.Conversation{}, s.convErr
	}
	return s.stubConversations.ConversationByID(ctx, conversationID)
}

func (s *errorConversations) ConversationMemberByConversationAndAccount(
	ctx context.Context,
	conversationID string,
	accountID string,
) (conversation.ConversationMember, error) {
	if s.memberErr != nil {
		return conversation.ConversationMember{}, s.memberErr
	}
	return s.stubConversations.ConversationMemberByConversationAndAccount(ctx, conversationID, accountID)
}

func TestListCallsAndEventsErrorBranches(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 22, 0, 0, 0, time.UTC)
	baseStore := newStubStore()
	baseStore.calls["call-1"] = Call{
		ID:             "call-1",
		ConversationID: "conv-direct",
		State:          StateActive,
		StartedAt:      now,
		UpdatedAt:      now,
	}
	store := &errorStore{stubStore: baseStore}
	conversations := newStubConversations()
	service, err := NewService(store, conversations, stubRuntime{}, WithNow(func() time.Time { return now }))
	require.NoError(t, err)

	_, err = service.ListCalls(context.Background(), ListParams{})
	require.ErrorIs(t, err, ErrInvalidInput)

	store.callsErr = errors.New("calls failed")
	_, err = service.ListCalls(context.Background(), ListParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
	})
	require.ErrorContains(t, err, "calls failed")
	store.callsErr = nil

	_, err = service.Events(context.Background(), EventParams{AccountID: "acc-a"})
	require.ErrorIs(t, err, ErrInvalidInput)

	store.eventsErr = errors.New("events failed")
	_, err = service.Events(context.Background(), EventParams{
		ConversationID: "conv-direct",
		AccountID:      "acc-a",
	})
	require.ErrorContains(t, err, "events failed")
}

func TestVisibleConversationErrorMapping(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 22, 30, 0, 0, time.UTC)
	store := newStubStore()
	base := newStubConversations()
	conversations := &errorConversations{stubConversations: base}
	service, err := NewService(store, conversations, stubRuntime{}, WithNow(func() time.Time { return now }))
	require.NoError(t, err)

	_, _, err = service.visibleConversation(context.Background(), "", "acc-a")
	require.ErrorIs(t, err, ErrInvalidInput)

	conversations.convErr = conversation.ErrNotFound
	_, _, err = service.visibleConversation(context.Background(), "conv-direct", "acc-a")
	require.ErrorIs(t, err, ErrNotFound)
	conversations.convErr = errors.New("conv failed")
	_, _, err = service.visibleConversation(context.Background(), "conv-direct", "acc-a")
	require.ErrorContains(t, err, "conv failed")

	conversations.convErr = nil
	conversations.memberErr = conversation.ErrNotFound
	_, _, err = service.visibleConversation(context.Background(), "conv-direct", "acc-a")
	require.ErrorIs(t, err, ErrForbidden)
	conversations.memberErr = errors.New("member failed")
	_, _, err = service.visibleConversation(context.Background(), "conv-direct", "acc-a")
	require.ErrorContains(t, err, "member failed")
}

func TestHydrateCallAndEndedSummaryBranches(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 27, 23, 0, 0, 0, time.UTC)
	store := &errorStore{stubStore: newStubStore()}
	store.calls["call-1"] = Call{
		ID:             "call-1",
		ConversationID: "conv-direct",
		State:          StateEnded,
		StartedAt:      now.Add(-time.Minute),
		EndedAt:        now,
		UpdatedAt:      now,
	}
	store.events = []Event{
		{
			CallID:    "call-1",
			EventType: EventTypeMediaUpdated,
			Sequence:  1,
			Metadata: map[string]string{
				"transport_quality":          "failed",
				"video_fallback_recommended": "true",
				"screen_share_priority":      "true",
				"reconnect_recommended":      "true",
				"suppress_camera_video":      "true",
				"suppress_outgoing_video":    "true",
				"suppress_incoming_video":    "true",
				"suppress_outgoing_audio":    "true",
				"suppress_incoming_audio":    "true",
				"reconnect_attempt":          "2",
				callMetadataTargetAccountID:  "acc-a",
				callMetadataTargetDeviceID:   "dev-a",
			},
			CreatedAt: now,
		},
	}
	service, err := NewService(store, newStubConversations(), stubRuntime{}, WithNow(func() time.Time { return now }))
	require.NoError(t, err)

	callRow, err := service.hydrateCall(context.Background(), store, "call-1")
	require.NoError(t, err)
	require.Equal(t, "failed", callRow.QualitySummary.WorstQuality)
	require.Equal(t, uint32(1), callRow.QualitySummary.VideoFallbackParticipants)
	require.Equal(t, uint32(1), callRow.QualitySummary.ScreenSharePriorityParticipants)
	require.Equal(t, uint32(1), callRow.QualitySummary.ReconnectParticipants)
	require.Equal(t, uint32(1), callRow.QualitySummary.CameraVideoSuppressed)
	require.Equal(t, uint32(1), callRow.QualitySummary.OutgoingVideoSuppressed)
	require.Equal(t, uint32(1), callRow.QualitySummary.IncomingVideoSuppressed)
	require.Equal(t, uint32(1), callRow.QualitySummary.OutgoingAudioSuppressed)
	require.Equal(t, uint32(1), callRow.QualitySummary.IncomingAudioSuppressed)

	store.invitesErr = errors.New("invites failed")
	_, err = service.hydrateCall(context.Background(), store, "call-1")
	require.ErrorContains(t, err, "invites failed")
	store.invitesErr = nil
	store.participantsErr = errors.New("participants failed")
	_, err = service.hydrateCall(context.Background(), store, "call-1")
	require.ErrorContains(t, err, "participants failed")
}
