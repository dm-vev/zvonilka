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
