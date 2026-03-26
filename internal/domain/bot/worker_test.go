package bot_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
	bottest "github.com/dm-vev/zvonilka/internal/domain/bot/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestWebhookDeliveryAcknowledgesUpdate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)
	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)
	botStore := bottest.NewMemoryStore()
	settings := domainbot.DefaultSettings()
	settings.FanoutPollInterval = 10 * time.Millisecond
	service, err := domainbot.NewService(
		botStore,
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{},
		domainbot.WithSettings(settings),
	)
	require.NoError(t, err)

	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "hookbot",
		DisplayName: "Hook Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		require.Equal(t, "s3cr3t", request.Header.Get("X-Telegram-Bot-Api-Secret-Token"))
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err = service.SetWebhook(ctx, domainbot.SetWebhookParams{
		BotToken:    botToken,
		URL:         server.URL,
		SecretToken: "s3cr3t",
	})
	require.NoError(t, err)

	_, err = botStore.SaveUpdate(ctx, domainbot.QueueEntry{
		BotAccountID: botAccount.ID,
		EventID:      "evt-1",
		UpdateType:   domainbot.UpdateTypeMessage,
		Payload: domainbot.Update{
			Message: &domainbot.Message{
				MessageID: "msg-1",
				Text:      "hello",
			},
		},
	})
	require.NoError(t, err)

	worker, err := domainbot.NewWorker(service, server.Client())
	require.NoError(t, err)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- worker.Run(runCtx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	defer func() {
		cancel()
		<-done
	}()

	require.Eventually(t, func() bool {
		return calls.Load() == 1
	}, time.Second, 20*time.Millisecond)

	count, err := botStore.PendingUpdateCount(ctx, botAccount.ID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestWorkerEnqueuesMyChatMemberAndChatMemberUpdates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)
	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)
	botStore := bottest.NewMemoryStore()
	settings := domainbot.DefaultSettings()
	settings.FanoutPollInterval = 10 * time.Millisecond
	settings.LongPollStep = 10 * time.Millisecond
	service, err := domainbot.NewService(
		botStore,
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{},
		domainbot.WithSettings(settings),
	)
	require.NoError(t, err)

	owner, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "owner",
		DisplayName: "Owner",
		AccountKind: identity.AccountKindUser,
		Email:       "owner@example.org",
	})
	require.NoError(t, err)
	target, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "target",
		DisplayName: "Target",
		AccountKind: identity.AccountKindUser,
		Email:       "target@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "memberbot",
		DisplayName: "Member Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	group, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID: owner.ID,
		Kind:           conversation.ConversationKindGroup,
		Title:          "ops",
	})
	require.NoError(t, err)

	worker, err := domainbot.NewWorker(service, nil)
	require.NoError(t, err)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- worker.Run(runCtx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	defer func() {
		cancel()
		<-done
	}()

	_, err = conversationService.AddMembers(ctx, conversation.AddMembersParams{
		ConversationID:     group.ID,
		ActorAccountID:     owner.ID,
		InvitedByAccountID: owner.ID,
		AccountIDs:         []string{botAccount.ID},
	})
	require.NoError(t, err)

	var offset int64
	require.Eventually(t, func() bool {
		updates, err := service.GetUpdates(ctx, domainbot.GetUpdatesParams{BotToken: botToken, Offset: offset})
		require.NoError(t, err)
		if len(updates) != 1 || updates[0].MyChatMember == nil ||
			updates[0].MyChatMember.NewChatMember.Status != domainbot.MemberStatusMember {
			return false
		}
		offset = updates[0].UpdateID + 1
		return true
	}, time.Second, 20*time.Millisecond)

	_, err = conversationService.UpdateMemberRole(ctx, conversation.UpdateMemberRoleParams{
		ConversationID:  group.ID,
		ActorAccountID:  owner.ID,
		TargetAccountID: botAccount.ID,
		Role:            conversation.MemberRoleAdmin,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		updates, err := service.GetUpdates(ctx, domainbot.GetUpdatesParams{BotToken: botToken, Offset: offset})
		require.NoError(t, err)
		if len(updates) != 1 || updates[0].MyChatMember == nil ||
			updates[0].MyChatMember.OldChatMember.Status != domainbot.MemberStatusMember ||
			updates[0].MyChatMember.NewChatMember.Status != domainbot.MemberStatusAdministrator {
			return false
		}
		offset = updates[0].UpdateID + 1
		return true
	}, time.Second, 20*time.Millisecond)

	_, err = conversationService.AddMembers(ctx, conversation.AddMembersParams{
		ConversationID:     group.ID,
		ActorAccountID:     owner.ID,
		InvitedByAccountID: owner.ID,
		AccountIDs:         []string{target.ID},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		updates, err := service.GetUpdates(ctx, domainbot.GetUpdatesParams{BotToken: botToken, Offset: offset})
		require.NoError(t, err)
		return len(updates) == 1 && updates[0].ChatMember != nil &&
			updates[0].ChatMember.NewChatMember.User.ID == target.ID &&
			updates[0].ChatMember.NewChatMember.Status == domainbot.MemberStatusMember
	}, time.Second, 20*time.Millisecond)
}
