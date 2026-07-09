package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"time"

	authv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/auth/v1"
	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type account struct {
	name  string
	phone string
	code  string
	token string
	user  string
}

func main() {
	addr := flag.String("addr", "zvonilka.geartech.club:443", "public gateway address")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, *addr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	auth := authv1.NewAuthServiceClient(conn)
	chat := conversationv1.NewConversationServiceClient(conn)
	accounts := []account{
		{name: "alpha", phone: "+88807777777", code: "77777"},
		{name: "beta", phone: "+88810000001", code: "00001"},
		{name: "gamma", phone: "+88810000002", code: "00002"},
	}
	for i := range accounts {
		if err := login(ctx, auth, &accounts[i], time.Now().UnixNano()); err != nil {
			log.Fatalf("login %s: %v", accounts[i].phone, err)
		}
		log.Printf("account %s: %s", accounts[i].name, accounts[i].user)
	}

	owner := accounts[0]
	direct := create(ctx, chat, owner.token, &conversationv1.CreateConversationRequest{
		IdempotencyKey: "bulb-devseed-direct-alpha-beta",
		Kind:           commonv1.ConversationKind_CONVERSATION_KIND_DIRECT,
		MemberUserIds:  []string{accounts[1].user},
	})
	group := create(ctx, chat, owner.token, &conversationv1.CreateConversationRequest{
		IdempotencyKey: "bulb-devseed-group",
		Kind:           commonv1.ConversationKind_CONVERSATION_KIND_GROUP,
		Title:          "Тестовая группа Звон",
		Description:    "Группа для проверки списка чатов, профиля и сообщений.",
		MemberUserIds:  []string{accounts[1].user, accounts[2].user},
	})
	channel := create(ctx, chat, owner.token, &conversationv1.CreateConversationRequest{
		IdempotencyKey: "bulb-devseed-channel",
		Kind:           commonv1.ConversationKind_CONVERSATION_KIND_CHANNEL,
		Title:          "Канал разработки Звон",
		Description:    "Канал для проверки режима без поля ввода у подписчиков.",
		MemberUserIds:  []string{accounts[1].user, accounts[2].user},
		Settings:       &conversationv1.ConversationSettings{OnlyAdminsCanWrite: true},
	})

	sendSeedMessages(ctx, chat, direct, []seedMessage{
		{token: owner.token, text: "Привет. Это тестовый личный диалог."},
		{token: accounts[1].token, text: "Ответ с другого debug-аккаунта."},
	})
	sendSeedMessages(ctx, chat, group, []seedMessage{
		{token: owner.token, text: "Добро пожаловать в тестовую группу."},
		{token: accounts[2].token, text: "Проверяем групповые сообщения и аватарки."},
	})
	sendSeedMessages(ctx, chat, channel, []seedMessage{
		{token: owner.token, text: "Первый пост тестового канала."},
	})

	verifySeed(ctx, chat, owner.token)
	log.Printf("seeded direct=%s group=%s channel=%s", direct, group, channel)
}

func login(ctx context.Context, client authv1.AuthServiceClient, acc *account, runID int64) error {
	begin, err := client.BeginLogin(ctx, &authv1.BeginLoginRequest{
		IdempotencyKey:  fmt.Sprintf("bulb-devseed-login-%s-%d", acc.name, runID),
		Identifier:      &authv1.BeginLoginRequest_Phone{Phone: acc.phone},
		DeliveryChannel: authv1.LoginDeliveryChannel_LOGIN_DELIVERY_CHANNEL_MANUAL,
		DeviceName:      "Bulb devseed " + acc.name,
		DevicePlatform:  commonv1.DevicePlatform_DEVICE_PLATFORM_DESKTOP,
		ClientVersion:   "devseed",
		Locale:          "ru_RU",
	})
	if err != nil {
		return err
	}
	verify, err := client.VerifyLoginCode(ctx, &authv1.VerifyLoginCodeRequest{
		ChallengeId:    begin.GetChallengeId(),
		Code:           acc.code,
		DeviceName:     "Bulb devseed " + acc.name,
		DevicePlatform: commonv1.DevicePlatform_DEVICE_PLATFORM_DESKTOP,
		DeviceKey:      &commonv1.PublicKeyBundle{PublicKey: []byte("bulb-devseed-" + acc.name)},
		IdempotencyKey: fmt.Sprintf("bulb-devseed-verify-%s-%d", acc.name, runID),
	})
	if err != nil {
		return err
	}
	acc.token = verify.GetTokens().GetAccessToken()
	acc.user = verify.GetSession().GetUserId()
	if acc.token == "" || acc.user == "" {
		return fmt.Errorf("empty token or user")
	}
	return nil
}

func create(ctx context.Context, client conversationv1.ConversationServiceClient, token string, req *conversationv1.CreateConversationRequest) string {
	if id := existingConversation(ctx, client, token, req.GetKind(), req.GetTitle()); id != "" {
		log.Printf("conversation %s: %s reused", req.GetIdempotencyKey(), id)
		return id
	}
	resp, err := client.CreateConversation(authCtx(ctx, token), req)
	if err != nil {
		log.Fatalf("create %s: %v", req.GetIdempotencyKey(), err)
	}
	id := resp.GetConversation().GetConversationId()
	if id == "" {
		log.Fatalf("create %s: empty conversation id", req.GetIdempotencyKey())
	}
	log.Printf("conversation %s: %s", req.GetIdempotencyKey(), id)
	return id
}

func existingConversation(ctx context.Context, client conversationv1.ConversationServiceClient, token string, kind commonv1.ConversationKind, title string) string {
	resp, err := client.ListConversations(authCtx(ctx, token), &conversationv1.ListConversationsRequest{
		Page:  &commonv1.PageRequest{PageSize: 50},
		Kinds: []commonv1.ConversationKind{kind},
	})
	if err != nil {
		log.Fatalf("list conversations before create: %v", err)
	}
	for _, conversation := range resp.GetConversations() {
		if kind == commonv1.ConversationKind_CONVERSATION_KIND_DIRECT || conversation.GetTitle() == title {
			return conversation.GetConversationId()
		}
	}
	return ""
}

type seedMessage struct {
	token string
	text  string
}

func sendSeedMessages(ctx context.Context, client conversationv1.ConversationServiceClient, conversationID string, messages []seedMessage) {
	existing, err := client.ListMessages(authCtx(ctx, messages[0].token), &conversationv1.ListMessagesRequest{
		ConversationId: conversationID,
		Page:           &commonv1.PageRequest{PageSize: 1},
	})
	if err != nil {
		log.Fatalf("list messages before send %s: %v", conversationID, err)
	}
	if len(existing.GetMessages()) > 0 {
		log.Printf("messages %s: reused", conversationID)
		return
	}
	for _, message := range messages {
		send(ctx, client, message.token, conversationID, message.text)
	}
}

func send(ctx context.Context, client conversationv1.ConversationServiceClient, token, conversationID, text string) {
	_, err := client.SendMessage(authCtx(ctx, token), &conversationv1.SendMessageRequest{
		ConversationId: conversationID,
		Draft: &commonv1.MessageDraft{
			ClientMessageId: "bulb-devseed-msg-" + conversationID + "-" + text,
			Kind:            commonv1.MessageKind_MESSAGE_KIND_TEXT,
			Payload:         &commonv1.EncryptedPayload{Ciphertext: []byte(text)},
		},
		IdempotencyKey: "bulb-devseed-send-" + conversationID + "-" + text,
	})
	if err != nil {
		log.Fatalf("send %s: %v", conversationID, err)
	}
}

func verifySeed(ctx context.Context, client conversationv1.ConversationServiceClient, token string) {
	resp, err := client.ListConversations(authCtx(ctx, token), &conversationv1.ListConversationsRequest{
		Page: &commonv1.PageRequest{PageSize: 20},
	})
	if err != nil {
		log.Fatalf("list seeded conversations: %v", err)
	}
	for _, conversation := range resp.GetConversations() {
		messages, err := client.ListMessages(authCtx(ctx, token), &conversationv1.ListMessagesRequest{
			ConversationId: conversation.GetConversationId(),
			Page:           &commonv1.PageRequest{PageSize: 20},
		})
		if err != nil {
			log.Fatalf("list messages %s: %v", conversation.GetConversationId(), err)
		}
		log.Printf("visible %s %q messages=%d", conversation.GetKind(), conversation.GetTitle(), len(messages.GetMessages()))
	}
}

func authCtx(ctx context.Context, token string) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
}
