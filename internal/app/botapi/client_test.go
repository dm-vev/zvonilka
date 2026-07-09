package botapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
)

func TestGoTelegramClientGetMeSendMessageAndDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	boundary, botService, _, _, _, botToken, convID := compatBoundary(t, ctx)

	server := httptest.NewServer(boundary.routes())
	defer server.Close()

	client, err := telegrambot.New(
		botToken,
		telegrambot.WithServerURL(server.URL),
		telegrambot.WithSkipGetMe(),
	)
	require.NoError(t, err)

	me, err := client.GetMe(ctx)
	require.NoError(t, err)
	require.NotZero(t, me.ID)
	require.Equal(t, "compatbot", me.Username)

	publicChatID, err := botService.PublicChatID(ctx, convID)
	require.NoError(t, err)

	message, err := client.SendMessage(ctx, &telegrambot.SendMessageParams{
		ChatID: publicChatID,
		Text:   "hello from client",
	})
	require.NoError(t, err)
	require.NotZero(t, message.ID)
	require.Equal(t, "hello from client", message.Text)
	require.Equal(t, publicChatID, message.Chat.ID)

	ok, err := client.DeleteMessage(ctx, &telegrambot.DeleteMessageParams{
		ChatID:    publicChatID,
		MessageID: message.ID,
	})
	require.NoError(t, err)
	require.True(t, ok)
}

func TestGoTelegramClientWebhookAndInlineUpdates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	boundary, botService, identityService, _, _, botToken, _ := compatBoundary(t, ctx)

	server := httptest.NewServer(boundary.routes())
	defer server.Close()

	client, err := telegrambot.New(
		botToken,
		telegrambot.WithServerURL(server.URL),
		telegrambot.WithSkipGetMe(),
	)
	require.NoError(t, err)

	ok, err := client.SetWebhook(ctx, &telegrambot.SetWebhookParams{
		URL:            "https://example.org/hook",
		AllowedUpdates: []string{"inline_query", "chosen_inline_result"},
	})
	require.NoError(t, err)
	require.True(t, ok)

	info, err := client.GetWebhookInfo(ctx)
	require.NoError(t, err)
	require.Equal(t, "https://example.org/hook", info.URL)

	ok, err = client.DeleteWebhook(ctx, nil)
	require.NoError(t, err)
	require.True(t, ok)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "inlineuser",
		DisplayName: "Inline User",
		AccountKind: identity.AccountKindUser,
		Email:       "inlineuser@example.org",
	})
	require.NoError(t, err)
	botAccount, err := identityService.BotAccountByToken(ctx, botToken)
	require.NoError(t, err)

	query, err := botService.TriggerInlineQuery(ctx, domainbot.TriggerInlineQueryParams{
		BotAccountID:  botAccount.ID,
		FromAccountID: userAccount.ID,
		Query:         "docs",
	})
	require.NoError(t, err)

	ok, err = client.AnswerInlineQuery(ctx, &telegrambot.AnswerInlineQueryParams{
		InlineQueryID: query.ID,
		Results: []tgmodels.InlineQueryResult{
			&tgmodels.InlineQueryResultArticle{
				ID:    "article-1",
				Title: "Docs",
				InputMessageContent: tgmodels.InputTextMessageContent{
					MessageText: "hello",
				},
			},
			&tgmodels.InlineQueryResultPhoto{
				ID:           "photo-1",
				PhotoURL:     "https://cdn.example.org/p.jpg",
				ThumbnailURL: "https://cdn.example.org/t.jpg",
				Title:        "Photo",
			},
		},
	})
	require.NoError(t, err)
	require.True(t, ok)

	chosen, err := botService.TriggerChosenInlineResult(ctx, domainbot.TriggerChosenInlineResultParams{
		InlineQueryID: query.ID,
		FromAccountID: userAccount.ID,
		ResultID:      "photo-1",
	})
	require.NoError(t, err)

	updates, err := compatUpdates(ctx, server.Client(), server.URL, botToken, 0, nil)
	require.NoError(t, err)
	require.Len(t, updates, 2)
	require.NotNil(t, updates[0].InlineQuery)
	require.Equal(t, "docs", updates[0].InlineQuery.Query)
	require.NotNil(t, updates[1].ChosenInlineResult)
	require.Equal(t, chosen.ResultID, updates[1].ChosenInlineResult.ResultID)
	require.Equal(t, "docs", updates[1].ChosenInlineResult.Query)
}

func TestGoTelegramModelsDecodeChatMemberUpdates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	boundary, _, identityService, conversationService, botStore, botToken, _ := compatBoundary(t, ctx)

	server := httptest.NewServer(boundary.routes())
	defer server.Close()

	owner, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "owner2",
		DisplayName: "Owner Two",
		AccountKind: identity.AccountKindUser,
		Email:       "owner2@example.org",
	})
	require.NoError(t, err)
	target, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "target2",
		DisplayName: "Target Two",
		AccountKind: identity.AccountKindUser,
		Email:       "target2@example.org",
	})
	require.NoError(t, err)
	botAccount, err := identityService.BotAccountByToken(ctx, botToken)
	require.NoError(t, err)

	group, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID: owner.ID,
		Kind:           conversation.ConversationKindGroup,
		Title:          "ops",
	})
	require.NoError(t, err)

	_, err = conversationService.AddMembers(ctx, conversation.AddMembersParams{
		ConversationID:     group.ID,
		ActorAccountID:     owner.ID,
		InvitedByAccountID: owner.ID,
		AccountIDs:         []string{botAccount.ID},
	})
	require.NoError(t, err)
	_, err = conversationService.AddMembers(ctx, conversation.AddMembersParams{
		ConversationID:     group.ID,
		ActorAccountID:     owner.ID,
		InvitedByAccountID: owner.ID,
		AccountIDs:         []string{target.ID},
	})
	require.NoError(t, err)

	myChatMember := domainbot.ChatMemberUpdated{
		Chat: domainbot.Chat{
			ID:    group.ID,
			Type:  domainbot.ChatTypeGroup,
			Title: "ops",
		},
		From: domainbot.User{
			ID:        owner.ID,
			FirstName: "Owner Two",
			Username:  "owner2",
		},
		Date: time.Now().UTC().Unix(),
		OldChatMember: domainbot.ChatMember{
			User: domainbot.User{
				ID:        botAccount.ID,
				IsBot:     true,
				FirstName: "Compat Bot",
				Username:  "compatbot",
			},
			Status: domainbot.MemberStatusLeft,
		},
		NewChatMember: domainbot.ChatMember{
			User: domainbot.User{
				ID:        botAccount.ID,
				IsBot:     true,
				FirstName: "Compat Bot",
				Username:  "compatbot",
			},
			Status: domainbot.MemberStatusMember,
		},
	}
	_, err = botStore.SaveUpdate(ctx, domainbot.QueueEntry{
		BotAccountID: botAccount.ID,
		EventID:      "evt-my-member",
		UpdateType:   domainbot.UpdateTypeMyChatMember,
		Payload: domainbot.Update{
			MyChatMember: &myChatMember,
		},
	})
	require.NoError(t, err)

	chatMember := domainbot.ChatMemberUpdated{
		Chat: domainbot.Chat{
			ID:    group.ID,
			Type:  domainbot.ChatTypeGroup,
			Title: "ops",
		},
		From: domainbot.User{
			ID:        owner.ID,
			FirstName: "Owner Two",
			Username:  "owner2",
		},
		Date: time.Now().UTC().Unix(),
		OldChatMember: domainbot.ChatMember{
			User: domainbot.User{
				ID:        target.ID,
				FirstName: "Target Two",
				Username:  "target2",
			},
			Status: domainbot.MemberStatusLeft,
		},
		NewChatMember: domainbot.ChatMember{
			User: domainbot.User{
				ID:        target.ID,
				FirstName: "Target Two",
				Username:  "target2",
			},
			Status: domainbot.MemberStatusMember,
		},
	}
	_, err = botStore.SaveUpdate(ctx, domainbot.QueueEntry{
		BotAccountID: botAccount.ID,
		EventID:      "evt-chat-member",
		UpdateType:   domainbot.UpdateTypeChatMember,
		Payload: domainbot.Update{
			ChatMember: &chatMember,
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		updates, loadErr := compatUpdates(
			ctx,
			server.Client(),
			server.URL,
			botToken,
			0,
			[]string{"my_chat_member", "chat_member"},
		)
		require.NoError(t, loadErr)
		var sawMyMember bool
		var sawTargetMember bool
		for _, update := range updates {
			if update.MyChatMember != nil &&
				update.MyChatMember.NewChatMember.Type == tgmodels.ChatMemberTypeMember {
				sawMyMember = true
			}
			if update.ChatMember != nil &&
				update.ChatMember.NewChatMember.Type == tgmodels.ChatMemberTypeMember &&
				update.ChatMember.NewChatMember.Member != nil &&
				update.ChatMember.NewChatMember.Member.User.Username == "target2" {
				sawTargetMember = true
			}
		}
		return sawMyMember && sawTargetMember
	}, time.Second, 20*time.Millisecond)
}

func compatBoundary(
	t *testing.T,
	ctx context.Context,
) (*api, *domainbot.Service, *identity.Service, *conversation.Service, domainbot.Store, string, string) {
	t.Helper()

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
	botService, err := domainbot.NewService(
		botStore,
		identityService,
		conversationService,
		conversationStore,
		mediaFixture{},
		domainbot.WithSettings(settings),
	)
	require.NoError(t, err)

	userAccount, _, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "compatuser",
		DisplayName: "Compat User",
		AccountKind: identity.AccountKindUser,
		Email:       "compat@example.org",
	})
	require.NoError(t, err)
	botAccount, botToken, err := identityService.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "compatbot",
		DisplayName: "Compat Bot",
		AccountKind: identity.AccountKindBot,
	})
	require.NoError(t, err)

	conv, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   userAccount.ID,
		Kind:             conversation.ConversationKindDirect,
		MemberAccountIDs: []string{botAccount.ID},
	})
	require.NoError(t, err)

	return &api{bot: botService, media: &uploadFixture{}, uploadLimit: 1024}, botService, identityService, conversationService, botStore, botToken, conv.ID
}

func compatUpdates(
	ctx context.Context,
	client *http.Client,
	serverURL string,
	botToken string,
	offset int64,
	allowed []string,
) ([]tgmodels.Update, error) {
	payload := map[string]any{
		"offset": offset,
	}
	if len(allowed) > 0 {
		payload["allowed_updates"] = allowed
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		serverURL+"/bot"+botToken+"/getUpdates",
		strings.NewReader(string(body)),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	var envelope struct {
		OK     bool              `json:"ok"`
		Result []tgmodels.Update `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	if !envelope.OK {
		return nil, domainbot.ErrConflict
	}

	return envelope.Result, nil
}
