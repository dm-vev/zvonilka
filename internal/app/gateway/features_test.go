package gateway

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	callv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/call/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	mediav1 "github.com/dm-vev/zvonilka/gen/proto/contracts/media/v1"
	notificationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/notification/v1"
	syncv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/sync/v1"
	usersv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/users/v1"
	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	calltest "github.com/dm-vev/zvonilka/internal/domain/call/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/media"
	"github.com/dm-vev/zvonilka/internal/domain/notification"
	notificationtest "github.com/dm-vev/zvonilka/internal/domain/notification/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
	presencetest "github.com/dm-vev/zvonilka/internal/domain/presence/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/search"
	searchtest "github.com/dm-vev/zvonilka/internal/domain/search/teststore"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	domaintranslation "github.com/dm-vev/zvonilka/internal/domain/translation"
	translationtest "github.com/dm-vev/zvonilka/internal/domain/translation/teststore"
	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
	usertest "github.com/dm-vev/zvonilka/internal/domain/user/teststore"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	platformrtc "github.com/dm-vev/zvonilka/internal/platform/rtc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCreateThreadAcceptsRootMessageID(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	account, authCtx := fixture.mustCreateUserAndLogin(t, "threads-owner", "threads-owner@example.com")
	created, err := fixture.api.CreateConversation(authCtx, &conversationv1.CreateConversationRequest{
		Kind:  commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title: "Threads",
		Settings: &conversationv1.ConversationSettings{
			AllowThreads: true,
		},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	sent, err := fixture.api.SendMessage(authCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft:          testMessageDraft("root"),
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	thread, err := fixture.api.CreateThread(authCtx, &conversationv1.CreateThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		RootMessageId:  sent.Message.MessageId,
		Title:          "Announcements",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if thread.Thread.RootMessageId != sent.Message.MessageId {
		t.Fatalf("expected root message id %s, got %s", sent.Message.MessageId, thread.Thread.RootMessageId)
	}

	loaded, err := fixture.api.GetThread(authCtx, &conversationv1.GetThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		ThreadId:       thread.Thread.ThreadId,
	})
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if loaded.Thread.RootMessageId != sent.Message.MessageId {
		t.Fatalf("expected persisted root message id %s, got %s", sent.Message.MessageId, loaded.Thread.RootMessageId)
	}

	if loaded.Thread.ConversationId != created.Conversation.ConversationId || account.ID == "" {
		t.Fatalf("unexpected loaded thread: %+v", loaded.Thread)
	}
}

func TestThreadLifecycleRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, authCtx := fixture.mustCreateUserAndLogin(t, "thread-lifecycle-owner", "thread-lifecycle-owner@example.com")
	created, err := fixture.api.CreateConversation(authCtx, &conversationv1.CreateConversationRequest{
		Kind:  commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title: "Thread Lifecycle",
		Settings: &conversationv1.ConversationSettings{
			AllowThreads: true,
		},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	thread, err := fixture.api.CreateThread(authCtx, &conversationv1.CreateThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		Title:          "Announcements",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	renamed, err := fixture.api.RenameThread(authCtx, &conversationv1.RenameThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		ThreadId:       thread.Thread.ThreadId,
		Title:          "Town Hall",
	})
	if err != nil {
		t.Fatalf("rename thread: %v", err)
	}
	if renamed.Thread.Title != "Town Hall" {
		t.Fatalf("expected renamed thread title, got %+v", renamed.Thread)
	}

	listed, err := fixture.api.ListThreads(authCtx, &conversationv1.ListThreadsRequest{
		ConversationId: created.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}

	found := false
	for _, listedThread := range listed.Threads {
		if listedThread.GetThreadId() != thread.Thread.ThreadId {
			continue
		}
		found = true
		if listedThread.GetTitle() != "Town Hall" {
			t.Fatalf("unexpected listed thread: %+v", listedThread)
		}
	}
	if !found {
		t.Fatalf("thread %s missing from list response", thread.Thread.ThreadId)
	}

	archived, err := fixture.api.ArchiveThread(authCtx, &conversationv1.ArchiveThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		ThreadId:       thread.Thread.ThreadId,
		Archived:       true,
	})
	if err != nil {
		t.Fatalf("archive thread: %v", err)
	}
	if !archived.Thread.Archived || archived.Thread.ArchivedAt == nil {
		t.Fatalf("expected archived thread state, got %+v", archived.Thread)
	}

	closed, err := fixture.api.CloseThread(authCtx, &conversationv1.CloseThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		ThreadId:       thread.Thread.ThreadId,
		Closed:         true,
	})
	if err != nil {
		t.Fatalf("close thread: %v", err)
	}
	if !closed.Thread.Closed || closed.Thread.ClosedAt == nil {
		t.Fatalf("expected closed thread state, got %+v", closed.Thread)
	}

	pinned, err := fixture.api.PinThread(authCtx, &conversationv1.PinThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		ThreadId:       thread.Thread.ThreadId,
		Pinned:         true,
	})
	if err != nil {
		t.Fatalf("pin thread: %v", err)
	}
	if !pinned.Thread.Pinned || pinned.Thread.LastSequence == 0 {
		t.Fatalf("expected pinned thread with sequence, got %+v", pinned.Thread)
	}

	loaded, err := fixture.api.GetThread(authCtx, &conversationv1.GetThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		ThreadId:       thread.Thread.ThreadId,
	})
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if loaded.Thread.Title != "Town Hall" || !loaded.Thread.Archived || !loaded.Thread.Closed || !loaded.Thread.Pinned {
		t.Fatalf("unexpected persisted thread state: %+v", loaded.Thread)
	}

	hidden, err := fixture.api.ListThreads(authCtx, &conversationv1.ListThreadsRequest{
		ConversationId: created.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("list threads after archive and close: %v", err)
	}
	for _, listedThread := range hidden.Threads {
		if listedThread.GetThreadId() == thread.Thread.ThreadId {
			t.Fatalf("archived and closed thread must be hidden by default: %+v", listedThread)
		}
	}

	visible, err := fixture.api.ListThreads(authCtx, &conversationv1.ListThreadsRequest{
		ConversationId:  created.Conversation.ConversationId,
		IncludeArchived: true,
		IncludeClosed:   true,
	})
	if err != nil {
		t.Fatalf("list threads with archived and closed included: %v", err)
	}

	found = false
	for _, listedThread := range visible.Threads {
		if listedThread.GetThreadId() != thread.Thread.ThreadId {
			continue
		}
		found = true
		if !listedThread.GetArchived() || !listedThread.GetClosed() || !listedThread.GetPinned() {
			t.Fatalf("unexpected listed archived thread: %+v", listedThread)
		}
	}
	if !found {
		t.Fatalf("thread %s missing from inclusive list response", thread.Thread.ThreadId)
	}
}

func TestModerationRPCFlow(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "moderation-rpc-owner", "moderation-rpc-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "moderation-rpc-peer", "moderation-rpc-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Moderation RPC",
		MemberUserIds: []string{peer.ID},
		Settings: &conversationv1.ConversationSettings{
			AllowThreads: true,
		},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	thread, err := fixture.api.CreateThread(ownerCtx, &conversationv1.CreateThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		Title:          "Reports",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	target := &conversationv1.ModerationTarget{
		Kind:           conversationv1.ModerationTargetKind_MODERATION_TARGET_KIND_THREAD,
		ConversationId: created.Conversation.ConversationId,
		ThreadId:       thread.Thread.ThreadId,
	}

	setPolicy, err := fixture.api.SetModerationPolicy(ownerCtx, &conversationv1.SetModerationPolicyRequest{
		Policy: &conversationv1.ModerationPolicy{
			Target:                   target,
			AllowThreads:             true,
			AllowReactions:           false,
			RequireEncryptedMessages: true,
			RequireTrustedDevices:    true,
			RequireJoinApproval:      true,
			PinnedMessagesOnlyAdmins: true,
			SlowModeInterval:         durationpb.New(10 * time.Second),
			AntiSpamWindow:           durationpb.New(time.Minute),
			AntiSpamBurstLimit:       3,
			ShadowMode:               true,
		},
	})
	if err != nil {
		t.Fatalf("set moderation policy: %v", err)
	}
	if setPolicy.Policy == nil || setPolicy.Policy.GetTarget().GetThreadId() != thread.Thread.ThreadId {
		t.Fatalf("unexpected moderation policy response: %+v", setPolicy.Policy)
	}
	if setPolicy.Policy.GetAllowReactions() || !setPolicy.Policy.GetRequireTrustedDevices() || !setPolicy.Policy.GetShadowMode() {
		t.Fatalf("unexpected moderation policy fields: %+v", setPolicy.Policy)
	}

	gotPolicy, err := fixture.api.GetModerationPolicy(ownerCtx, &conversationv1.GetModerationPolicyRequest{
		Target: target,
	})
	if err != nil {
		t.Fatalf("get moderation policy: %v", err)
	}
	if gotPolicy.Policy == nil || gotPolicy.Policy.GetTarget().GetConversationId() != created.Conversation.ConversationId {
		t.Fatalf("unexpected moderation policy target: %+v", gotPolicy.Policy)
	}
	if gotPolicy.Policy.GetAntiSpamBurstLimit() != 3 || gotPolicy.Policy.GetSlowModeInterval().AsDuration() != 10*time.Second {
		t.Fatalf("unexpected moderation policy timings: %+v", gotPolicy.Policy)
	}

	applied, err := fixture.api.ApplyModerationRestriction(ownerCtx, &conversationv1.ApplyModerationRestrictionRequest{
		Target:   target,
		UserId:   peer.ID,
		State:    conversationv1.ModerationRestrictionState_MODERATION_RESTRICTION_STATE_MUTED,
		Reason:   "slow mode abuse",
		Duration: durationpb.New(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("apply moderation restriction: %v", err)
	}
	if applied.Restriction == nil || applied.Restriction.GetState() != conversationv1.ModerationRestrictionState_MODERATION_RESTRICTION_STATE_MUTED {
		t.Fatalf("unexpected moderation restriction: %+v", applied.Restriction)
	}

	restrictions, err := fixture.api.ListModerationRestrictions(ownerCtx, &conversationv1.ListModerationRestrictionsRequest{
		Target: target,
	})
	if err != nil {
		t.Fatalf("list moderation restrictions: %v", err)
	}
	if len(restrictions.Restrictions) != 1 || restrictions.Restrictions[0].GetUserId() != peer.ID {
		t.Fatalf("unexpected moderation restrictions: %+v", restrictions.Restrictions)
	}

	submitted, err := fixture.api.SubmitModerationReport(peerCtx, &conversationv1.SubmitModerationReportRequest{
		Target:       target,
		TargetUserId: owner.ID,
		Reason:       "spam",
		Details:      "too many announcements",
	})
	if err != nil {
		t.Fatalf("submit moderation report: %v", err)
	}
	if submitted.Report == nil || submitted.Report.GetStatus() != conversationv1.ModerationReportStatus_MODERATION_REPORT_STATUS_PENDING {
		t.Fatalf("unexpected moderation report: %+v", submitted.Report)
	}

	report, err := fixture.api.GetModerationReport(ownerCtx, &conversationv1.GetModerationReportRequest{
		ReportId: submitted.Report.ReportId,
	})
	if err != nil {
		t.Fatalf("get moderation report: %v", err)
	}
	if report.Report == nil || report.Report.GetReportId() != submitted.Report.ReportId {
		t.Fatalf("unexpected moderation report lookup: %+v", report.Report)
	}

	reportList, err := fixture.api.ListModerationReports(ownerCtx, &conversationv1.ListModerationReportsRequest{
		Target: target,
	})
	if err != nil {
		t.Fatalf("list moderation reports: %v", err)
	}
	if len(reportList.Reports) != 1 || reportList.Reports[0].GetReportId() != submitted.Report.ReportId {
		t.Fatalf("unexpected moderation report list: %+v", reportList.Reports)
	}

	resolved, err := fixture.api.ResolveModerationReport(ownerCtx, &conversationv1.ResolveModerationReportRequest{
		ReportId:   submitted.Report.ReportId,
		Resolved:   true,
		Resolution: "reviewed",
	})
	if err != nil {
		t.Fatalf("resolve moderation report: %v", err)
	}
	if resolved.Report == nil || resolved.Report.GetStatus() != conversationv1.ModerationReportStatus_MODERATION_REPORT_STATUS_RESOLVED {
		t.Fatalf("unexpected resolved report: %+v", resolved.Report)
	}

	if _, err := fixture.api.LiftModerationRestriction(ownerCtx, &conversationv1.LiftModerationRestrictionRequest{
		Target: target,
		UserId: peer.ID,
		Reason: "cooldown complete",
	}); err != nil {
		t.Fatalf("lift moderation restriction: %v", err)
	}

	afterLift, err := fixture.api.ListModerationRestrictions(ownerCtx, &conversationv1.ListModerationRestrictionsRequest{
		Target: target,
	})
	if err != nil {
		t.Fatalf("list moderation restrictions after lift: %v", err)
	}
	if len(afterLift.Restrictions) != 0 {
		t.Fatalf("expected no active moderation restrictions, got %+v", afterLift.Restrictions)
	}

	actions, err := fixture.api.ListModerationActions(ownerCtx, &conversationv1.ListModerationActionsRequest{
		Target: target,
	})
	if err != nil {
		t.Fatalf("list moderation actions: %v", err)
	}
	if len(actions.Actions) < 4 {
		t.Fatalf("expected moderation audit actions, got %+v", actions.Actions)
	}

	actionTypes := make(map[conversationv1.ModerationActionType]struct{}, len(actions.Actions))
	for _, action := range actions.Actions {
		actionTypes[action.GetActionType()] = struct{}{}
	}
	for _, expected := range []conversationv1.ModerationActionType{
		conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_POLICY_SET,
		conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_MUTE,
		conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_REPORT_SET,
		conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_REPORT_RESOLVE,
		conversationv1.ModerationActionType_MODERATION_ACTION_TYPE_UNMUTE,
	} {
		if _, ok := actionTypes[expected]; !ok {
			t.Fatalf("expected moderation action %s in %+v", expected, actions.Actions)
		}
	}
}

func TestModerationRateStateAndCheckWriteRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, ownerCtx := fixture.mustCreateUserAndLogin(t, "rate-state-owner", "rate-state-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "rate-state-peer", "rate-state-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Rate State",
		MemberUserIds: []string{peer.ID},
		Settings: &conversationv1.ConversationSettings{
			AllowThreads: true,
		},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	thread, err := fixture.api.CreateThread(ownerCtx, &conversationv1.CreateThreadRequest{
		ConversationId: created.Conversation.ConversationId,
		Title:          "Slow Mode",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	target := &conversationv1.ModerationTarget{
		Kind:           conversationv1.ModerationTargetKind_MODERATION_TARGET_KIND_THREAD,
		ConversationId: created.Conversation.ConversationId,
		ThreadId:       thread.Thread.ThreadId,
	}

	if _, err := fixture.api.SetModerationPolicy(ownerCtx, &conversationv1.SetModerationPolicyRequest{
		Policy: &conversationv1.ModerationPolicy{
			Target:             target,
			AllowThreads:       true,
			SlowModeInterval:   durationpb.New(3 * time.Minute),
			AntiSpamWindow:     durationpb.New(10 * time.Minute),
			AntiSpamBurstLimit: 2,
		},
	}); err != nil {
		t.Fatalf("set moderation policy: %v", err)
	}

	firstDraft := testMessageDraft("rate-state-1")
	firstDraft.ThreadId = thread.Thread.ThreadId
	if _, err := fixture.api.SendMessage(peerCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft:          firstDraft,
	}); err != nil {
		t.Fatalf("send first thread message: %v", err)
	}

	state, err := fixture.api.GetModerationRateState(ownerCtx, &conversationv1.GetModerationRateStateRequest{
		Target: target,
		UserId: peer.ID,
	})
	if err != nil {
		t.Fatalf("get moderation rate state: %v", err)
	}
	if state.State == nil || state.State.GetUserId() != peer.ID {
		t.Fatalf("unexpected moderation rate state: %+v", state.State)
	}
	if state.State.GetTarget().GetThreadId() != thread.Thread.ThreadId || state.State.GetWindowCount() != 1 {
		t.Fatalf("unexpected moderation rate state counters: %+v", state.State)
	}
	if state.State.GetLastWriteAt() == nil || state.State.GetUpdatedAt() == nil {
		t.Fatalf("expected moderation rate timestamps, got %+v", state.State)
	}

	decision, err := fixture.api.CheckModerationWrite(peerCtx, &conversationv1.CheckModerationWriteRequest{
		Target: target,
	})
	if err != nil {
		t.Fatalf("check moderation write: %v", err)
	}
	if decision.Decision == nil || decision.Decision.GetAllowed() {
		t.Fatalf("expected disallowed moderation decision, got %+v", decision.Decision)
	}
	if decision.Decision.GetRetryAfter().AsDuration() != 3*time.Minute {
		t.Fatalf("unexpected retry after, got %+v", decision.Decision)
	}

	secondDraft := testMessageDraft("rate-state-2")
	secondDraft.ThreadId = thread.Thread.ThreadId
	_, err = fixture.api.SendMessage(peerCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft:          secondDraft,
	})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected resource exhausted on second write, got %v", err)
	}
}

func TestConversationMembershipAndInviteRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "membership-owner", "membership-owner@example.com")
	peer, _ := fixture.mustCreateUserAndLogin(t, "membership-peer", "membership-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:  commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title: "Members",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	title := "Updated Members"
	updated, err := fixture.api.UpdateConversation(ownerCtx, &conversationv1.UpdateConversationRequest{
		Conversation: &conversationv1.Conversation{
			ConversationId: created.Conversation.ConversationId,
			Title:          title,
			Settings: &conversationv1.ConversationSettings{
				OnlyAdminsCanAddMembers: true,
				AllowReactions:          false,
			},
		},
	})
	if err != nil {
		t.Fatalf("update conversation: %v", err)
	}
	if updated.Conversation.Title != title || !updated.Conversation.Settings.OnlyAdminsCanAddMembers {
		t.Fatalf("unexpected updated conversation: %+v", updated.Conversation)
	}

	added, err := fixture.api.AddMembers(ownerCtx, &conversationv1.AddMembersRequest{
		ConversationId: created.Conversation.ConversationId,
		UserIds:        []string{peer.ID},
		Role:           commonv1.MemberRole_MEMBER_ROLE_MEMBER,
	})
	if err != nil {
		t.Fatalf("add members: %v", err)
	}
	if len(added.Members) != 1 || added.Members[0].UserId != peer.ID {
		t.Fatalf("unexpected added members: %+v", added.Members)
	}

	roleUpdated, err := fixture.api.UpdateMemberRole(ownerCtx, &conversationv1.UpdateMemberRoleRequest{
		ConversationId: created.Conversation.ConversationId,
		UserId:         peer.ID,
		Role:           commonv1.MemberRole_MEMBER_ROLE_ADMIN,
	})
	if err != nil {
		t.Fatalf("update member role: %v", err)
	}
	if roleUpdated.Member.Role != commonv1.MemberRole_MEMBER_ROLE_ADMIN {
		t.Fatalf("unexpected updated member: %+v", roleUpdated.Member)
	}

	listed, err := fixture.api.ListMembers(ownerCtx, &conversationv1.ListMembersRequest{
		ConversationId: created.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(listed.Members) != 2 {
		t.Fatalf("expected owner and peer in member list, got %+v", listed.Members)
	}

	invite, err := fixture.api.CreateInvite(ownerCtx, &conversationv1.CreateInviteRequest{
		ConversationId: created.Conversation.ConversationId,
		AllowedRoles:   []commonv1.MemberRole{commonv1.MemberRole_MEMBER_ROLE_MEMBER},
		MaxUses:        5,
	})
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	if invite.Invite == nil || invite.Invite.InviteId == "" || invite.Invite.Code == "" {
		t.Fatalf("unexpected invite: %+v", invite.Invite)
	}

	invites, err := fixture.api.ListInvites(ownerCtx, &conversationv1.ListInvitesRequest{
		ConversationId: created.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(invites.Invites) != 1 || invites.Invites[0].InviteId != invite.Invite.InviteId {
		t.Fatalf("unexpected invites: %+v", invites.Invites)
	}

	revoked, err := fixture.api.RevokeInvite(ownerCtx, &conversationv1.RevokeInviteRequest{
		ConversationId: created.Conversation.ConversationId,
		InviteId:       invite.Invite.InviteId,
	})
	if err != nil {
		t.Fatalf("revoke invite: %v", err)
	}
	if revoked.Invite == nil || !revoked.Invite.Revoked {
		t.Fatalf("expected revoked invite, got %+v", revoked.Invite)
	}

	removed, err := fixture.api.RemoveMembers(ownerCtx, &conversationv1.RemoveMembersRequest{
		ConversationId: created.Conversation.ConversationId,
		UserIds:        []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("remove members: %v", err)
	}
	if removed.RemovedMembers != 1 || owner.ID == "" {
		t.Fatalf("unexpected remove result: %+v", removed)
	}
}

func TestMediaFiltersVariantAndHardDelete(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	account, authCtx := fixture.mustCreateUserAndLogin(t, "media-owner", "media-owner@example.com")

	first, err := fixture.api.InitiateUpload(authCtx, &mediav1.InitiateUploadRequest{
		Purpose:        commonv1.MediaPurpose_MEDIA_PURPOSE_MESSAGE_ATTACHMENT,
		FileName:       "photo.jpg",
		MimeType:       "image/jpeg",
		SizeBytes:      1024,
		ConversationId: "conv-1",
	})
	if err != nil {
		t.Fatalf("initiate first upload: %v", err)
	}

	_, err = fixture.api.InitiateUpload(authCtx, &mediav1.InitiateUploadRequest{
		Purpose:        commonv1.MediaPurpose_MEDIA_PURPOSE_STICKER_ASSET,
		FileName:       "sticker.webp",
		MimeType:       "image/webp",
		SizeBytes:      512,
		ConversationId: "conv-2",
	})
	if err != nil {
		t.Fatalf("initiate second upload: %v", err)
	}

	listed, err := fixture.api.ListMedia(authCtx, &mediav1.ListMediaRequest{
		Purposes:       []commonv1.MediaPurpose{commonv1.MediaPurpose_MEDIA_PURPOSE_MESSAGE_ATTACHMENT},
		ConversationId: "conv-1",
	})
	if err != nil {
		t.Fatalf("list media: %v", err)
	}
	if len(listed.Media) != 1 || listed.Media[0].MediaId != first.Media.MediaId {
		t.Fatalf("unexpected filtered media result: %+v", listed.Media)
	}

	now := fixture.now()
	if _, err := fixture.mediaStore.SaveMediaAsset(context.Background(), media.MediaAsset{
		ID:              "media-variant",
		OwnerAccountID:  account.ID,
		Kind:            media.MediaKindImage,
		Status:          media.MediaStatusReady,
		StorageProvider: "object",
		Bucket:          fixture.mediaBlob.bucket,
		ObjectKey:       "media/" + account.ID + "/media-variant",
		FileName:        "variant.jpg",
		ContentType:     "image/jpeg",
		SizeBytes:       2048,
		Metadata: map[string]string{
			"variant_object_key.thumb": "media/" + account.ID + "/media-variant-thumb",
			media.MetadataPurposeKey:   "message_attachment",
		},
		UploadExpiresAt: now.Add(time.Minute),
		ReadyAt:         now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("seed variant asset: %v", err)
	}

	download, err := fixture.api.GetDownloadUrl(authCtx, &mediav1.GetDownloadUrlRequest{
		MediaId: "media-variant",
		Variant: "thumb",
	})
	if err != nil {
		t.Fatalf("get variant download url: %v", err)
	}
	if !strings.Contains(download.Url, "media-variant-thumb") {
		t.Fatalf("expected variant object key in download url, got %s", download.Url)
	}

	if _, err := fixture.api.DeleteMedia(authCtx, &mediav1.DeleteMediaRequest{
		MediaId:    first.Media.MediaId,
		HardDelete: true,
	}); err != nil {
		t.Fatalf("hard delete media: %v", err)
	}

	_, err = fixture.api.GetMedia(authCtx, &mediav1.GetMediaRequest{MediaId: first.Media.MediaId})
	if status.Code(err).String() == "OK" || err == nil {
		t.Fatal("expected deleted media to disappear after hard delete")
	}
}

func TestNotificationRPC(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "notification-owner", "notification-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "notification-peer", "notification-peer@example.com")

	defaultPreference, err := fixture.api.GetNotificationPreference(ownerCtx, &notificationv1.GetNotificationPreferenceRequest{})
	if err != nil {
		t.Fatalf("get default notification preference: %v", err)
	}
	if !defaultPreference.GetPreference().GetEnabled() || !defaultPreference.GetPreference().GetDirectEnabled() {
		t.Fatalf("unexpected default preference: %+v", defaultPreference.GetPreference())
	}

	savedPreference, err := fixture.api.SetNotificationPreference(ownerCtx, &notificationv1.SetNotificationPreferenceRequest{
		Preference: &notificationv1.NotificationPreference{
			Enabled:        true,
			DirectEnabled:  true,
			GroupEnabled:   false,
			ChannelEnabled: false,
			MentionEnabled: true,
			ReplyEnabled:   false,
			QuietHours: &notificationv1.QuietHours{
				Enabled:     true,
				StartMinute: 60,
				EndMinute:   420,
				Timezone:    "Europe/Moscow",
			},
			MutedUntil: protoTime(fixture.now().Add(30 * time.Minute)),
		},
	})
	if err != nil {
		t.Fatalf("set notification preference: %v", err)
	}
	if savedPreference.GetPreference().GetGroupEnabled() || savedPreference.GetPreference().GetReplyEnabled() {
		t.Fatalf("unexpected saved preference: %+v", savedPreference.GetPreference())
	}
	if savedPreference.GetPreference().GetQuietHours().GetTimezone() != "Europe/Moscow" {
		t.Fatalf("expected quiet hours timezone to round-trip, got %+v", savedPreference.GetPreference().GetQuietHours())
	}

	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Notification Overrides",
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	override, err := fixture.api.SetConversationNotificationOverride(ownerCtx, &notificationv1.SetConversationNotificationOverrideRequest{
		Override: &notificationv1.ConversationNotificationOverride{
			ConversationId: created.GetConversation().GetConversationId(),
			Muted:          true,
			MentionsOnly:   true,
			MutedUntil:     protoTime(fixture.now().Add(2 * time.Hour)),
		},
	})
	if err != nil {
		t.Fatalf("set conversation notification override: %v", err)
	}
	if !override.GetOverride().GetMuted() || !override.GetOverride().GetMentionsOnly() {
		t.Fatalf("unexpected override: %+v", override.GetOverride())
	}

	loadedOverride, err := fixture.api.GetConversationNotificationOverride(ownerCtx, &notificationv1.GetConversationNotificationOverrideRequest{
		ConversationId: created.GetConversation().GetConversationId(),
	})
	if err != nil {
		t.Fatalf("get conversation notification override: %v", err)
	}
	if loadedOverride.GetOverride().GetConversationId() != created.GetConversation().GetConversationId() {
		t.Fatalf("unexpected loaded override: %+v", loadedOverride.GetOverride())
	}

	registered, err := fixture.api.RegisterPushToken(ownerCtx, &notificationv1.RegisterPushTokenRequest{
		Provider: "apns",
		Token:    "push-token-1",
	})
	if err != nil {
		t.Fatalf("register push token: %v", err)
	}
	if registered.GetPushToken().GetProvider() != "apns" || registered.GetPushToken().GetDeviceId() == "" {
		t.Fatalf("unexpected registered push token: %+v", registered.GetPushToken())
	}

	listed, err := fixture.api.ListPushTokens(ownerCtx, &notificationv1.ListPushTokensRequest{})
	if err != nil {
		t.Fatalf("list push tokens: %v", err)
	}
	if len(listed.GetPushTokens()) != 1 || listed.GetPushTokens()[0].GetTokenId() != registered.GetPushToken().GetTokenId() {
		t.Fatalf("unexpected push token list: %+v", listed.GetPushTokens())
	}

	_, err = fixture.api.RevokePushToken(peerCtx, &notificationv1.RevokePushTokenRequest{
		TokenId: registered.GetPushToken().GetTokenId(),
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied revoking foreign push token, got %v", err)
	}

	revoked, err := fixture.api.RevokePushToken(ownerCtx, &notificationv1.RevokePushTokenRequest{
		TokenId: registered.GetPushToken().GetTokenId(),
	})
	if err != nil {
		t.Fatalf("revoke push token: %v", err)
	}
	if revoked.GetPushToken().GetRevokedAt() == nil {
		t.Fatalf("expected revoked timestamp, got %+v", revoked.GetPushToken())
	}

	listedAfterRevoke, err := fixture.api.ListPushTokens(ownerCtx, &notificationv1.ListPushTokensRequest{})
	if err != nil {
		t.Fatalf("list push tokens after revoke: %v", err)
	}
	if len(listedAfterRevoke.GetPushTokens()) != 0 {
		t.Fatalf("expected no active push tokens after revoke, got %+v", listedAfterRevoke.GetPushTokens())
	}

	if owner.ID == "" {
		t.Fatal("expected owner account id to be populated")
	}
}

func TestSubscribeEventsWakesOnConversationChanges(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, authCtx := fixture.mustCreateUserAndLogin(t, "sync-owner", "sync-owner@example.com")
	created, err := fixture.api.CreateConversation(authCtx, &conversationv1.CreateConversationRequest{
		Kind:  commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title: "Sync",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	stream := newTestSubscribeEventsStream(authCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeEvents(&syncv1.SubscribeEventsRequest{
			FromSequence:      created.Conversation.LastSequence,
			ConversationIds:   []string{created.Conversation.ConversationId},
			IncludePresence:   false,
			IncludeModeration: false,
		}, stream)
	}()

	time.Sleep(20 * time.Millisecond)

	if _, err := fixture.api.SendMessage(authCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft:          testMessageDraft("sync"),
	}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	select {
	case response := <-stream.responses:
		if response.GetEvent() == nil || response.GetEvent().GetConversationId() != created.Conversation.ConversationId {
			t.Fatalf("unexpected subscribe response: %+v", response)
		}
		stream.cancel()
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting for subscribe event")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribe loop to stop")
	}
}

func TestPullEventsPresenceFilteringAdvancesSequence(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "presence-owner", "presence-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "presence-peer", "presence-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Presence",
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	baseline := created.Conversation.LastSequence
	if _, err := fixture.api.SetPresence(ownerCtx, &usersv1.SetPresenceRequest{
		Presence: commonv1.PresenceState_PRESENCE_STATE_ONLINE,
	}); err != nil {
		t.Fatalf("set presence: %v", err)
	}

	skipped, err := fixture.api.PullEvents(peerCtx, &syncv1.PullEventsRequest{
		FromSequence:    baseline,
		ConversationIds: []string{created.Conversation.ConversationId},
	})
	if err != nil {
		t.Fatalf("pull events without presence: %v", err)
	}
	if len(skipped.Events) != 0 {
		t.Fatalf("expected presence events to be skipped, got %+v", skipped.Events)
	}
	if skipped.NextSequence <= baseline {
		t.Fatalf("expected next sequence to advance beyond %d, got %d", baseline, skipped.NextSequence)
	}

	if _, err := fixture.api.SetPresence(ownerCtx, &usersv1.SetPresenceRequest{
		Presence: commonv1.PresenceState_PRESENCE_STATE_AWAY,
	}); err != nil {
		t.Fatalf("set second presence: %v", err)
	}

	included, err := fixture.api.PullEvents(peerCtx, &syncv1.PullEventsRequest{
		FromSequence:    skipped.NextSequence,
		ConversationIds: []string{created.Conversation.ConversationId},
		IncludePresence: true,
	})
	if err != nil {
		t.Fatalf("pull events with presence: %v", err)
	}
	if len(included.Events) != 1 {
		t.Fatalf("expected one presence event, got %+v", included.Events)
	}
	if included.Events[0].GetEventType() != commonv1.EventType_EVENT_TYPE_USER_UPDATED {
		t.Fatalf("expected user updated event, got %s", included.Events[0].GetEventType())
	}
	if included.Events[0].GetPayloadType() != "presence" {
		t.Fatalf("expected presence payload type, got %s", included.Events[0].GetPayloadType())
	}
	if included.Events[0].GetMetadata()["user_id"] != owner.ID {
		t.Fatalf("expected presence metadata to reference %s, got %+v", owner.ID, included.Events[0].GetMetadata())
	}
}

func TestPullEventsModerationFilteringAdvancesSequence(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "moderation-owner", "moderation-owner@example.com")
	peer, _ := fixture.mustCreateUserAndLogin(t, "moderation-peer", "moderation-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Moderation",
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	baseline := created.Conversation.LastSequence
	if _, err := fixture.api.conversation.ApplyModerationRestriction(context.Background(), conversation.ApplyModerationRestrictionParams{
		TargetKind:      conversation.ModerationTargetKindConversation,
		TargetID:        created.Conversation.ConversationId,
		ActorAccountID:  owner.ID,
		TargetAccountID: peer.ID,
		State:           conversation.ModerationRestrictionStateMuted,
		CreatedAt:       fixture.now(),
	}); err != nil {
		t.Fatalf("apply moderation restriction: %v", err)
	}

	skipped, err := fixture.api.PullEvents(ownerCtx, &syncv1.PullEventsRequest{
		FromSequence:    baseline,
		ConversationIds: []string{created.Conversation.ConversationId},
	})
	if err != nil {
		t.Fatalf("pull events without moderation: %v", err)
	}
	if len(skipped.Events) != 0 {
		t.Fatalf("expected moderation events to be skipped, got %+v", skipped.Events)
	}
	if skipped.NextSequence <= baseline {
		t.Fatalf("expected next sequence to advance beyond %d, got %d", baseline, skipped.NextSequence)
	}

	if err := fixture.api.conversation.LiftModerationRestriction(context.Background(), conversation.LiftModerationRestrictionParams{
		TargetKind:      conversation.ModerationTargetKindConversation,
		TargetID:        created.Conversation.ConversationId,
		ActorAccountID:  owner.ID,
		TargetAccountID: peer.ID,
		Reason:          "resolved",
		CreatedAt:       fixture.now().Add(time.Minute),
	}); err != nil {
		t.Fatalf("lift moderation restriction: %v", err)
	}

	included, err := fixture.api.PullEvents(ownerCtx, &syncv1.PullEventsRequest{
		FromSequence:      skipped.NextSequence,
		ConversationIds:   []string{created.Conversation.ConversationId},
		IncludeModeration: true,
	})
	if err != nil {
		t.Fatalf("pull events with moderation: %v", err)
	}
	if len(included.Events) != 1 {
		t.Fatalf("expected one moderation event, got %+v", included.Events)
	}
	if included.Events[0].GetEventType() != commonv1.EventType_EVENT_TYPE_ADMIN_ACTION_RECORDED {
		t.Fatalf("expected admin action event, got %s", included.Events[0].GetEventType())
	}
	if included.Events[0].GetPayloadType() != "moderation_action" {
		t.Fatalf("expected moderation payload type, got %s", included.Events[0].GetPayloadType())
	}
	if included.Events[0].GetMetadata()["action_type"] != string(conversation.ModerationActionTypeUnmute) {
		t.Fatalf("expected unmute action metadata, got %+v", included.Events[0].GetMetadata())
	}
}

func TestSubscribeEventsFiltersPresenceUntilRequested(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "subscribe-presence-owner", "subscribe-presence-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "subscribe-presence-peer", "subscribe-presence-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Subscribe Presence",
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	stream := newTestSubscribeEventsStream(peerCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeEvents(&syncv1.SubscribeEventsRequest{
			FromSequence:      created.Conversation.LastSequence,
			ConversationIds:   []string{created.Conversation.ConversationId},
			IncludePresence:   false,
			IncludeModeration: false,
		}, stream)
	}()

	time.Sleep(20 * time.Millisecond)

	if _, err := fixture.api.SetPresence(ownerCtx, &usersv1.SetPresenceRequest{
		Presence: commonv1.PresenceState_PRESENCE_STATE_BUSY,
	}); err != nil {
		t.Fatalf("set presence: %v", err)
	}

	select {
	case response := <-stream.responses:
		t.Fatalf("expected presence event to be filtered, got %+v", response)
	case <-time.After(150 * time.Millisecond):
	}

	stream.cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for filtered subscribe loop to stop")
	}

	includingStream := newTestSubscribeEventsStream(peerCtx)
	errCh = make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeEvents(&syncv1.SubscribeEventsRequest{
			FromSequence:      created.Conversation.LastSequence,
			ConversationIds:   []string{created.Conversation.ConversationId},
			IncludePresence:   true,
			IncludeModeration: false,
		}, includingStream)
	}()

	time.Sleep(20 * time.Millisecond)

	if _, err := fixture.api.SetPresence(ownerCtx, &usersv1.SetPresenceRequest{
		Presence: commonv1.PresenceState_PRESENCE_STATE_AWAY,
	}); err != nil {
		t.Fatalf("set second presence: %v", err)
	}

	select {
	case response := <-includingStream.responses:
		if response.GetEvent() == nil {
			t.Fatal("expected sync event")
		}
		if response.GetEvent().GetEventType() != commonv1.EventType_EVENT_TYPE_USER_UPDATED {
			t.Fatalf("expected user updated event, got %s", response.GetEvent().GetEventType())
		}
		includingStream.cancel()
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting for included presence event")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribe loop to stop")
	}
	_ = owner
}

func TestSubscribeEventsFiltersModerationUntilRequested(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "subscribe-moderation-owner", "subscribe-moderation-owner@example.com")
	peer, _ := fixture.mustCreateUserAndLogin(t, "subscribe-moderation-peer", "subscribe-moderation-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:         "Subscribe Moderation",
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	stream := newTestSubscribeEventsStream(ownerCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeEvents(&syncv1.SubscribeEventsRequest{
			FromSequence:      created.Conversation.LastSequence,
			ConversationIds:   []string{created.Conversation.ConversationId},
			IncludePresence:   false,
			IncludeModeration: false,
		}, stream)
	}()

	time.Sleep(20 * time.Millisecond)

	if _, err := fixture.api.conversation.ApplyModerationRestriction(context.Background(), conversation.ApplyModerationRestrictionParams{
		TargetKind:      conversation.ModerationTargetKindConversation,
		TargetID:        created.Conversation.ConversationId,
		ActorAccountID:  owner.ID,
		TargetAccountID: peer.ID,
		State:           conversation.ModerationRestrictionStateMuted,
		CreatedAt:       fixture.now(),
	}); err != nil {
		t.Fatalf("apply moderation restriction: %v", err)
	}

	select {
	case response := <-stream.responses:
		t.Fatalf("expected moderation event to be filtered, got %+v", response)
	case <-time.After(150 * time.Millisecond):
	}

	stream.cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for filtered subscribe loop to stop")
	}

	afterMute, err := fixture.api.PullEvents(ownerCtx, &syncv1.PullEventsRequest{
		FromSequence:      created.Conversation.LastSequence,
		ConversationIds:   []string{created.Conversation.ConversationId},
		IncludeModeration: true,
	})
	if err != nil {
		t.Fatalf("pull moderation backlog: %v", err)
	}
	if len(afterMute.Events) != 1 {
		t.Fatalf("expected one moderation backlog event, got %+v", afterMute.Events)
	}

	if err := fixture.api.conversation.LiftModerationRestriction(context.Background(), conversation.LiftModerationRestrictionParams{
		TargetKind:      conversation.ModerationTargetKindConversation,
		TargetID:        created.Conversation.ConversationId,
		ActorAccountID:  owner.ID,
		TargetAccountID: peer.ID,
		Reason:          "resolved",
		CreatedAt:       fixture.now().Add(time.Minute),
	}); err != nil {
		t.Fatalf("lift moderation restriction: %v", err)
	}

	includingStream := newTestSubscribeEventsStream(ownerCtx)
	errCh = make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeEvents(&syncv1.SubscribeEventsRequest{
			FromSequence:      afterMute.NextSequence,
			ConversationIds:   []string{created.Conversation.ConversationId},
			IncludePresence:   false,
			IncludeModeration: true,
		}, includingStream)
	}()

	select {
	case response := <-includingStream.responses:
		if response.GetEvent() == nil {
			t.Fatal("expected sync event")
		}
		if response.GetEvent().GetEventType() != commonv1.EventType_EVENT_TYPE_ADMIN_ACTION_RECORDED {
			t.Fatalf("expected admin action event, got %s", response.GetEvent().GetEventType())
		}
		if response.GetEvent().GetPayloadType() != "moderation_action" {
			t.Fatalf("expected moderation payload type, got %s", response.GetEvent().GetPayloadType())
		}
		if response.GetEvent().GetMetadata()["action_type"] != string(conversation.ModerationActionTypeUnmute) {
			t.Fatalf("expected unmute action metadata, got %+v", response.GetEvent().GetMetadata())
		}
		includingStream.cancel()
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting for included moderation event")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribe loop to stop")
	}
}

func TestGetMessageDoesNotWakeSubscribeEvents(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, authCtx := fixture.mustCreateUserAndLogin(t, "readonly-owner", "readonly-owner@example.com")
	created, err := fixture.api.CreateConversation(authCtx, &conversationv1.CreateConversationRequest{
		Kind:  commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title: "Readonly",
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	sent, err := fixture.api.SendMessage(authCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft:          testMessageDraft("readonly-message"),
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	stream := newTestSubscribeEventsStream(authCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- fixture.api.SubscribeEvents(&syncv1.SubscribeEventsRequest{
			FromSequence:      sent.Event.Sequence,
			ConversationIds:   []string{created.Conversation.ConversationId},
			IncludePresence:   false,
			IncludeModeration: false,
		}, stream)
	}()

	time.Sleep(20 * time.Millisecond)

	if _, err := fixture.api.GetMessage(authCtx, &conversationv1.GetMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		MessageId:      sent.Message.MessageId,
	}); err != nil {
		t.Fatalf("get message: %v", err)
	}

	select {
	case response := <-stream.responses:
		t.Fatalf("expected no sync event for get message, got %+v", response)
	case <-time.After(150 * time.Millisecond):
	}

	stream.cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("subscribe events returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscribe loop to stop")
	}
}

func TestScheduledMessageRPCLifecycle(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	owner, ownerCtx := fixture.mustCreateUserAndLogin(t, "scheduled-owner", "scheduled-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "scheduled-peer", "scheduled-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if created.Conversation.OwnerUserId != owner.ID {
		t.Fatalf("unexpected owner: %+v", created.Conversation)
	}

	deliverAt := fixture.now().Add(10 * time.Minute)
	scheduled, err := fixture.api.SendMessage(ownerCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft: &commonv1.MessageDraft{
			ClientMessageId: "scheduled-rpc",
			Kind:            commonv1.MessageKind_MESSAGE_KIND_TEXT,
			Payload: &commonv1.EncryptedPayload{
				Ciphertext: []byte("scheduled rpc body"),
			},
			DeliverAt: timestamppb.New(deliverAt),
		},
	})
	if err != nil {
		t.Fatalf("send scheduled message: %v", err)
	}
	if scheduled.Event != nil {
		t.Fatalf("expected no sync event before dispatch, got %+v", scheduled.Event)
	}
	if scheduled.Message.Status != commonv1.MessageStatus_MESSAGE_STATUS_PENDING {
		t.Fatalf("expected pending message, got %+v", scheduled.Message)
	}
	if scheduled.Message.DeliverAt == nil || !scheduled.Message.DeliverAt.AsTime().Equal(deliverAt) {
		t.Fatalf("unexpected deliver_at: %+v", scheduled.Message)
	}

	scheduledList, err := fixture.api.ListScheduledMessages(ownerCtx, &conversationv1.ListScheduledMessagesRequest{
		ConversationId: created.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("list scheduled messages: %v", err)
	}
	if len(scheduledList.Messages) != 1 || scheduledList.Messages[0].MessageId != scheduled.Message.MessageId {
		t.Fatalf("expected scheduled message in private list, got %+v", scheduledList.Messages)
	}

	peerMessages, err := fixture.api.ListMessages(peerCtx, &conversationv1.ListMessagesRequest{
		ConversationId: created.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("list peer messages before dispatch: %v", err)
	}
	if len(peerMessages.Messages) != 0 {
		t.Fatalf("expected no visible peer messages before dispatch, got %+v", peerMessages.Messages)
	}
	if _, err := fixture.api.GetMessage(peerCtx, &conversationv1.GetMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		MessageId:      scheduled.Message.MessageId,
	}); status.Code(err) != codes.NotFound {
		t.Fatalf("expected scheduled message to stay hidden from peer, got %v", err)
	}

	events, err := fixture.api.conversation.DispatchDueMessages(context.Background(), deliverAt.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("dispatch scheduled messages: %v", err)
	}
	if err := fixture.api.HandleScheduledMessageEvents(context.Background(), events); err != nil {
		t.Fatalf("publish scheduled message events: %v", err)
	}

	published, err := fixture.api.GetMessage(peerCtx, &conversationv1.GetMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		MessageId:      scheduled.Message.MessageId,
	})
	if err != nil {
		t.Fatalf("get dispatched message: %v", err)
	}
	if published.Message.Status != commonv1.MessageStatus_MESSAGE_STATUS_SENT {
		t.Fatalf("expected sent status after dispatch, got %+v", published.Message)
	}

	scheduledList, err = fixture.api.ListScheduledMessages(ownerCtx, &conversationv1.ListScheduledMessagesRequest{
		ConversationId: created.Conversation.ConversationId,
	})
	if err != nil {
		t.Fatalf("list scheduled messages after dispatch: %v", err)
	}
	if len(scheduledList.Messages) != 0 {
		t.Fatalf("expected empty scheduled queue after dispatch, got %+v", scheduledList.Messages)
	}
}

func TestTranslateMessageRPCUsesCache(t *testing.T) {
	t.Parallel()

	fixture := newGatewayFeatureFixture(t)

	_, ownerCtx := fixture.mustCreateUserAndLogin(t, "translate-owner", "translate-owner@example.com")
	peer, peerCtx := fixture.mustCreateUserAndLogin(t, "translate-peer", "translate-peer@example.com")
	created, err := fixture.api.CreateConversation(ownerCtx, &conversationv1.CreateConversationRequest{
		Kind:          commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds: []string{peer.ID},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	sent, err := fixture.api.SendMessage(ownerCtx, &conversationv1.SendMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		Draft: &commonv1.MessageDraft{
			ClientMessageId: "translate-message",
			Kind:            commonv1.MessageKind_MESSAGE_KIND_TEXT,
			Payload: &commonv1.EncryptedPayload{
				Ciphertext: []byte("hello translation"),
			},
		},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	first, err := fixture.api.TranslateMessage(peerCtx, &conversationv1.TranslateMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		MessageId:      sent.Message.MessageId,
		TargetLanguage: "ru",
	})
	if err != nil {
		t.Fatalf("translate message: %v", err)
	}
	if first.Translation == nil || first.Translation.Cached {
		t.Fatalf("expected first translation to bypass cache, got %+v", first.Translation)
	}
	if first.Translation.TranslatedText != "[ru] hello translation" {
		t.Fatalf("unexpected translated text: %+v", first.Translation)
	}

	second, err := fixture.api.TranslateMessage(peerCtx, &conversationv1.TranslateMessageRequest{
		ConversationId: created.Conversation.ConversationId,
		MessageId:      sent.Message.MessageId,
		TargetLanguage: "ru",
	})
	if err != nil {
		t.Fatalf("translate message from cache: %v", err)
	}
	if second.Translation == nil || !second.Translation.Cached {
		t.Fatalf("expected cached translation, got %+v", second.Translation)
	}
	if second.Translation.Provider != "test-translation" {
		t.Fatalf("unexpected cached provider: %+v", second.Translation)
	}
}

type gatewayFeatureFixture struct {
	api        *api
	sender     *recordingSender
	mediaStore *gatewayMediaStore
	mediaBlob  *gatewayBlobStore
	nowFunc    func() time.Time
}

func newGatewayFeatureFixture(t *testing.T) *gatewayFeatureFixture {
	t.Helper()

	now := time.Date(2026, time.March, 26, 15, 0, 0, 0, time.UTC)
	nowFunc := func() time.Time { return now }

	identityStore := identitytest.NewMemoryStore()
	sender := &recordingSender{}
	searchService, err := search.NewService(searchtest.New())
	if err != nil {
		t.Fatalf("new search service: %v", err)
	}
	identityService, err := identity.NewService(
		identityStore,
		sender,
		identity.WithNow(nowFunc),
		identity.WithIndexer(searchService),
	)
	if err != nil {
		t.Fatalf("new indexed identity service: %v", err)
	}

	presenceService, err := presence.NewService(presencetest.NewMemoryStore(), identityStore, presence.WithNow(nowFunc))
	if err != nil {
		t.Fatalf("new presence service: %v", err)
	}

	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(
		conversationStore,
		conversation.WithNow(nowFunc),
		conversation.WithIndexer(searchService),
	)
	if err != nil {
		t.Fatalf("new conversation service: %v", err)
	}
	callService, err := domaincall.NewService(
		calltest.NewMemoryStore(),
		conversationStore,
		mustTestRTCCluster(t),
		domaincall.WithNow(nowFunc),
		domaincall.WithRTC(domaincall.RTCConfig{
			PublicEndpoint: "webrtc://node-a/calls",
			CredentialTTL:  15 * time.Minute,
			NodeID:         "node-a",
			CandidateHost:  "127.0.0.1",
			UDPPortMin:     41000,
			UDPPortMax:     41019,
			Nodes: []domaincall.RTCNode{
				{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
				{ID: "node-b", Endpoint: "webrtc://node-b/calls"},
			},
		}),
	)
	if err != nil {
		t.Fatalf("new call service: %v", err)
	}

	mediaStore := newGatewayMediaStore()
	mediaBlob := newGatewayBlobStore("media-bucket")
	mediaService, err := media.NewService(
		mediaStore,
		mediaBlob,
		media.WithNow(nowFunc),
		media.WithSettings(media.Settings{
			UploadURLTTL:   15 * time.Minute,
			DownloadURLTTL: 15 * time.Minute,
			MaxUploadSize:  10 << 20,
		}),
		media.WithIndexer(searchService),
	)
	if err != nil {
		t.Fatalf("new media service: %v", err)
	}
	notificationService, err := notification.NewService(
		notificationtest.NewMemoryStore(),
		identityStore,
		notification.WithNow(nowFunc),
	)
	if err != nil {
		t.Fatalf("new notification service: %v", err)
	}
	translationService, err := domaintranslation.NewService(
		translationtest.NewMemoryStore(),
		conversationService,
		testTranslationProvider{},
		domaintranslation.WithNow(nowFunc),
	)
	if err != nil {
		t.Fatalf("new translation service: %v", err)
	}
	userService, err := domainuser.NewService(usertest.NewMemoryStore(), identityService, domainuser.WithNow(nowFunc))
	if err != nil {
		t.Fatalf("new user service: %v", err)
	}

	return &gatewayFeatureFixture{
		api: &api{
			call:         callService,
			identity:     identityService,
			presence:     presenceService,
			conversation: conversationService,
			media:        mediaService,
			notification: notificationService,
			search:       searchService,
			translation:  translationService,
			user:         userService,
			callNotifier: newCallNotifier(),
			syncNotifier: newSyncNotifier(),
			features: config.FeatureConfig{
				CallsEnabled:             true,
				SearchEnabled:            true,
				ScheduledMessagesEnabled: true,
				TranslationEnabled:       true,
			},
		},
		sender:     sender,
		mediaStore: mediaStore,
		mediaBlob:  mediaBlob,
		nowFunc:    nowFunc,
	}
}

func mustTestRTCCluster(t *testing.T) domaincall.Runtime {
	t.Helper()

	local := platformrtc.NewManager(
		"webrtc://node-a/calls",
		15*time.Minute,
		platformrtc.WithCandidateHost("127.0.0.1"),
		platformrtc.WithUDPPortRange(41000, 41009),
	)
	cluster, err := platformrtc.NewCluster(domaincall.RTCConfig{
		PublicEndpoint: "webrtc://node-a/calls",
		CredentialTTL:  15 * time.Minute,
		NodeID:         "node-a",
		CandidateHost:  "127.0.0.1",
		UDPPortMin:     41000,
		UDPPortMax:     41019,
		Nodes: []domaincall.RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls"},
		},
	}, local)
	if err != nil {
		t.Fatalf("new test rtc cluster: %v", err)
	}

	return cluster
}

func (f *gatewayFeatureFixture) now() time.Time {
	return f.nowFunc()
}

func (f *gatewayFeatureFixture) mustCreateUserAndLogin(
	t *testing.T,
	username string,
	email string,
) (identity.Account, context.Context) {
	t.Helper()

	account, _, err := f.api.identity.CreateAccount(context.Background(), identity.CreateAccountParams{
		Username:    username,
		DisplayName: username,
		Email:       email,
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	begin, err := f.api.BeginLogin(context.Background(), &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: account.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      "test-device",
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}

	verify, err := f.api.VerifyLoginCode(context.Background(), &authv1.VerifyLoginCodeRequest{
		ChallengeId:    begin.ChallengeId,
		Code:           f.sender.code(begin.Targets[0].DestinationMask),
		DeviceName:     "test-device",
		DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey:      &commonv1.PublicKeyBundle{PublicKey: []byte("device-key")},
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	authCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization",
		"Bearer "+verify.Tokens.AccessToken,
	))

	return account, authCtx
}

func (f *gatewayFeatureFixture) mustLoginAccountOnNewDevice(
	t *testing.T,
	approverCtx context.Context,
	account identity.Account,
	deviceName string,
) context.Context {
	t.Helper()

	begin, err := f.api.BeginLogin(context.Background(), &authv1.BeginLoginRequest{
		Identifier:      &authv1.BeginLoginRequest_Username{Username: account.Username},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_EMAIL,
		DeviceName:      deviceName,
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
	})
	if err != nil {
		t.Fatalf("begin login on new device: %v", err)
	}

	verify, err := f.api.VerifyLoginCode(context.Background(), &authv1.VerifyLoginCodeRequest{
		ChallengeId:    begin.ChallengeId,
		Code:           f.sender.code(begin.Targets[0].DestinationMask),
		DeviceName:     deviceName,
		DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_IOS,
		DeviceKey:      &commonv1.PublicKeyBundle{PublicKey: []byte("device-key-" + deviceName)},
	})
	if err != nil {
		t.Fatalf("verify login on new device: %v", err)
	}

	authCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization",
		"Bearer "+verify.Tokens.AccessToken,
	))
	if _, err := f.api.ApproveDeviceLink(approverCtx, &authv1.ApproveDeviceLinkRequest{
		TargetDeviceId: verify.Device.DeviceId,
	}); err != nil {
		t.Fatalf("approve device link for %s: %v", deviceName, err)
	}

	return authCtx
}

func testMessageDraft(id string) *commonv1.MessageDraft {
	return &commonv1.MessageDraft{
		ClientMessageId: id,
		Kind:            commonv1.MessageKind_MESSAGE_KIND_TEXT,
		Payload: &commonv1.EncryptedPayload{
			KeyId:      "key-" + id,
			Algorithm:  "xchacha20poly1305",
			Nonce:      []byte("nonce-" + id),
			Ciphertext: []byte("ciphertext-" + id),
		},
	}
}

type testTranslationProvider struct{}

func (testTranslationProvider) Translate(
	_ context.Context,
	request domaintranslation.ProviderRequest,
) (domaintranslation.ProviderResult, error) {
	return domaintranslation.ProviderResult{
		TranslatedText: "[" + request.TargetLanguage + "] " + request.Text,
		SourceLanguage: "en",
		Provider:       "test-translation",
	}, nil
}

type testSubscribeEventsStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	responses chan *syncv1.SubscribeEventsResponse
}

func newTestSubscribeEventsStream(ctx context.Context) *testSubscribeEventsStream {
	streamCtx, cancel := context.WithCancel(ctx)
	return &testSubscribeEventsStream{
		ctx:       streamCtx,
		cancel:    cancel,
		responses: make(chan *syncv1.SubscribeEventsResponse, 4),
	}
}

func (s *testSubscribeEventsStream) Context() context.Context { return s.ctx }
func (s *testSubscribeEventsStream) Send(resp *syncv1.SubscribeEventsResponse) error {
	s.responses <- resp
	return nil
}
func (*testSubscribeEventsStream) SetHeader(metadata.MD) error  { return nil }
func (*testSubscribeEventsStream) SendHeader(metadata.MD) error { return nil }
func (*testSubscribeEventsStream) SetTrailer(metadata.MD)       {}
func (*testSubscribeEventsStream) SendMsg(any) error            { return nil }
func (*testSubscribeEventsStream) RecvMsg(any) error            { return nil }

type testSubscribeCallEventsStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	responses chan *callv1.SubscribeCallEventsResponse
}

func newTestSubscribeCallEventsStream(ctx context.Context) *testSubscribeCallEventsStream {
	streamCtx, cancel := context.WithCancel(ctx)
	return &testSubscribeCallEventsStream{
		ctx:       streamCtx,
		cancel:    cancel,
		responses: make(chan *callv1.SubscribeCallEventsResponse, 64),
	}
}

func (s *testSubscribeCallEventsStream) Context() context.Context { return s.ctx }
func (s *testSubscribeCallEventsStream) Send(resp *callv1.SubscribeCallEventsResponse) error {
	s.responses <- resp
	return nil
}
func (*testSubscribeCallEventsStream) SetHeader(metadata.MD) error  { return nil }
func (*testSubscribeCallEventsStream) SendHeader(metadata.MD) error { return nil }
func (*testSubscribeCallEventsStream) SetTrailer(metadata.MD)       {}
func (*testSubscribeCallEventsStream) SendMsg(any) error            { return nil }
func (*testSubscribeCallEventsStream) RecvMsg(any) error            { return nil }

type testSubscribeCallStatsStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	responses chan *callv1.SubscribeCallStatsResponse
}

func newTestSubscribeCallStatsStream(ctx context.Context) *testSubscribeCallStatsStream {
	streamCtx, cancel := context.WithCancel(ctx)
	return &testSubscribeCallStatsStream{
		ctx:       streamCtx,
		cancel:    cancel,
		responses: make(chan *callv1.SubscribeCallStatsResponse, 64),
	}
}

func (s *testSubscribeCallStatsStream) Context() context.Context { return s.ctx }
func (s *testSubscribeCallStatsStream) Send(resp *callv1.SubscribeCallStatsResponse) error {
	s.responses <- resp
	return nil
}
func (*testSubscribeCallStatsStream) SetHeader(metadata.MD) error  { return nil }
func (*testSubscribeCallStatsStream) SendHeader(metadata.MD) error { return nil }
func (*testSubscribeCallStatsStream) SetTrailer(metadata.MD)       {}
func (*testSubscribeCallStatsStream) SendMsg(any) error            { return nil }
func (*testSubscribeCallStatsStream) RecvMsg(any) error            { return nil }

type gatewayMediaStore struct {
	mu     sync.Mutex
	assets map[string]media.MediaAsset
}

func newGatewayMediaStore() *gatewayMediaStore {
	return &gatewayMediaStore{assets: make(map[string]media.MediaAsset)}
}

func (s *gatewayMediaStore) WithinTx(_ context.Context, fn func(media.Store) error) error {
	return fn(s)
}

func (s *gatewayMediaStore) SaveMediaAsset(_ context.Context, asset media.MediaAsset) (media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.assets == nil {
		s.assets = make(map[string]media.MediaAsset)
	}
	s.assets[asset.ID] = cloneGatewayMediaAsset(asset)
	return cloneGatewayMediaAsset(asset), nil
}

func (s *gatewayMediaStore) MediaAssetByID(_ context.Context, mediaID string) (media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	asset, ok := s.assets[mediaID]
	if !ok {
		return media.MediaAsset{}, media.ErrNotFound
	}
	return cloneGatewayMediaAsset(asset), nil
}

func (s *gatewayMediaStore) MediaAssetsByOwner(_ context.Context, ownerAccountID string, limit int) ([]media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	assets := make([]media.MediaAsset, 0, len(s.assets))
	for _, asset := range s.assets {
		if asset.OwnerAccountID != ownerAccountID {
			continue
		}
		assets = append(assets, cloneGatewayMediaAsset(asset))
	}
	if limit > 0 && len(assets) > limit {
		assets = assets[:limit]
	}

	return assets, nil
}

func (s *gatewayMediaStore) MediaActiveAssetsByOwner(_ context.Context, ownerAccountID string, limit int) ([]media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	assets := make([]media.MediaAsset, 0, len(s.assets))
	for _, asset := range s.assets {
		if asset.OwnerAccountID != ownerAccountID || asset.Status == media.MediaStatusDeleted {
			continue
		}
		assets = append(assets, cloneGatewayMediaAsset(asset))
	}
	if limit > 0 && len(assets) > limit {
		assets = assets[:limit]
	}

	return assets, nil
}

func (s *gatewayMediaStore) MediaAssetByObjectKey(_ context.Context, objectKey string) (media.MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, asset := range s.assets {
		if asset.ObjectKey == objectKey {
			return cloneGatewayMediaAsset(asset), nil
		}
	}

	return media.MediaAsset{}, media.ErrNotFound
}

func (s *gatewayMediaStore) DeleteMediaAsset(_ context.Context, mediaID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.assets[mediaID]; !ok {
		return media.ErrNotFound
	}
	delete(s.assets, mediaID)
	return nil
}

type gatewayBlobStore struct {
	mu      sync.Mutex
	bucket  string
	payload map[string][]byte
}

func newGatewayBlobStore(bucket string) *gatewayBlobStore {
	return &gatewayBlobStore{
		bucket:  bucket,
		payload: make(map[string][]byte),
	}
}

func (*gatewayBlobStore) Name() string                   { return "object" }
func (*gatewayBlobStore) Kind() domainstorage.Kind       { return domainstorage.KindObject }
func (*gatewayBlobStore) Purpose() domainstorage.Purpose { return domainstorage.PurposeObject }
func (*gatewayBlobStore) Capabilities() domainstorage.Capability {
	return domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityBlob
}
func (*gatewayBlobStore) Close(context.Context) error { return nil }
func (s *gatewayBlobStore) Bucket() string            { return s.bucket }

func (s *gatewayBlobStore) PutObject(
	_ context.Context,
	key string,
	body io.Reader,
	_ int64,
	_ domainstorage.PutObjectOptions,
) (domainstorage.BlobObject, error) {
	payload, _ := io.ReadAll(body)
	s.mu.Lock()
	s.payload[key] = append([]byte(nil), payload...)
	s.mu.Unlock()

	return domainstorage.BlobObject{Bucket: s.bucket, Key: key}, nil
}

func (s *gatewayBlobStore) GetObject(_ context.Context, key string) (io.ReadCloser, domainstorage.BlobObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, ok := s.payload[key]
	if !ok {
		return nil, domainstorage.BlobObject{}, domainstorage.ErrNotFound
	}

	return io.NopCloser(bytes.NewReader(payload)), domainstorage.BlobObject{Bucket: s.bucket, Key: key}, nil
}

func (s *gatewayBlobStore) HeadObject(_ context.Context, key string) (domainstorage.BlobObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.payload[key]; !ok {
		return domainstorage.BlobObject{}, domainstorage.ErrNotFound
	}

	return domainstorage.BlobObject{Bucket: s.bucket, Key: key}, nil
}

func (s *gatewayBlobStore) DeleteObject(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.payload, key)
	return nil
}

func (s *gatewayBlobStore) PresignPutObject(
	_ context.Context,
	key string,
	expires time.Duration,
	options domainstorage.PutObjectOptions,
) (domainstorage.PresignedRequest, error) {
	return domainstorage.PresignedRequest{
		URL:       "https://example.invalid/upload/" + key,
		Method:    http.MethodPut,
		Headers:   map[string]string{"content-type": options.ContentType},
		ExpiresAt: time.Now().UTC().Add(expires),
		Bucket:    s.bucket,
		ObjectKey: key,
	}, nil
}

func (s *gatewayBlobStore) PresignGetObject(
	_ context.Context,
	key string,
	expires time.Duration,
) (domainstorage.PresignedRequest, error) {
	return domainstorage.PresignedRequest{
		URL:       "https://example.invalid/download/" + key,
		Method:    http.MethodGet,
		ExpiresAt: time.Now().UTC().Add(expires),
		Bucket:    s.bucket,
		ObjectKey: key,
	}, nil
}

func cloneGatewayMediaAsset(asset media.MediaAsset) media.MediaAsset {
	clone := asset
	if len(asset.Metadata) > 0 {
		clone.Metadata = make(map[string]string, len(asset.Metadata))
		for key, value := range asset.Metadata {
			clone.Metadata[key] = value
		}
	}
	return clone
}
