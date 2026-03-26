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
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
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
		mediaFixture{},
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
		mediaFixture{},
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

func TestBotSendPhoto(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)

	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "carol",
		DisplayName: "Carol",
		AccountKind: identity.AccountKindUser,
		Email:       "carol@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "photobot",
		DisplayName: "Photo Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	group, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	service, err := domainbot.NewService(
		bottest.NewMemoryStore(),
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{
			assets: map[string]domainmedia.MediaAsset{
				"media-photo": {
					ID:             "media-photo",
					OwnerAccountID: botAccount.ID,
					Kind:           domainmedia.MediaKindImage,
					Status:         domainmedia.MediaStatusReady,
					FileName:       "photo.jpg",
					ContentType:    "image/jpeg",
					SizeBytes:      1024,
					Width:          640,
					Height:         480,
				},
			},
		},
	)
	require.NoError(t, err)

	message, err := service.SendPhoto(ctx, domainbot.SendPhotoParams{
		BotToken: botToken,
		ChatID:   group.ID,
		MediaID:  "media-photo",
		Caption:  "hello photo",
	})
	require.NoError(t, err)
	require.Equal(t, "hello photo", message.Caption)
	require.Len(t, message.Photo, 1)
	require.Equal(t, "media-photo", message.Photo[0].FileID)
	require.Equal(t, uint32(640), message.Photo[0].Width)
	require.Empty(t, message.Text)
}

func TestBotSendAnimationAudioAndVideoNote(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)

	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "maya",
		DisplayName: "Maya",
		AccountKind: identity.AccountKindUser,
		Email:       "maya@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "mediabot",
		DisplayName: "Media Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	chat, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	service, err := domainbot.NewService(
		bottest.NewMemoryStore(),
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{
			assets: map[string]domainmedia.MediaAsset{
				"media-animation": {
					ID:             "media-animation",
					OwnerAccountID: botAccount.ID,
					Kind:           domainmedia.MediaKindGIF,
					Status:         domainmedia.MediaStatusReady,
					FileName:       "clip.gif",
					ContentType:    "image/gif",
					SizeBytes:      4096,
					Width:          320,
					Height:         240,
					Duration:       3 * time.Second,
				},
				"media-audio": {
					ID:             "media-audio",
					OwnerAccountID: botAccount.ID,
					Kind:           domainmedia.MediaKindFile,
					Status:         domainmedia.MediaStatusReady,
					FileName:       "track.mp3",
					ContentType:    "audio/mpeg",
					SizeBytes:      8192,
					Duration:       12 * time.Second,
				},
				"media-video-note": {
					ID:             "media-video-note",
					OwnerAccountID: botAccount.ID,
					Kind:           domainmedia.MediaKindVideo,
					Status:         domainmedia.MediaStatusReady,
					FileName:       "note.mp4",
					ContentType:    "video/mp4",
					SizeBytes:      6144,
					Width:          240,
					Height:         240,
					Duration:       7 * time.Second,
				},
			},
		},
	)
	require.NoError(t, err)

	animation, err := service.SendAnimation(ctx, domainbot.SendAnimationParams{
		BotToken: botToken,
		ChatID:   chat.ID,
		MediaID:  "media-animation",
		Caption:  "loop",
	})
	require.NoError(t, err)
	require.NotNil(t, animation.Animation)
	require.Equal(t, "media-animation", animation.Animation.FileID)
	require.Equal(t, "loop", animation.Caption)
	require.Nil(t, animation.Video)

	audio, err := service.SendAudio(ctx, domainbot.SendAudioParams{
		BotToken: botToken,
		ChatID:   chat.ID,
		MediaID:  "media-audio",
		Caption:  "track",
	})
	require.NoError(t, err)
	require.NotNil(t, audio.Audio)
	require.Equal(t, "media-audio", audio.Audio.FileID)
	require.Equal(t, "track", audio.Caption)
	require.NotZero(t, audio.Audio.Duration)
	require.Nil(t, audio.Document)

	videoNote, err := service.SendVideoNote(ctx, domainbot.SendVideoNoteParams{
		BotToken: botToken,
		ChatID:   chat.ID,
		MediaID:  "media-video-note",
	})
	require.NoError(t, err)
	require.NotNil(t, videoNote.VideoNote)
	require.Equal(t, "media-video-note", videoNote.VideoNote.FileID)
	require.Equal(t, uint32(240), videoNote.VideoNote.Length)
	require.Nil(t, videoNote.Video)
}

func TestBotSendLocationContactAndPoll(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)

	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "nora",
		DisplayName: "Nora",
		AccountKind: identity.AccountKindUser,
		Email:       "nora@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "structbot",
		DisplayName: "Struct Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	chat, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
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

	location, err := service.SendLocation(ctx, domainbot.SendLocationParams{
		BotToken:  botToken,
		ChatID:    chat.ID,
		Latitude:  55.7522,
		Longitude: 37.6156,
	})
	require.NoError(t, err)
	require.NotNil(t, location.Location)
	require.InDelta(t, 55.7522, location.Location.Latitude, 0.000001)
	require.InDelta(t, 37.6156, location.Location.Longitude, 0.000001)
	require.Empty(t, location.Text)

	contact, err := service.SendContact(ctx, domainbot.SendContactParams{
		BotToken:    botToken,
		ChatID:      chat.ID,
		PhoneNumber: "+79990001122",
		FirstName:   "Pavel",
		LastName:    "Ivanov",
	})
	require.NoError(t, err)
	require.NotNil(t, contact.Contact)
	require.Equal(t, "+79990001122", contact.Contact.PhoneNumber)
	require.Equal(t, "Pavel", contact.Contact.FirstName)
	require.Empty(t, contact.Text)

	poll, err := service.SendPoll(ctx, domainbot.SendPollParams{
		BotToken: botToken,
		ChatID:   chat.ID,
		Question: "ship it?",
		Options:  []string{"yes", "no"},
	})
	require.NoError(t, err)
	require.NotNil(t, poll.Poll)
	require.Equal(t, "ship it?", poll.Poll.Question)
	require.Len(t, poll.Poll.Options, 2)
	require.Equal(t, "yes", poll.Poll.Options[0].Text)
	require.Empty(t, poll.Text)
}

func TestBotCallbackQueryLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)

	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "erin",
		DisplayName: "Erin",
		AccountKind: identity.AccountKindUser,
		Email:       "erin@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "buttonsbot",
		DisplayName: "Buttons Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	conv, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
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

	message, err := service.SendMessage(ctx, domainbot.SendMessageParams{
		BotToken: botToken,
		ChatID:   conv.ID,
		Text:     "pick one",
		ReplyMarkup: &domainbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]domainbot.InlineKeyboardButton{{
				{Text: "OK", CallbackData: "ok"},
			}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, message.ReplyMarkup)

	callback, err := service.TriggerCallbackQuery(ctx, domainbot.TriggerCallbackParams{
		ConversationID: conv.ID,
		MessageID:      message.MessageID,
		FromAccountID:  userAccount.ID,
		Data:           "ok",
	})
	require.NoError(t, err)
	require.Equal(t, "ok", callback.Data)

	updates, err := service.GetUpdates(ctx, domainbot.GetUpdatesParams{BotToken: botToken})
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.NotNil(t, updates[0].CallbackQuery)
	require.Equal(t, callback.ID, updates[0].CallbackQuery.ID)
	require.Equal(t, "ok", updates[0].CallbackQuery.Data)
	require.NotNil(t, updates[0].CallbackQuery.Message)
	require.NotNil(t, updates[0].CallbackQuery.Message.ReplyMarkup)

	err = service.AnswerCallbackQuery(ctx, domainbot.AnswerCallbackQueryParams{
		BotToken:        botToken,
		CallbackQueryID: callback.ID,
		Text:            "done",
	})
	require.NoError(t, err)
}

func TestBotInlineQueryLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)

	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "rita",
		DisplayName: "Rita",
		AccountKind: identity.AccountKindUser,
		Email:       "rita@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "inlinebot",
		DisplayName: "Inline Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	botStore := bottest.NewMemoryStore()
	service, err := domainbot.NewService(
		botStore,
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{},
	)
	require.NoError(t, err)

	query, err := service.TriggerInlineQuery(ctx, domainbot.TriggerInlineQueryParams{
		BotAccountID:  botAccount.ID,
		FromAccountID: userAccount.ID,
		Query:         "help",
		Offset:        "0",
		ChatType:      "private",
	})
	require.NoError(t, err)

	updates, err := service.GetUpdates(ctx, domainbot.GetUpdatesParams{BotToken: botToken})
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.NotNil(t, updates[0].InlineQuery)
	require.Equal(t, query.ID, updates[0].InlineQuery.ID)
	require.Equal(t, "help", updates[0].InlineQuery.Query)

	err = service.AnswerInlineQuery(ctx, domainbot.AnswerInlineQueryParams{
		BotToken:      botToken,
		InlineQueryID: query.ID,
		Results: []domainbot.InlineQueryResult{{
			Type:        "article",
			ID:          "article-1",
			Title:       "Answer",
			Description: "Inline answer",
			InputMessageContent: &domainbot.InputTextMessageContent{
				MessageText: "hello from inline",
			},
		}},
		CacheTime:  30,
		IsPersonal: true,
		NextOffset: "next",
	})
	require.NoError(t, err)

	state, err := botStore.InlineQueryByID(ctx, query.ID)
	require.NoError(t, err)
	require.True(t, state.Answered)
	require.Equal(t, 30, state.CacheTime)
	require.True(t, state.IsPersonal)
	require.Equal(t, "next", state.NextOffset)
	require.Len(t, state.Results, 1)
	require.NotNil(t, state.Results[0].InputMessageContent)
	require.Equal(t, "hello from inline", state.Results[0].InputMessageContent.MessageText)
}

func TestBotChosenInlineResultLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)

	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "sasha",
		DisplayName: "Sasha",
		AccountKind: identity.AccountKindUser,
		Email:       "sasha@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "chosenbot",
		DisplayName: "Chosen Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	botStore := bottest.NewMemoryStore()
	service, err := domainbot.NewService(
		botStore,
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{},
	)
	require.NoError(t, err)

	query, err := service.TriggerInlineQuery(ctx, domainbot.TriggerInlineQueryParams{
		BotAccountID:  botAccount.ID,
		FromAccountID: userAccount.ID,
		Query:         "choose",
	})
	require.NoError(t, err)

	err = service.AnswerInlineQuery(ctx, domainbot.AnswerInlineQueryParams{
		BotToken:      botToken,
		InlineQueryID: query.ID,
		Results: []domainbot.InlineQueryResult{
			{
				Type:        "article",
				ID:          "article-1",
				Title:       "Article",
				Description: "Article result",
				InputMessageContent: &domainbot.InputTextMessageContent{
					MessageText: "article body",
				},
			},
			{
				Type:     "photo",
				ID:       "photo-1",
				PhotoURL: "https://cdn.example.org/photo.jpg",
				ThumbURL: "https://cdn.example.org/thumb.jpg",
			},
		},
	})
	require.NoError(t, err)

	chosen, err := service.TriggerChosenInlineResult(ctx, domainbot.TriggerChosenInlineResultParams{
		InlineQueryID: query.ID,
		FromAccountID: userAccount.ID,
		ResultID:      "photo-1",
	})
	require.NoError(t, err)
	require.Equal(t, "photo-1", chosen.ResultID)

	updates, err := service.GetUpdates(ctx, domainbot.GetUpdatesParams{BotToken: botToken, Offset: 2})
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.NotNil(t, updates[0].ChosenInlineResult)
	require.Equal(t, "photo-1", updates[0].ChosenInlineResult.ResultID)
	require.Equal(t, "choose", updates[0].ChosenInlineResult.Query)
}

type mediaFixture struct {
	assets map[string]domainmedia.MediaAsset
}

func (f mediaFixture) MediaAssetByID(_ context.Context, mediaID string) (domainmedia.MediaAsset, error) {
	if asset, ok := f.assets[mediaID]; ok {
		return asset, nil
	}

	return domainmedia.MediaAsset{}, domainmedia.ErrNotFound
}
