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

	editedMedia, err := client.EditMessageMedia(ctx, &telegrambot.EditMessageMediaParams{
		ChatID:    directChatID,
		MessageID: photo.ID,
		Media: &tgmodels.InputMediaDocument{
			Media:   "media-document",
			Caption: "edited media caption",
		},
		ReplyMarkup: &tgmodels.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgmodels.InlineKeyboardButton{{
				{Text: "Edited", CallbackData: "edited"},
			}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, editedMedia.Document)
	require.Equal(t, "media-document", editedMedia.Document.FileID)
	require.Equal(t, "edited media caption", editedMedia.Caption)
	require.NotNil(t, editedMedia.ReplyMarkup)
	require.Equal(t, "Edited", editedMedia.ReplyMarkup.InlineKeyboard[0][0].Text)

	batchFirst, err := client.SendMessage(ctx, &telegrambot.SendMessageParams{
		ChatID: directChatID,
		Text:   "batch one",
	})
	require.NoError(t, err)
	batchSecond, err := client.SendMessage(ctx, &telegrambot.SendMessageParams{
		ChatID: directChatID,
		Text:   "batch two",
	})
	require.NoError(t, err)

	forwardedBatch, err := client.ForwardMessages(ctx, &telegrambot.ForwardMessagesParams{
		ChatID:     groupChatID,
		FromChatID: directChatID,
		MessageIDs: []int{batchFirst.ID, batchSecond.ID},
	})
	require.NoError(t, err)
	require.Len(t, forwardedBatch, 2)
	require.NotZero(t, forwardedBatch[0].ID)
	require.NotZero(t, forwardedBatch[1].ID)

	internalForwardedID, err := world.bot.ResolveMessageID(ctx, strconv.Itoa(forwardedBatch[0].ID))
	require.NoError(t, err)
	forwardedMessage, err := world.bot.GetMessage(ctx, domainbot.GetMessageParams{
		BotToken:  world.botToken,
		ChatID:    world.groupID,
		MessageID: internalForwardedID,
	})
	require.NoError(t, err)
	require.Equal(t, "batch one", forwardedMessage.Text)

	firstMediaCopy, err := client.SendPhoto(ctx, &telegrambot.SendPhotoParams{
		ChatID:  directChatID,
		Photo:   &tgmodels.InputFileString{Data: "media-photo"},
		Caption: "first caption",
	})
	require.NoError(t, err)
	secondMediaCopy, err := client.SendDocument(ctx, &telegrambot.SendDocumentParams{
		ChatID:   directChatID,
		Document: &tgmodels.InputFileString{Data: "media-document"},
		Caption:  "second caption",
	})
	require.NoError(t, err)

	copiedBatch, err := client.CopyMessages(ctx, &telegrambot.CopyMessagesParams{
		ChatID:        groupChatID,
		FromChatID:    directChatID,
		MessageIDs:    []int{firstMediaCopy.ID, secondMediaCopy.ID},
		RemoveCaption: true,
	})
	require.NoError(t, err)
	require.Len(t, copiedBatch, 2)

	internalBatchCopyID, err := world.bot.ResolveMessageID(ctx, strconv.Itoa(copiedBatch[0].ID))
	require.NoError(t, err)
	copiedBatchMessage, err := world.bot.GetMessage(ctx, domainbot.GetMessageParams{
		BotToken:  world.botToken,
		ChatID:    world.groupID,
		MessageID: internalBatchCopyID,
	})
	require.NoError(t, err)
	require.Empty(t, copiedBatchMessage.Caption)

	game, err := client.SendGame(ctx, &telegrambot.SendGameParams{
		ChatID:       directChatID,
		GameShorName: "runner",
	})
	require.NoError(t, err)
	require.NotNil(t, game.Game)
	require.Equal(t, "runner", game.Game.Title)

	ownerUserID := world.publicAccountID(t, world.owner.ID)
	peerUserID := world.publicAccountID(t, world.peer.ID)

	scoredGame, err := client.SetGameScore(ctx, &telegrambot.SetGameScoreParams{
		UserID:    ownerUserID,
		Score:     10,
		ChatID:    directChatID,
		MessageID: game.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, scoredGame.Game)
	require.Equal(t, game.ID, scoredGame.ID)

	_, err = client.SetGameScore(ctx, &telegrambot.SetGameScoreParams{
		UserID:    peerUserID,
		Score:     25,
		ChatID:    directChatID,
		MessageID: game.ID,
	})
	require.NoError(t, err)

	scores, err := client.GetGameHighScores(ctx, &telegrambot.GetGameHighScoresParams{
		UserID:    ownerUserID,
		ChatID:    directChatID,
		MessageID: game.ID,
	})
	require.NoError(t, err)
	require.Len(t, scores, 2)
	require.Equal(t, 1, scores[0].Position)
	require.Equal(t, peerUserID, scores[0].User.ID)
	require.Equal(t, 25, scores[0].Score)
	require.Equal(t, 2, scores[1].Position)
	require.Equal(t, ownerUserID, scores[1].User.ID)
	require.Equal(t, 10, scores[1].Score)
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

func TestGoTelegramClientProfileSuite(t *testing.T) {
	t.Parallel()

	world := newCompatWorld(t)
	server := httptestServer(t, world)
	client := world.client(t, server)
	ctx := context.Background()

	ok, err := client.SetMyName(ctx, &telegrambot.SetMyNameParams{
		Name:         "Помощник",
		LanguageCode: "ru",
	})
	require.NoError(t, err)
	require.True(t, ok)

	name, err := client.GetMyName(ctx, &telegrambot.GetMyNameParams{
		LanguageCode: "ru",
	})
	require.NoError(t, err)
	require.Equal(t, "Помощник", name.Name)

	ok, err = client.SetMyDescription(ctx, &telegrambot.SetMyDescriptionParams{
		Description:  "Long bot description",
		LanguageCode: "en",
	})
	require.NoError(t, err)
	require.True(t, ok)

	description, err := client.GetMyDescription(ctx, &telegrambot.GetMyDescriptionParams{
		LanguageCode: "en",
	})
	require.NoError(t, err)
	require.Equal(t, "Long bot description", description.Description)

	ok, err = client.SetMyShortDescription(ctx, &telegrambot.SetMyShortDescriptionParams{
		ShortDescription: "Short bio",
		LanguageCode:     "en",
	})
	require.NoError(t, err)
	require.True(t, ok)

	shortDescription, err := client.GetMyShortDescription(ctx, &telegrambot.GetMyShortDescriptionParams{
		LanguageCode: "en",
	})
	require.NoError(t, err)
	require.Equal(t, "Short bio", shortDescription.ShortDescription)

	ok, err = client.SetMyDefaultAdministratorRights(ctx, &telegrambot.SetMyDefaultAdministratorRightsParams{
		Rights: &tgmodels.ChatAdministratorRights{
			CanManageChat:     true,
			CanDeleteMessages: true,
			CanPostMessages:   true,
		},
	})
	require.NoError(t, err)
	require.True(t, ok)

	rights, err := client.GetMyDefaultAdministratorRights(ctx, &telegrambot.GetMyDefaultAdministratorRightsParams{})
	require.NoError(t, err)
	require.NotNil(t, rights)
	require.True(t, rights.CanManageChat)
	require.True(t, rights.CanDeleteMessages)
	require.True(t, rights.CanPostMessages)

	ok, err = client.SetMyDefaultAdministratorRights(ctx, &telegrambot.SetMyDefaultAdministratorRightsParams{
		ForChannels: true,
		Rights: &tgmodels.ChatAdministratorRights{
			CanManageChat:   true,
			CanEditMessages: true,
		},
	})
	require.NoError(t, err)
	require.True(t, ok)

	channelRights, err := client.GetMyDefaultAdministratorRights(ctx, &telegrambot.GetMyDefaultAdministratorRightsParams{
		ForChannels: true,
	})
	require.NoError(t, err)
	require.NotNil(t, channelRights)
	require.True(t, channelRights.CanManageChat)
	require.True(t, channelRights.CanEditMessages)
}

func TestGoTelegramClientStructuredEditSuite(t *testing.T) {
	t.Parallel()

	world := newCompatWorld(t)
	server := httptestServer(t, world)
	client := world.client(t, server)
	ctx := context.Background()
	directChatID := world.publicChatID(t, world.directID)

	location, err := client.SendLocation(ctx, &telegrambot.SendLocationParams{
		ChatID:     directChatID,
		Latitude:   55.75,
		Longitude:  37.61,
		LivePeriod: 60,
	})
	require.NoError(t, err)
	require.NotNil(t, location.Location)

	editedLocation, err := client.EditMessageLiveLocation(ctx, &telegrambot.EditMessageLiveLocationParams{
		ChatID:     directChatID,
		MessageID:  location.ID,
		Latitude:   59.93,
		Longitude:  30.31,
		LivePeriod: 120,
	})
	require.NoError(t, err)
	require.NotNil(t, editedLocation.Location)
	require.Equal(t, 59.93, editedLocation.Location.Latitude)
	require.Equal(t, 30.31, editedLocation.Location.Longitude)

	poll, err := client.SendPoll(ctx, &telegrambot.SendPollParams{
		ChatID:   directChatID,
		Question: "Ship it?",
		Options: []tgmodels.InputPollOption{
			{Text: "Yes"},
			{Text: "No"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, poll.Poll)
	require.False(t, poll.Poll.IsClosed)

	stopped, err := client.StopPoll(ctx, &telegrambot.StopPollParams{
		ChatID:    directChatID,
		MessageID: poll.ID,
	})
	require.NoError(t, err)
	require.True(t, stopped.IsClosed)
	require.Equal(t, "Ship it?", stopped.Question)
}

func httptestServer(t *testing.T, world *compatWorld) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(world.boundary.routes())
	t.Cleanup(server.Close)

	return server
}
