package translation_test

import (
	"context"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationtest "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/translation"
	translationtest "github.com/dm-vev/zvonilka/internal/domain/translation/teststore"
)

type recordingProvider struct {
	calls int
}

func (p *recordingProvider) Translate(
	_ context.Context,
	request translation.ProviderRequest,
) (translation.ProviderResult, error) {
	p.calls++
	return translation.ProviderResult{
		TranslatedText: "[" + request.TargetLanguage + "] " + request.Text,
		SourceLanguage: "en",
		Provider:       "recording-provider",
	}, nil
}

func TestTranslateMessageCachesProviderResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 25, 9, 0, 0, 0, time.UTC)
	conversationService, err := conversation.NewService(
		conversationtest.NewMemoryStore(),
		conversation.WithNow(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("new conversation service: %v", err)
	}

	created, _, err := conversationService.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Translate",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        now,
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	message, _, err := conversationService.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			ClientMessageID: "msg-1",
			Kind:            conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte("hello world"),
			},
		},
		CreatedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	provider := &recordingProvider{}
	service, err := translation.NewService(
		translationtest.NewMemoryStore(),
		conversationService,
		provider,
		translation.WithNow(func() time.Time { return now.Add(2 * time.Minute) }),
	)
	if err != nil {
		t.Fatalf("new translation service: %v", err)
	}

	first, cached, err := service.TranslateMessage(ctx, translation.TranslateMessageParams{
		ConversationID: created.ID,
		MessageID:      message.ID,
		RequesterID:    "acc-peer",
		TargetLanguage: "ru",
	})
	if err != nil {
		t.Fatalf("translate message: %v", err)
	}
	if cached {
		t.Fatal("expected first translation response to miss cache")
	}
	if provider.calls != 1 {
		t.Fatalf("expected one provider call, got %d", provider.calls)
	}

	second, cached, err := service.TranslateMessage(ctx, translation.TranslateMessageParams{
		ConversationID: created.ID,
		MessageID:      message.ID,
		RequesterID:    "acc-peer",
		TargetLanguage: "ru",
	})
	if err != nil {
		t.Fatalf("translate message from cache: %v", err)
	}
	if !cached {
		t.Fatal("expected cached translation response")
	}
	if provider.calls != 1 {
		t.Fatalf("expected cached response without extra provider call, got %d", provider.calls)
	}
	if second.TranslatedText != first.TranslatedText {
		t.Fatalf("expected cached translation text %q, got %q", first.TranslatedText, second.TranslatedText)
	}
}
