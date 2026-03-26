package botapi

import (
	"context"
	"encoding/json"
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

type mediaFixture struct {
	assets map[string]domainmedia.MediaAsset
}

func (f mediaFixture) MediaAssetByID(_ context.Context, mediaID string) (domainmedia.MediaAsset, error) {
	if asset, ok := f.assets[mediaID]; ok {
		return asset, nil
	}

	return domainmedia.MediaAsset{}, domainmedia.ErrNotFound
}
