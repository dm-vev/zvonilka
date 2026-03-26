package bot_test

import (
	"context"
	"io"
	"log/slog"
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

func TestBotDirectUpdatesAndWebhookConflict(t *testing.T) {
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
		domainbot.WithSettings(settings),
	)
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

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "alice",
		DisplayName: "Alice",
		AccountKind: identity.AccountKindUser,
		Email:       "alice@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "helperbot",
		DisplayName: "Helper Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)
	require.NotEmpty(t, botToken)

	conv, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	_, _, err = conversationService.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  conv.ID,
		SenderAccountID: userAccount.ID,
		SenderDeviceID:  "dev-user",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte("hello bot"),
			},
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		updates, err := service.GetUpdates(ctx, domainbot.GetUpdatesParams{
			BotToken: botToken,
		})
		require.NoError(t, err)
		if len(updates) != 1 || updates[0].Message == nil {
			return false
		}
		return updates[0].Message.Text == "hello bot" &&
			updates[0].Message.Chat.Type == domainbot.ChatTypePrivate
	}, time.Second, 20*time.Millisecond)

	info, err := service.SetWebhook(ctx, domainbot.SetWebhookParams{
		BotToken: tokenOrFail(t, botToken),
		URL:      "https://example.org/hook",
	})
	require.NoError(t, err)
	require.Equal(t, "https://example.org/hook", info.URL)
	require.Equal(t, 1, info.PendingUpdateCount)

	_, err = service.GetUpdates(ctx, domainbot.GetUpdatesParams{BotToken: botToken})
	require.ErrorIs(t, err, domainbot.ErrConflict)
}

func TestBotGroupPrivacyMode(t *testing.T) {
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
		domainbot.WithSettings(settings),
	)
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

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bob",
		DisplayName: "Bob",
		AccountKind: identity.AccountKindUser,
		Email:       "bob@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "modbot",
		DisplayName: "Mod Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	group, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindGroup,
		Title:            "ops",
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	_, _, err = conversationService.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  group.ID,
		SenderAccountID: userAccount.ID,
		SenderDeviceID:  "dev-user",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte("plain group chatter"),
			},
		},
	})
	require.NoError(t, err)

	updates, err := service.GetUpdates(ctx, domainbot.GetUpdatesParams{BotToken: botToken})
	require.NoError(t, err)
	require.Empty(t, updates)

	_, _, err = conversationService.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  group.ID,
		SenderAccountID: userAccount.ID,
		SenderDeviceID:  "dev-user",
		Draft: conversation.MessageDraft{
			Kind:              conversation.MessageKindText,
			MentionAccountIDs: []string{botAccount.ID},
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte("hi @modbot"),
			},
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		updates, err = service.GetUpdates(ctx, domainbot.GetUpdatesParams{BotToken: botToken})
		require.NoError(t, err)
		return len(updates) == 1 && updates[0].Message != nil && updates[0].Message.Text == "hi @modbot"
	}, time.Second, 20*time.Millisecond)
}

func tokenOrFail(t *testing.T, token string) string {
	t.Helper()
	require.NotEmpty(t, token)
	return token
}
