package gateway

import (
	"testing"
	"time"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	callv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/call/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/pion/webrtc/v4"
)

func TestCallLifecycleRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "call-owner", "call-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "call-peer", "call-peer@example.com")

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	started, err := fixture.api.StartCall(ownerCtx, &callv1.StartCallRequest{
		ConversationId: created.Conversation.ConversationId,
		WithVideo:      true,
	})
	if err != nil {
		t.Fatalf("start call: %v", err)
	}
	if started.Call.State != callv1.CallState_CALL_STATE_RINGING || started.Call.ConversationId != created.Conversation.ConversationId || owner.ID == "" {
		t.Fatalf("unexpected started call: %+v", started.Call)
	}

	accepted, err := fixture.api.AcceptCall(peerCtx, &callv1.AcceptCallRequest{CallId: started.Call.CallId})
	if err != nil {
		t.Fatalf("accept call: %v", err)
	}
	if accepted.Call.State != callv1.CallState_CALL_STATE_ACTIVE {
		t.Fatalf("unexpected accepted call: %+v", accepted.Call)
	}

	joined, err := fixture.api.JoinCall(peerCtx, &callv1.JoinCallRequest{
		CallId:    started.Call.CallId,
		WithVideo: true,
	})
	if err != nil {
		t.Fatalf("join call: %v", err)
	}
	if joined.Transport == nil || joined.Transport.SessionId == "" || joined.Transport.RuntimeEndpoint == "" {
		t.Fatalf("unexpected join transport: %+v", joined.Transport)
	}
	if joined.Transport.CandidateHost == "" || joined.Transport.CandidatePort == 0 || joined.Transport.IceUfrag == "" || joined.Transport.IcePwd == "" || joined.Transport.DtlsFingerprint == "" {
		t.Fatalf("unexpected media-plane transport details: %+v", joined.Transport)
	}

	updated, err := fixture.api.UpdateCallMediaState(peerCtx, &callv1.UpdateCallMediaStateRequest{
		CallId: started.Call.CallId,
		MediaState: &callv1.CallMediaState{
			AudioMuted:         true,
			VideoMuted:         false,
			CameraEnabled:      true,
			ScreenShareEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("update media state: %v", err)
	}
	if updated.Participant == nil || !updated.Participant.MediaState.AudioMuted || !updated.Participant.MediaState.ScreenShareEnabled {
		t.Fatalf("unexpected participant media state: %+v", updated.Participant)
	}

	ice, err := fixture.api.GetIceConfig(peerCtx, &callv1.GetIceConfigRequest{CallId: started.Call.CallId})
	if err != nil {
		t.Fatalf("get ice config: %v", err)
	}
	if ice.RuntimeEndpoint == "" {
		t.Fatalf("expected runtime endpoint in ice config")
	}

	ended, err := fixture.api.EndCall(ownerCtx, &callv1.EndCallRequest{
		CallId: started.Call.CallId,
		Reason: callv1.CallEndReason_CALL_END_REASON_ENDED,
	})
	if err != nil {
		t.Fatalf("end call: %v", err)
	}
	if ended.Call.State != callv1.CallState_CALL_STATE_ENDED {
		t.Fatalf("unexpected ended call: %+v", ended.Call)
	}
}

func TestCallHandoffRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "call-handoff-owner", "call-handoff-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "call-handoff-peer", "call-handoff-peer@example.com")
	peerSecondCtx := fixture.mustLoginAccountOnNewDevice(t, peer, "peer-second-device")

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	started, err := fixture.api.StartCall(ownerCtx, &callv1.StartCallRequest{
		ConversationId: created.Conversation.ConversationId,
		WithVideo:      true,
	})
	if err != nil {
		t.Fatalf("start call: %v", err)
	}
	if _, err := fixture.api.AcceptCall(peerCtx, &callv1.AcceptCallRequest{CallId: started.Call.CallId}); err != nil {
		t.Fatalf("accept call: %v", err)
	}
	if _, err := fixture.api.JoinCall(peerCtx, &callv1.JoinCallRequest{
		CallId:    started.Call.CallId,
		WithVideo: true,
	}); err != nil {
		t.Fatalf("join call: %v", err)
	}
	if _, err := fixture.api.UpdateCallMediaState(peerCtx, &callv1.UpdateCallMediaStateRequest{
		CallId: started.Call.CallId,
		MediaState: &callv1.CallMediaState{
			AudioMuted:         true,
			VideoMuted:         false,
			CameraEnabled:      true,
			ScreenShareEnabled: true,
		},
	}); err != nil {
		t.Fatalf("update media state: %v", err)
	}

	devices, err := fixture.api.ListDevices(peerCtx, &authv1.ListDevicesRequest{})
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices.Devices) < 2 {
		t.Fatalf("expected at least two devices, got %+v", devices.Devices)
	}
	var oldDeviceID string
	for _, device := range devices.Devices {
		if device.DeviceName == "test-device" {
			oldDeviceID = device.DeviceId
			break
		}
	}
	if oldDeviceID == "" {
		t.Fatalf("expected old device in %+v", devices.Devices)
	}

	handoff, err := fixture.api.HandoffCall(peerSecondCtx, &callv1.HandoffCallRequest{
		CallId:       started.Call.CallId,
		FromDeviceId: oldDeviceID,
	})
	if err != nil {
		t.Fatalf("handoff call: %v", err)
	}
	if handoff.Transport == nil || handoff.Transport.SessionId == "" {
		t.Fatalf("unexpected handoff transport: %+v", handoff.Transport)
	}
	if handoff.Participant == nil || !handoff.Participant.MediaState.ScreenShareEnabled || !handoff.Participant.MediaState.AudioMuted {
		t.Fatalf("unexpected handoff participant: %+v", handoff.Participant)
	}
	if owner.ID == "" {
		t.Fatal("expected owner account")
	}
}

func TestGroupCallScreenShareRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, ownerCtx := fixture.mustCreateUserAndLogin(t, "group-call-owner", "group-call-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "group-call-peer", "group-call-peer@example.com")
	extra, _ := fixture.mustCreateUserAndLogin(t, "group-call-extra", "group-call-extra@example.com")

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Group Call",
		MemberUserIds: []string{peer.ID, extra.ID},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	started, err := fixture.api.StartCall(ownerCtx, &callv1.StartCallRequest{
		ConversationId: created.Conversation.ConversationId,
		WithVideo:      true,
	})
	if err != nil {
		t.Fatalf("start group call: %v", err)
	}
	if started.Call.State != callv1.CallState_CALL_STATE_ACTIVE {
		t.Fatalf("expected active group call, got %+v", started.Call)
	}

	joined, err := fixture.api.JoinCall(peerCtx, &callv1.JoinCallRequest{
		CallId:    started.Call.CallId,
		WithVideo: true,
	})
	if err != nil {
		t.Fatalf("join group call: %v", err)
	}
	if joined.Transport == nil || joined.Transport.SessionId == "" {
		t.Fatalf("unexpected join transport: %+v", joined.Transport)
	}

	updated, err := fixture.api.UpdateCallMediaState(peerCtx, &callv1.UpdateCallMediaStateRequest{
		CallId: started.Call.CallId,
		MediaState: &callv1.CallMediaState{
			AudioMuted:         false,
			VideoMuted:         false,
			CameraEnabled:      true,
			ScreenShareEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("update group call media state: %v", err)
	}
	if updated.Participant == nil || !updated.Participant.MediaState.ScreenShareEnabled {
		t.Fatalf("unexpected group call participant media state: %+v", updated.Participant)
	}
}

func TestGroupCallModerationRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, ownerCtx := fixture.mustCreateUserAndLogin(t, "group-mod-owner", "group-mod-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "group-mod-peer", "group-mod-peer@example.com")
	extra, extraCtx := fixture.mustCreateUserAndLogin(t, "group-mod-extra", "group-mod-extra@example.com")

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Moderated Group Call",
		MemberUserIds: []string{peer.ID, extra.ID},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	started, err := fixture.api.StartCall(ownerCtx, &callv1.StartCallRequest{
		ConversationId: created.Conversation.ConversationId,
		WithVideo:      true,
	})
	if err != nil {
		t.Fatalf("start group call: %v", err)
	}
	if _, err := fixture.api.JoinCall(peerCtx, &callv1.JoinCallRequest{
		CallId:    started.Call.CallId,
		WithVideo: true,
	}); err != nil {
		t.Fatalf("join peer: %v", err)
	}
	if _, err := fixture.api.JoinCall(extraCtx, &callv1.JoinCallRequest{
		CallId:    started.Call.CallId,
		WithVideo: true,
	}); err != nil {
		t.Fatalf("join extra: %v", err)
	}

	raised, err := fixture.api.RaiseCallHand(peerCtx, &callv1.RaiseCallHandRequest{
		CallId: started.Call.CallId,
		Raised: true,
	})
	if err != nil {
		t.Fatalf("raise hand: %v", err)
	}
	if raised.Participant == nil || !raised.Participant.HandRaised {
		t.Fatalf("expected raised hand participant, got %+v", raised.Participant)
	}

	moderated, err := fixture.api.ModerateCallParticipant(ownerCtx, &callv1.ModerateCallParticipantRequest{
		CallId:         started.Call.CallId,
		TargetDeviceId: raised.Participant.DeviceId,
		HostMutedAudio: true,
		HostMutedVideo: true,
		LowerHand:      true,
	})
	if err != nil {
		t.Fatalf("moderate participant: %v", err)
	}
	if moderated.Participant == nil || !moderated.Participant.HostMutedAudio || !moderated.Participant.HostMutedVideo || moderated.Participant.HandRaised {
		t.Fatalf("unexpected moderated participant: %+v", moderated.Participant)
	}

	if _, err := fixture.api.ModerateCallParticipant(peerCtx, &callv1.ModerateCallParticipantRequest{
		CallId:         started.Call.CallId,
		TargetDeviceId: moderated.Participant.DeviceId,
		HostMutedAudio: true,
	}); err == nil {
		t.Fatal("expected member moderation to fail")
	}

	raisedHands, err := fixture.api.ListRaisedHands(ownerCtx, &callv1.ListRaisedHandsRequest{
		CallId: started.Call.CallId,
	})
	if err != nil {
		t.Fatalf("list raised hands: %v", err)
	}
	if len(raisedHands.Participants) != 0 {
		t.Fatalf("expected lowered hands after moderation, got %+v", raisedHands.Participants)
	}

	mutedAll, err := fixture.api.MuteAllCallParticipants(ownerCtx, &callv1.MuteAllCallParticipantsRequest{
		CallId:     started.Call.CallId,
		MuteAudio:  true,
		MuteVideo:  true,
		LowerHands: true,
	})
	if err != nil {
		t.Fatalf("mute all participants: %v", err)
	}
	if len(mutedAll.Participants) != 2 {
		t.Fatalf("unexpected muted participants: %+v", mutedAll.Participants)
	}
	var removedDeviceID string
	for _, participant := range mutedAll.Participants {
		if participant.UserId == extra.ID {
			removedDeviceID = participant.DeviceId
			break
		}
	}
	if removedDeviceID == "" {
		t.Fatalf("expected extra participant in muted set: %+v", mutedAll.Participants)
	}

	transferred, err := fixture.api.TransferCallHost(ownerCtx, &callv1.TransferCallHostRequest{
		CallId:       started.Call.CallId,
		TargetUserId: peer.ID,
	})
	if err != nil {
		t.Fatalf("transfer call host: %v", err)
	}
	if transferred.Call == nil || transferred.Call.HostUserId != peer.ID {
		t.Fatalf("unexpected transferred host: %+v", transferred.Call)
	}

	removed, err := fixture.api.RemoveCallParticipant(peerCtx, &callv1.RemoveCallParticipantRequest{
		CallId:         started.Call.CallId,
		TargetDeviceId: removedDeviceID,
	})
	if err != nil {
		t.Fatalf("remove participant: %v", err)
	}
	if removed.Participant == nil || removed.Participant.State != callv1.CallParticipantState_CALL_PARTICIPANT_STATE_LEFT {
		t.Fatalf("unexpected removed participant: %+v", removed.Participant)
	}
}

func TestGroupCallStageModeRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, ownerCtx := fixture.mustCreateUserAndLogin(t, "group-stage-owner", "group-stage-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "group-stage-peer", "group-stage-peer@example.com")
	extra, extraCtx := fixture.mustCreateUserAndLogin(t, "group-stage-extra", "group-stage-extra@example.com")

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Stage Group Call",
		MemberUserIds: []string{peer.ID, extra.ID},
	})
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	started, err := fixture.api.StartCall(ownerCtx, &callv1.StartCallRequest{
		ConversationId: created.Conversation.ConversationId,
		WithVideo:      true,
	})
	if err != nil {
		t.Fatalf("start group call: %v", err)
	}
	if _, err := fixture.api.JoinCall(peerCtx, &callv1.JoinCallRequest{
		CallId:    started.Call.CallId,
		WithVideo: true,
	}); err != nil {
		t.Fatalf("join peer: %v", err)
	}
	if _, err := fixture.api.JoinCall(extraCtx, &callv1.JoinCallRequest{
		CallId:    started.Call.CallId,
		WithVideo: true,
	}); err != nil {
		t.Fatalf("join extra: %v", err)
	}

	stage, err := fixture.api.UpdateCallStageMode(ownerCtx, &callv1.UpdateCallStageModeRequest{
		CallId:  started.Call.CallId,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("update stage mode: %v", err)
	}
	if stage.Call == nil || !stage.Call.StageModeEnabled {
		t.Fatalf("unexpected stage mode call: %+v", stage.Call)
	}

	current, err := fixture.api.GetCall(ownerCtx, &callv1.GetCallRequest{CallId: started.Call.CallId})
	if err != nil {
		t.Fatalf("get call: %v", err)
	}
	peerDeviceID := ""
	for _, participant := range current.Call.Participants {
		if participant.UserId == peer.ID {
			peerDeviceID = participant.DeviceId
			break
		}
	}
	if peerDeviceID == "" {
		t.Fatalf("expected joined peer participant in %+v", current.Call.Participants)
	}

	pinned, err := fixture.api.PinCallSpeaker(ownerCtx, &callv1.PinCallSpeakerRequest{
		CallId:         started.Call.CallId,
		TargetDeviceId: peerDeviceID,
		Pinned:         true,
	})
	if err != nil {
		t.Fatalf("pin speaker: %v", err)
	}
	if pinned.Call == nil || pinned.Call.PinnedSpeakerUserId != peer.ID || pinned.Call.PinnedSpeakerDeviceId != peerDeviceID {
		t.Fatalf("unexpected pinned call: %+v", pinned.Call)
	}
	if pinned.Participant == nil || !pinned.Participant.PinnedSpeaker || !pinned.Participant.StageSlot {
		t.Fatalf("unexpected pinned participant: %+v", pinned.Participant)
	}
}

func TestGetCallDiagnosticsRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "call-diagnostics-owner", "call-diagnostics-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "call-diagnostics-peer", "call-diagnostics-peer@example.com")

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	started, err := fixture.api.StartCall(ownerCtx, &callv1.StartCallRequest{
		ConversationId: created.Conversation.ConversationId,
		WithVideo:      true,
	})
	if err != nil {
		t.Fatalf("start call: %v", err)
	}
	if _, err := fixture.api.AcceptCall(peerCtx, &callv1.AcceptCallRequest{CallId: started.Call.CallId}); err != nil {
		t.Fatalf("accept call: %v", err)
	}

	report, err := fixture.api.GetCallDiagnostics(ownerCtx, &callv1.GetCallDiagnosticsRequest{
		CallId: started.Call.CallId,
	})
	if err != nil {
		t.Fatalf("get call diagnostics: %v", err)
	}
	if report.Diagnostics == nil || report.Diagnostics.Call == nil {
		t.Fatalf("expected diagnostics report")
	}
	if report.Diagnostics.Call.CallId != started.Call.CallId || report.Diagnostics.Call.InitiatorUserId != owner.ID {
		t.Fatalf("unexpected diagnostics payload: %+v", report.Diagnostics)
	}
}

func TestSubscribeCallEventsStreamsNewEvents(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, ownerCtx := fixture.mustCreateUserAndLogin(t, "call-stream-owner", "call-stream-owner@example.com")
	peer, _ := fixture.mustCreateUserAndLogin(t, "call-stream-peer", "call-stream-peer@example.com")

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	stream := newTestSubscribeCallEventsStream(ownerCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeCallEvents(&callv1.SubscribeCallEventsRequest{
			ConversationId: created.Conversation.ConversationId,
		}, stream)
	}()

	time.Sleep(20 * time.Millisecond)

	started, err := fixture.api.StartCall(ownerCtx, &callv1.StartCallRequest{
		ConversationId: created.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("start call: %v", err)
	}

	select {
	case response := <-stream.responses:
		if response.GetEvent().GetCallId() != started.Call.CallId {
			t.Fatalf("unexpected streamed event: %+v", response.GetEvent())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for streamed call event")
	}

	stream.cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe call events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribe call loop to stop")
	}
}

func TestCallSignalingRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "call-signal-owner", "call-signal-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "call-signal-peer", "call-signal-peer@example.com")

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	started, err := fixture.api.StartCall(ownerCtx, &callv1.StartCallRequest{
		ConversationId: created.Conversation.ConversationId,
		WithVideo:      true,
	})
	if err != nil {
		t.Fatalf("start call: %v", err)
	}
	if _, err := fixture.api.AcceptCall(peerCtx, &callv1.AcceptCallRequest{CallId: started.Call.CallId}); err != nil {
		t.Fatalf("accept call: %v", err)
	}

	joined, err := fixture.api.JoinCall(peerCtx, &callv1.JoinCallRequest{
		CallId:    started.Call.CallId,
		WithVideo: true,
	})
	if err != nil {
		t.Fatalf("join call: %v", err)
	}

	stream := newTestSubscribeCallEventsStream(ownerCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeCallEvents(&callv1.SubscribeCallEventsRequest{
			CallId: started.Call.CallId,
		}, stream)
	}()

	time.Sleep(20 * time.Millisecond)

	offer := mustCreateGatewayCallOffer(t)

	description, err := fixture.api.PublishCallDescription(peerCtx, &callv1.PublishCallDescriptionRequest{
		CallId:    started.Call.CallId,
		SessionId: joined.Transport.SessionId,
		Description: &callv1.SessionDescription{
			Type: "offer",
			Sdp:  offer,
		},
	})
	if err != nil {
		t.Fatalf("publish description: %v", err)
	}
	if description.Event == nil || description.Event.EventType != callv1.CallEventType_CALL_EVENT_TYPE_SIGNAL_DESCRIPTION {
		t.Fatalf("unexpected description event: %+v", description.Event)
	}

	candidate, err := fixture.api.PublishCallIceCandidate(peerCtx, &callv1.PublishCallIceCandidateRequest{
		CallId:    started.Call.CallId,
		SessionId: joined.Transport.SessionId,
		IceCandidate: &callv1.IceCandidate{
			Candidate:        "candidate:1 1 udp 2130706431 127.0.0.1 41000 typ host",
			SdpMid:           "0",
			SdpMlineIndex:    0,
			UsernameFragment: joined.Transport.IceUfrag,
		},
	})
	if err != nil {
		t.Fatalf("publish candidate: %v", err)
	}
	if candidate.Event == nil || candidate.Event.EventType != callv1.CallEventType_CALL_EVENT_TYPE_SIGNAL_CANDIDATE {
		t.Fatalf("unexpected candidate event: %+v", candidate.Event)
	}

	var sawDescription bool
	var sawCandidate bool
	timeout := time.After(time.Second)
	for !(sawDescription && sawCandidate) {
		select {
		case response := <-stream.responses:
			event := response.GetEvent()
			if event == nil {
				t.Fatal("expected streamed call event")
			}
			switch event.GetEventType() {
			case callv1.CallEventType_CALL_EVENT_TYPE_SIGNAL_DESCRIPTION:
				sawDescription = true
				if event.GetSessionId() != joined.Transport.SessionId {
					t.Fatalf("unexpected streamed description event: %+v", event)
				}
				if event.GetDescription().GetType() != "offer" && event.GetDescription().GetType() != "answer" {
					t.Fatalf("unexpected streamed description type: %+v", event)
				}
			case callv1.CallEventType_CALL_EVENT_TYPE_SIGNAL_CANDIDATE:
				sawCandidate = true
				if event.GetIceCandidate().GetCandidate() == "" {
					t.Fatalf("unexpected streamed candidate event: %+v", event)
				}
			}
		case <-timeout:
			t.Fatal("timed out waiting for streamed signaling events")
		}
	}

	stream.cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe call events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribe call loop to stop")
	}

	if owner.ID == "" {
		t.Fatal("expected owner account")
	}
}

func TestSubscribeCallEventsFiltersTargetedSignals(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, ownerCtx := fixture.mustCreateUserAndLogin(t, "call-target-owner", "call-target-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "call-target-peer", "call-target-peer@example.com")

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	started, err := fixture.api.StartCall(ownerCtx, &callv1.StartCallRequest{
		ConversationId: created.Conversation.ConversationId,
		WithVideo:      true,
	})
	if err != nil {
		t.Fatalf("start call: %v", err)
	}
	if _, err := fixture.api.AcceptCall(peerCtx, &callv1.AcceptCallRequest{CallId: started.Call.CallId}); err != nil {
		t.Fatalf("accept call: %v", err)
	}
	joined, err := fixture.api.JoinCall(peerCtx, &callv1.JoinCallRequest{
		CallId:    started.Call.CallId,
		WithVideo: true,
	})
	if err != nil {
		t.Fatalf("join call: %v", err)
	}

	stream := newTestSubscribeCallEventsStream(ownerCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeCallEvents(&callv1.SubscribeCallEventsRequest{
			CallId: started.Call.CallId,
		}, stream)
	}()
	time.Sleep(20 * time.Millisecond)

	offer := mustCreateGatewayCallOffer(t)
	if _, err := fixture.api.PublishCallDescription(peerCtx, &callv1.PublishCallDescriptionRequest{
		CallId:    started.Call.CallId,
		SessionId: joined.Transport.SessionId,
		Description: &callv1.SessionDescription{
			Type: "offer",
			Sdp:  offer,
		},
	}); err != nil {
		t.Fatalf("publish description: %v", err)
	}

	timeout := time.After(300 * time.Millisecond)
	for {
		select {
		case response := <-stream.responses:
			event := response.GetEvent()
			if event == nil {
				continue
			}
			if event.GetEventType() == callv1.CallEventType_CALL_EVENT_TYPE_SIGNAL_DESCRIPTION &&
				event.GetDescription().GetType() == "answer" {
				t.Fatalf("owner stream must not receive peer-targeted answer: %+v", event)
			}
		case <-timeout:
			stream.cancel()
			select {
			case err := <-errCh:
				if err != nil {
					t.Fatalf("subscribe call events returned error: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for subscribe call loop to stop")
			}
			return
		}
	}
}

func TestSubscribeCallStatsStreamsDedicatedSnapshots(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, ownerCtx := fixture.mustCreateUserAndLogin(t, "call-stats-owner", "call-stats-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "call-stats-peer", "call-stats-peer@example.com")

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	started, err := fixture.api.StartCall(ownerCtx, &callv1.StartCallRequest{
		ConversationId: created.Conversation.ConversationId,
		WithVideo:      true,
	})
	if err != nil {
		t.Fatalf("start call: %v", err)
	}
	if _, err := fixture.api.AcceptCall(peerCtx, &callv1.AcceptCallRequest{CallId: started.Call.CallId}); err != nil {
		t.Fatalf("accept call: %v", err)
	}
	if _, err := fixture.api.JoinCall(peerCtx, &callv1.JoinCallRequest{
		CallId:    started.Call.CallId,
		WithVideo: true,
	}); err != nil {
		t.Fatalf("join call: %v", err)
	}

	stream := newTestSubscribeCallStatsStream(ownerCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeCallStats(&callv1.SubscribeCallStatsRequest{
			CallId:     started.Call.CallId,
			IntervalMs: 100,
		}, stream)
	}()

	select {
	case response := <-stream.responses:
		snapshot := response.GetSnapshot()
		if snapshot == nil || snapshot.GetCall() == nil {
			t.Fatalf("expected initial stats snapshot, got %+v", response)
		}
		if snapshot.GetCall().GetCallId() != started.Call.CallId {
			t.Fatalf("unexpected initial snapshot: %+v", snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial stats snapshot")
	}

	if _, err := fixture.api.UpdateCallMediaState(peerCtx, &callv1.UpdateCallMediaStateRequest{
		CallId: started.Call.CallId,
		MediaState: &callv1.CallMediaState{
			AudioMuted:         true,
			VideoMuted:         false,
			CameraEnabled:      true,
			ScreenShareEnabled: true,
		},
	}); err != nil {
		t.Fatalf("update media state: %v", err)
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case response := <-stream.responses:
			snapshot := response.GetSnapshot()
			if snapshot == nil || snapshot.GetCall() == nil {
				continue
			}
			for _, participant := range snapshot.GetCall().GetParticipants() {
				if participant.GetUserId() != peer.ID {
					continue
				}
				if !participant.GetMediaState().GetScreenShareEnabled() || !participant.GetMediaState().GetAudioMuted() {
					continue
				}
				stream.cancel()
				select {
				case err := <-errCh:
					if err != nil {
						t.Fatalf("subscribe call stats returned error: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for subscribe call stats loop to stop")
				}
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for updated call stats snapshot")
		}
	}
}

func TestCallEventProtoCarriesScreenShareAdaptationMetadata(t *testing.T) {
	t.Parallel()

	event := callEventProto(domaincall.Event{
		EventID:        "evt-1",
		CallID:         "call-1",
		ConversationID: "conv-1",
		EventType:      domaincall.EventTypeMediaUpdated,
		Sequence:       7,
		Metadata: map[string]string{
			"recommended_profile":   "screen_share_only",
			"screen_share_priority": "true",
			"suppress_camera_video": "true",
		},
		CreatedAt: time.Date(2026, time.March, 27, 18, 0, 0, 0, time.UTC),
	})

	if event.GetMetadata()["recommended_profile"] != "screen_share_only" {
		t.Fatalf("expected recommended_profile to survive conversion, got %+v", event.GetMetadata())
	}
	if event.GetMetadata()["screen_share_priority"] != "true" {
		t.Fatalf("expected screen_share_priority to survive conversion, got %+v", event.GetMetadata())
	}
	if event.GetMetadata()["suppress_camera_video"] != "true" {
		t.Fatalf("expected suppress_camera_video to survive conversion, got %+v", event.GetMetadata())
	}
}

func mustCreateGatewayCallOffer(t *testing.T) string {
	t.Helper()

	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("new peer connection: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Fatalf("close peer connection: %v", err)
		}
	}()

	if _, err := client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		t.Fatalf("add transceiver: %v", err)
	}

	offer, err := client.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	if err := client.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local description: %v", err)
	}
	if client.LocalDescription() == nil || client.LocalDescription().SDP == "" {
		t.Fatal("expected local description sdp")
	}

	return client.LocalDescription().SDP
}
