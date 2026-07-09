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
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
)

func TestGetFileRejectsForeignAsset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)

	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	owner, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "ownerbot",
		DisplayName: "Owner Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)
	other, otherToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "otherbot",
		DisplayName: "Other Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	service, err := domainbot.NewService(
		bottest.NewMemoryStore(),
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{
			assets: map[string]domainmedia.MediaAsset{
				"foreign-file": {
					ID:             "foreign-file",
					OwnerAccountID: owner.ID,
					Kind:           domainmedia.MediaKindDocument,
					Status:         domainmedia.MediaStatusReady,
					SizeBytes:      128,
				},
			},
		},
	)
	require.NoError(t, err)

	_, err = service.GetFile(ctx, domainbot.GetFileParams{
		BotToken: otherToken,
		FileID:   "foreign-file",
	})
	require.ErrorIs(t, err, domainbot.ErrNotFound)

	_ = other
}
