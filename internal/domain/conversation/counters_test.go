package conversation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	teststore "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
)

func TestConversationCountersTrackUnreadMentions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	fixedNow := time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        fixedNow,
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	message, _, err := svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind:              conversation.MessageKindText,
			MentionAccountIDs: []string{"acc-peer"},
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
			},
		},
		CreatedAt: fixedNow.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	conversations, err := svc.ListConversations(ctx, conversation.ListConversationsParams{
		AccountID: "acc-peer",
	})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(conversations))
	}
	if conversations[0].UnreadCount != 1 {
		t.Fatalf("expected 1 unread message, got %d", conversations[0].UnreadCount)
	}
	if conversations[0].UnreadMentionCount != 1 {
		t.Fatalf("expected 1 unread mention, got %d", conversations[0].UnreadMentionCount)
	}

	loadedConversation, members, err := svc.GetConversation(ctx, conversation.GetConversationParams{
		ConversationID: created.ID,
		AccountID:      "acc-peer",
	})
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	if loadedConversation.UnreadCount != 1 {
		t.Fatalf("expected 1 unread message on get, got %d", loadedConversation.UnreadCount)
	}
	if loadedConversation.UnreadMentionCount != 1 {
		t.Fatalf("expected 1 unread mention on get, got %d", loadedConversation.UnreadMentionCount)
	}

	if _, _, err := svc.MarkRead(ctx, conversation.MarkReadParams{
		ConversationID:      created.ID,
		AccountID:           "acc-peer",
		DeviceID:            "dev-peer",
		ReadThroughSequence: message.Sequence,
		CreatedAt:           fixedNow.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	conversations, err = svc.ListConversations(ctx, conversation.ListConversationsParams{
		AccountID: "acc-peer",
	})
	if err != nil {
		t.Fatalf("list conversations after read: %v", err)
	}
	if conversations[0].UnreadCount != 0 {
		t.Fatalf("expected 0 unread messages after read, got %d", conversations[0].UnreadCount)
	}
	if conversations[0].UnreadMentionCount != 0 {
		t.Fatalf("expected 0 unread mentions after read, got %d", conversations[0].UnreadMentionCount)
	}
}

func TestConversationCountersIgnoreHistoryBeforeJoin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	fixedNow := time.Date(2026, time.March, 24, 14, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID: "acc-owner",
		Kind:           conversation.ConversationKindGroup,
		Title:          "Group",
		CreatedAt:      fixedNow,
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	firstMessage, _, err := svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce-1"),
				Ciphertext: []byte("ciphertext-1"),
			},
		},
		CreatedAt: fixedNow.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("send first message: %v", err)
	}

	if _, err := store.SaveConversationMember(ctx, conversation.ConversationMember{
		ConversationID: created.ID,
		AccountID:      "acc-peer",
		Role:           conversation.MemberRoleMember,
		JoinedAt:       fixedNow.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("save peer member: %v", err)
	}

	secondMessage, _, err := svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind:              conversation.MessageKindText,
			MentionAccountIDs: []string{"acc-peer"},
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-2",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce-2"),
				Ciphertext: []byte("ciphertext-2"),
			},
		},
		CreatedAt: fixedNow.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("send second message: %v", err)
	}
	if firstMessage.Sequence >= secondMessage.Sequence {
		t.Fatalf("expected first message sequence to stay behind second message")
	}

	conversations, err := svc.ListConversations(ctx, conversation.ListConversationsParams{
		AccountID: "acc-peer",
	})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(conversations))
	}
	if conversations[0].UnreadCount != 1 {
		t.Fatalf("expected 1 unread message after join, got %d", conversations[0].UnreadCount)
	}
	if conversations[0].UnreadMentionCount != 1 {
		t.Fatalf("expected 1 unread mention after join, got %d", conversations[0].UnreadMentionCount)
	}
	if conversations[0].LastSequence != secondMessage.Sequence {
		t.Fatalf("expected last sequence %d, got %d", secondMessage.Sequence, conversations[0].LastSequence)
	}
}

func TestSendMessageRejectsUnknownMentionTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	fixedNow := time.Date(2026, time.March, 24, 13, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        fixedNow,
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	_, _, err = svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind:              conversation.MessageKindText,
			MentionAccountIDs: []string{"acc-ghost"},
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
			},
		},
		CreatedAt: fixedNow.Add(time.Minute),
	})
	if !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("expected forbidden mention target, got %v", err)
	}
}
