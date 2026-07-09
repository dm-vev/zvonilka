package pgstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func TestSaveCommandsRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 15, 0, 0, 0, time.UTC)
	set, err := bot.NormalizeCommandSet(bot.CommandSet{
		BotAccountID: "acc-bot",
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
	}, now)
	require.NoError(t, err)

	raw, err := json.Marshal(set.Commands)
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "bot"\."bot_commands".*RETURNING bot_account_id, scope_type, scope_chat_id, scope_user_id, language_code, commands, created_at, updated_at`).
		WithArgs(
			"acc-bot",
			bot.CommandScopeChat,
			"conv-1",
			"",
			"en",
			raw,
			now.UTC(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"bot_account_id",
			"scope_type",
			"scope_chat_id",
			"scope_user_id",
			"language_code",
			"commands",
			"created_at",
			"updated_at",
		}).AddRow(
			"acc-bot",
			bot.CommandScopeChat,
			"conv-1",
			"",
			"en",
			raw,
			now.UTC(),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveCommands(context.Background(), set)
	require.NoError(t, err)
	require.Equal(t, "en", saved.LanguageCode)
	require.Len(t, saved.Commands, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveMenuRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 15, 0, 0, 0, time.UTC)
	state, err := bot.NormalizeMenuState(bot.MenuState{
		BotAccountID: "acc-bot",
		ChatID:       "conv-1",
		Button: bot.MenuButton{
			Type:      bot.MenuButtonWebApp,
			Text:      "Open",
			WebAppURL: "https://example.org/app",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}, now)
	require.NoError(t, err)

	raw, err := json.Marshal(state.Button)
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "bot"\."bot_menu_buttons".*RETURNING bot_account_id, chat_id, button, created_at, updated_at`).
		WithArgs(
			"acc-bot",
			"conv-1",
			raw,
			now.UTC(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"bot_account_id",
			"chat_id",
			"button",
			"created_at",
			"updated_at",
		}).AddRow(
			"acc-bot",
			"conv-1",
			raw,
			now.UTC(),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveMenu(context.Background(), state)
	require.NoError(t, err)
	require.Equal(t, bot.MenuButtonWebApp, saved.Button.Type)
	require.Equal(t, "https://example.org/app", saved.Button.WebAppURL)
	require.NoError(t, mock.ExpectationsWereMet())
}
