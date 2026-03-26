package gateway

import (
	"testing"
	"time"

	callv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/call/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
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

	updated, err := fixture.api.UpdateCallMediaState(peerCtx, &callv1.UpdateCallMediaStateRequest{
		CallId: started.Call.CallId,
		MediaState: &callv1.CallMediaState{
			AudioMuted:    true,
			VideoMuted:    false,
			CameraEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("update media state: %v", err)
	}
	if updated.Participant == nil || !updated.Participant.MediaState.AudioMuted {
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
