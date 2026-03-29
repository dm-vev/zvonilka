package conversation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	teststore "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
)

func TestEncryptedPayloadPolicyIsOptIn(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		kind    conversation.ConversationKind
		members []string
	}

	cases := []testCase{
		{
			name:    "direct",
			kind:    conversation.ConversationKindDirect,
			members: []string{"acc-peer"},
		},
		{
			name:    "group",
			kind:    conversation.ConversationKindGroup,
			members: []string{"acc-peer"},
		},
		{
			name:    "channel",
			kind:    conversation.ConversationKindChannel,
			members: []string{"acc-peer"},
		},
		{
			name:    "saved_messages",
			kind:    conversation.ConversationKindSavedMessages,
			members: nil,
		},
	}

	fixedNow := time.Date(2026, time.March, 24, 16, 0, 0, 0, time.UTC)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := teststore.NewMemoryStore()
			svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time { return fixedNow }))
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			created, _, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
				OwnerAccountID:   "acc-owner",
				Kind:             tc.kind,
				MemberAccountIDs: tc.members,
				Title:            "Encrypted",
				CreatedAt:        fixedNow,
			})
			if err != nil {
				t.Fatalf("create conversation: %v", err)
			}
			if created.Settings.RequireEncryptedMessages {
				t.Fatal("expected encryption policy to be opt-in by default")
			}

			message, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
				ConversationID:  created.ID,
				SenderAccountID: "acc-owner",
				SenderDeviceID:  "dev-owner",
				Draft: conversation.MessageDraft{
					Kind: conversation.MessageKindText,
					Payload: conversation.EncryptedPayload{
						Ciphertext: []byte("plaintext-body"),
					},
				},
				CreatedAt: fixedNow.Add(time.Minute),
			})
			if err != nil {
				t.Fatalf("send message with optional encryption: %v", err)
			}
			if string(message.Payload.Ciphertext) != "plaintext-body" {
				t.Fatalf("expected ciphertext body to round-trip, got %q", string(message.Payload.Ciphertext))
			}
		})
	}
}

func TestEncryptedPayloadCanBeRequiredPerConversation(t *testing.T) {
	t.Parallel()

	store := teststore.NewMemoryStore()
	fixedNow := time.Date(2026, time.March, 24, 17, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID: "acc-owner",
		Kind:           conversation.ConversationKindGroup,
		Title:          "Group",
		Settings: conversation.ConversationSettings{
			RequireEncryptedMessages: true,
		},
		CreatedAt: fixedNow,
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if !created.Settings.RequireEncryptedMessages {
		t.Fatal("expected encrypted message policy to persist")
	}

	_, _, err = svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte("plaintext-body"),
			},
		},
		CreatedAt: fixedNow.Add(time.Minute),
	})
	if !errors.Is(err, conversation.ErrInvalidInput) {
		t.Fatalf("expected plaintext payload to be rejected for encrypted conversation, got %v", err)
	}

	message, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
			},
		},
		CreatedAt: fixedNow.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("send encrypted message: %v", err)
	}
	if string(message.Payload.Ciphertext) != "ciphertext" {
		t.Fatalf("expected encrypted ciphertext to round-trip, got %q", string(message.Payload.Ciphertext))
	}
}

func TestTrustedDevicePolicyImpliesEncryptedMessages(t *testing.T) {
	t.Parallel()

	store := teststore.NewMemoryStore()
	svc, err := conversation.NewService(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID: "acc-owner",
		Kind:           conversation.ConversationKindGroup,
		Title:          "Trusted Group",
		Settings: conversation.ConversationSettings{
			RequireTrustedDevices: true,
		},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if !created.Settings.RequireTrustedDevices || !created.Settings.RequireEncryptedMessages {
		t.Fatalf("expected trusted device policy to imply encrypted messages, got %+v", created.Settings)
	}
}

func TestEncryptedHintsAreStrippedFromMessages(t *testing.T) {
	t.Parallel()

	store := teststore.NewMemoryStore()
	fixedNow := time.Date(2026, time.March, 24, 18, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindGroup,
		MemberAccountIDs: []string{"acc-peer"},
		Title:            "Group",
		CreatedAt:        fixedNow,
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	rootMessage, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte("root-body"),
			},
		},
		CreatedAt: fixedNow.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("send root message: %v", err)
	}

	replyMessage, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-peer",
		SenderDeviceID:  "dev-peer",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte("reply-body"),
			},
			ReplyTo: conversation.MessageReference{
				MessageID: rootMessage.ID,
				Snippet:   "plaintext quote",
			},
			Attachments: []conversation.AttachmentRef{
				{
					MediaID:   "media-1",
					Kind:      conversation.AttachmentKindImage,
					FileName:  "photo.jpg",
					MimeType:  "image/jpeg",
					SizeBytes: 1024,
					SHA256Hex: "abc123",
					Caption:   "plaintext caption",
				},
			},
		},
		CreatedAt: fixedNow.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("send reply message: %v", err)
	}
	if replyMessage.ReplyTo.Snippet != "" {
		t.Fatalf("expected reply snippet to be stripped, got %q", replyMessage.ReplyTo.Snippet)
	}
	if len(replyMessage.Attachments) != 1 {
		t.Fatalf("expected one attachment, got %d", len(replyMessage.Attachments))
	}
	if replyMessage.Attachments[0].Caption != "" {
		t.Fatalf("expected attachment caption to be stripped, got %q", replyMessage.Attachments[0].Caption)
	}

	messages, err := svc.ListMessages(context.Background(), conversation.ListMessagesParams{
		AccountID:      "acc-owner",
		ConversationID: created.ID,
	})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[1].ReplyTo.Snippet != "" {
		t.Fatalf("expected persisted reply snippet to be stripped, got %q", messages[1].ReplyTo.Snippet)
	}
	if messages[1].Attachments[0].Caption != "" {
		t.Fatalf("expected persisted attachment caption to be stripped, got %q", messages[1].Attachments[0].Caption)
	}
}
