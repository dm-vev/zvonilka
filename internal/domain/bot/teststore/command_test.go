package teststore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func TestCommandAndMenuRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, time.March, 26, 15, 0, 0, 0, time.UTC)

	savedCommands, err := store.SaveCommands(ctx, bot.CommandSet{
		BotAccountID: "bot-1",
		Scope: bot.CommandScope{
			Type:   bot.CommandScopeChat,
			ChatID: "conv-1",
		},
		LanguageCode: "EN",
		Commands: []bot.Command{
			{Command: "start", Description: "Start"},
			{Command: "help", Description: "Help"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, "en", savedCommands.LanguageCode)
	require.Len(t, savedCommands.Commands, 2)

	loadedCommands, err := store.CommandsByScope(ctx, "bot-1", bot.CommandScope{
		Type:   bot.CommandScopeChat,
		ChatID: "conv-1",
	}, "en")
	require.NoError(t, err)
	require.Equal(t, savedCommands.Commands, loadedCommands.Commands)

	err = store.DeleteCommands(ctx, "bot-1", bot.CommandScope{
		Type:   bot.CommandScopeChat,
		ChatID: "conv-1",
	}, "en")
	require.NoError(t, err)

	_, err = store.CommandsByScope(ctx, "bot-1", bot.CommandScope{
		Type:   bot.CommandScopeChat,
		ChatID: "conv-1",
	}, "en")
	require.ErrorIs(t, err, bot.ErrNotFound)

	savedMenu, err := store.SaveMenu(ctx, bot.MenuState{
		BotAccountID: "bot-1",
		ChatID:       "conv-1",
		Button: bot.MenuButton{
			Type:      bot.MenuButtonWebApp,
			Text:      "Open",
			WebAppURL: "https://example.org/app",
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, bot.MenuButtonWebApp, savedMenu.Button.Type)

	loadedMenu, err := store.MenuByChat(ctx, "bot-1", "conv-1")
	require.NoError(t, err)
	require.Equal(t, savedMenu.Button, loadedMenu.Button)
}
