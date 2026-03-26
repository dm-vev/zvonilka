package bot_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
	bottest "github.com/dm-vev/zvonilka/internal/domain/bot/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestBotProfileAndRightsLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)

	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	_, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "profilebot",
		DisplayName: "Profile Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	service, err := domainbot.NewService(
		bottest.NewMemoryStore(),
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{},
	)
	require.NoError(t, err)

	name, err := service.GetMyName(ctx, domainbot.GetNameParams{BotToken: botToken})
	require.NoError(t, err)
	require.Equal(t, "Profile Bot", name)

	err = service.SetMyName(ctx, domainbot.SetNameParams{
		BotToken:     botToken,
		LanguageCode: "ru",
		Name:         "Помощник",
	})
	require.NoError(t, err)

	name, err = service.GetMyName(ctx, domainbot.GetNameParams{
		BotToken:     botToken,
		LanguageCode: "ru",
	})
	require.NoError(t, err)
	require.Equal(t, "Помощник", name)

	description, err := service.GetMyDescription(ctx, domainbot.GetDescriptionParams{BotToken: botToken})
	require.NoError(t, err)
	require.Empty(t, description)

	err = service.SetMyDescription(ctx, domainbot.SetDescriptionParams{
		BotToken:    botToken,
		Description: "Bot description",
	})
	require.NoError(t, err)

	description, err = service.GetMyDescription(ctx, domainbot.GetDescriptionParams{BotToken: botToken})
	require.NoError(t, err)
	require.Equal(t, "Bot description", description)

	err = service.SetMyDefaultAdministratorRights(ctx, domainbot.SetRightsParams{
		BotToken:    botToken,
		ForChannels: true,
		Rights: &domainbot.AdminRights{
			CanManageChat:   true,
			CanPostMessages: true,
		},
	})
	require.NoError(t, err)

	rights, err := service.GetMyDefaultAdministratorRights(ctx, domainbot.GetRightsParams{
		BotToken:    botToken,
		ForChannels: true,
	})
	require.NoError(t, err)
	require.True(t, rights.CanPostMessages)
}
