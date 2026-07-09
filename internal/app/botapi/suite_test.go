package botapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	telegrambot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	"github.com/stretchr/testify/require"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
	bottest "github.com/dm-vev/zvonilka/internal/domain/bot/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identitytest "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
)

func TestGoTelegramClientCoreSuite(t *testing.T) {
	t.Parallel()

	world := newCompatWorld(t)
	server := httptest.NewServer(world.boundary.routes())
	defer server.Close()

	client := world.client(t, server)
	ctx := context.Background()

	me, err := client.GetMe(ctx)
	require.NoError(t, err)
	require.NotZero(t, me.ID)
	require.Equal(t, "compatbot", me.Username)

	ok, err := client.SetWebhook(ctx, &telegrambot.SetWebhookParams{
		URL:                "https://example.org/hook",
		AllowedUpdates:     []string{"message", "callback_query", "inline_query"},
		DropPendingUpdates: true,
		SecretToken:        "secret",
	})
	require.NoError(t, err)
	require.True(t, ok)

	info, err := client.GetWebhookInfo(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://example.org/hook", info.URL)
	require.Contains(t, info.AllowedUpdates, "message")
	require.Contains(t, info.AllowedUpdates, "callback_query")
	require.Contains(t, info.AllowedUpdates, "inline_query")

	ok, err = client.DeleteWebhook(ctx, &telegrambot.DeleteWebhookParams{DropPendingUpdates: true})
	require.NoError(t, err)
	require.True(t, ok)

	groupChatID := world.publicChatID(t, world.groupID)
	topicID := world.publicTopicID(t, world.topicID)
	peerUserID := world.publicAccountID(t, world.peer.ID)

	first, err := client.SendMessage(ctx, &telegrambot.SendMessageParams{
		ChatID:          groupChatID,
		MessageThreadID: int(topicID),
		Text:            "hello from suite",
		LinkPreviewOptions: &tgmodels.LinkPreviewOptions{
			IsDisabled: telegrambot.True(),
		},
		ReplyMarkup: &tgmodels.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgmodels.InlineKeyboardButton{{
				{Text: "Tap", CallbackData: "tap"},
			}},
		},
	})
	require.NoError(t, err)
	require.NotZero(t, first.ID)
	require.Equal(t, "hello from suite", first.Text)
	require.Equal(t, groupChatID, first.Chat.ID)
	require.Equal(t, int(topicID), first.MessageThreadID)

	reply, err := client.SendMessage(ctx, &telegrambot.SendMessageParams{
		ChatID:          groupChatID,
		MessageThreadID: int(topicID),
		Text:            "reply from suite",
		ReplyParameters: &tgmodels.ReplyParameters{
			MessageID: first.ID,
		},
	})
	require.NoError(t, err)
	require.NotZero(t, reply.ID)
	require.NotNil(t, reply.ReplyToMessage)
	require.Equal(t, first.ID, reply.ReplyToMessage.ID)

	edited, err := client.EditMessageText(ctx, &telegrambot.EditMessageTextParams{
		ChatID:    groupChatID,
		MessageID: first.ID,
		Text:      "hello edited",
		LinkPreviewOptions: &tgmodels.LinkPreviewOptions{
			IsDisabled: telegrambot.True(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, edited.ID)
	require.Equal(t, "hello edited", edited.Text)

	chat, err := client.GetChat(ctx, &telegrambot.GetChatParams{ChatID: groupChatID})
	require.NoError(t, err)
	require.Equal(t, groupChatID, chat.ID)
	require.Equal(t, tgmodels.ChatTypeSupergroup, chat.Type)
	require.True(t, chat.IsForum)

	member, err := client.GetChatMember(ctx, &telegrambot.GetChatMemberParams{
		ChatID: groupChatID,
		UserID: peerUserID,
	})
	require.NoError(t, err)
	require.Equal(t, tgmodels.ChatMemberTypeMember, member.Type)
	require.NotNil(t, member.Member)
	require.NotNil(t, member.Member.User)
	require.Equal(t, "compatpeer", member.Member.User.Username)

	internalMessageID, err := world.bot.ResolveMessageID(ctx, intString(first.ID))
	require.NoError(t, err)

	callback, err := world.bot.TriggerCallbackQuery(ctx, domainbot.TriggerCallbackParams{
		ConversationID: world.groupID,
		MessageID:      internalMessageID,
		FromAccountID:  world.owner.ID,
		Data:           "tap",
	})
	require.NoError(t, err)

	ok, err = client.AnswerCallbackQuery(ctx, &telegrambot.AnswerCallbackQueryParams{
		CallbackQueryID: callback.ID,
		Text:            "done",
	})
	require.NoError(t, err)
	require.True(t, ok)

	query, err := world.bot.TriggerInlineQuery(ctx, domainbot.TriggerInlineQueryParams{
		BotAccountID:  world.botAccount.ID,
		FromAccountID: world.owner.ID,
		Query:         "docs",
	})
	require.NoError(t, err)

	ok, err = client.AnswerInlineQuery(ctx, &telegrambot.AnswerInlineQueryParams{
		InlineQueryID: query.ID,
		Results: []tgmodels.InlineQueryResult{
			&tgmodels.InlineQueryResultArticle{
				ID:    "article-1",
				Title: "Article",
				InputMessageContent: tgmodels.InputTextMessageContent{
					MessageText: "article text",
				},
			},
			&tgmodels.InlineQueryResultPhoto{
				ID:           "photo-1",
				PhotoURL:     "https://cdn.example.org/photo.jpg",
				ThumbnailURL: "https://cdn.example.org/thumb.jpg",
				Title:        "Photo",
			},
			&tgmodels.InlineQueryResultDocument{
				ID:           "document-1",
				Title:        "Document",
				DocumentURL:  "https://cdn.example.org/report.pdf",
				MimeType:     "application/pdf",
				ThumbnailURL: "https://cdn.example.org/report-thumb.jpg",
			},
			&tgmodels.InlineQueryResultVideo{
				ID:           "video-1",
				Title:        "Video",
				VideoURL:     "https://cdn.example.org/clip.mp4",
				MimeType:     "video/mp4",
				ThumbnailURL: "https://cdn.example.org/clip-thumb.jpg",
			},
			&tgmodels.InlineQueryResultAudio{
				ID:       "audio-1",
				Title:    "Audio",
				AudioURL: "https://cdn.example.org/track.mp3",
			},
			&tgmodels.InlineQueryResultGif{
				ID:           "gif-1",
				GifURL:       "https://cdn.example.org/loop.gif",
				ThumbnailURL: "https://cdn.example.org/loop-thumb.jpg",
			},
			&tgmodels.InlineQueryResultMpeg4Gif{
				ID:           "mpeg4-1",
				Mpeg4URL:     "https://cdn.example.org/loop.mp4",
				ThumbnailURL: "https://cdn.example.org/loop-mp4-thumb.jpg",
			},
		},
	})
	require.NoError(t, err)
	require.True(t, ok)

	state, err := world.store.InlineQueryByID(ctx, query.ID)
	require.NoError(t, err)
	require.True(t, state.Answered)
	require.Len(t, state.Results, 7)

	ok, err = client.DeleteMessage(ctx, &telegrambot.DeleteMessageParams{
		ChatID:    groupChatID,
		MessageID: reply.ID,
	})
	require.NoError(t, err)
	require.True(t, ok)
}

func TestGoTelegramClientMediaSuite(t *testing.T) {
	t.Parallel()

	world := newCompatWorld(t)
	server := httptest.NewServer(world.boundary.routes())
	defer server.Close()

	client := world.client(t, server)
	ctx := context.Background()
	directChatID := world.publicChatID(t, world.directID)

	cases := []struct {
		name   string
		send   func(context.Context, *telegrambot.Bot, int64) (*tgmodels.Message, error)
		assert func(*testing.T, *tgmodels.Message)
	}{
		{
			name: "sendPhoto",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendPhoto(ctx, &telegrambot.SendPhotoParams{
					ChatID:  chatID,
					Photo:   &tgmodels.InputFileString{Data: "media-photo"},
					Caption: "photo caption",
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.Len(t, message.Photo, 1)
				require.Equal(t, "media-photo", message.Photo[0].FileID)
				require.Equal(t, "photo caption", message.Caption)
			},
		},
		{
			name: "sendDocument",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendDocument(ctx, &telegrambot.SendDocumentParams{
					ChatID:   chatID,
					Document: &tgmodels.InputFileString{Data: "media-document"},
					Caption:  "document caption",
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Document)
				require.Equal(t, "media-document", message.Document.FileID)
				require.Equal(t, "document caption", message.Caption)
			},
		},
		{
			name: "sendVideo",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendVideo(ctx, &telegrambot.SendVideoParams{
					ChatID:  chatID,
					Video:   &tgmodels.InputFileString{Data: "media-video"},
					Caption: "video caption",
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Video)
				require.Equal(t, "media-video", message.Video.FileID)
				require.Equal(t, "video caption", message.Caption)
			},
		},
		{
			name: "sendAnimation",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendAnimation(ctx, &telegrambot.SendAnimationParams{
					ChatID:    chatID,
					Animation: &tgmodels.InputFileString{Data: "media-animation"},
					Caption:   "animation caption",
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Animation)
				require.Equal(t, "media-animation", message.Animation.FileID)
				require.Equal(t, "animation caption", message.Caption)
			},
		},
		{
			name: "sendAudio",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendAudio(ctx, &telegrambot.SendAudioParams{
					ChatID:  chatID,
					Audio:   &tgmodels.InputFileString{Data: "media-audio"},
					Caption: "audio caption",
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Audio)
				require.Equal(t, "media-audio", message.Audio.FileID)
				require.Equal(t, "audio caption", message.Caption)
			},
		},
		{
			name: "sendVideoNote",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendVideoNote(ctx, &telegrambot.SendVideoNoteParams{
					ChatID:    chatID,
					VideoNote: &tgmodels.InputFileString{Data: "media-video-note"},
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.VideoNote)
				require.Equal(t, "media-video-note", message.VideoNote.FileID)
			},
		},
		{
			name: "sendVoice",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendVoice(ctx, &telegrambot.SendVoiceParams{
					ChatID:  chatID,
					Voice:   &tgmodels.InputFileString{Data: "media-voice"},
					Caption: "voice caption",
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Voice)
				require.Equal(t, "media-voice", message.Voice.FileID)
				require.Equal(t, "voice caption", message.Caption)
			},
		},
		{
			name: "sendSticker",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendSticker(ctx, &telegrambot.SendStickerParams{
					ChatID:  chatID,
					Sticker: &tgmodels.InputFileString{Data: "media-sticker"},
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Sticker)
				require.Equal(t, "media-sticker", message.Sticker.FileID)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			message, err := tc.send(ctx, client, directChatID)
			require.NoError(t, err)
			require.NotNil(t, message)
			require.Equal(t, directChatID, message.Chat.ID)
			tc.assert(t, message)
		})
	}
}

func TestGoTelegramClientUploadSuite(t *testing.T) {
	t.Parallel()

	world := newCompatWorld(t)
	server := httptest.NewServer(world.boundary.routes())
	defer server.Close()

	client := world.client(t, server)
	ctx := context.Background()
	directChatID := world.publicChatID(t, world.directID)

	cases := []struct {
		name   string
		send   func(context.Context, *telegrambot.Bot, int64) (*tgmodels.Message, error)
		assert func(*testing.T, *tgmodels.Message)
	}{
		{
			name: "sendPhoto",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendPhoto(ctx, &telegrambot.SendPhotoParams{
					ChatID: chatID,
					Photo: &tgmodels.InputFileUpload{
						Filename: "photo.jpg",
						Data:     strings.NewReader("photo"),
					},
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.Len(t, message.Photo, 1)
				require.Equal(t, "upload-image", message.Photo[0].FileID)
			},
		},
		{
			name: "sendDocument",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendDocument(ctx, &telegrambot.SendDocumentParams{
					ChatID: chatID,
					Document: &tgmodels.InputFileUpload{
						Filename: "report.pdf",
						Data:     strings.NewReader("document"),
					},
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Document)
				require.Equal(t, "upload-document", message.Document.FileID)
			},
		},
		{
			name: "sendVideo",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendVideo(ctx, &telegrambot.SendVideoParams{
					ChatID: chatID,
					Video: &tgmodels.InputFileUpload{
						Filename: "clip.mp4",
						Data:     strings.NewReader("video"),
					},
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Video)
				require.Equal(t, "upload-video", message.Video.FileID)
			},
		},
		{
			name: "sendAnimation",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendAnimation(ctx, &telegrambot.SendAnimationParams{
					ChatID: chatID,
					Animation: &tgmodels.InputFileUpload{
						Filename: "loop.gif",
						Data:     strings.NewReader("gif"),
					},
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Animation)
				require.Equal(t, "upload-gif", message.Animation.FileID)
			},
		},
		{
			name: "sendAudio",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendAudio(ctx, &telegrambot.SendAudioParams{
					ChatID: chatID,
					Audio: &tgmodels.InputFileUpload{
						Filename: "track.mp3",
						Data:     strings.NewReader("audio"),
					},
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Audio)
				require.Equal(t, "upload-file", message.Audio.FileID)
			},
		},
		{
			name: "sendVideoNote",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendVideoNote(ctx, &telegrambot.SendVideoNoteParams{
					ChatID: chatID,
					VideoNote: &tgmodels.InputFileUpload{
						Filename: "note.mp4",
						Data:     strings.NewReader("video-note"),
					},
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.VideoNote)
				require.Equal(t, "upload-video", message.VideoNote.FileID)
			},
		},
		{
			name: "sendVoice",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendVoice(ctx, &telegrambot.SendVoiceParams{
					ChatID: chatID,
					Voice: &tgmodels.InputFileUpload{
						Filename: "voice.ogg",
						Data:     strings.NewReader("voice"),
					},
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Voice)
				require.Equal(t, "upload-voice", message.Voice.FileID)
			},
		},
		{
			name: "sendSticker",
			send: func(ctx context.Context, client *telegrambot.Bot, chatID int64) (*tgmodels.Message, error) {
				return client.SendSticker(ctx, &telegrambot.SendStickerParams{
					ChatID: chatID,
					Sticker: &tgmodels.InputFileUpload{
						Filename: "sticker.webp",
						Data:     strings.NewReader("sticker"),
					},
				})
			},
			assert: func(t *testing.T, message *tgmodels.Message) {
				t.Helper()
				require.NotNil(t, message.Sticker)
				require.Equal(t, "upload-sticker", message.Sticker.FileID)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			message, err := tc.send(ctx, client, directChatID)
			require.NoError(t, err)
			require.NotNil(t, message)
			require.Equal(t, directChatID, message.Chat.ID)
			tc.assert(t, message)
		})
	}
}

func TestGoTelegramClientStructuredSuite(t *testing.T) {
	t.Parallel()

	world := newCompatWorld(t)
	server := httptest.NewServer(world.boundary.routes())
	defer server.Close()

	client := world.client(t, server)
	ctx := context.Background()
	directChatID := world.publicChatID(t, world.directID)
	peerUserID := world.publicAccountID(t, world.peer.ID)

	location, err := client.SendLocation(ctx, &telegrambot.SendLocationParams{
		ChatID:    directChatID,
		Latitude:  55.75,
		Longitude: 37.61,
	})
	require.NoError(t, err)
	require.NotNil(t, location.Location)
	require.Equal(t, 55.75, location.Location.Latitude)
	require.Equal(t, 37.61, location.Location.Longitude)

	contact, err := client.SendContact(ctx, &telegrambot.SendContactParams{
		ChatID:      directChatID,
		PhoneNumber: "+79990000000",
		FirstName:   "Ivan",
		LastName:    "Petrov",
	})
	require.NoError(t, err)
	require.NotNil(t, contact.Contact)
	require.Equal(t, "+79990000000", contact.Contact.PhoneNumber)
	require.Equal(t, "Ivan", contact.Contact.FirstName)

	contactWithUser, err := client.SendContact(ctx, &telegrambot.SendContactParams{
		ChatID:      directChatID,
		PhoneNumber: "+79991111111",
		FirstName:   "Peer",
		LastName:    "User",
		VCard:       "BEGIN:VCARD\nEND:VCARD",
	})
	require.NoError(t, err)
	require.NotNil(t, contactWithUser.Contact)
	require.Equal(t, "Peer", contactWithUser.Contact.FirstName)
	_ = peerUserID

	poll, err := client.SendPoll(ctx, &telegrambot.SendPollParams{
		ChatID:   directChatID,
		Question: "Choose",
		Options: []tgmodels.InputPollOption{
			{Text: "A"},
			{Text: "B"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, poll.Poll)
	require.Equal(t, "Choose", poll.Poll.Question)
	require.Len(t, poll.Poll.Options, 2)
	require.Equal(t, "A", poll.Poll.Options[0].Text)
	require.Equal(t, "B", poll.Poll.Options[1].Text)
}

func TestGoTelegramClientUpdateSuite(t *testing.T) {
	t.Parallel()

	world := newCompatWorld(t)
	server := httptest.NewServer(world.boundary.routes())
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker, err := domainbot.NewWorker(world.bot, &http.Client{Timeout: time.Second})
	require.NoError(t, err)

	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	go func() {
		_ = worker.Run(workerCtx, logger)
	}()

	var (
		seenMu sync.Mutex
		seen   = make(map[string]*tgmodels.Update)
	)

	directMessage, _, err := world.conversations.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  world.directID,
		SenderAccountID: world.owner.ID,
		SenderDeviceID:  "ios",
		Draft:           textDraft("direct inbound"),
	})
	require.NoError(t, err)

	_, _, err = world.conversations.EditMessage(context.Background(), conversation.EditMessageParams{
		ConversationID: world.directID,
		MessageID:      directMessage.ID,
		ActorAccountID: world.owner.ID,
		ActorDeviceID:  "ios",
		Draft:          textDraft("direct inbound edited"),
	})
	require.NoError(t, err)

	channel, _, err := world.conversations.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID:   world.owner.ID,
		Kind:             conversation.ConversationKindChannel,
		Title:            "announcements",
		MemberAccountIDs: []string{world.botAccount.ID},
	})
	require.NoError(t, err)

	channelMessage, _, err := world.conversations.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  channel.ID,
		SenderAccountID: world.owner.ID,
		SenderDeviceID:  "ios",
		Draft:           textDraft("channel post"),
	})
	require.NoError(t, err)

	_, _, err = world.conversations.EditMessage(context.Background(), conversation.EditMessageParams{
		ConversationID: channel.ID,
		MessageID:      channelMessage.ID,
		ActorAccountID: world.owner.ID,
		ActorDeviceID:  "ios",
		Draft:          textDraft("channel post edited"),
	})
	require.NoError(t, err)

	outbound, err := world.bot.SendMessage(context.Background(), domainbot.SendMessageParams{
		BotToken: world.botToken,
		ChatID:   world.directID,
		Text:     "tap me",
		ReplyMarkup: &domainbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]domainbot.InlineKeyboardButton{{
				{Text: "Tap", CallbackData: "tap"},
			}},
		},
	})
	require.NoError(t, err)

	_, err = world.bot.TriggerCallbackQuery(context.Background(), domainbot.TriggerCallbackParams{
		ConversationID: world.directID,
		MessageID:      outbound.MessageID,
		FromAccountID:  world.owner.ID,
		Data:           "tap",
	})
	require.NoError(t, err)

	query, err := world.bot.TriggerInlineQuery(context.Background(), domainbot.TriggerInlineQueryParams{
		BotAccountID:  world.botAccount.ID,
		FromAccountID: world.owner.ID,
		Query:         "inline docs",
	})
	require.NoError(t, err)

	err = world.bot.AnswerInlineQuery(context.Background(), domainbot.AnswerInlineQueryParams{
		BotToken:      world.botToken,
		InlineQueryID: query.ID,
		Results: []domainbot.InlineQueryResult{{
			Type:  "article",
			ID:    "article-1",
			Title: "Article",
			InputMessageContent: &domainbot.InputTextMessageContent{
				MessageText: "inline article",
			},
		}},
	})
	require.NoError(t, err)

	_, err = world.bot.TriggerChosenInlineResult(context.Background(), domainbot.TriggerChosenInlineResultParams{
		InlineQueryID: query.ID,
		FromAccountID: world.owner.ID,
		ResultID:      "article-1",
	})
	require.NoError(t, err)

	group, _, err := world.conversations.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID: world.owner.ID,
		Kind:           conversation.ConversationKindGroup,
		Title:          "ops-updates",
	})
	require.NoError(t, err)

	_, err = world.conversations.AddMembers(context.Background(), conversation.AddMembersParams{
		ConversationID:     group.ID,
		ActorAccountID:     world.owner.ID,
		InvitedByAccountID: world.owner.ID,
		AccountIDs:         []string{world.botAccount.ID},
		Role:               conversation.MemberRoleAdmin,
	})
	require.NoError(t, err)

	_, err = world.conversations.AddMembers(context.Background(), conversation.AddMembersParams{
		ConversationID:     group.ID,
		ActorAccountID:     world.owner.ID,
		InvitedByAccountID: world.owner.ID,
		AccountIDs:         []string{world.peer.ID},
	})
	require.NoError(t, err)

	require.Eventuallyf(t, func() bool {
		entries, loadErr := world.store.PendingUpdates(context.Background(), world.botAccount.ID, 0, nil, time.Time{}, 100)
		require.NoError(t, loadErr)

		queued := make(map[string]struct{}, len(entries))
		for _, entry := range entries {
			queued[string(entry.UpdateType)] = struct{}{}
		}
		for _, key := range requiredUpdateKeys() {
			if _, ok := queued[key]; !ok {
				return false
			}
		}

		return true
	}, 5*time.Second, 25*time.Millisecond, "bot queue missing update types")

	client, err := telegrambot.New(
		world.botToken,
		telegrambot.WithServerURL(server.URL),
		telegrambot.WithSkipGetMe(),
		telegrambot.WithHTTPClient(250*time.Millisecond, server.Client()),
		telegrambot.WithWorkers(1),
		telegrambot.WithNotAsyncHandlers(),
		telegrambot.WithAllowedUpdates(telegrambot.AllowedUpdates(requiredUpdateKeys())),
		telegrambot.WithDefaultHandler(func(_ context.Context, _ *telegrambot.Bot, update *tgmodels.Update) {
			seenMu.Lock()
			defer seenMu.Unlock()
			for _, key := range updateKeys(update) {
				updateCopy := cloneTelegramUpdate(t, update)
				seen[key] = updateCopy
			}
		}),
	)
	require.NoError(t, err)

	clientCtx, stopClient := context.WithCancel(context.Background())
	defer stopClient()
	go client.Start(clientCtx)

	require.Eventuallyf(t, func() bool {
		seenMu.Lock()
		defer seenMu.Unlock()
		for _, key := range requiredUpdateKeys() {
			if _, ok := seen[key]; !ok {
				return false
			}
		}
		return true
	}, 5*time.Second, 25*time.Millisecond, "missing updates: %v", missingUpdateKeys(seen))

	seenMu.Lock()
	defer seenMu.Unlock()
	require.Equal(t, "direct inbound", seen["message"].Message.Text)
	require.Equal(t, "direct inbound edited", seen["edited_message"].EditedMessage.Text)
	require.Equal(t, "channel post", seen["channel_post"].ChannelPost.Text)
	require.Equal(t, "channel post edited", seen["edited_channel_post"].EditedChannelPost.Text)
	require.Equal(t, "tap", seen["callback_query"].CallbackQuery.Data)
	require.Equal(t, "inline docs", seen["inline_query"].InlineQuery.Query)
	require.Equal(t, "article-1", seen["chosen_inline_result"].ChosenInlineResult.ResultID)
	require.NotNil(t, seen["chat_member"].ChatMember)
	require.NotNil(t, seen["my_chat_member"].MyChatMember)
}

type compatWorld struct {
	boundary      *api
	bot           *domainbot.Service
	store         domainbot.Store
	identity      *identity.Service
	conversations *conversation.Service
	owner         identity.Account
	peer          identity.Account
	botAccount    identity.Account
	botToken      string
	directID      string
	groupID       string
	topicID       string
	media         *compatMedia
}

func newCompatWorld(t *testing.T) *compatWorld {
	t.Helper()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)

	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	owner, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "compatowner",
		DisplayName: "Compat Owner",
		AccountKind: identity.AccountKindUser,
		Email:       "owner@example.org",
	})
	require.NoError(t, err)
	peer, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "compatpeer",
		DisplayName: "Compat Peer",
		AccountKind: identity.AccountKindUser,
		Email:       "peer@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "compatbot",
		DisplayName: "Compat Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	media := newCompatMedia(botAccount.ID)
	botStore := bottest.NewMemoryStore()
	settings := domainbot.DefaultSettings()
	settings.FanoutPollInterval = 10 * time.Millisecond
	settings.LongPollStep = 10 * time.Millisecond
	botService, err := domainbot.NewService(
		botStore,
		identityService,
		conversationService,
		conversationStore,
		media,
		domainbot.WithSettings(settings),
	)
	require.NoError(t, err)

	direct, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   owner.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	group, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   owner.ID,
		Kind:             conversation.ConversationKindGroup,
		Title:            "compat group",
		MemberAccountIDs: []string{botAccount.ID, peer.ID},
		Settings: conversation.ConversationSettings{
			AllowThreads: true,
		},
	})
	require.NoError(t, err)

	topic, _, err := conversationService.CreateTopic(ctx, conversation.CreateTopicParams{
		ConversationID:   group.ID,
		CreatorAccountID: owner.ID,
		Title:            "Incidents",
	})
	require.NoError(t, err)

	return &compatWorld{
		boundary:      &api{bot: botService, media: media, uploadLimit: 1024 * 1024},
		bot:           botService,
		store:         botStore,
		identity:      identityService,
		conversations: conversationService,
		owner:         owner,
		peer:          peer,
		botAccount:    botAccount,
		botToken:      botToken,
		directID:      direct.ID,
		groupID:       group.ID,
		topicID:       topic.ID,
		media:         media,
	}
}

func (w *compatWorld) client(t *testing.T, server *httptest.Server, opts ...telegrambot.Option) *telegrambot.Bot {
	t.Helper()

	options := []telegrambot.Option{
		telegrambot.WithServerURL(server.URL),
		telegrambot.WithSkipGetMe(),
	}
	options = append(options, opts...)

	client, err := telegrambot.New(w.botToken, options...)
	require.NoError(t, err)

	return client
}

func (w *compatWorld) publicAccountID(t *testing.T, accountID string) int64 {
	t.Helper()

	value, err := w.bot.PublicAccountID(context.Background(), accountID)
	require.NoError(t, err)

	return value
}

func (w *compatWorld) publicChatID(t *testing.T, chatID string) int64 {
	t.Helper()

	value, err := w.bot.PublicChatID(context.Background(), chatID)
	require.NoError(t, err)

	return value
}

func (w *compatWorld) publicTopicID(t *testing.T, topicID string) int64 {
	t.Helper()

	value, err := w.bot.PublicTopicID(context.Background(), topicID)
	require.NoError(t, err)

	return value
}

func textDraft(text string) conversation.MessageDraft {
	return conversation.MessageDraft{
		Kind: conversation.MessageKindText,
		Payload: conversation.EncryptedPayload{
			Ciphertext: []byte(text),
		},
	}
}

func intString(value int) string {
	return strconv.Itoa(value)
}

func updateKeys(update *tgmodels.Update) []string {
	keys := make([]string, 0, 9)
	switch {
	case update.Message != nil:
		keys = append(keys, "message")
	case update.EditedMessage != nil:
		keys = append(keys, "edited_message")
	case update.ChannelPost != nil:
		keys = append(keys, "channel_post")
	case update.EditedChannelPost != nil:
		keys = append(keys, "edited_channel_post")
	case update.CallbackQuery != nil:
		keys = append(keys, "callback_query")
	case update.InlineQuery != nil:
		keys = append(keys, "inline_query")
	case update.ChosenInlineResult != nil:
		keys = append(keys, "chosen_inline_result")
	case update.ChatMember != nil:
		keys = append(keys, "chat_member")
	case update.MyChatMember != nil:
		keys = append(keys, "my_chat_member")
	}

	return keys
}

func cloneTelegramUpdate(t *testing.T, update *tgmodels.Update) *tgmodels.Update {
	t.Helper()

	raw, err := json.Marshal(update)
	require.NoError(t, err)

	var clone tgmodels.Update
	require.NoError(t, json.Unmarshal(raw, &clone))

	return &clone
}

func requiredUpdateKeys() []string {
	return []string{
		"message",
		"edited_message",
		"channel_post",
		"edited_channel_post",
		"callback_query",
		"inline_query",
		"chosen_inline_result",
		"chat_member",
		"my_chat_member",
	}
}

func missingUpdateKeys(seen map[string]*tgmodels.Update) []string {
	missing := make([]string, 0)
	for _, key := range requiredUpdateKeys() {
		if _, ok := seen[key]; ok {
			continue
		}
		missing = append(missing, key)
	}

	return missing
}

type compatMedia struct {
	mu      sync.Mutex
	assets  map[string]domainmedia.MediaAsset
	uploads map[domainmedia.MediaKind]domainmedia.MediaAsset
}

func newCompatMedia(botAccountID string) *compatMedia {
	return &compatMedia{
		assets: map[string]domainmedia.MediaAsset{
			"media-photo": {
				ID:             "media-photo",
				OwnerAccountID: botAccountID,
				Kind:           domainmedia.MediaKindImage,
				Status:         domainmedia.MediaStatusReady,
				FileName:       "photo.jpg",
				ContentType:    "image/jpeg",
				SizeBytes:      1024,
				Width:          64,
				Height:         48,
			},
			"media-document": {
				ID:             "media-document",
				OwnerAccountID: botAccountID,
				Kind:           domainmedia.MediaKindDocument,
				Status:         domainmedia.MediaStatusReady,
				FileName:       "report.pdf",
				ContentType:    "application/pdf",
				SizeBytes:      2048,
			},
			"media-video": {
				ID:             "media-video",
				OwnerAccountID: botAccountID,
				Kind:           domainmedia.MediaKindVideo,
				Status:         domainmedia.MediaStatusReady,
				FileName:       "clip.mp4",
				ContentType:    "video/mp4",
				SizeBytes:      4096,
				Width:          320,
				Height:         180,
				Duration:       3 * time.Second,
			},
			"media-animation": {
				ID:             "media-animation",
				OwnerAccountID: botAccountID,
				Kind:           domainmedia.MediaKindGIF,
				Status:         domainmedia.MediaStatusReady,
				FileName:       "loop.gif",
				ContentType:    "image/gif",
				SizeBytes:      4096,
				Width:          240,
				Height:         180,
				Duration:       2 * time.Second,
			},
			"media-audio": {
				ID:             "media-audio",
				OwnerAccountID: botAccountID,
				Kind:           domainmedia.MediaKindFile,
				Status:         domainmedia.MediaStatusReady,
				FileName:       "track.mp3",
				ContentType:    "audio/mpeg",
				SizeBytes:      3072,
				Duration:       5 * time.Second,
			},
			"media-video-note": {
				ID:             "media-video-note",
				OwnerAccountID: botAccountID,
				Kind:           domainmedia.MediaKindVideo,
				Status:         domainmedia.MediaStatusReady,
				FileName:       "note.mp4",
				ContentType:    "video/mp4",
				SizeBytes:      3072,
				Width:          240,
				Height:         240,
				Duration:       4 * time.Second,
			},
			"media-voice": {
				ID:             "media-voice",
				OwnerAccountID: botAccountID,
				Kind:           domainmedia.MediaKindVoice,
				Status:         domainmedia.MediaStatusReady,
				FileName:       "voice.ogg",
				ContentType:    "audio/ogg",
				SizeBytes:      1024,
				Duration:       2 * time.Second,
			},
			"media-sticker": {
				ID:             "media-sticker",
				OwnerAccountID: botAccountID,
				Kind:           domainmedia.MediaKindSticker,
				Status:         domainmedia.MediaStatusReady,
				FileName:       "sticker.webp",
				ContentType:    "image/webp",
				SizeBytes:      512,
				Width:          128,
				Height:         128,
			},
		},
		uploads: map[domainmedia.MediaKind]domainmedia.MediaAsset{
			domainmedia.MediaKindImage: {
				ID:          "upload-image",
				Kind:        domainmedia.MediaKindImage,
				Status:      domainmedia.MediaStatusReady,
				FileName:    "upload.jpg",
				ContentType: "image/jpeg",
				SizeBytes:   5,
				Width:       32,
				Height:      24,
			},
			domainmedia.MediaKindDocument: {
				ID:          "upload-document",
				Kind:        domainmedia.MediaKindDocument,
				Status:      domainmedia.MediaStatusReady,
				FileName:    "upload.pdf",
				ContentType: "application/pdf",
				SizeBytes:   8,
			},
			domainmedia.MediaKindVideo: {
				ID:          "upload-video",
				Kind:        domainmedia.MediaKindVideo,
				Status:      domainmedia.MediaStatusReady,
				FileName:    "upload.mp4",
				ContentType: "video/mp4",
				SizeBytes:   10,
				Width:       240,
				Height:      240,
				Duration:    4 * time.Second,
			},
			domainmedia.MediaKindGIF: {
				ID:          "upload-gif",
				Kind:        domainmedia.MediaKindGIF,
				Status:      domainmedia.MediaStatusReady,
				FileName:    "upload.gif",
				ContentType: "image/gif",
				SizeBytes:   6,
				Width:       64,
				Height:      64,
				Duration:    1 * time.Second,
			},
			domainmedia.MediaKindFile: {
				ID:          "upload-file",
				Kind:        domainmedia.MediaKindFile,
				Status:      domainmedia.MediaStatusReady,
				FileName:    "upload.mp3",
				ContentType: "audio/mpeg",
				SizeBytes:   9,
				Duration:    5 * time.Second,
			},
			domainmedia.MediaKindVoice: {
				ID:          "upload-voice",
				Kind:        domainmedia.MediaKindVoice,
				Status:      domainmedia.MediaStatusReady,
				FileName:    "upload.ogg",
				ContentType: "audio/ogg",
				SizeBytes:   7,
				Duration:    3 * time.Second,
			},
			domainmedia.MediaKindSticker: {
				ID:          "upload-sticker",
				Kind:        domainmedia.MediaKindSticker,
				Status:      domainmedia.MediaStatusReady,
				FileName:    "upload.webp",
				ContentType: "image/webp",
				SizeBytes:   4,
				Width:       128,
				Height:      128,
			},
		},
	}
}

func (m *compatMedia) MediaAssetByID(_ context.Context, mediaID string) (domainmedia.MediaAsset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	asset, ok := m.assets[mediaID]
	if !ok {
		return domainmedia.MediaAsset{}, domainmedia.ErrNotFound
	}

	return asset, nil
}

func (m *compatMedia) Upload(_ context.Context, params domainmedia.UploadParams) (domainmedia.MediaAsset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	asset, ok := m.uploads[params.Kind]
	if !ok {
		return domainmedia.MediaAsset{}, domainmedia.ErrInvalidInput
	}

	asset.OwnerAccountID = params.OwnerAccountID
	if params.FileName != "" {
		asset.FileName = params.FileName
	}
	if params.ContentType != "" {
		asset.ContentType = params.ContentType
	}
	if params.SizeBytes > 0 {
		asset.SizeBytes = params.SizeBytes
	}
	m.assets[asset.ID] = asset

	return asset, nil
}
