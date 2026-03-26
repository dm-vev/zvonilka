package botapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHTTPGetMeAndSendMessage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)
	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)
	botStore := bottest.NewMemoryStore()
	botService, err := domainbot.NewService(botStore, identityService, conversationService, conversationStore, mediaFixture{})
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "eve",
		DisplayName: "Eve",
		AccountKind: identity.AccountKindUser,
		Email:       "eve@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "httpbot",
		DisplayName: "HTTP Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	conv, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	boundary := &api{bot: botService}

	request := httptest.NewRequest(http.MethodGet, "/bot"+botToken+"/getMe", nil)
	recorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"ok":true`)
	require.Contains(t, recorder.Body.String(), `"username":"httpbot"`)

	sendRequest := httptest.NewRequest(
		http.MethodPost,
		"/bot"+botToken+"/sendMessage",
		strings.NewReader(`{"chat_id":"`+conv.ID+`","text":"ping"}`),
	)
	sendRequest.Header.Set("Content-Type", "application/json")
	sendRecorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(sendRecorder, sendRequest)
	require.Equal(t, http.StatusOK, sendRecorder.Code)
	require.Contains(t, sendRecorder.Body.String(), `"text":"ping"`)

	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(sendRecorder.Body.Bytes(), &envelope))
	require.Equal(t, "true", strings.TrimSpace(string(envelope["ok"])))
}

func TestHTTPSendDocument(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)
	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "dave",
		DisplayName: "Dave",
		AccountKind: identity.AccountKindUser,
		Email:       "dave@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "filesbot",
		DisplayName: "Files Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	conv, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	botService, err := domainbot.NewService(
		bottest.NewMemoryStore(),
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{
			assets: map[string]domainmedia.MediaAsset{
				"media-doc": {
					ID:             "media-doc",
					OwnerAccountID: botAccount.ID,
					Kind:           domainmedia.MediaKindDocument,
					Status:         domainmedia.MediaStatusReady,
					FileName:       "report.pdf",
					ContentType:    "application/pdf",
					SizeBytes:      2048,
				},
			},
		},
	)
	require.NoError(t, err)

	boundary := &api{bot: botService}
	request := httptest.NewRequest(
		http.MethodPost,
		"/bot"+botToken+"/sendDocument",
		strings.NewReader("chat_id="+conv.ID+"&document=media-doc&caption=report"),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"caption":"report"`)
	require.Contains(t, recorder.Body.String(), `"document"`)
	require.Contains(t, recorder.Body.String(), `"file_id":"media-doc"`)
}

func TestHTTPAnswerCallbackQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)
	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "frank",
		DisplayName: "Frank",
		AccountKind: identity.AccountKindUser,
		Email:       "frank@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "callbackbot",
		DisplayName: "Callback Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	conv, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	botService, err := domainbot.NewService(
		bottest.NewMemoryStore(),
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{},
	)
	require.NoError(t, err)

	message, err := botService.SendMessage(ctx, domainbot.SendMessageParams{
		BotToken: botToken,
		ChatID:   conv.ID,
		Text:     "tap",
		ReplyMarkup: &domainbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]domainbot.InlineKeyboardButton{{
				{Text: "Tap", CallbackData: "tap"},
			}},
		},
	})
	require.NoError(t, err)
	callback, err := botService.TriggerCallbackQuery(ctx, domainbot.TriggerCallbackParams{
		ConversationID: conv.ID,
		MessageID:      message.MessageID,
		FromAccountID:  userAccount.ID,
		Data:           "tap",
	})
	require.NoError(t, err)

	boundary := &api{bot: botService}
	request := httptest.NewRequest(
		http.MethodPost,
		"/bot"+botToken+"/answerCallbackQuery",
		strings.NewReader(`{"callback_query_id":"`+callback.ID+`","text":"done"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"ok":true`)
	require.Contains(t, recorder.Body.String(), `"result":true`)
}

func TestHTTPAnswerInlineQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)
	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "yura",
		DisplayName: "Yura",
		AccountKind: identity.AccountKindUser,
		Email:       "yura@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "inlinehttpbot",
		DisplayName: "Inline HTTP Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	botStore := bottest.NewMemoryStore()
	botService, err := domainbot.NewService(
		botStore,
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{},
	)
	require.NoError(t, err)

	query, err := botService.TriggerInlineQuery(ctx, domainbot.TriggerInlineQueryParams{
		BotAccountID:  botAccount.ID,
		FromAccountID: userAccount.ID,
		Query:         "q",
	})
	require.NoError(t, err)

	boundary := &api{bot: botService}
	request := httptest.NewRequest(
		http.MethodPost,
		"/bot"+botToken+"/answerInlineQuery",
		strings.NewReader(`{"inline_query_id":"`+query.ID+`","results":[{"type":"article","id":"r1","title":"Result","input_message_content":{"message_text":"hello"}}],"cache_time":10}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"ok":true`)
	require.Contains(t, recorder.Body.String(), `"result":true`)

	state, err := botStore.InlineQueryByID(ctx, query.ID)
	require.NoError(t, err)
	require.True(t, state.Answered)
	require.Len(t, state.Results, 1)
	require.Equal(t, "hello", state.Results[0].InputMessageContent.MessageText)
}

func TestHTTPSendPhotoMultipartUpload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)
	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "gina",
		DisplayName: "Gina",
		AccountKind: identity.AccountKindUser,
		Email:       "gina@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "uploadbot",
		DisplayName: "Upload Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	conv, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	uploader := &uploadFixture{asset: domainmedia.MediaAsset{
		ID:             "media-uploaded",
		OwnerAccountID: botAccount.ID,
		Kind:           domainmedia.MediaKindImage,
		Status:         domainmedia.MediaStatusReady,
		FileName:       "photo.jpg",
		ContentType:    "image/jpeg",
		SizeBytes:      5,
		Width:          32,
		Height:         24,
	}}
	botService, err := domainbot.NewService(
		bottest.NewMemoryStore(),
		identityService,
		conversationService,
		conversationStore,
		uploader,
	)
	require.NoError(t, err)
	boundary := &api{bot: botService, media: uploader, uploadLimit: 1024}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("chat_id", conv.ID))
	part, err := writer.CreateFormFile("photo", "photo.jpg")
	require.NoError(t, err)
	_, err = io.Copy(part, strings.NewReader("photo"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/bot"+botToken+"/sendPhoto", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"photo"`)
	require.Contains(t, recorder.Body.String(), `"file_id":"media-uploaded"`)
	require.NotNil(t, uploader.last)
	require.Equal(t, botAccount.ID, uploader.last.OwnerAccountID)
	require.Equal(t, domainmedia.MediaKindImage, uploader.last.Kind)
	require.Equal(t, "photo.jpg", uploader.last.FileName)
}

func TestHTTPSendAnimationAndAudio(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)
	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "helen",
		DisplayName: "Helen",
		AccountKind: identity.AccountKindUser,
		Email:       "helen@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "mediashapebot",
		DisplayName: "Media Shape Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	conv, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	botService, err := domainbot.NewService(
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
					Height:         200,
					Duration:       2,
				},
				"media-audio": {
					ID:             "media-audio",
					OwnerAccountID: botAccount.ID,
					Kind:           domainmedia.MediaKindFile,
					Status:         domainmedia.MediaStatusReady,
					FileName:       "track.mp3",
					ContentType:    "audio/mpeg",
					SizeBytes:      8192,
					Duration:       14,
				},
			},
		},
	)
	require.NoError(t, err)

	boundary := &api{bot: botService}

	animationRequest := httptest.NewRequest(
		http.MethodPost,
		"/bot"+botToken+"/sendAnimation",
		strings.NewReader("chat_id="+conv.ID+"&animation=media-animation&caption=loop"),
	)
	animationRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	animationRecorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(animationRecorder, animationRequest)
	require.Equal(t, http.StatusOK, animationRecorder.Code)
	require.Contains(t, animationRecorder.Body.String(), `"animation"`)
	require.Contains(t, animationRecorder.Body.String(), `"file_id":"media-animation"`)
	require.Contains(t, animationRecorder.Body.String(), `"caption":"loop"`)

	audioRequest := httptest.NewRequest(
		http.MethodPost,
		"/bot"+botToken+"/sendAudio",
		strings.NewReader(`{"chat_id":"`+conv.ID+`","audio":"media-audio","caption":"track"}`),
	)
	audioRequest.Header.Set("Content-Type", "application/json")
	audioRecorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(audioRecorder, audioRequest)
	require.Equal(t, http.StatusOK, audioRecorder.Code)
	require.Contains(t, audioRecorder.Body.String(), `"audio"`)
	require.Contains(t, audioRecorder.Body.String(), `"file_id":"media-audio"`)
	require.Contains(t, audioRecorder.Body.String(), `"caption":"track"`)
}

func TestHTTPSendVideoNoteMultipartUpload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)
	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "ian",
		DisplayName: "Ian",
		AccountKind: identity.AccountKindUser,
		Email:       "ian@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "videonotebot",
		DisplayName: "Video Note Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	conv, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	uploader := &uploadFixture{asset: domainmedia.MediaAsset{
		ID:             "media-video-note",
		OwnerAccountID: botAccount.ID,
		Kind:           domainmedia.MediaKindVideo,
		Status:         domainmedia.MediaStatusReady,
		FileName:       "note.mp4",
		ContentType:    "video/mp4",
		SizeBytes:      9,
		Width:          240,
		Height:         240,
		Duration:       4,
	}}
	botService, err := domainbot.NewService(
		bottest.NewMemoryStore(),
		identityService,
		conversationService,
		conversationStore,
		uploader,
	)
	require.NoError(t, err)
	boundary := &api{bot: botService, media: uploader, uploadLimit: 1024}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("chat_id", conv.ID))
	require.NoError(t, writer.WriteField("video_note", "attach://clip"))
	part, err := writer.CreateFormFile("clip", "note.mp4")
	require.NoError(t, err)
	_, err = io.Copy(part, strings.NewReader("video-note"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/bot"+botToken+"/sendVideoNote", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"video_note"`)
	require.Contains(t, recorder.Body.String(), `"file_id":"media-video-note"`)
	require.NotNil(t, uploader.last)
	require.Equal(t, botAccount.ID, uploader.last.OwnerAccountID)
	require.Equal(t, domainmedia.MediaKindVideo, uploader.last.Kind)
	require.Equal(t, "note.mp4", uploader.last.FileName)
}

func TestHTTPSendLocationContactAndPoll(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identitytest.NewMemoryStore()
	identityService, err := identity.NewService(identityStore, identity.NoopCodeSender{})
	require.NoError(t, err)
	conversationStore := conversationtest.NewMemoryStore()
	conversationService, err := conversation.NewService(conversationStore)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "olga",
		DisplayName: "Olga",
		AccountKind: identity.AccountKindUser,
		Email:       "olga@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "structuredbot",
		DisplayName: "Structured Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	conv, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	botService, err := domainbot.NewService(
		bottest.NewMemoryStore(),
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{},
	)
	require.NoError(t, err)

	boundary := &api{bot: botService}

	locationRequest := httptest.NewRequest(
		http.MethodPost,
		"/bot"+botToken+"/sendLocation",
		strings.NewReader(`{"chat_id":"`+conv.ID+`","latitude":55.75,"longitude":37.61}`),
	)
	locationRequest.Header.Set("Content-Type", "application/json")
	locationRecorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(locationRecorder, locationRequest)
	require.Equal(t, http.StatusOK, locationRecorder.Code)
	require.Contains(t, locationRecorder.Body.String(), `"location"`)
	require.Contains(t, locationRecorder.Body.String(), `"latitude":55.75`)

	contactRequest := httptest.NewRequest(
		http.MethodPost,
		"/bot"+botToken+"/sendContact",
		strings.NewReader(`{"chat_id":"`+conv.ID+`","phone_number":"+79990000000","first_name":"Ivan","last_name":"Petrov"}`),
	)
	contactRequest.Header.Set("Content-Type", "application/json")
	contactRecorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(contactRecorder, contactRequest)
	require.Equal(t, http.StatusOK, contactRecorder.Code)
	require.Contains(t, contactRecorder.Body.String(), `"contact"`)
	require.Contains(t, contactRecorder.Body.String(), `"phone_number":"+79990000000"`)

	pollRequest := httptest.NewRequest(
		http.MethodPost,
		"/bot"+botToken+"/sendPoll",
		strings.NewReader(`{"chat_id":"`+conv.ID+`","question":"Choose","options":["A","B"]}`),
	)
	pollRequest.Header.Set("Content-Type", "application/json")
	pollRecorder := httptest.NewRecorder()
	boundary.routes().ServeHTTP(pollRecorder, pollRequest)
	require.Equal(t, http.StatusOK, pollRecorder.Code)
	require.Contains(t, pollRecorder.Body.String(), `"poll"`)
	require.Contains(t, pollRecorder.Body.String(), `"question":"Choose"`)
	require.Contains(t, pollRecorder.Body.String(), `"text":"A"`)
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

type uploadFixture struct {
	last  *domainmedia.UploadParams
	asset domainmedia.MediaAsset
}

func (f *uploadFixture) Upload(_ context.Context, params domainmedia.UploadParams) (domainmedia.MediaAsset, error) {
	copyParams := params
	f.last = &copyParams

	return f.asset, nil
}

func (f *uploadFixture) MediaAssetByID(_ context.Context, mediaID string) (domainmedia.MediaAsset, error) {
	if f.asset.ID == mediaID {
		return f.asset, nil
	}

	return domainmedia.MediaAsset{}, domainmedia.ErrNotFound
}
