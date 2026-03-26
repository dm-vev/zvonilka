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
	botService, err := domainbot.NewService(botStore, identityService, conversationService, conversationStore)
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
