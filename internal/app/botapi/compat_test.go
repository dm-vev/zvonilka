package botapi

import (
	"context"
	"net/http/httptest"
	"strconv"
	"testing"

	telegrambot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/require"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func TestGoTelegramClientUtilitySuite(t *testing.T) {
	t.Parallel()

	world := newCompatWorld(t)
	server := httptestServer(t, world)
	client := world.client(t, server)
	ctx := context.Background()
	directChatID := world.publicChatID(t, world.directID)
	groupChatID := world.publicChatID(t, world.groupID)

	venue, err := client.SendVenue(ctx, &telegrambot.SendVenueParams{
		ChatID:          directChatID,
		Latitude:        55.75,
		Longitude:       37.61,
		Title:           "Office",
		Address:         "Moscow",
		FoursquareID:    "4sq",
		GooglePlaceID:   "gpid",
		GooglePlaceType: "corporate",
	})
	require.NoError(t, err)
	require.NotNil(t, venue.Venue)
	require.Equal(t, "Office", venue.Venue.Title)
	require.Equal(t, "Moscow", venue.Venue.Address)
	require.Equal(t, 55.75, venue.Venue.Location.Latitude)

	dice, err := client.SendDice(ctx, &telegrambot.SendDiceParams{
		ChatID: groupChatID,
		Emoji:  "🎯",
	})
	require.NoError(t, err)
	require.NotNil(t, dice.Dice)
	require.Equal(t, "🎯", dice.Dice.Emoji)
	require.Equal(t, 1, dice.Dice.Value)

	source, err := client.SendMessage(ctx, &telegrambot.SendMessageParams{
		ChatID: directChatID,
		Text:   "forward me",
	})
	require.NoError(t, err)

	forwarded, err := client.ForwardMessage(ctx, &telegrambot.ForwardMessageParams{
		ChatID:     groupChatID,
		FromChatID: directChatID,
		MessageID:  source.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "forward me", forwarded.Text)
	require.Equal(t, groupChatID, forwarded.Chat.ID)

	photo, err := client.SendPhoto(ctx, &telegrambot.SendPhotoParams{
		ChatID:  directChatID,
		Photo:   &tgmodels.InputFileString{Data: "media-photo"},
		Caption: "before caption",
	})
	require.NoError(t, err)
	require.Len(t, photo.Photo, 1)

	editedCaption, err := client.EditMessageCaption(ctx, &telegrambot.EditMessageCaptionParams{
		ChatID:    directChatID,
		MessageID: photo.ID,
		Caption:   "after caption",
		ReplyMarkup: &tgmodels.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgmodels.InlineKeyboardButton{{
				{Text: "Site", URL: "https://example.org"},
			}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "after caption", editedCaption.Caption)
	require.NotNil(t, editedCaption.ReplyMarkup)
	require.Equal(t, "Site", editedCaption.ReplyMarkup.InlineKeyboard[0][0].Text)

	file, err := client.GetFile(ctx, &telegrambot.GetFileParams{
		FileID: photo.Photo[0].FileID,
	})
	require.NoError(t, err)
	require.Equal(t, photo.Photo[0].FileID, file.FileID)
	require.Equal(t, "/media/"+photo.Photo[0].FileID+"/download", file.FilePath)

	message, err := client.SendMessage(ctx, &telegrambot.SendMessageParams{
		ChatID: groupChatID,
		Text:   "markup source",
		ReplyMarkup: &tgmodels.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgmodels.InlineKeyboardButton{{
				{Text: "Old", CallbackData: "old"},
			}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, message.ReplyMarkup)

	editedMarkup, err := client.EditMessageReplyMarkup(ctx, &telegrambot.EditMessageReplyMarkupParams{
		ChatID:    groupChatID,
		MessageID: message.ID,
		ReplyMarkup: &tgmodels.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgmodels.InlineKeyboardButton{{
				{Text: "New", CallbackData: "new"},
			}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, editedMarkup.ReplyMarkup)
	require.Equal(t, "New", editedMarkup.ReplyMarkup.InlineKeyboard[0][0].Text)
	require.Equal(t, "new", editedMarkup.ReplyMarkup.InlineKeyboard[0][0].CallbackData)

	copied, err := client.CopyMessage(ctx, &telegrambot.CopyMessageParams{
		ChatID:      groupChatID,
		FromChatID:  directChatID,
		MessageID:   photo.ID,
		Caption:     "copy caption",
		ReplyMarkup: &tgmodels.InlineKeyboardMarkup{InlineKeyboard: [][]tgmodels.InlineKeyboardButton{{{Text: "Copy", CallbackData: "copy"}}}},
	})
	require.NoError(t, err)
	require.NotZero(t, copied.ID)

	internalCopiedID, err := world.bot.ResolveMessageID(ctx, strconv.Itoa(copied.ID))
	require.NoError(t, err)
	copiedMessage, err := world.bot.GetMessage(ctx, domainbot.GetMessageParams{
		BotToken:  world.botToken,
		ChatID:    world.groupID,
		MessageID: internalCopiedID,
	})
	require.NoError(t, err)
	require.Equal(t, "copy caption", copiedMessage.Caption)
	require.NotNil(t, copiedMessage.ReplyMarkup)
	require.Equal(t, "Copy", copiedMessage.ReplyMarkup.InlineKeyboard[0][0].Text)
}

func TestGoTelegramClientCommandAndMenuSuite(t *testing.T) {
	t.Parallel()

	world := newCompatWorld(t)
	server := httptestServer(t, world)
	client := world.client(t, server)
	ctx := context.Background()
	directChatID := world.publicChatID(t, world.directID)

	commands := []tgmodels.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "help", Description: "Show help"},
	}
	ok, err := client.SetMyCommands(ctx, &telegrambot.SetMyCommandsParams{
		Commands: commands,
	})
	require.NoError(t, err)
	require.True(t, ok)

	gotCommands, err := client.GetMyCommands(ctx, &telegrambot.GetMyCommandsParams{})
	require.NoError(t, err)
	require.Equal(t, commands, gotCommands)

	chatScoped := []tgmodels.BotCommand{
		{Command: "topic", Description: "Chat only"},
	}
	ok, err = client.SetMyCommands(ctx, &telegrambot.SetMyCommandsParams{
		Commands: chatScoped,
		Scope:    &tgmodels.BotCommandScopeChat{ChatID: directChatID},
	})
	require.NoError(t, err)
	require.True(t, ok)

	gotScoped, err := client.GetMyCommands(ctx, &telegrambot.GetMyCommandsParams{
		Scope: &tgmodels.BotCommandScopeChat{ChatID: directChatID},
	})
	require.NoError(t, err)
	require.Equal(t, chatScoped, gotScoped)

	ok, err = client.DeleteMyCommands(ctx, &telegrambot.DeleteMyCommandsParams{
		Scope: &tgmodels.BotCommandScopeChat{ChatID: directChatID},
	})
	require.NoError(t, err)
	require.True(t, ok)

	gotScoped, err = client.GetMyCommands(ctx, &telegrambot.GetMyCommandsParams{
		Scope: &tgmodels.BotCommandScopeChat{ChatID: directChatID},
	})
	require.NoError(t, err)
	require.Empty(t, gotScoped)

	ok, err = client.SetChatMenuButton(ctx, &telegrambot.SetChatMenuButtonParams{
		MenuButton: &tgmodels.MenuButtonCommands{Type: tgmodels.MenuButtonTypeCommands},
	})
	require.NoError(t, err)
	require.True(t, ok)

	defaultMenu, err := client.GetChatMenuButton(ctx, &telegrambot.GetChatMenuButtonParams{})
	require.NoError(t, err)
	require.Equal(t, tgmodels.MenuButtonTypeCommands, defaultMenu.Type)
	require.NotNil(t, defaultMenu.Commands)

	ok, err = client.SetChatMenuButton(ctx, &telegrambot.SetChatMenuButtonParams{
		ChatID: directChatID,
		MenuButton: &tgmodels.MenuButtonWebApp{
			Type: tgmodels.MenuButtonTypeWebApp,
			Text: "Open",
			WebApp: tgmodels.WebAppInfo{
				URL: "https://example.org/app",
			},
		},
	})
	require.NoError(t, err)
	require.True(t, ok)

	chatMenu, err := client.GetChatMenuButton(ctx, &telegrambot.GetChatMenuButtonParams{
		ChatID: directChatID,
	})
	require.NoError(t, err)
	require.Equal(t, tgmodels.MenuButtonTypeWebApp, chatMenu.Type)
	require.NotNil(t, chatMenu.WebApp)
	require.Equal(t, "Open", chatMenu.WebApp.Text)
	require.Equal(t, "https://example.org/app", chatMenu.WebApp.WebApp.URL)
}

func httptestServer(t *testing.T, world *compatWorld) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(world.boundary.routes())
	t.Cleanup(server.Close)

	return server
}
